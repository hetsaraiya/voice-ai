// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_llm_agentkit

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math"
	"net"
	"time"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (e *agentkitExecutor) Initialize(ctx context.Context, comm internal_type.Communication, cfg *protos.ConversationInitialization) error {
	start := time.Now()
	provider := comm.Assistant().AssistantProviderAgentkit

	e.stateMu.Lock()
	e.closing = false
	e.stateMu.Unlock()

	opts := []grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, "tcp", addr)
		}),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(math.MaxInt64), grpc.MaxCallSendMsgSize(math.MaxInt64)),
	}
	if provider.Certificate != "" {
		if provider.Certificate == "insecure" || provider.Certificate == "skip-verify" {
			e.logger.Warnf("Using insecure TLS (skipping certificate verification)")
			opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
			})))
		} else {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM([]byte(provider.Certificate)) {
				return fmt.Errorf("TLS credentials failed: invalid certificate: failed to parse PEM")
			}
			opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
				RootCAs:    pool,
				MinVersion: tls.VersionTLS12,
			})))
		}
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(provider.Url, opts...)
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	streamCtx := ctx
	if len(provider.Metadata) > 0 {
		streamCtx = metadata.NewOutgoingContext(ctx, metadata.New(map[string]string(provider.Metadata)))
	}
	talkStream, err := protos.NewAgentKitClient(conn).Talk(streamCtx)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("stream start failed: %w", err)
	}

	e.stateMu.Lock()
	e.transport.conn = conn
	e.transport.stream = talkStream
	e.stateMu.Unlock()

	// Ensure initialization reaches the external agent before starting the listener.
	// This avoids a leaked listener/stream when Initialize returns an error upstream.
	if err := e.send(&protos.TalkInput{
		Request: &protos.TalkInput_Initialization{
			Initialization: &protos.ConversationInitialization{
				AssistantConversationId: comm.Conversation().Id,
				Assistant: &protos.AssistantDefinition{
					AssistantId: provider.AssistantId,
					Version:     utils.GetVersionString(provider.Id),
				},
				Args: cfg.GetArgs(), Metadata: cfg.GetMetadata(),
				Options: cfg.GetOptions(), StreamMode: cfg.GetStreamMode(),
				UserIdentity: cfg.GetUserIdentity(), Time: timestamppb.Now(),
			},
		},
	}); err != nil {
		e.stateMu.Lock()
		transport := e.clearTransportLocked()
		e.activeContextID = ""
		e.stateMu.Unlock()
		if transport.stream != nil {
			_ = transport.stream.CloseSend()
		}
		if transport.conn != nil {
			_ = transport.conn.Close()
		}
		return fmt.Errorf("failed to send initialization: %w", err)
	}

	listenerDoneCh := make(chan struct{})
	e.stateMu.Lock()
	e.transport.listenerDone = listenerDoneCh
	e.stateMu.Unlock()
	utils.Go(ctx, func() {
		defer close(listenerDoneCh)
		e.listen(ctx, comm)
	})

	comm.OnPacket(ctx, internal_type.ConversationEventPacket{
		Name: "agentkit",
		Data: map[string]string{
			"type": "agentkit_initialized", "provider": "agentkit",
			"url": provider.Url, "init_ms": fmt.Sprintf("%d", time.Since(start).Milliseconds()),
		},
		Time: time.Now(),
	})
	return nil
}

func (e *agentkitExecutor) Close(ctx context.Context) error {
	// Mark closing before tearing down transport so listener recv errors are treated
	// as expected shutdown and do not emit end_conversation directives.
	e.stateMu.Lock()
	e.closing = true
	transport := e.clearTransportLocked()
	e.activeContextID = ""
	e.stateMu.Unlock()

	if transport.stream != nil {
		if err := transport.stream.CloseSend(); err != nil {
			e.logger.Warnf("failed to close agentkit stream send-side: %v", err)
		}
	}
	if transport.conn != nil {
		if err := transport.conn.Close(); err != nil {
			e.logger.Warnf("failed to close agentkit connection: %v", err)
		}
	}

	if transport.listenerDone != nil {
		select {
		case <-transport.listenerDone:
		case <-time.After(5 * time.Second):
			e.logger.Errorf("timed out waiting for listener goroutine to exit")
		}
	}
	return nil
}
