// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package adapter_internal

import (
	"context"
	"fmt"
	"time"

	adapter_lifecycle "github.com/rapidaai/api/assistant-api/internal/adapters/lifecycle"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

// =============================================================================
// Talk - Main Entry Point
// =============================================================================

// Talk handles the main conversation loop for different streamer types.
// It processes incoming messages and manages the connection lifecycle.
//
// Shutdown relies on Recv() returning an error (EOF or context-cancelled)
// or a ConversationDisconnection message. All streamer implementations
// guarantee one of these when the connection ends.
func (t *genericRequestor) Talk(_ context.Context, auth types.SimplePrinciple) error {
	totalTime := time.Now()
	for {
		req, err := t.streamer.Recv()
		if err != nil {
			t.OnCallCompletion(totalTime)
			t.OnDisconnect(context.Background())
			return nil
		}
		switch payload := req.(type) {
		case *protos.ConversationInitialization:
			t.OnConnect(t.streamer.Context(), auth, payload)
		case *protos.ConversationConfiguration:
			t.OnStreamModeSwitch(t.streamer.Context(), payload)
		case *protos.ConversationUserMessage:
			t.OnStreamUserMessage(t.streamer.Context(), payload)
		case *protos.ConversationToolCallResult:
			t.OnPacket(t.streamer.Context(), internal_type.LLMToolResultPacket{
				ToolID:    payload.GetToolId(),
				Name:      payload.GetName(),
				ContextID: payload.GetId(),
				Action:    payload.GetAction(),
				Result:    payload.GetResult(),
			})
		case *protos.ConversationBridgeUserAudio:
			t.OnPacket(t.streamer.Context(), internal_type.RecordUserAudioPacket{ContextID: t.GetID(), Audio: payload.Audio, Timestamp: payload.Time.AsTime()})
		case *protos.ConversationBridgeOperatorAudio:
			t.OnPacket(t.streamer.Context(), internal_type.RecordAssistantAudioPacket{ContextID: t.GetID(), Audio: payload.Audio, Timestamp: payload.Time.AsTime()})
		case *protos.ConversationMetadata:
			t.OnPacket(t.streamer.Context(), internal_type.ObservabilityMetadataRecordPacket{
				ContextID: fmt.Sprintf("%d", payload.GetAssistantConversationId()),
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record:    observability.NewConversationMetadataRecord(payload.GetMetadata()),
			})
		case *protos.ConversationMetric:
			t.OnPacket(t.streamer.Context(), internal_type.ObservabilityMetricRecordPacket{
				ContextID: fmt.Sprintf("%d", payload.GetAssistantConversationId()),
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record:    observability.NewConversationMetricRecord(payload.GetMetrics()),
			})
		case *protos.ConversationEvent:
			eventTime := time.Now()
			if payload.Time != nil {
				eventTime = payload.Time.AsTime()
			}
			t.OnPacket(t.streamer.Context(), internal_type.ObservabilityEventRecordPacket{
				ContextID: payload.GetId(),
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordEvent{
					Component:  observability.ComponentConversation,
					Event:      observability.EventName(payload.Name),
					Attributes: observability.Attributes(payload.Data),
					OccurredAt: eventTime,
				},
			})
		case *protos.ConversationDisconnection:
			if t.Conversation() != nil {
				ctx := context.Background()
				t.OnPacket(ctx,
					internal_type.ObservabilityEventRecordPacket{
						ContextID: t.GetID(),
						Scope:     internal_type.ObservabilityRecordScopeConversation,
						Record: observability.RecordEvent{
							Component: observability.ComponentSession,
							Event:     observability.SessionDisconnectRequested,
							Attributes: observability.Attributes{
								"reason": payload.GetType().String(),
							},
							OccurredAt: time.Now(),
						},
					},
					internal_type.ObservabilityMetadataRecordPacket{
						ContextID: fmt.Sprintf("%d", t.Conversation().Id),
						Scope:     internal_type.ObservabilityRecordScopeConversation,
						Record: observability.NewConversationMetadataRecord([]*protos.Metadata{{
							Key:   "disconnect_reason",
							Value: payload.GetType().String(),
						}}),
					})
			}

		}

	}
}

func (t *genericRequestor) OnStreamModeSwitch(ctx context.Context, payload *protos.ConversationConfiguration) {
	t.OnPacket(ctx, internal_type.ModeSwitchRequestedPacket{
		ContextID:   t.GetID(),
		StreamMode:  payload.GetStreamMode(),
		RequestedAt: time.Now(),
	})
}

func (t *genericRequestor) OnStreamUserMessage(ctx context.Context, payload *protos.ConversationUserMessage) {
	switch msg := payload.GetMessage().(type) {
	case *protos.ConversationUserMessage_Audio:
		t.OnPacket(ctx, internal_type.UserAudioReceivedPacket{ContextID: t.GetID(), Audio: msg.Audio})
	case *protos.ConversationUserMessage_Text:
		t.OnPacket(ctx, internal_type.UserTextReceivedPacket{ContextID: t.GetID(), Text: msg.Text})
	default:
		t.logger.Errorf("illegal input from the user %+v", msg)
	}
}

// OnCallCompletion emits final metrics + an EventCompleted event when the talk
// loop exits. Persistence and telemetry collection happen in the existing
// background-channel handlers, so this function only enqueues packets.
func (t *genericRequestor) OnCallCompletion(startTime time.Time) {
	conv := t.Conversation()
	if conv == nil {
		return
	}
	duration := time.Since(startTime)
	t.OnPacket(context.Background(),
		internal_type.ObservabilityMetricRecordPacket{
			ContextID: fmt.Sprintf("%d", conv.Id),
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.NewConversationMetricRecord([]*protos.Metric{
				{
					Name:        type_enums.CONVERSATION_STATUS.String(),
					Value:       type_enums.CONVERSATION_COMPLETE.String(),
					Description: "Status of current conversation",
				},
				{
					Name:        type_enums.CONVERSATION_DURATION.String(),
					Value:       fmt.Sprintf("%d", duration),
					Description: "Conversation duration from first message to end",
				},
			}),
		},
		internal_type.ObservabilityEventRecordPacket{
			ContextID: t.GetID(),
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordEvent{
				Component: observability.ComponentSession,
				Event:     observability.SessionCleanup,
				Attributes: observability.Attributes{
					"duration_ms": fmt.Sprintf("%d", duration.Milliseconds()),
					"messages":    fmt.Sprintf("%d", len(t.GetHistories())),
				},
				OccurredAt: time.Now(),
			},
		},
	)
}

// Notify sends notifications to websocket for various events.
func (t *genericRequestor) Notify(ctx context.Context, actionDatas ...internal_type.Stream) error {
	for _, actionData := range actionDatas {
		t.streamer.Send(actionData)
	}
	return nil
}

// =============================================================================
// Session Lifecycle
// =============================================================================

// Connect starts bootstrap/background dispatchers and enqueues the init chain.
// Runtime dispatchers (critical/ingress/egress) are started after
// InitializationCompleted. Connect always returns nil because initialization
// runs asynchronously on the bootstrap dispatcher goroutine.
// The gRPC stream is already open by the time Connect is called; any init errors
// are delivered to the client via InitializationFailedPacket → ConversationError
// proto on the stream, not via this return value.
func (r *genericRequestor) OnConnect(ctx context.Context, auth types.SimplePrinciple, config *protos.ConversationInitialization) {
	if err := r.sessionLifecycle.Transition(adapter_lifecycle.EventConnectRequested); err != nil {
		r.logger.Tracef(ctx, "connect ignored due to session lifecycle transition: %v", err)
		return
	}
	r.SetAuth(auth)
	utils.WithDeadline(r.sessionCtx, connectDeadline, func() {
		if r.sessionLifecycle.Current() != adapter_lifecycle.StateInitializing {
			return
		}
		if r.Ready() {
			r.OnPacket(r.sessionCtx,
				internal_type.ObservabilityEventRecordPacket{
					ContextID: r.GetID(),
					Scope:     internal_type.ObservabilityRecordScopeConversation,
					Record: observability.RecordEvent{
						Component: observability.ComponentSession,
						Event:     observability.SessionDisconnectRequested,
						Attributes: observability.Attributes{
							"reason": protos.ConversationDisconnection_DISCONNECTION_TYPE_ERROR.String(),
						},
						OccurredAt: time.Now(),
					},
				},
				internal_type.ObservabilityMetadataRecordPacket{
					ContextID: fmt.Sprintf("%d", r.Conversation().Id),
					Scope:     internal_type.ObservabilityRecordScopeConversation,
					Record: observability.NewConversationMetadataRecord([]*protos.Metadata{{
						Key:   "disconnect_reason",
						Value: protos.ConversationDisconnection_DISCONNECTION_TYPE_ERROR.String(),
					}}),
				},
			)
		}
		r.Notify(r.sessionCtx,
			&protos.ConversationError{Message: "initialization timeout"},
			&protos.ConversationDisconnection{Type: protos.ConversationDisconnection_DISCONNECTION_TYPE_ERROR},
		)
		r.cancelSession()
	}, func(connectCtx context.Context) {
		r.OnPacket(r.sessionCtx,
			internal_type.ObservabilityEventRecordPacket{
				ContextID: r.GetID(),
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordEvent{
					Component: observability.ComponentSession,
					Event:     observability.SessionInitializing,
					Attributes: observability.Attributes{
						"mode": config.GetStreamMode().String(),
					},
					OccurredAt: time.Now(),
				},
			}, internal_type.InitializeAssistantPacket{
				ContextID: r.GetID(),
				Config:    config,
			})
	})
}

// OnDisconnect enqueues the disconnect chain. sessionCtx is cancelled either by
// HandleFinalizationCompleted (normal completion) or by the watchdog if the
// chain exceeds disconnectDeadline.
func (r *genericRequestor) OnDisconnect(ctx context.Context) {
	if err := r.sessionLifecycle.Transition(adapter_lifecycle.EventDisconnectRequested); err != nil {
		r.logger.Tracef(ctx, "disconnect ignored due to session lifecycle transition: %v", err)
		return
	}
	utils.WithDeadline(r.sessionCtx, disconnectDeadline, func() {
		r.logger.Warnf("disconnect deadline %v exceeded, force-cancelling session", disconnectDeadline)
		r.cancelSession()
	}, func(disconnectCtx context.Context) {
		r.OnPacket(disconnectCtx, internal_type.FinalizeBehaviorPacket{ContextID: r.GetID()})
	})
}
