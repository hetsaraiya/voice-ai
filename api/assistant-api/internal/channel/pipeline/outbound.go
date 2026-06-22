// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package channel_pipeline

import (
	"context"
	"fmt"

	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/api/assistant-api/internal/observability/collectors"
	observability_collector_requestlog "github.com/rapidaai/api/assistant-api/internal/observability/collectors/requestlog"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

func (d *Dispatcher) runOutbound(ctx context.Context, v OutboundRequestedPipeline) *PipelineResult {
	assistantScopedCollectors := make([]observability.Collector, 0)
	assistantScopedCollectors = append(assistantScopedCollectors,
		observability_collector_requestlog.New(observability_collector_requestlog.Config{
			Logger:         d.logger,
			HTTPLogService: d.httpLogService,
		}),
	)
	assistantScopedCollectors = append(assistantScopedCollectors, collectors.NewWithAssistantWebhook(ctx, d.logger, v.Auth, v.AssistantID, d.webhookService, v.Observer)...)
	if err := v.Observer.AddCollectors(assistantScopedCollectors...); err != nil {
		d.logger.Warnw("observability collector registration failed",
			"component", "call",
			"operation", "add_assistant_collectors",
			"assistant_id", v.AssistantID,
			"direction", "outbound",
			"error", err,
		)
	}

	assistant, err := d.assistantService.Get(ctx, v.Auth, v.AssistantID, utils.GetVersionDefinition("latest"), &internal_services.GetAssistantOption{InjectPhoneDeployment: true})
	if err != nil {
		_ = v.Observer.Record(ctx, observability.AssistantScope{AssistantID: v.AssistantID}, observability.RecordLog{
			Level:   observability.LevelError,
			Message: "Outbound call pipeline failed to load assistant",
			Attributes: observability.Attributes{
				"assistant_id": fmt.Sprintf("%d", v.AssistantID),
				"to":           v.ToPhone,
				"from":         v.FromPhone,
				"error":        err.Error(),
			},
		}, observability.RecordWebhook{
			Event: observability.CallFailed,
			Payload: map[string]interface{}{
				"event": observability.CallFailed.String(),
				"assistant": map[string]interface{}{
					"id": v.AssistantID,
				},
				"data": map[string]interface{}{
					"stage":     "assistant_load",
					"to":        v.ToPhone,
					"from":      v.FromPhone,
					"direction": "outbound",
					"error":     err.Error(),
				},
			},
		})
		return &PipelineResult{Error: fmt.Errorf("invalid assistant: %w", err)}
	}
	if assistant.AssistantPhoneDeployment == nil {
		_ = v.Observer.Record(ctx, observability.AssistantScope{AssistantID: v.AssistantID}, observability.RecordLog{
			Level:   observability.LevelError,
			Message: "Outbound call pipeline failed because phone deployment is not enabled",
			Attributes: observability.Attributes{
				"assistant_id": fmt.Sprintf("%d", assistant.Id),
				"to":           v.ToPhone,
				"from":         v.FromPhone,
			},
		}, observability.RecordWebhook{
			Event: observability.CallFailed,
			Payload: map[string]interface{}{
				"event": observability.CallFailed.String(),
				"assistant": map[string]interface{}{
					"id": assistant.Id,
				},
				"data": map[string]interface{}{
					"stage":     "phone_deployment",
					"to":        v.ToPhone,
					"from":      v.FromPhone,
					"direction": "outbound",
					"error":     "phone deployment not enabled",
				},
			},
		})
		return &PipelineResult{Error: fmt.Errorf("phone deployment not enabled")}
	}

	fromPhone := v.FromPhone
	if fromPhone == "" {
		fn, err := assistant.AssistantPhoneDeployment.GetOptions().GetString("phone")
		if err != nil {
			_ = v.Observer.Record(ctx, observability.AssistantScope{AssistantID: v.AssistantID}, observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Outbound call pipeline failed to resolve from phone number",
				Attributes: observability.Attributes{
					"assistant_id": fmt.Sprintf("%d", assistant.Id),
					"to":           v.ToPhone,
					"error":        err.Error(),
				},
			}, observability.RecordWebhook{
				Event: observability.CallFailed,
				Payload: map[string]interface{}{
					"event": observability.CallFailed.String(),
					"assistant": map[string]interface{}{
						"id": assistant.Id,
					},
					"data": map[string]interface{}{
						"stage":     "from_phone_resolve",
						"to":        v.ToPhone,
						"direction": "outbound",
						"error":     err.Error(),
					},
				},
			})
			return &PipelineResult{Error: fmt.Errorf("no phone number configured: %w", err)}
		}
		fromPhone = fn
	}
	provider := assistant.AssistantPhoneDeployment.TelephonyProvider

	conversation, err := d.conversationService.CreateConversation(ctx, v.Auth, v.ToPhone, assistant.Id, assistant.AssistantProviderId, type_enums.DIRECTION_OUTBOUND, utils.PhoneCall)
	if err != nil {
		_ = v.Observer.Record(ctx, observability.AssistantScope{AssistantID: v.AssistantID}, observability.RecordLog{
			Level:   observability.LevelError,
			Message: "Outbound call pipeline failed to create conversation",
			Attributes: observability.Attributes{
				"assistant_id": fmt.Sprintf("%d", assistant.Id),
				"provider":     assistant.AssistantPhoneDeployment.TelephonyProvider,
				"to":           v.ToPhone,
				"from":         fromPhone,
				"error":        err.Error(),
			},
		}, observability.RecordWebhook{
			Event: observability.CallFailed,
			Payload: map[string]interface{}{
				"event": observability.CallFailed.String(),
				"assistant": map[string]interface{}{
					"id": assistant.Id,
				},
				"data": map[string]interface{}{
					"stage":     "conversation_create",
					"provider":  assistant.AssistantPhoneDeployment.TelephonyProvider,
					"to":        v.ToPhone,
					"from":      fromPhone,
					"direction": "outbound",
					"error":     err.Error(),
				},
			},
		})
		return &PipelineResult{Error: fmt.Errorf("failed to create conversation: %w", err)}
	}

	_ = v.Observer.Record(ctx, observability.ConversationScope{
		AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
		ConversationID: conversation.Id,
	}, observability.RecordEvent{
		Event: observability.CallConversationCreated,
		Attributes: observability.Attributes{
			"provider": assistant.AssistantPhoneDeployment.TelephonyProvider,
			"to":       v.ToPhone,
			"from":     fromPhone,
		},
	})

	if len(v.Options) > 0 {
		if _, err := d.conversationService.CreateOrUpdateConversationOption(ctx, v.Auth, assistant.Id, conversation.Id, v.Options); err != nil {
			d.logger.Warnw("Failed to CreateOrUpdate conversation extras", "error", err)
		}
	}
	if len(v.Args) > 0 {
		if _, err := d.conversationService.CreateOrUpdateConversationArgument(ctx, v.Auth, assistant.Id, conversation.Id, v.Args); err != nil {
			d.logger.Warnw("Failed to CreateOrUpdate conversation extras", "error", err)
		}
	}
	if len(v.Metadata) > 0 {
		conversationMetadata := make([]*protos.Metadata, 0, len(v.Metadata))
		for key, value := range v.Metadata {
			conversationMetadata = append(conversationMetadata, &protos.Metadata{Key: key, Value: fmt.Sprintf("%v", value)})
		}
		_ = v.Observer.Record(ctx, observability.ConversationScope{
			AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
			ConversationID: conversation.Id,
		}, observability.RecordMetadata{
			Metadata: conversationMetadata,
		})
	}
	callInfo := &internal_type.CallInfo{CallerNumber: v.ToPhone, FromNumber: fromPhone, Direction: "outbound", Provider: provider, Status: callcontext.CallStatusNew}
	contextID, err := d.inboundDispatcher.SaveCallContext(ctx, v.Auth, assistant, conversation.Id, callInfo, provider)
	if err != nil {
		_ = v.Observer.Record(ctx, observability.ConversationScope{
			AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
			ConversationID: conversation.Id,
		}, observability.RecordLog{
			Level:   observability.LevelError,
			Message: "Outbound call pipeline failed to save call context",
			Attributes: observability.Attributes{
				"provider": assistant.AssistantPhoneDeployment.TelephonyProvider,
				"to":       v.ToPhone,
				"from":     fromPhone,
				"error":    err.Error(),
			},
		}, observability.RecordWebhook{
			Event: observability.CallFailed,
			Payload: map[string]interface{}{
				"event": observability.CallFailed.String(),
				"assistant": map[string]interface{}{
					"id": assistant.Id,
				},
				"conversation": map[string]interface{}{
					"id": conversation.Id,
				},
				"data": map[string]interface{}{
					"stage":     "call_context_save",
					"provider":  assistant.AssistantPhoneDeployment.TelephonyProvider,
					"to":        v.ToPhone,
					"from":      fromPhone,
					"direction": "outbound",
					"error":     err.Error(),
				},
			},
		})
		return &PipelineResult{Error: fmt.Errorf("failed to save call context: %w", err)}
	}
	_ = v.Observer.Record(ctx, observability.ConversationScope{
		AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
		ConversationID: conversation.Id,
	},
		observability.RecordEvent{
			Event: observability.CallContextSaved,
			Attributes: observability.Attributes{
				"provider":   assistant.AssistantPhoneDeployment.TelephonyProvider,
				"to":         v.ToPhone,
				"from":       fromPhone,
				"context_id": contextID,
			},
		},
		observability.RecordMetadata{
			Metadata: observability.ClientMetadata(
				v.ToPhone, fromPhone, "outbound", assistant.AssistantPhoneDeployment.TelephonyProvider,
				"", contextID, "", "",
			),
		},
		observability.RecordEvent{
			Event: observability.CallOutboundRequested,
			Attributes: observability.Attributes{
				"provider":   assistant.AssistantPhoneDeployment.TelephonyProvider,
				"to":         v.ToPhone,
				"from":       fromPhone,
				"context_id": contextID,
			},
		},
		observability.RecordWebhook{
			Event:     observability.CallOutboundRequested,
			ContextID: contextID,
			Payload: map[string]interface{}{
				"event": observability.CallOutboundRequested.String(),
				"assistant": map[string]interface{}{
					"id": assistant.Id,
				},
				"conversation": map[string]interface{}{
					"id": conversation.Id,
				},
				"data": map[string]interface{}{
					"provider":   assistant.AssistantPhoneDeployment.TelephonyProvider,
					"to":         v.ToPhone,
					"from":       fromPhone,
					"context_id": contextID,
					"direction":  "outbound",
				},
			},
		})

	callInfo, err = d.outboundDispatcher.Dispatch(ctx, contextID)
	if err != nil {
		_ = v.Observer.Record(ctx, observability.ConversationScope{
			AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
			ConversationID: conversation.Id,
		}, observability.RecordLog{
			Level:   observability.LevelError,
			Message: "Outbound call pipeline failed to dispatch provider call",
			Attributes: observability.Attributes{
				"provider":   assistant.AssistantPhoneDeployment.TelephonyProvider,
				"to":         v.ToPhone,
				"from":       fromPhone,
				"context_id": contextID,
				"error":      err.Error(),
			},
		}, observability.RecordEvent{
			Event: observability.CallOutboundDispatchFailed,
			Attributes: observability.Attributes{
				"provider":   assistant.AssistantPhoneDeployment.TelephonyProvider,
				"context_id": contextID,
				"error":      err.Error(),
			},
		}, observability.RecordWebhook{
			Event:     observability.CallOutboundDispatchFailed,
			ContextID: contextID,
			Payload: map[string]interface{}{
				"event": observability.CallOutboundDispatchFailed.String(),
				"assistant": map[string]interface{}{
					"id": assistant.Id,
				},
				"conversation": map[string]interface{}{
					"id": conversation.Id,
				},
				"data": map[string]interface{}{
					"provider":   assistant.AssistantPhoneDeployment.TelephonyProvider,
					"to":         v.ToPhone,
					"from":       fromPhone,
					"context_id": contextID,
					"direction":  "outbound",
					"error":      err.Error(),
				},
			},
		}, observability.RecordWebhook{
			Event:     observability.CallFailed,
			ContextID: contextID,
			Payload: map[string]interface{}{
				"event": observability.CallFailed.String(),
				"assistant": map[string]interface{}{
					"id": assistant.Id,
				},
				"conversation": map[string]interface{}{
					"id": conversation.Id,
				},
				"data": map[string]interface{}{
					"stage":      "provider_dispatch",
					"provider":   assistant.AssistantPhoneDeployment.TelephonyProvider,
					"to":         v.ToPhone,
					"from":       fromPhone,
					"context_id": contextID,
					"direction":  "outbound",
					"error":      err.Error(),
				},
			},
		})
		_ = v.Observer.Record(ctx, observability.ConversationScope{
			AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
			ConversationID: conversation.Id,
		}, observability.RecordMetric{
			Metrics: observability.CallStatusMetric("FAILED", err.Error()),
		})
		return &PipelineResult{ContextID: contextID, ConversationID: conversation.Id, Error: err}
	}

	_ = v.Observer.Record(ctx, observability.ConversationScope{
		AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
		ConversationID: conversation.Id,
	}, observability.RecordEvent{
		Event: observability.CallOutboundDispatched,
		Attributes: observability.Attributes{
			"provider":          assistant.AssistantPhoneDeployment.TelephonyProvider,
			"context_id":        contextID,
			"channel_uuid":      callInfo.ChannelUUID,
			"status_event":      callInfo.StatusInfo.Event,
			"provider_response": observability.AttributeValue(callInfo.StatusInfo.Payload),
		},
	}, observability.RecordWebhook{
		Event:     observability.CallOutboundDispatched,
		ContextID: contextID,
		Payload: map[string]interface{}{
			"event": observability.CallOutboundDispatched.String(),
			"assistant": map[string]interface{}{
				"id": assistant.Id,
			},
			"conversation": map[string]interface{}{
				"id": conversation.Id,
			},
			"data": map[string]interface{}{
				"provider":          assistant.AssistantPhoneDeployment.TelephonyProvider,
				"to":                v.ToPhone,
				"from":              fromPhone,
				"context_id":        contextID,
				"direction":         "outbound",
				"channel_uuid":      callInfo.ChannelUUID,
				"status_event":      callInfo.StatusInfo.Event,
				"provider_response": callInfo.StatusInfo.Payload,
			},
		},
	})

	return &PipelineResult{ContextID: contextID, ConversationID: conversation.Id}
}
