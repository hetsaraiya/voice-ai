// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package assistant_talk_api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/api/assistant-api/internal/observability/collectors"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
)

func (cApi *ConversationApi) UnviersalCallback(c *gin.Context) {
	provider := c.Param("telephony")
	if !validator.NotBlank(provider) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing telephony provider"})
		return
	}
	assistantID, err := utils.StringToUint64(c.Param("assistantId"))
	if err != nil || !validator.NonZero(assistantID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assistantId"})
		return
	}

	statusInfo, err := cApi.inboundDispatcher.HandleCatchAllStatusCallback(c, provider)
	if err != nil {
		cApi.logger.Errorf("catch-all status callback failed for provider %s: %v", provider, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event to process"})
		return
	}
	if statusInfo == nil {
		c.Status(http.StatusCreated)
		return
	}

	cc, err := cApi.callContextStore.GetByChannelUUID(c, provider, assistantID, statusInfo.ChannelUUID)
	if err != nil {
		cApi.logger.Errorf("failed to resolve call context for provider %s assistant %d uuid %s: %v", provider, assistantID, statusInfo.ChannelUUID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event to process"})
		return
	}

	auth := cc.ToAuth()
	observer := cApi.Observability(c, auth, observability.WithGracePeriod())
	if err := observer.AddCollectors(collectors.NewWithWebhookConfiguration(c, cApi.logger, auth, cc.AssistantID, cApi.configurationService, cApi.httpLogService)); err != nil {
		cApi.logger.Warnw("observability collector registration failed",
			"component", "callback",
			"operation", "add_assistant_collectors",
			"assistant_id", cc.AssistantID,
			"context_id", cc.ContextID,
			"error", err,
		)
	}

	observer.Record(c,
		observability.ConversationScope{
			AssistantScope: observability.AssistantScope{AssistantID: cc.AssistantID},
			ConversationID: cc.ConversationID,
		},
		observability.RecordLog{
			Level:   observability.LevelInfo,
			Message: "telephony provider callback received",
			Attributes: observability.Attributes{
				"provider":     cc.Provider,
				"status_event": statusInfo.Event,
				"context_id":   cc.ContextID,
				"direction":    cc.Direction,
				"channel_uuid": statusInfo.ChannelUUID,
				"raw_payload":  statusInfo.RawPayload,
			},
		},
		observability.RecordEvent{
			Event: observability.CallStatus,
			Attributes: observability.Attributes{
				"provider":     cc.Provider,
				"status_event": statusInfo.Event,
				"context_id":   cc.ContextID,
				"direction":    cc.Direction,
				"channel_uuid": statusInfo.ChannelUUID,
			},
		})
	if statusInfo.Error != nil {
		observer.Record(c,
			observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: cc.AssistantID},
				ConversationID: cc.ConversationID,
			},
			observability.RecordWebhook{
				Event:     observability.CallFailed,
				ContextID: cc.ContextID,
				Payload: map[string]interface{}{
					"provider":     cc.Provider,
					"direction":    cc.Direction,
					"to":           cc.CallerNumber,
					"from":         cc.FromNumber,
					"call_id":      statusInfo.ChannelUUID,
					"context_id":   cc.ContextID,
					"source":       "provider_callback",
					"status_event": statusInfo.Event,
					"raw_payload":  statusInfo.RawPayload,
					"payload":      statusInfo.Payload,
					"error":        statusInfo.Error.Error,
					"reason":       statusInfo.Error.Reason,
				},
			})
		if err := cApi.callContextStore.UpdateCallStatus(c, cc.ContextID, callcontext.CallStatusUpdate{
			CallStatus:       callcontext.CallStatusFailed,
			CallError:        statusInfo.Error.Error,
			FailureClass:     "provider_response",
			FailureReason:    statusInfo.Error.Reason,
			DisconnectReason: statusInfo.Error.Reason,
		}); err != nil {
			cApi.logger.Warnf("failed to update call context %s from failed callback: %v", cc.ContextID, err)
		}
		observer.Record(c,
			observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: cc.AssistantID},
				ConversationID: cc.ConversationID,
			},
			observability.RecordMetric{
				Metrics: observability.CallStatusMetric("FAILED", statusInfo.Error.Reason),
			})
		if validator.NotBlank(statusInfo.Error.Reason) {
			observer.Record(c,
				observability.ConversationScope{
					AssistantScope: observability.AssistantScope{AssistantID: cc.AssistantID},
					ConversationID: cc.ConversationID,
				},
				observability.RecordMetadata{
					Metadata: observability.DisconnectMetadata(statusInfo.Error.Reason, "", ""),
				})
		}
	} else if statusInfo.Completed {
		if err := cApi.callContextStore.UpdateCallStatus(c, cc.ContextID, callcontext.CallStatusUpdate{
			CallStatus:       callcontext.CallStatusCompleted,
			DisconnectReason: statusInfo.Event,
		}); err != nil {
			cApi.logger.Warnf("failed to update call context %s from completed callback: %v", cc.ContextID, err)
		}
	} else if validator.NotBlank(statusInfo.Event) {
		if err := cApi.callContextStore.UpdateCallStatus(c, cc.ContextID, callcontext.CallStatusUpdate{
			CallStatus: statusInfo.Event,
		}); err != nil {
			cApi.logger.Warnf("failed to update call context %s from callback event %s: %v", cc.ContextID, statusInfo.Event, err)
		}
	}
	if validator.NonNil(statusInfo.Duration) {
		observer.Record(c,
			observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: cc.AssistantID},
				ConversationID: cc.ConversationID,
			},
			observability.RecordMetric{Metrics: []*protos.Metric{
				{Name: observability.MetricTelephonyDuration, Value: strconv.FormatInt(statusInfo.Duration.Nanoseconds(), 10)},
			}},
		)
	}
	if validator.NotBlank(statusInfo.Price) {
		observer.Record(c,
			observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: cc.AssistantID},
				ConversationID: cc.ConversationID,
			},
			observability.RecordMetric{Metrics: []*protos.Metric{
				{Name: observability.MetricTelephonyPrice, Value: statusInfo.Price},
			}},
		)
	}

	observer.Close(context.Background())
	c.Status(http.StatusCreated)
}

// CallbackByContext handles status callback webhooks using a contextId stored in Postgres.
func (cApi *ConversationApi) CallbackByContext(c *gin.Context) {
	contextID := c.Param("contextId")
	if !validator.NotBlank(contextID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing contextId"})
		return
	}

	cc, err := cApi.callContextStore.Get(c, contextID)
	if err != nil {
		cApi.logger.Errorf("failed to resolve call context %s for event callback: %v", contextID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event to process"})
		return
	}

	statusInfo, err := cApi.inboundDispatcher.HandleStatusCallback(c, cc.Provider, cc.ToAuth(), cc.AssistantID, cc.ConversationID)
	if err != nil {
		cApi.logger.Errorf("status callback failed for context %s: %v", contextID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event to process"})
		return
	}

	if !validator.NonNil(statusInfo) {
		c.Status(http.StatusBadRequest)
		return
	}

	auth := cc.ToAuth()
	observer := cApi.Observability(c, auth, observability.WithGracePeriod())
	if err := observer.AddCollectors(collectors.NewWithWebhookConfiguration(c, cApi.logger, auth, cc.AssistantID, cApi.configurationService, cApi.httpLogService)); err != nil {
		cApi.logger.Warnw("observability collector registration failed",
			"component", "callback",
			"operation", "add_assistant_collectors",
			"assistant_id", cc.AssistantID,
			"context_id", cc.ContextID,
			"error", err,
		)
	}

	observer.Record(c,
		observability.ConversationScope{
			AssistantScope: observability.AssistantScope{AssistantID: cc.AssistantID},
			ConversationID: cc.ConversationID,
		},
		observability.RecordLog{
			Level:   observability.LevelInfo,
			Message: "telephony provider callback received",
			Attributes: observability.Attributes{
				"provider":     cc.Provider,
				"status_event": statusInfo.Event,
				"context_id":   contextID,
				"direction":    cc.Direction,
				"channel_uuid": statusInfo.ChannelUUID,
				"raw_payload":  statusInfo.RawPayload,
			},
		},
		observability.RecordEvent{
			Event: observability.CallStatus,
			Attributes: observability.Attributes{
				"provider":     cc.Provider,
				"status_event": statusInfo.Event,
				"context_id":   contextID,
				"direction":    cc.Direction,
				"channel_uuid": statusInfo.ChannelUUID,
			},
		})
	if statusInfo.Error != nil {
		observer.Record(c,
			observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: cc.AssistantID},
				ConversationID: cc.ConversationID,
			},
			observability.RecordWebhook{
				Event:     observability.CallFailed,
				ContextID: cc.ContextID,
				Payload: map[string]interface{}{
					"provider":     cc.Provider,
					"to":           cc.CallerNumber,
					"from":         cc.FromNumber,
					"call_id":      statusInfo.ChannelUUID,
					"context_id":   cc.ContextID,
					"direction":    cc.Direction,
					"source":       "provider_callback",
					"status_event": statusInfo.Event,
					"raw_payload":  statusInfo.RawPayload,
					"payload":      statusInfo.Payload,
					"error":        statusInfo.Error.Error,
					"reason":       statusInfo.Error.Reason,
				},
			})
		if err := cApi.callContextStore.UpdateCallStatus(c, cc.ContextID, callcontext.CallStatusUpdate{
			CallStatus:       callcontext.CallStatusFailed,
			CallError:        statusInfo.Error.Error,
			FailureClass:     "provider_response",
			FailureReason:    statusInfo.Error.Reason,
			DisconnectReason: statusInfo.Error.Reason,
		}); err != nil {
			cApi.logger.Warnf("failed to update call context %s from failed callback: %v", cc.ContextID, err)
		}
		observer.Record(c,
			observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: cc.AssistantID},
				ConversationID: cc.ConversationID,
			},
			observability.RecordMetric{
				Metrics: observability.CallStatusMetric("FAILED", statusInfo.Error.Reason),
			})
		if validator.NotBlank(statusInfo.Error.Reason) {
			observer.Record(c, observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: cc.AssistantID},
				ConversationID: cc.ConversationID,
			},
				observability.RecordMetadata{
					Metadata: observability.DisconnectMetadata(statusInfo.Error.Reason, "", ""),
				})
		}
	} else if statusInfo.Completed {
		if err := cApi.callContextStore.UpdateCallStatus(c, cc.ContextID, callcontext.CallStatusUpdate{
			CallStatus:       callcontext.CallStatusCompleted,
			DisconnectReason: statusInfo.Event,
		}); err != nil {
			cApi.logger.Warnf("failed to update call context %s from completed callback: %v", cc.ContextID, err)
		}
	} else if validator.NotBlank(statusInfo.Event) {
		if err := cApi.callContextStore.UpdateCallStatus(c, cc.ContextID, callcontext.CallStatusUpdate{
			CallStatus: statusInfo.Event,
		}); err != nil {
			cApi.logger.Warnf("failed to update call context %s from callback event %s: %v", cc.ContextID, statusInfo.Event, err)
		}
	}

	if validator.NonNil(statusInfo.Duration) {
		observer.Record(c, observability.ConversationScope{
			AssistantScope: observability.AssistantScope{AssistantID: cc.AssistantID},
			ConversationID: cc.ConversationID,
		},
			observability.RecordMetric{Metrics: []*protos.Metric{&protos.Metric{Name: observability.MetricTelephonyDuration, Value: strconv.FormatInt(statusInfo.Duration.Nanoseconds(), 10), Description: "Call duration in nanoseconds"}}})
	}
	if validator.NotBlank(statusInfo.Price) {
		observer.Record(c,
			observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: cc.AssistantID},
				ConversationID: cc.ConversationID,
			},
			observability.RecordMetric{Metrics: []*protos.Metric{&protos.Metric{Name: observability.MetricTelephonyPrice, Value: statusInfo.Price, Description: "Call price"}}})
	}

	if err := observer.Close(context.Background()); err != nil {
		cApi.logger.Warnf("failed to close callback observability recorder: %v", err)
	}
	c.Status(http.StatusCreated)
}
