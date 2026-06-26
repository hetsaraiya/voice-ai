// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package channel_telephony

import (
	"cmp"
	"context"
	"strconv"
	"time"

	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/api/assistant-api/internal/observability/collectors"
	observability_collector_conversationmetadata "github.com/rapidaai/api/assistant-api/internal/observability/collectors/conversationmetadata"
	observability_collector_conversationmetric "github.com/rapidaai/api/assistant-api/internal/observability/collectors/conversationmetric"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/protos"
)

func (d *OutboundDispatcher) NewStatusReporter(contextID string) internal_type.ProviderCallStatusReporter {
	return func(update internal_type.ProviderCallStatusUpdate) {
		ctx := context.Background()
		if update.ChannelUUID != "" {
			if err := d.store.UpdateField(ctx, contextID, "channel_uuid", update.ChannelUUID); err != nil {
				d.logger.Warnw("Failed to persist outbound channel UUID",
					"contextId", contextID,
					"channel_uuid", update.ChannelUUID,
					"error", err)
			}
		}
		if update.CallStatus == "" || update.CallStatus == callcontext.CallStatusNew {
			return
		}
		var currentCallContext *callcontext.CallContext
		if update.CallStatus == callcontext.CallStatusRinging {
			var getErr error
			currentCallContext, getErr = d.store.Get(ctx, contextID)
			if getErr != nil {
				d.logger.Warnw("Failed to resolve outbound call context for ringing status",
					"contextId", contextID,
					"call_status", update.CallStatus,
					"error", getErr)
			} else if currentCallContext.Status != callcontext.StatusPending {
				return
			}
		}
		if err := d.store.UpdateCallStatus(ctx, contextID, callcontext.CallStatusUpdate{
			ExpectedCallStatus: update.ExpectedCallStatus,
			CallStatus:         update.CallStatus,
			CallError:          update.ErrorMessage,
			FailureClass:       update.FailureClass,
			FailureReason:      update.FailureReason,
			DisconnectReason:   update.DisconnectReason,
			Retryable:          update.Retryable,
			ProviderStatusCode: update.ProviderStatusCode,
		}); err != nil {
			d.logger.Warnw("Failed to persist outbound status",
				"contextId", contextID,
				"call_status", update.CallStatus,
				"failure_class", update.FailureClass,
				"error", err)
			return
		}
		if update.CallStatus != callcontext.CallStatusRinging &&
			update.CallStatus != callcontext.CallStatusFailed &&
			update.CallStatus != callcontext.CallStatusCancelled {
			return
		}
		if d.conversationService == nil {
			return
		}

		if currentCallContext == nil {
			var err error
			currentCallContext, err = d.store.Get(ctx, contextID)
			if err != nil {
				d.logger.Warnw("Failed to resolve outbound call context for provider status observability",
					"contextId", contextID,
					"call_status", update.CallStatus,
					"failure_class", update.FailureClass,
					"error", err)
				return
			}
		}
		if update.CallStatus == callcontext.CallStatusRinging && currentCallContext.Status != callcontext.StatusPending {
			return
		}
		auth := currentCallContext.ToAuth()
		scope := observability.ConversationScope{
			AssistantScope: observability.AssistantScope{AssistantID: currentCallContext.AssistantID},
			ConversationID: currentCallContext.ConversationID,
		}
		observer := observability.New(
			observability.WithLogger(d.logger),
			observability.WithAuth(auth),
			observability.WithContext(ctx),
			observability.WithCustomGracePeriod(2*time.Second),
			observability.WithCollectors(
				observability_collector_conversationmetric.New(observability_collector_conversationmetric.Config{
					Logger:              d.logger,
					ConversationService: d.conversationService,
				}),
				observability_collector_conversationmetadata.New(observability_collector_conversationmetadata.Config{
					Logger:              d.logger,
					ConversationService: d.conversationService,
				}),
				collectors.NewWithEnv(ctx, d.logger, d.cfg),
				collectors.NewWithWebhookConfiguration(ctx, d.logger, auth, currentCallContext.AssistantID, d.configurationService, d.httpLogService),
			),
		)
		defer observer.Close(ctx)
		if update.CallStatus == callcontext.CallStatusRinging {
			observer.Record(ctx,
				scope,
				observability.RecordLog{
					Level:   observability.LevelInfo,
					Message: "Outbound provider call is ringing",
					Attributes: observability.Attributes{
						"component":            observability.ComponentCall.String(),
						"context_id":           currentCallContext.ContextID,
						"provider":             currentCallContext.Provider,
						"call_id":              currentCallContext.ChannelUUID,
						"call_status":          update.CallStatus,
						"failure_class":        update.FailureClass,
						"failure_reason":       update.FailureReason,
						"disconnect_reason":    update.DisconnectReason,
						"provider_status_code": strconv.Itoa(update.ProviderStatusCode),
						"retryable":            strconv.FormatBool(update.Retryable),
						"error":                update.ErrorMessage,
					},
				},
				observability.RecordEvent{
					Component: observability.ComponentCall,
					Event:     observability.CallRinging,
					Attributes: observability.Attributes{
						"component":            observability.ComponentCall.String(),
						"context_id":           currentCallContext.ContextID,
						"provider":             currentCallContext.Provider,
						"call_id":              currentCallContext.ChannelUUID,
						"call_status":          update.CallStatus,
						"provider_status_code": strconv.Itoa(update.ProviderStatusCode),
					},
				},
				observability.RecordWebhook{
					Event:     observability.CallRinging,
					ContextID: currentCallContext.ContextID,
					Payload: map[string]interface{}{
						"provider":             currentCallContext.Provider,
						"to":                   currentCallContext.CallerNumber,
						"from":                 currentCallContext.FromNumber,
						"call_id":              currentCallContext.ChannelUUID,
						"context_id":           currentCallContext.ContextID,
						"direction":            currentCallContext.Direction,
						"call_status":          update.CallStatus,
						"provider_status_code": strconv.Itoa(update.ProviderStatusCode),
					},
				},
				observability.RecordMetric{
					Metrics: observability.CallStatusMetric("RINGING", cmp.Or(update.FailureReason, update.DisconnectReason, update.ErrorMessage, update.CallStatus)),
					Attributes: observability.Attributes{
						"component":     observability.ComponentCall.String(),
						"context_id":    currentCallContext.ContextID,
						"to":            currentCallContext.CallerNumber,
						"from":          currentCallContext.FromNumber,
						"provider":      currentCallContext.Provider,
						"failure_class": update.FailureClass,
					},
				},
			)
			return
		}

		if update.CallStatus == callcontext.CallStatusCancelled {
			observer.Record(ctx,
				scope,
				observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Outbound provider call ended before media connection",
					Attributes: observability.Attributes{
						"component":            observability.ComponentCall.String(),
						"context_id":           currentCallContext.ContextID,
						"provider":             currentCallContext.Provider,
						"call_id":              currentCallContext.ChannelUUID,
						"call_status":          update.CallStatus,
						"failure_class":        update.FailureClass,
						"failure_reason":       update.FailureReason,
						"disconnect_reason":    update.DisconnectReason,
						"provider_status_code": strconv.Itoa(update.ProviderStatusCode),
						"retryable":            strconv.FormatBool(update.Retryable),
						"error":                update.ErrorMessage,
					},
				},
				observability.RecordEvent{
					Component: observability.ComponentCall,
					Event:     observability.CallCancelled,
					Attributes: observability.Attributes{
						"component":            observability.ComponentCall.String(),
						"context_id":           currentCallContext.ContextID,
						"provider":             currentCallContext.Provider,
						"call_id":              currentCallContext.ChannelUUID,
						"call_status":          update.CallStatus,
						"failure_class":        update.FailureClass,
						"failure_reason":       update.FailureReason,
						"disconnect_reason":    update.DisconnectReason,
						"provider_status_code": strconv.Itoa(update.ProviderStatusCode),
						"retryable":            strconv.FormatBool(update.Retryable),
					},
				},
				observability.RecordWebhook{
					Event:     observability.CallCancelled,
					ContextID: currentCallContext.ContextID,
					Payload: map[string]interface{}{
						"provider":   currentCallContext.Provider,
						"to":         currentCallContext.CallerNumber,
						"from":       currentCallContext.FromNumber,
						"call_id":    currentCallContext.ChannelUUID,
						"context_id": currentCallContext.ContextID,
						"direction":  currentCallContext.Direction,

						"call_status":          update.CallStatus,
						"failure_class":        update.FailureClass,
						"failure_reason":       update.FailureReason,
						"disconnect_reason":    update.DisconnectReason,
						"provider_status_code": update.ProviderStatusCode,
						"retryable":            update.Retryable,
						"error":                update.ErrorMessage,
					},
				},
				observability.RecordMetric{
					Metrics: observability.CallStatusMetric("CANCELLED", cmp.Or(update.FailureReason, update.DisconnectReason, update.ErrorMessage, update.CallStatus)),
					Attributes: observability.Attributes{
						"component":     observability.ComponentCall.String(),
						"context_id":    currentCallContext.ContextID,
						"to":            currentCallContext.CallerNumber,
						"from":          currentCallContext.FromNumber,
						"provider":      currentCallContext.Provider,
						"failure_class": update.FailureClass,
					},
				},
			)
			metadata := observability.DisconnectMetadata(update.DisconnectReason, update.FailureReason, update.ErrorMessage)
			metadata = append(metadata,
				&protos.Metadata{Key: observability.MetadataCallStatus, Value: update.CallStatus},
				&protos.Metadata{Key: observability.MetadataFailureClass, Value: update.FailureClass},
				&protos.Metadata{Key: observability.MetadataFailureReason, Value: update.FailureReason},
				&protos.Metadata{Key: observability.MetadataRetryable, Value: strconv.FormatBool(update.Retryable)},
			)
			if update.ProviderStatusCode != 0 {
				metadata = append(metadata, &protos.Metadata{Key: observability.MetadataProviderStatusCode, Value: strconv.Itoa(update.ProviderStatusCode)})
			}
			if update.ErrorMessage != "" {
				metadata = append(metadata, &protos.Metadata{Key: observability.MetadataCallError, Value: update.ErrorMessage})
			}
			observer.Record(ctx,
				scope,
				observability.RecordMetadata{Metadata: metadata},
			)
			return
		}

		if update.CallStatus == callcontext.CallStatusFailed {
			observer.Record(ctx,
				scope,
				observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Outbound provider call ended before media connection",
					Attributes: observability.Attributes{
						"component":            observability.ComponentCall.String(),
						"context_id":           currentCallContext.ContextID,
						"provider":             currentCallContext.Provider,
						"call_id":              currentCallContext.ChannelUUID,
						"call_status":          update.CallStatus,
						"failure_class":        update.FailureClass,
						"failure_reason":       update.FailureReason,
						"disconnect_reason":    update.DisconnectReason,
						"provider_status_code": strconv.Itoa(update.ProviderStatusCode),
						"retryable":            strconv.FormatBool(update.Retryable),
						"error":                update.ErrorMessage,
					},
				},
				observability.RecordEvent{
					Component: observability.ComponentCall,
					Event:     observability.CallFailed,
					Attributes: observability.Attributes{
						"component":            observability.ComponentCall.String(),
						"context_id":           currentCallContext.ContextID,
						"provider":             currentCallContext.Provider,
						"call_id":              currentCallContext.ChannelUUID,
						"call_status":          update.CallStatus,
						"failure_class":        update.FailureClass,
						"failure_reason":       update.FailureReason,
						"disconnect_reason":    update.DisconnectReason,
						"provider_status_code": strconv.Itoa(update.ProviderStatusCode),
						"retryable":            strconv.FormatBool(update.Retryable),
					},
				},
				observability.RecordWebhook{
					Event:     observability.CallFailed,
					ContextID: currentCallContext.ContextID,
					Payload: map[string]interface{}{
						"provider":   currentCallContext.Provider,
						"to":         currentCallContext.CallerNumber,
						"from":       currentCallContext.FromNumber,
						"call_id":    currentCallContext.ChannelUUID,
						"context_id": currentCallContext.ContextID,
						"direction":  currentCallContext.Direction,

						"call_status":          update.CallStatus,
						"failure_class":        update.FailureClass,
						"failure_reason":       update.FailureReason,
						"disconnect_reason":    update.DisconnectReason,
						"provider_status_code": update.ProviderStatusCode,
						"retryable":            update.Retryable,
						"error":                update.ErrorMessage,
					},
				},
				observability.RecordMetric{
					Metrics: observability.CallStatusMetric("FAILED", cmp.Or(update.FailureReason, update.DisconnectReason, update.ErrorMessage, update.CallStatus)),
					Attributes: observability.Attributes{
						"to":            currentCallContext.CallerNumber,
						"from":          currentCallContext.FromNumber,
						"provider":      currentCallContext.Provider,
						"context_id":    currentCallContext.ContextID,
						"call_id":       currentCallContext.ChannelUUID,
						"component":     observability.ComponentCall.String(),
						"failure_class": update.FailureClass,
					},
				},
			)
			metadata := observability.DisconnectMetadata(update.DisconnectReason, update.FailureReason, update.ErrorMessage)
			metadata = append(metadata,
				&protos.Metadata{Key: observability.MetadataCallStatus, Value: update.CallStatus},
				&protos.Metadata{Key: observability.MetadataFailureClass, Value: update.FailureClass},
				&protos.Metadata{Key: observability.MetadataFailureReason, Value: update.FailureReason},
				&protos.Metadata{Key: observability.MetadataRetryable, Value: strconv.FormatBool(update.Retryable)},
			)
			if update.ProviderStatusCode != 0 {
				metadata = append(metadata, &protos.Metadata{Key: observability.MetadataProviderStatusCode, Value: strconv.Itoa(update.ProviderStatusCode)})
			}
			if update.ErrorMessage != "" {
				metadata = append(metadata, &protos.Metadata{Key: observability.MetadataCallError, Value: update.ErrorMessage})
			}
			observer.Record(ctx,
				scope,
				observability.RecordMetadata{Metadata: metadata},
			)
			return
		}
	}
}
