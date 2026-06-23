// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package channel_pipeline

import (
	"context"
	"strconv"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/api/assistant-api/internal/observability/collectors"
	observability_collector_requestlog "github.com/rapidaai/api/assistant-api/internal/observability/collectors/requestlog"
	observability_collector_toollog "github.com/rapidaai/api/assistant-api/internal/observability/collectors/toollog"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

func (d *Dispatcher) runInboundCall(ctx context.Context, v CallReceivedPipeline) *PipelineResult {
	if err := v.Observer.AddCollectors(
		observability_collector_requestlog.New(observability_collector_requestlog.Config{
			Logger:         d.logger,
			HTTPLogService: d.httpLogService,
		}),
		observability_collector_toollog.New(observability_collector_toollog.Config{
			Logger:      d.logger,
			ToolService: d.assistantToolService,
		}),
		collectors.NewWithAssistantWebhook(ctx, d.logger, v.Auth, v.AssistantID, d.webhookService, d.httpLogService)); err != nil {
		d.logger.Warnw("observability collector registration failed",
			"component", "call",
			"operation", "add_assistant_collectors",
			"assistant_id", v.AssistantID,
			"provider", v.Provider,
			"error", err,
		)
	}

	callInfo, err := d.inboundDispatcher.ReceiveCall(v.GinContext, v.Provider)
	if err != nil {
		_ = v.Observer.Record(ctx, observability.AssistantScope{AssistantID: v.AssistantID}, observability.RecordLog{
			Level:   observability.LevelError,
			Message: "inbound call receive failed",
			Attributes: observability.Attributes{
				"provider": v.Provider,
				"error":    err.Error(),
			},
		})
		return &PipelineResult{Error: err}
	}
	if callInfo == nil {
		_ = v.Observer.Record(ctx, observability.AssistantScope{AssistantID: v.AssistantID}, observability.RecordLog{
			Level:   observability.LevelDebug,
			Message: "inbound call ignored",
			Attributes: observability.Attributes{
				"provider": v.Provider,
			},
		})
		return &PipelineResult{}
	}

	_ = v.Observer.Record(ctx, observability.AssistantScope{AssistantID: v.AssistantID}, observability.RecordEvent{
		Event: observability.CallReceived,
		Attributes: observability.Attributes{
			"provider": v.Provider,
			"caller":   callInfo.CallerNumber,
		},
	}, observability.RecordWebhook{
		Event: observability.CallReceived,
		Payload: map[string]interface{}{
			"event": observability.CallReceived.String(),
			"assistant": map[string]interface{}{
				"id": v.AssistantID,
			},
			"data": map[string]interface{}{
				"provider":  v.Provider,
				"caller":    callInfo.CallerNumber,
				"from":      callInfo.FromNumber,
				"direction": "inbound",
			},
		},
	})

	assistant, err := d.assistantService.Get(ctx, v.Auth, v.AssistantID, utils.GetVersionDefinition("latest"), &internal_services.GetAssistantOption{InjectPhoneDeployment: true})
	if err != nil {
		_ = v.Observer.Record(ctx, observability.AssistantScope{AssistantID: v.AssistantID}, observability.RecordLog{
			Level:   observability.LevelError,
			Message: "inbound assistant load failed",
			Attributes: observability.Attributes{
				"provider":     v.Provider,
				"assistant_id": strconv.FormatUint(v.AssistantID, 10),
				"caller":       callInfo.CallerNumber,
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
					"provider":  v.Provider,
					"caller":    callInfo.CallerNumber,
					"from":      callInfo.FromNumber,
					"direction": "inbound",
					"error":     err.Error(),
				},
			},
		})
		return &PipelineResult{Error: err}
	}

	_ = v.Observer.Record(ctx, observability.AssistantScope{AssistantID: assistant.Id}, observability.RecordEvent{
		Event: observability.CallAssistantLoaded,
		Attributes: observability.Attributes{
			"provider": v.Provider,
			"caller":   callInfo.CallerNumber,
		},
	})

	conversation, err := d.conversationService.CreateConversation(ctx, v.Auth, callInfo.CallerNumber, assistant.Id, assistant.AssistantProviderId, type_enums.DIRECTION_INBOUND, utils.PhoneCall)
	if err != nil {
		_ = v.Observer.Record(ctx, observability.AssistantScope{AssistantID: v.AssistantID}, observability.RecordLog{
			Level:   observability.LevelError,
			Message: "inbound conversation create failed",
			Attributes: observability.Attributes{
				"provider": v.Provider,
				"caller":   callInfo.CallerNumber,
				"error":    err.Error(),
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
					"provider":  v.Provider,
					"caller":    callInfo.CallerNumber,
					"from":      callInfo.FromNumber,
					"direction": "inbound",
					"error":     err.Error(),
				},
			},
		})
		return &PipelineResult{Error: err}
	}

	_ = v.Observer.Record(ctx, observability.ConversationScope{
		AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
		ConversationID: conversation.Id,
	}, observability.RecordEvent{
		Event: observability.CallConversationCreated,
		Attributes: observability.Attributes{
			"provider": v.Provider,
			"caller":   callInfo.CallerNumber,
		},
	})

	contextID, err := d.inboundDispatcher.SaveCallContext(ctx, v.Auth, assistant, conversation.Id, callInfo, v.Provider)
	if err != nil {
		_ = v.Observer.Record(ctx, observability.ConversationScope{
			AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
			ConversationID: conversation.Id,
		}, observability.RecordLog{
			Level:   observability.LevelError,
			Message: "inbound call context save failed",
			Attributes: observability.Attributes{
				"provider": v.Provider,
				"caller":   callInfo.CallerNumber,
				"error":    err.Error(),
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
					"stage":     "call_context_save",
					"provider":  v.Provider,
					"caller":    callInfo.CallerNumber,
					"from":      callInfo.FromNumber,
					"direction": "inbound",
					"error":     err.Error(),
				},
			},
		})
		return &PipelineResult{Error: err}
	}
	_ = v.Observer.Record(ctx, observability.ConversationScope{
		AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
		ConversationID: conversation.Id,
	}, observability.RecordEvent{
		Event: observability.CallContextSaved,
		Attributes: observability.Attributes{
			"provider":   v.Provider,
			"caller":     callInfo.CallerNumber,
			"context_id": contextID,
		},
	})

	if len(callInfo.Extra) > 0 {
		metadata := make([]*protos.Metadata, 0, len(callInfo.Extra))
		for key, value := range callInfo.Extra {
			metadata = append(metadata, &protos.Metadata{Key: key, Value: value})
		}
		_ = v.Observer.Record(ctx, observability.ConversationScope{
			AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
			ConversationID: conversation.Id,
		}, observability.RecordMetadata{
			Metadata: metadata,
		})
	}
	if callInfo.StatusInfo.Event != "" {
		_ = v.Observer.Record(ctx,
			observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
				ConversationID: conversation.Id,
			}, observability.RecordEvent{
				Event: observability.CallStatus,
				Attributes: observability.Attributes{
					"provider":     v.Provider,
					"caller":       callInfo.CallerNumber,
					"status_event": callInfo.StatusInfo.Event,
				},
			})
	}

	v.GinContext.Set("contextId", contextID)
	if err := d.inboundDispatcher.AnswerProvider(v.GinContext, v.Auth, v.Provider, v.AssistantID, callInfo.CallerNumber, conversation.Id); err != nil {
		_ = v.Observer.Record(ctx, observability.ConversationScope{
			AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
			ConversationID: conversation.Id,
		}, observability.RecordLog{
			Level:   observability.LevelError,
			Message: "inbound provider answer failed",
			Attributes: observability.Attributes{
				"provider":   v.Provider,
				"caller":     callInfo.CallerNumber,
				"context_id": contextID,
				"error":      err.Error(),
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
					"stage":      "provider_answer",
					"provider":   v.Provider,
					"caller":     callInfo.CallerNumber,
					"from":       callInfo.FromNumber,
					"context_id": contextID,
					"direction":  "inbound",
					"error":      err.Error(),
				},
			},
		})
		return &PipelineResult{Error: err}
	}
	_ = v.Observer.Record(ctx,
		observability.ConversationScope{
			AssistantScope: observability.AssistantScope{AssistantID: v.AssistantID},
			ConversationID: conversation.Id,
		},
		observability.RecordEvent{
			Event: observability.CallProviderAnswered,
			Attributes: observability.Attributes{
				"provider":   v.Provider,
				"caller":     callInfo.CallerNumber,
				"context_id": contextID,
			},
		},
		observability.RecordWebhook{
			Event:     observability.CallProviderAnswered,
			ContextID: contextID,
			Payload: map[string]interface{}{
				"event": observability.CallProviderAnswered.String(),
				"assistant": map[string]interface{}{
					"id": v.AssistantID,
				},
				"conversation": map[string]interface{}{
					"id": conversation.Id,
				},
				"data": map[string]interface{}{
					"provider":   v.Provider,
					"caller":     callInfo.CallerNumber,
					"from":       callInfo.FromNumber,
					"context_id": contextID,
					"direction":  "inbound",
				},
			},
		})

	return &PipelineResult{ContextID: contextID, ConversationID: conversation.Id}
}
