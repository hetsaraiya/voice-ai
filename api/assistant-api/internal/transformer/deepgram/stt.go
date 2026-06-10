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
	"github.com/rapidaai/api/assistant-api/internal/observability"
	deepgram_internal "github.com/rapidaai/api/assistant-api/internal/transformer/deepgram/internal"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	utils "github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

type deepgramSTT struct {
	*deepgramOption
	mu             sync.Mutex
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
	return "deepgram-stt"
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

func (dg *deepgramSTT) getAndClearStartTime() time.Time {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	currentTime := dg.startedAt
	dg.startedAt = time.Time{}
	return currentTime
}

// The `Initialize` method in the `deepgram` struct is responsible for establishing a connection to the
// Deepgram service using the WebSocket client `dg.client`.
func (dg *deepgramSTT) Initialize() error {
	start := time.Now()
	dgClient, err := client.NewWSUsingCallback(
		dg.ctx,
		dg.GetKey(),
		&interfaces.ClientOptions{APIKey: dg.GetKey(), EnableKeepAlive: true},
		dg.SpeechToTextOptions(), deepgram_internal.NewDeepgramSttCallback(dg.logger, dg.onPacket, dg.deepgramOption.mdlOpts, dg.getAndClearStartTime, dg.getContextID, dg.Name()))
	if err != nil {
		dg.onPacket(
			internal_type.ObservabilityLogRecordPacket{
				Scope: internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: fmt.Sprintf("deepgram-stt: error while initialization %s", err.Error()),
					Attributes: observability.Attributes{
						"component": observability.ComponentSTT.String(),
						"provider":  dg.Name(),
						"options":   observability.AttributeValue(dg.SpeechToTextOptions()),
					},
					OccurredAt: time.Now(),
				},
			})
		return err
	}
	if !dgClient.Connect() {
		dg.onPacket(
			internal_type.ObservabilityLogRecordPacket{
				Scope: internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "deepgram-stt: error while performing connect",
					Attributes: observability.Attributes{
						"component": observability.ComponentSTT.String(),
						"provider":  dg.Name(),
						"options":   observability.AttributeValue(dg.SpeechToTextOptions()),
					},
					OccurredAt: time.Now(),
				},
			})
		return fmt.Errorf("deepgram-stt: connection failed")
	}

	dg.mu.Lock()
	dg.client = dgClient
	dg.sttConnectedAt = time.Now()
	dg.mu.Unlock()
	dg.onPacket(
		internal_type.ObservabilityMetricRecordPacket{
			Scope:  internal_type.ObservabilityRecordScopeConversation,
			Record: observability.NewMetricSTTInitLatencyMs(time.Since(start), observability.Attributes{"provider": dg.Name()}),
		},
		internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelInfo,
				Message: "deepgram-stt: initialization completed",
				Attributes: observability.Attributes{
					"component": observability.ComponentSTT.String(),
					"provider":  dg.Name(),
					"options":   observability.AttributeValue(dg.SpeechToTextOptions()),
				},
				OccurredAt: time.Now(),
			},
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
	case internal_type.SpeechToTextStartPacket:
		dg.mu.Lock()
		dg.startedAt = time.Now()
		dg.mu.Unlock()
		return nil
	case internal_type.SpeechToTextAudioPacket:
		dg.mu.Lock()
		if dg.startedAt.IsZero() {
			dg.startedAt = time.Now()
		}
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
	connectedAt := dg.sttConnectedAt
	dg.sttConnectedAt = time.Time{}
	if dg.client != nil {
		dg.client.Stop()
	}
	dg.mu.Unlock()

	if !connectedAt.IsZero() {
		duration := time.Since(connectedAt)
		dg.onPacket(
			internal_type.ObservabilityMetricRecordPacket{
				Scope:  internal_type.ObservabilityRecordScopeConversation,
				Record: observability.NewMetricSTTDuration(duration, observability.Attributes{"provider": dg.Name()}),
			},
			internal_type.ObservabilityUsageRecordPacket{
				Scope:  internal_type.ObservabilityRecordScopeConversation,
				Record: observability.NewSTTDurationUsageRecord(dg.Name(), duration, observability.Attributes{}),
			},
		)
	}
	dg.onPacket(
		internal_type.ObservabilityEventRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordEvent{
				Component: observability.ComponentSTT,
				Event:     observability.STTClosed,
				Attributes: observability.Attributes{
					"type":     "closed",
					"provider": dg.Name(),
				},
				OccurredAt: time.Now(),
			},
		})
	return nil
}
