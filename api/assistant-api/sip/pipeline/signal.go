// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package sip_pipeline

import (
	"context"
	"fmt"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	"github.com/rapidaai/protos"
)

func (d *Dispatcher) handleByeReceived(ctx context.Context, v sip_infra.ByeReceivedPipeline) {
	d.logger.Infow("Pipeline: ByeReceived",
		"call_id", v.ID,
		"reason", v.Reason,
		"direction", v.Session.GetInfo().Direction)
}

func (d *Dispatcher) handleCancelReceived(ctx context.Context, v sip_infra.CancelReceivedPipeline) {
	d.logger.Infow("Pipeline: CancelReceived",
		"call_id", v.ID,
		"direction", v.Session.GetInfo().Direction)
}

func (d *Dispatcher) handleCallEnded(ctx context.Context, v sip_infra.CallEndedPipeline) {
	d.logger.Infow("Pipeline: CallEnded",
		"call_id", v.ID,
		"duration", v.Duration,
		"reason", v.Reason)
}

// handleCallFailed creates a short-lived observer to persist the FAILED status
// metric so the conversation is not left indeterminate. This handles early
// failures (outbound call rejected, setup error) that occur before the main
// SessionEstablished pipeline creates its own observer.
func (d *Dispatcher) handleCallFailed(ctx context.Context, v sip_infra.CallFailedPipeline) {
	reason := "call_failed"
	if v.Error != nil {
		reason = v.Error.Error()
	}

	d.logger.Warnw("Pipeline: CallFailed",
		"call_id", v.ID,
		"error", fmt.Sprintf("%v", v.Error),
		"sip_code", v.SIPCode)

	// Emit failure metric via observer if session has enough context
	if v.Session == nil {
		return
	}
	auth := v.Session.GetAuth()
	convID := v.Session.GetConversationID()
	if auth == nil || convID == 0 {
		return
	}

	var assistantID uint64
	if assistant := v.Session.GetAssistant(); assistant != nil {
		assistantID = assistant.Id
	}

	setup := &CallSetupResult{
		AssistantID:    assistantID,
		ConversationID: convID,
	}
	if auth.GetCurrentProjectId() != nil {
		setup.ProjectID = *auth.GetCurrentProjectId()
	}
	if auth.GetCurrentOrganizationId() != nil {
		setup.OrganizationID = *auth.GetCurrentOrganizationId()
	}

	observer := d.createObserver(ctx, setup, auth)
	scope := observability.ConversationScope{
		AssistantScope: observability.AssistantScope{AssistantID: assistantID},
		ConversationID: convID,
	}
	attributes := observability.Attributes{
		"provider":  "sip",
		"reason":    reason,
		"direction": string(v.Session.GetInfo().Direction),
		"call_id":   v.ID,
	}
	contextID := v.Session.GetContextID()
	if contextID == "" {
		contextID = v.ID
	}
	if v.SIPCode > 0 {
		attributes["sip_code"] = fmt.Sprintf("%d", v.SIPCode)
	}
	if v.Error != nil {
		attributes["error"] = v.Error.Error()
	}
	_ = observer.Record(
		ctx,
		scope,
		observability.RecordLog{
			Level:      observability.LevelError,
			Message:    "SIP call failed",
			Attributes: attributes,
		},
		observability.RecordEvent{
			Component:  observability.ComponentCall,
			Event:      observability.CallFailed,
			Attributes: attributes,
		},
		observability.RecordWebhook{
			Event:     observability.CallFailed,
			ContextID: contextID,
			Payload: map[string]interface{}{
				"context_id": contextID,
				"provider":   "sip",
				"reason":     reason,
				"direction":  string(v.Session.GetInfo().Direction),
				"call_id":    v.ID,
				"sip_code":   v.SIPCode,
				"error":      fmt.Sprintf("%v", v.Error),
			},
		},
		observability.RecordMetric{
			Metrics: []*protos.Metric{{
				Name:        observability.MetricCallStatus,
				Value:       observability.MetricCallStatusFailed,
				Description: reason,
			}},
		},
	)
	_ = observer.Close(ctx)
}
