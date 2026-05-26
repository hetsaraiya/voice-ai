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
	observe "github.com/rapidaai/api/assistant-api/internal/observe"
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
			t.OnPacket(t.streamer.Context(), internal_type.RecordUserAudioPacket{ContextID: t.GetID(), Audio: payload.Audio})
		case *protos.ConversationBridgeOperatorAudio:
			t.OnPacket(t.streamer.Context(), internal_type.RecordAssistantAudioPacket{ContextID: t.GetID(), Audio: payload.Audio})
		case *protos.ConversationMetadata:
			t.OnPacket(t.streamer.Context(), internal_type.ConversationMetadataPacket{
				ContextID: payload.GetAssistantConversationId(),
				Metadata:  payload.GetMetadata(),
			})
		case *protos.ConversationMetric:
			t.OnPacket(t.streamer.Context(), internal_type.ConversationMetricPacket{
				ContextID: payload.GetAssistantConversationId(),
				Metrics:   payload.GetMetrics(),
			})
		case *protos.ConversationEvent:
			eventTime := time.Now()
			if payload.Time != nil {
				eventTime = payload.Time.AsTime()
			}
			t.OnPacket(t.streamer.Context(), internal_type.ConversationEventPacket{
				Name: payload.Name,
				Data: payload.Data,
				Time: eventTime,
			})
		case *protos.ConversationDisconnection:
			if t.Conversation() == nil {
				return nil
			}
			ctx := context.Background()
			t.Notify(ctx, payload)
			t.OnPacket(ctx,
				internal_type.ConversationEventPacket{
					ContextID: t.GetID(),
					Name:      observe.ComponentSession,
					Data: map[string]string{
						observe.DataType:   observe.EventDisconnectRequested,
						observe.DataReason: payload.GetType().String()},
					Time: time.Now(),
				},
				internal_type.ConversationMetadataPacket{
					ContextID: t.Conversation().Id,
					Metadata: []*protos.Metadata{{
						Key:   "disconnect_reason",
						Value: payload.GetType().String(),
					}},
				})
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
		internal_type.ConversationMetricPacket{
			ContextID: conv.Id,
			Metrics: []*protos.Metric{
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
			},
		},
		internal_type.ConversationEventPacket{
			ContextID: t.GetID(),
			Name:      observe.ComponentSession,
			Data: map[string]string{
				observe.DataType:     observe.EventCleanup,
				observe.DataDuration: fmt.Sprintf("%d", duration.Milliseconds()),
				observe.DataMessages: fmt.Sprintf("%d", len(t.GetHistories())),
			},
			Time: time.Now(),
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
				internal_type.ConversationEventPacket{
					ContextID: r.GetID(),
					Name:      observe.ComponentSession,
					Data: map[string]string{
						observe.DataType:   observe.EventDisconnectRequested,
						observe.DataReason: protos.ConversationDisconnection_DISCONNECTION_TYPE_ERROR.String()},
					Time: time.Now(),
				},
				internal_type.ConversationMetadataPacket{
					ContextID: r.Conversation().Id,
					Metadata: []*protos.Metadata{{
						Key:   "disconnect_reason",
						Value: protos.ConversationDisconnection_DISCONNECTION_TYPE_ERROR.String(),
					}},
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
			internal_type.ConversationEventPacket{
				ContextID: r.GetID(),
				Name:      observe.ComponentSession,
				Data:      map[string]string{observe.DataType: observe.EventInitializing, observe.DataMode: config.GetStreamMode().String()},
				Time:      time.Now(),
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
