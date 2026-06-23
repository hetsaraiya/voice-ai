// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package channel_telephony

import (
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
		recordProgress := update.CallStatus == callcontext.CallStatusRinging
		recordTerminal := update.CallStatus == callcontext.CallStatusFailed || update.CallStatus == callcontext.CallStatusCancelled
		if !recordProgress && !recordTerminal {
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
		if recordProgress && currentCallContext.Status != callcontext.StatusPending {
			return
		}

		metricStatus := "FAILED"
		eventName := observability.CallFailed
		message := "Outbound provider call ended before media connection"
		logLevel := observability.LevelError
		if recordProgress {
			metricStatus = "RINGING"
			eventName = observability.CallRinging
			message = "Outbound provider call is ringing"
			logLevel = observability.LevelInfo
		} else if update.CallStatus == callcontext.CallStatusCancelled {
			metricStatus = "CANCELLED"
			eventName = observability.CallCancelled
		}
		metricReason := update.FailureReason
		if metricReason == "" {
			metricReason = update.DisconnectReason
		}
		if metricReason == "" {
			metricReason = update.ErrorMessage
		}
		if metricReason == "" {
			metricReason = update.CallStatus
		}

		auth := currentCallContext.ToAuth()
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
				collectors.NewWithEnv(ctx, d.logger, d.cfg)),
		)
		if err := observer.AddCollectors(collectors.NewWithAssistantWebhook(ctx, d.logger, auth, currentCallContext.AssistantID, d.webhookService, d.httpLogService)); err != nil {
			d.logger.Warnw("observability collector registration failed",
				"component", "call",
				"operation", "add_assistant_collectors",
				"assistant_id", currentCallContext.AssistantID,
				"context_id", currentCallContext.ContextID,
				"error", err,
			)
		}

		if err := observer.Record(ctx,
			observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: currentCallContext.AssistantID},
				ConversationID: currentCallContext.ConversationID,
			},
			observability.RecordLog{
				Level:   logLevel,
				Message: message,
				Attributes: observability.Attributes{
					"component":            observability.ComponentCall.String(),
					"context_id":           currentCallContext.ContextID,
					"provider":             currentCallContext.Provider,
					"channel_uuid":         currentCallContext.ChannelUUID,
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
				Event:     eventName,
				Attributes: observability.Attributes{
					"component":            observability.ComponentCall.String(),
					"context_id":           currentCallContext.ContextID,
					"provider":             currentCallContext.Provider,
					"channel_uuid":         currentCallContext.ChannelUUID,
					"call_status":          update.CallStatus,
					"failure_class":        update.FailureClass,
					"failure_reason":       update.FailureReason,
					"disconnect_reason":    update.DisconnectReason,
					"provider_status_code": strconv.Itoa(update.ProviderStatusCode),
					"retryable":            strconv.FormatBool(update.Retryable),
				},
			},
			observability.RecordWebhook{
				Event:     eventName,
				ContextID: currentCallContext.ContextID,
				Payload: map[string]interface{}{
					"event": eventName.String(),
					"assistant": map[string]interface{}{
						"id": currentCallContext.AssistantID,
					},
					"conversation": map[string]interface{}{
						"id": currentCallContext.ConversationID,
					},
					"data": map[string]interface{}{
						"context_id":           currentCallContext.ContextID,
						"provider":             currentCallContext.Provider,
						"direction":            currentCallContext.Direction,
						"caller":               currentCallContext.CallerNumber,
						"from":                 currentCallContext.FromNumber,
						"channel_uuid":         currentCallContext.ChannelUUID,
						"call_status":          update.CallStatus,
						"failure_class":        update.FailureClass,
						"failure_reason":       update.FailureReason,
						"disconnect_reason":    update.DisconnectReason,
						"provider_status_code": update.ProviderStatusCode,
						"retryable":            update.Retryable,
						"error":                update.ErrorMessage,
					},
				},
			},
			observability.RecordMetric{
				Metrics: observability.CallStatusMetric(metricStatus, metricReason),
				Attributes: observability.Attributes{
					"component":     observability.ComponentCall.String(),
					"context_id":    currentCallContext.ContextID,
					"provider":      currentCallContext.Provider,
					"failure_class": update.FailureClass,
				},
			},
		); err != nil {
			d.logger.Errorw("Failed to record outbound provider status observability",
				"contextId", contextID,
				"call_status", update.CallStatus,
				"failure_class", update.FailureClass,
				"error", err)
		}
		if recordTerminal {
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
			if err := observer.Record(ctx,
				observability.ConversationScope{
					AssistantScope: observability.AssistantScope{AssistantID: currentCallContext.AssistantID},
					ConversationID: currentCallContext.ConversationID,
				},
				observability.RecordMetadata{Metadata: metadata},
			); err != nil {
				d.logger.Errorw("Failed to record outbound provider status metadata",
					"contextId", contextID,
					"call_status", update.CallStatus,
					"failure_class", update.FailureClass,
					"error", err)
			}
		}

		if err := observer.Close(ctx); err != nil {
			d.logger.Warnw("Failed to close outbound provider status observability",
				"contextId", contextID,
				"call_status", update.CallStatus,
				"failure_class", update.FailureClass,
				"error", err)
		}
	}
}
