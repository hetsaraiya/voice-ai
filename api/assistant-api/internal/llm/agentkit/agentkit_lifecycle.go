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
	"net"
	"time"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (e *agentkitExecutor) initialize(ctx context.Context, comm internal_type.Communication, cfg *protos.ConversationInitialization) error {
	start := time.Now()
	provider := comm.Assistant().AssistantProviderAgentkit

	e.stateMu.Lock()
	e.closing = false
	e.stateMu.Unlock()

	transportSecurity := DefaultTransportSecurity
	if provider.TransportSecurity != nil && *provider.TransportSecurity != "" {
		transportSecurity = *provider.TransportSecurity
	}
	tlsVerification := DefaultTLSVerification
	if provider.TLSVerification != nil && *provider.TLSVerification != "" {
		tlsVerification = *provider.TLSVerification
	}
	connectTimeoutMs := DefaultConnectTimeoutMs
	if provider.ConnectTimeoutMs != nil {
		connectTimeoutMs = *provider.ConnectTimeoutMs
	}
	keepaliveTimeMs := DefaultKeepaliveTimeMs
	if provider.KeepaliveTimeMs != nil {
		keepaliveTimeMs = *provider.KeepaliveTimeMs
	}
	keepaliveTimeoutMs := DefaultKeepaliveTimeoutMs
	if provider.KeepaliveTimeoutMs != nil {
		keepaliveTimeoutMs = *provider.KeepaliveTimeoutMs
	}
	maxRecvMessageBytes := DefaultMaxRecvMessageBytes
	if provider.MaxRecvMessageBytes != nil {
		maxRecvMessageBytes = *provider.MaxRecvMessageBytes
	}
	maxSendMessageBytes := DefaultMaxSendMessageBytes
	if provider.MaxSendMessageBytes != nil {
		maxSendMessageBytes = *provider.MaxSendMessageBytes
	}

	connectTimeout := time.Duration(connectTimeoutMs) * time.Millisecond
	opts := []grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := net.Dialer{Timeout: connectTimeout}
			return d.DialContext(ctx, "tcp", addr)
		}),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(int(maxRecvMessageBytes)), grpc.MaxCallSendMsgSize(int(maxSendMessageBytes))),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    time.Duration(keepaliveTimeMs) * time.Millisecond,
			Timeout: time.Duration(keepaliveTimeoutMs) * time.Millisecond,
		}),
	}
	switch transportSecurity {
	case TransportSecurityPlaintext:
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	case TransportSecurityTLS:
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		if provider.TLSServerName != nil && *provider.TLSServerName != "" {
			tlsConfig.ServerName = *provider.TLSServerName
		}
		if tlsVerification == TLSVerificationSkipVerify {
			e.logger.Warnf("Using insecure TLS (skipping certificate verification)")
			tlsConfig.InsecureSkipVerify = true
		}
		if provider.Certificate != "" {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM([]byte(provider.Certificate)) {
				comm.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
					Scope: internal_type.ObservabilityRecordScopeConversation,
					Record: observability.RecordLog{
						Level:   observability.LevelError,
						Message: fmt.Sprintf("%s: error while initialization %s", e.Name(), "invalid certificate"),
						Attributes: observability.Attributes{
							"component": observability.ComponentLLM.String(),
							"provider":  e.Name(),
							"options":   observability.AttributeValue(cfg.GetOptions()),
							"url":       provider.Url,
							"error":     "invalid certificate",
						},
						OccurredAt: time.Now(),
					},
				})
				return fmt.Errorf("TLS credentials failed: invalid certificate: failed to parse PEM")
			}
			tlsConfig.RootCAs = pool
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	default:
		return fmt.Errorf("invalid transport security: %s", transportSecurity)
	}

	conn, err := grpc.NewClient(provider.Url, opts...)
	if err != nil {
		comm.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: fmt.Sprintf("%s: error while initialization %s", e.Name(), err.Error()),
				Attributes: observability.Attributes{
					"component":  observability.ComponentLLM.String(),
					"provider":   e.Name(),
					"options":    observability.AttributeValue(cfg.GetOptions()),
					"url":        provider.Url,
					"error":      err.Error(),
					"error_type": fmt.Sprintf("%T", err),
				},
				OccurredAt: time.Now(),
			},
		})
		return fmt.Errorf("connect failed: %w", err)
	}
	connectCtx, cancelConnect := context.WithTimeout(ctx, connectTimeout)
	defer cancelConnect()
	conn.Connect()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			break
		}
		if !conn.WaitForStateChange(connectCtx, state) {
			_ = conn.Close()
			err := connectCtx.Err()
			if err == nil {
				err = fmt.Errorf("connection not ready")
			}
			comm.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				Scope: internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: fmt.Sprintf("%s: error while initialization %s", e.Name(), err.Error()),
					Attributes: observability.Attributes{
						"component":  observability.ComponentLLM.String(),
						"provider":   e.Name(),
						"options":    observability.AttributeValue(cfg.GetOptions()),
						"url":        provider.Url,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
					OccurredAt: time.Now(),
				},
			})
			return fmt.Errorf("connect failed: %w", err)
		}
	}

	streamCtx := ctx
	if len(provider.Metadata) > 0 {
		streamCtx = metadata.NewOutgoingContext(ctx, metadata.New(map[string]string(provider.Metadata)))
	}
	talkStream, err := protos.NewAgentKitClient(conn).Talk(streamCtx)
	if err != nil {
		_ = conn.Close()
		comm.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: fmt.Sprintf("%s: error while initialization %s", e.Name(), err.Error()),
				Attributes: observability.Attributes{
					"component":  observability.ComponentLLM.String(),
					"provider":   e.Name(),
					"options":    observability.AttributeValue(cfg.GetOptions()),
					"url":        provider.Url,
					"error":      err.Error(),
					"error_type": fmt.Sprintf("%T", err),
				},
				OccurredAt: time.Now(),
			},
		})
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
		comm.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: fmt.Sprintf("%s: error while initialization %s", e.Name(), err.Error()),
				Attributes: observability.Attributes{
					"component":  observability.ComponentLLM.String(),
					"provider":   e.Name(),
					"options":    observability.AttributeValue(cfg.GetOptions()),
					"url":        provider.Url,
					"error":      err.Error(),
					"error_type": fmt.Sprintf("%T", err),
				},
				OccurredAt: time.Now(),
			},
		})
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

	comm.OnPacket(ctx,
		internal_type.ObservabilityMetricRecordPacket{
			Scope:  internal_type.ObservabilityRecordScopeConversation,
			Record: observability.NewMetricLLMInitLatencyMs(time.Since(start), observability.Attributes{"provider": e.Name()}),
		},
		internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelInfo,
				Message: fmt.Sprintf("%s: initialization completed", e.Name()),
				Attributes: observability.Attributes{
					"component": observability.ComponentLLM.String(),
					"provider":  e.Name(),
					"url":       provider.Url,
					"options":   observability.AttributeValue(cfg.GetOptions()),
				},
				OccurredAt: time.Now(),
			},
		},
	)
	return nil
}

func (e *agentkitExecutor) Close(ctx context.Context) error {
	// Mark closing before tearing down transport so listener recv errors are treated
	// as expected shutdown and do not emit end_conversation directives.
	e.stateMu.Lock()
	e.closing = true
	transport := e.clearTransportLocked()
	e.activeContextID = ""
	e.requestStartedAt = time.Time{}
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
