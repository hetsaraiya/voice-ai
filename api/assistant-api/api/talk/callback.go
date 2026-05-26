// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package assistant_talk_api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	observe "github.com/rapidaai/api/assistant-api/internal/observe"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
)

func (cApi *ConversationApi) UnviersalCallback(c *gin.Context) {
	provider := c.Param("telephony")
	if !validator.NotBlank(provider) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing telephony provider"})
		return
	}
	assistantID, err := strconv.ParseUint(c.Param("assistantId"), 10, 64)
	if err != nil || !validator.AllNonZero(assistantID) {
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

	observer := cApi.conversationObserver.ConversationObserver(cc.ToAuth(), cc.AssistantID, cc.ConversationID)
	observer.EmitEvent(c, observe.ComponentTelephony, map[string]string{
		observe.DataType:      observe.EventCallback,
		observe.DataProvider:  cc.Provider,
		observe.DataStatus:    statusInfo.Event,
		observe.DataContextID: cc.ContextID,
		observe.DataDirection: cc.Direction,
	})
	if statusInfo.Error != nil {
		observer.EmitMetric(c, observe.CallStatusMetric("FAILED", statusInfo.Error.Reason))
		if validator.NotBlank(statusInfo.Error.Reason) {
			observer.EmitMetadata(c, []*types.Metadata{types.NewMetadata("disconnect_reason", statusInfo.Error.Reason)})
		}
	}
	metrics := make([]*protos.Metric, 0, 2)
	if statusInfo.Duration != nil {
		metrics = append(metrics, &protos.Metric{Name: observe.MetricTelephonyDuration, Value: strconv.FormatInt(statusInfo.Duration.Nanoseconds(), 10)})
	}
	if validator.NotBlank(statusInfo.Price) {
		metrics = append(metrics, &protos.Metric{Name: observe.MetricTelephonyPrice, Value: statusInfo.Price})
	}
	observer.EmitMetric(c, metrics)
	if strings.EqualFold(statusInfo.Event, "completed") && statusInfo.Error == nil {
		if err := cApi.callContextStore.UpdateField(c, cc.ContextID, "status", callcontext.StatusCompleted); err != nil {
			cApi.logger.Warnf("failed to mark call context %s completed: %v", cc.ContextID, err)
		}
	}
	observer.Shutdown(c)

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
	if statusInfo != nil {
		observer := cApi.conversationObserver.ConversationObserver(cc.ToAuth(), cc.AssistantID, cc.ConversationID)
		observer.EmitEvent(c, observe.ComponentTelephony, map[string]string{
			observe.DataType:      observe.EventCallback,
			observe.DataProvider:  cc.Provider,
			observe.DataStatus:    statusInfo.Event,
			observe.DataContextID: contextID,
			observe.DataDirection: cc.Direction,
		})
		if statusInfo.Error != nil {
			observer.EmitMetric(c, observe.CallStatusMetric("FAILED", statusInfo.Error.Reason))
			if validator.NotBlank(statusInfo.Error.Reason) {
				observer.EmitMetadata(c, []*types.Metadata{types.NewMetadata("disconnect_reason", statusInfo.Error.Reason)})
			}
		}
		metrics := make([]*protos.Metric, 0, 2)
		if statusInfo.Duration != nil {
			metrics = append(metrics, &protos.Metric{Name: observe.MetricTelephonyDuration, Value: strconv.FormatInt(statusInfo.Duration.Nanoseconds(), 10), Description: "Call duration in nanoseconds"})
		}
		if validator.NotBlank(statusInfo.Price) {
			metrics = append(metrics, &protos.Metric{Name: observe.MetricTelephonyPrice, Value: statusInfo.Price, Description: "Call price"})
		}
		observer.EmitMetric(c, metrics)
		if strings.EqualFold(statusInfo.Event, "completed") && statusInfo.Error == nil {
			if err := cApi.callContextStore.UpdateField(c, cc.ContextID, "status", callcontext.StatusCompleted); err != nil {
				cApi.logger.Warnf("failed to mark call context %s completed: %v", cc.ContextID, err)
			}
		}
		observer.Shutdown(c)
	}

	c.Status(http.StatusCreated)
}
