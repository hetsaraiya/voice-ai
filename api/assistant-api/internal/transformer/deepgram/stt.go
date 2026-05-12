// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_transformer_deepgram

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	interfaces "github.com/deepgram/deepgram-go-sdk/v3/pkg/client/interfaces/v1"
	client "github.com/deepgram/deepgram-go-sdk/v3/pkg/client/listen"
	deepgram_internal "github.com/rapidaai/api/assistant-api/internal/transformer/deepgram/internal"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	type_enums "github.com/rapidaai/pkg/types/enums"
	utils "github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

type deepgramSTT struct {
	*deepgramOption
	mu sync.Mutex
	// context management
	ctx            context.Context
	ctxCancel      context.CancelFunc
	logger         commons.Logger
	client         *client.WSCallback
	onPacket       func(pkt ...internal_type.Packet) error
	contextId      string
	sttConnectedAt time.Time
	startedAt      time.Time
}

func (*deepgramSTT) Name() string {
	return "deepgram-speech-to-text"
}

func (dg *deepgramSTT) swapStartedAt() time.Time {
	dg.mu.Lock()
	t := dg.startedAt
	dg.startedAt = time.Time{}
	dg.mu.Unlock()
	return t
}

func NewDeepgramSpeechToText(ctx context.Context, logger commons.Logger, vaultCredential *protos.VaultCredential,
	onPacket func(pkt ...internal_type.Packet) error,
	opts utils.Option) (internal_type.SpeechToTextTransformer, error) {
	deepgramOpts, err := NewDeepgramOption(logger, vaultCredential, opts)
	if err != nil {
		logger.Errorf("deepgram-stt: Key from credential failed %+v", err)
		return nil, err
	}
	ct, ctxCancel := context.WithCancel(ctx)
	return &deepgramSTT{
		ctx:            ct,
		ctxCancel:      ctxCancel,
		logger:         logger,
		deepgramOption: deepgramOpts,
		onPacket:       onPacket,
	}, nil
}

// The `Initialize` method in the `deepgram` struct is responsible for establishing a connection to the
// Deepgram service using the WebSocket client `dg.client`.
func (dg *deepgramSTT) Initialize() error {
	start := time.Now()
	dgClient, err := client.NewWSUsingCallback(
		dg.ctx,
		dg.GetKey(),
		&interfaces.ClientOptions{APIKey: dg.GetKey(), EnableKeepAlive: true},
		dg.SpeechToTextOptions(), deepgram_internal.NewDeepgramSttCallback(dg.logger, dg.onPacket, dg.deepgramOption.mdlOpts, dg.swapStartedAt, dg.getContextID))
	if err != nil {
		dg.logger.Errorf("deepgram-stt: unable create dg client with error %+v", err.Error())
		return err
	}
	if !dgClient.Connect() {
		dg.logger.Errorf("deepgram-stt: unable to connect to deepgram service")
		return fmt.Errorf("deepgram-stt: connection failed")
	}

	dg.mu.Lock()
	dg.client = dgClient
	dg.sttConnectedAt = time.Now()
	dg.mu.Unlock()

	dg.onPacket(internal_type.ConversationEventPacket{
		ContextID: dg.getContextID(),
		Name:      "stt",
		Data: map[string]string{
			"type":     "initialized",
			"provider": dg.Name(),
			"init_ms":  fmt.Sprintf("%d", time.Since(start).Milliseconds()),
		},
		Time: time.Now(),
	})
	return nil
}

// Transform implements internal_transformer.SpeechToTextTransformer.
// The `Transform` method in the `deepgram` struct is taking an input audio byte array `in`, creating a
// new `bufio.Reader` from it, and then passing that reader to the `Stream` method of the `dg.client`
// WebSocket client. This method is responsible for streaming the audio data to the Deepgram service
// for transcription. If there are any errors during the streaming process, they will be returned by
// the method.
func (dg *deepgramSTT) Transform(ctx context.Context, in internal_type.Packet) error {
	switch pkt := in.(type) {
	case internal_type.TurnChangePacket:
		dg.mu.Lock()
		dg.contextId = pkt.ContextID
		dg.mu.Unlock()
		return nil
	case internal_type.SpeechToTextInterruptPacket:
		dg.mu.Lock()
		dg.startedAt = time.Now()
		dg.mu.Unlock()
		return nil
	case internal_type.SpeechToTextAudioPacket:
		dg.mu.Lock()
		client := dg.client
		dg.mu.Unlock()

		if client == nil {
			return fmt.Errorf("deepgram-stt: connection is not initialized")
		}
		err := client.Stream(bufio.NewReader(bytes.NewReader(pkt.Audio)))
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			dg.logger.Errorf("deepgram-stt: error while calling deepgram: %v", err)
			return fmt.Errorf("deepgram stream error: %w", err)
		}
		return err
	default:
		return nil
	}
}

func (dg *deepgramSTT) getContextID() string {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	return dg.contextId
}

func (dg *deepgramSTT) Close(ctx context.Context) error {
	dg.ctxCancel()

	dg.mu.Lock()
	ctxID := dg.contextId
	connectedAt := dg.sttConnectedAt
	dg.sttConnectedAt = time.Time{}

	if dg.client != nil {
		dg.client.Stop()
	}
	dg.mu.Unlock()

	if !connectedAt.IsZero() {
		dg.onPacket(
			internal_type.ConversationEventPacket{
				ContextID: ctxID,
				Name:      "stt",
				Data: map[string]string{
					"type":     "closed",
					"provider": dg.Name(),
				},
				Time: time.Now(),
			},
			internal_type.ConversationMetricPacket{
				Metrics: []*protos.Metric{{
					Name:        type_enums.CONVERSATION_STT_DURATION.String(),
					Value:       fmt.Sprintf("%d", time.Since(connectedAt).Nanoseconds()),
					Description: "Total STT connection duration in nanoseconds",
				}},
			},
		)
	}
	return nil
}
