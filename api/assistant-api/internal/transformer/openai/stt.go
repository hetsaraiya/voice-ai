// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_transformer_openai

import (
	"context"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/utils"
)

type openaiSpeechToText struct {
	logger    commons.Logger
	client    openai.Client
	ctx       context.Context
	cancel    context.CancelFunc
	onPacket  func(pkt ...internal_type.Packet) error
	contextId string

	sttConnectedAt time.Time
}

func (o *openaiSpeechToText) Initialize() error {
	start := time.Now()
	o.ctx, o.cancel = context.WithCancel(context.Background())
	o.client = openai.NewClient(option.WithAPIKey("YOUR_API_KEY"))
	o.sttConnectedAt = time.Now()

	o.onPacket(
		internal_type.ObservabilityMetricRecordPacket{
			Scope:  internal_type.ObservabilityRecordScopeConversation,
			Record: observability.NewMetricSTTInitLatencyMs(time.Since(start), observability.Attributes{"provider": o.Name()}),
		},
		internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelInfo,
				Message: "openai-stt: initialization completed",
				Attributes: observability.Attributes{
					"component": observability.ComponentSTT.String(),
					"provider":  o.Name(),
				},
				OccurredAt: time.Now(),
			},
		})
	return nil
}

func (o *openaiSpeechToText) Close(ctx context.Context) error {
	if o.cancel != nil {
		o.cancel()
	}
	connectedAt := o.sttConnectedAt
	o.sttConnectedAt = time.Time{}
	if !connectedAt.IsZero() {
		duration := time.Since(connectedAt)
		o.onPacket(
			internal_type.ObservabilityMetricRecordPacket{
				Scope:  internal_type.ObservabilityRecordScopeConversation,
				Record: observability.NewMetricSTTDuration(duration, observability.Attributes{"provider": o.Name()}),
			},
			internal_type.ObservabilityUsageRecordPacket{
				Scope:  internal_type.ObservabilityRecordScopeConversation,
				Record: observability.NewSTTDurationUsageRecord(o.Name(), duration, observability.Attributes{}),
			},
		)
	}
	o.onPacket(
		internal_type.ObservabilityEventRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordEvent{
				Component: observability.ComponentSTT,
				Event:     observability.STTClosed,
				Attributes: observability.Attributes{
					"type":     "closed",
					"provider": o.Name(),
				},
				OccurredAt: time.Now(),
			},
		})
	return nil
}

func (o *openaiSpeechToText) Name() string {
	return "openai-stt"
}

// Transform receives a stream of bytes (audioStream) and prints transcribed text in realtime.
func (o *openaiSpeechToText) Transform(ctx context.Context, byt internal_type.Packet) error {
	switch byt.(type) {
	case internal_type.TurnChangePacket:
		if pkt, ok := byt.(internal_type.TurnChangePacket); ok {
			o.contextId = pkt.ContextID
		}
		return nil
	case internal_type.SpeechToTextEndPacket:
		return nil
	case internal_type.SpeechToTextAudioPacket:
		return nil
	default:
		return nil
	}
}

func NewOpenaiSpeechToText(
	ctx context.Context,
	logger commons.Logger,
	onPacket func(pkt ...internal_type.Packet) error,
	opts utils.Option,
) (internal_type.SpeechToTextTransformer, error) {
	stt := &openaiSpeechToText{
		logger:   logger,
		onPacket: onPacket,
	}
	return stt, nil
}
