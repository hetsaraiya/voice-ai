// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package adapter_internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

// GetBehavior retrieves the deployment behavior configuration based on the source type.
func (r *genericRequestor) GetBehavior() (*internal_assistant_entity.AssistantDeploymentBehavior, error) {
	if r.assistant == nil {
		return nil, errDeploymentNotEnabled
	}

	switch r.source {
	case utils.PhoneCall:
		if r.assistant.AssistantPhoneDeployment != nil {
			return &r.assistant.AssistantPhoneDeployment.AssistantDeploymentBehavior, nil
		}
	case utils.Whatsapp:
		if r.assistant.AssistantWhatsappDeployment != nil {
			return &r.assistant.AssistantWhatsappDeployment.AssistantDeploymentBehavior, nil
		}
	case utils.SDK:
		if r.assistant.AssistantApiDeployment != nil {
			return &r.assistant.AssistantApiDeployment.AssistantDeploymentBehavior, nil
		}
	case utils.WebPlugin:
		if r.assistant.AssistantWebPluginDeployment != nil {
			return &r.assistant.AssistantWebPluginDeployment.AssistantDeploymentBehavior, nil
		}
	case utils.Debugger:
		if r.assistant.AssistantDebuggerDeployment != nil {
			return &r.assistant.AssistantDebuggerDeployment.AssistantDeploymentBehavior, nil
		}
	}

	return nil, errDeploymentNotEnabled
}

// initializeGreeting sends the greeting message if configured.
func (r *genericRequestor) initializeGreeting(ctx context.Context, behavior *internal_assistant_entity.AssistantDeploymentBehavior) {
	if behavior.Greeting == nil {
		return
	}
	greetingContent := *behavior.Greeting
	if strings.TrimSpace(greetingContent) == "" {
		return
	}
	contextID := r.GetID()
	if r.GetMode().Audio() && behavior.GreetingInterruptible != nil && !*behavior.GreetingInterruptible {
		_ = r.OnPacket(ctx,
			internal_type.DispatchPolicyPacket{
				ContextID: contextID,
				Policy: internal_type.DispatchPolicy{
					Target: internal_type.PacketNameUserAudioReceived,
					Action: internal_type.DispatchActionIgnore,
				},
			},
			internal_type.DispatchPolicyPacket{
				ContextID: contextID,
				Policy: internal_type.DispatchPolicy{
					Target: internal_type.PacketNameInterruptionDetected,
					Action: internal_type.DispatchActionIgnore,
				},
			},
		)
	}
	_ = r.OnPacket(ctx,
		internal_type.InjectMessagePacket{ContextID: contextID, Text: greetingContent},
		internal_type.ObservabilityEventRecordPacket{
			ContextID: contextID,
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.NewConversationEventRecord(observability.ConversationAgentStateChanged, observability.Attributes{
				"type":       "greeting",
				"text_chars": fmt.Sprintf("%d", len(greetingContent)),
			}),
		},
		internal_type.StartIdleTimeoutPacket{ContextID: contextID},
	)
}

// initializeIdleTimeout starts the idle timeout timer if configured.
// Goes through the packet pipeline so initialization shares the same timer
// lifecycle path as every other start/stop site.
func (r *genericRequestor) initializeIdleTimeout(ctx context.Context, behavior *internal_assistant_entity.AssistantDeploymentBehavior) {
	if behavior.IdleTimeout == nil || *behavior.IdleTimeout <= 0 {
		return
	}
	_ = r.OnPacket(ctx, internal_type.StartIdleTimeoutPacket{ContextID: r.GetID()})
}

// initializeMaxSessionDuration sets up the max session duration timer if configured.
func (r *genericRequestor) initializeMaxSessionDuration(ctx context.Context, behavior *internal_assistant_entity.AssistantDeploymentBehavior) {
	if behavior.MaxSessionDuration == nil || *behavior.MaxSessionDuration <= 0 {
		return
	}
	timeoutDuration := time.Duration(*behavior.MaxSessionDuration) * time.Second
	r.maxSessionTimer = time.AfterFunc(timeoutDuration, func() {
		if r.Ready() {
			r.OnPacket(r.sessionCtx,
				internal_type.ObservabilityEventRecordPacket{
					ContextID: r.GetID(),
					Scope:     internal_type.ObservabilityRecordScopeConversation,
					Record: observability.NewConversationEventRecord(observability.ConversationCompleted, observability.Attributes{
						"reason": protos.ConversationDisconnection_DISCONNECTION_TYPE_MAX_DURATION.String(),
					}),
				},
				internal_type.ObservabilityMetadataRecordPacket{
					ContextID: r.GetID(),
					Scope:     internal_type.ObservabilityRecordScopeConversation,
					Record: observability.NewConversationMetadataRecord([]*protos.Metadata{{
						Key:   "disconnect_reason",
						Value: protos.ConversationDisconnection_DISCONNECTION_TYPE_MAX_DURATION.String(),
					}}),
				},
			)
		}
		r.Notify(ctx, &protos.ConversationDisconnection{
			Type: protos.ConversationDisconnection_DISCONNECTION_TYPE_MAX_DURATION,
		})
	})
}

// OnError handles error scenarios by sending a configured or default error message.
func (r *genericRequestor) OnError(ctx context.Context) error {
	behavior, err := r.GetBehavior()
	if err != nil {
		return nil
	}

	const defaultMistakeMessage = "Oops! It looks like something went wrong. Let me look into that for you right away. I really appreciate your patience—hang tight while I get this sorted!"

	mistakeContent := defaultMistakeMessage
	if behavior.Mistake != nil {
		mistakeContent = *behavior.Mistake
	}

	r.Transition(Interrupted)
	_ = r.OnPacket(ctx,
		internal_type.TextToSpeechInterruptPacket{ContextID: r.GetID()},
		internal_type.InjectMessagePacket{ContextID: r.GetID(), Text: mistakeContent},
		internal_type.ObservabilityEventRecordPacket{
			ContextID: r.GetID(),
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.NewConversationEventRecord(observability.ConversationAgentStateChanged, observability.Attributes{
				"type":       "error",
				"text_chars": fmt.Sprintf("%d", len(mistakeContent)),
			}),
		},
		internal_type.StartIdleTimeoutPacket{ContextID: r.GetID()},
	)

	return nil
}

// OnIdleTimeout handles the behavior when the bot has spoken but the user
// has not responded within the idle timeout duration.
// If configured, it will prompt the user or end the conversation after max retries.
func (r *genericRequestor) onIdleTimeout(ctx context.Context) error {
	behavior, err := r.GetBehavior()
	if err != nil {
		return nil
	}

	if behavior.IdleTimeout == nil || *behavior.IdleTimeout == 0 {
		return nil
	}

	// Check if max backoff retries reached
	if behavior.IdleTimeoutBackoff != nil && *behavior.IdleTimeoutBackoff > 0 {
		if r.idleTimeoutCount >= *behavior.IdleTimeoutBackoff {
			if r.Ready() {
				r.OnPacket(r.sessionCtx,
					internal_type.ObservabilityEventRecordPacket{
						ContextID: r.GetID(),
						Scope:     internal_type.ObservabilityRecordScopeConversation,
						Record: observability.NewConversationEventRecord(observability.ConversationCompleted, observability.Attributes{
							"reason": protos.ConversationDisconnection_DISCONNECTION_TYPE_IDLE_TIMEOUT.String(),
						}),
					},
					internal_type.ObservabilityMetadataRecordPacket{
						ContextID: r.GetID(),
						Scope:     internal_type.ObservabilityRecordScopeConversation,
						Record: observability.NewConversationMetadataRecord([]*protos.Metadata{{
							Key:   "disconnect_reason",
							Value: protos.ConversationDisconnection_DISCONNECTION_TYPE_IDLE_TIMEOUT.String(),
						}}),
					},
				)
			}

			r.Notify(ctx, &protos.ConversationDisconnection{
				Type: protos.ConversationDisconnection_DISCONNECTION_TYPE_IDLE_TIMEOUT,
			})
			return nil
		}
	}

	r.idleTimeoutCount++
	timeoutContent := r.getIdleTimeoutMessage(behavior)
	if timeoutContent == "" {
		return nil
	}

	maxCount := 0
	if behavior.IdleTimeoutBackoff != nil {
		maxCount = int(*behavior.IdleTimeoutBackoff)
	}
	// Rotate context before injecting the timeout message.
	// InjectMessagePacket routes to outputCh; the context must be rotated
	// before enqueueing so GetID() returns the new context.
	r.Transition(Interrupted)
	contextID := r.GetID()

	_ = r.OnPacket(ctx,
		internal_type.TextToSpeechInterruptPacket{ContextID: contextID},
		internal_type.InjectMessagePacket{ContextID: contextID, Text: timeoutContent},
		internal_type.ObservabilityEventRecordPacket{
			ContextID: contextID,
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.NewConversationEventRecord(observability.ConversationAgentStateChanged, observability.Attributes{
				"type":      "idle_timeout",
				"count":     fmt.Sprintf("%d", r.idleTimeoutCount),
				"max_count": fmt.Sprintf("%d", maxCount),
			}),
		},
		internal_type.StartIdleTimeoutPacket{ContextID: contextID},
	)

	return nil
}

// getIdleTimeoutMessage returns the configured or default idle timeout message.
func (r *genericRequestor) getIdleTimeoutMessage(behavior *internal_assistant_entity.AssistantDeploymentBehavior) string {
	const defaultTimeoutMessage = "Are you still there?"

	if behavior.IdleTimeoutMessage != nil && strings.TrimSpace(*behavior.IdleTimeoutMessage) != "" {
		return *behavior.IdleTimeoutMessage
	}

	return defaultTimeoutMessage
}

// extendIdleTimeoutTimer pushes the existing idle timeout further into the future
// by the given duration. Used to account for buffered TTS audio that the client
// is still playing back. Per-chunk hot path called from handleTTSAudio on the
// output dispatcher goroutine — not converted to a packet to avoid per-chunk
// channel hops. No-op if the timer is not currently running.
func (r *genericRequestor) extendIdleTimeoutTimer(d time.Duration) {
	if r.idleTimeoutTimer == nil || d <= 0 {
		return
	}
	r.idleTimeoutDeadline = r.idleTimeoutDeadline.Add(d)
	if remaining := time.Until(r.idleTimeoutDeadline); remaining > 0 {
		r.idleTimeoutTimer.Reset(remaining)
	}
}
