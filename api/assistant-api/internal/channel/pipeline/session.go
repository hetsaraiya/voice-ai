// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package channel_pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/protos"
)

// runSession handles telephony media setup and keeps the Talk lifecycle synchronous.
func (d *Dispatcher) runSession(ctx context.Context, v SessionConnectedPipeline) *PipelineResult {
	startTime := time.Now()
	contextID := v.ContextID
	if contextID == "" {
		contextID = v.ID
	}
	auth := v.CallContext.ToAuth()
	scope := observability.ConversationScope{
		AssistantScope: observability.AssistantScope{AssistantID: v.CallContext.AssistantID},
		ConversationID: v.CallContext.ConversationID,
	}
	_ = v.Observer.Record(ctx, scope, observability.RecordMetadata{
		Metadata: observability.ClientMetadata(
			v.CallContext.CallerNumber, v.CallContext.FromNumber, v.CallContext.Direction, v.CallContext.Provider,
			v.CallContext.ChannelUUID, contextID, "", "", // codec/sampleRate set by streamer
		),
	})
	_ = v.Observer.Record(ctx, scope, observability.RecordEvent{
		Component: observability.ComponentCall,
		Event:     observability.CallStarted,
		Attributes: observability.Attributes{
			"context_id": contextID,
			"provider":   v.CallContext.Provider,
			"direction":  v.CallContext.Direction,
		},
	})
	_ = v.Observer.Record(ctx, scope, observability.RecordLog{
		Level:   observability.LevelDebug,
		Message: "Pipeline session connected",
		Attributes: observability.Attributes{
			"context_id": contextID,
			"provider":   v.CallContext.Provider,
			"direction":  v.CallContext.Direction,
		},
	})
	reason := "talk_completed"
	status := "COMPLETE"
	func() {
		defer func() {
			if r := recover(); r != nil {
				reason = fmt.Sprintf("panic: %v", r)
				status = "FAILED"
				_ = v.Observer.Record(ctx, scope, observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Pipeline talk panicked",
					Attributes: observability.Attributes{
						"context_id": contextID,
						"provider":   v.CallContext.Provider,
						"direction":  v.CallContext.Direction,
						"panic":      fmt.Sprintf("%v", r),
					},
				})
			}
		}()

		err := v.Talker.Talk(ctx, auth)
		if err != nil {
			reason = fmt.Sprintf("talk_error: %v", err)
			status = "FAILED"
			_ = v.Observer.Record(ctx, scope, observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Pipeline talk failed",
				Attributes: observability.Attributes{
					"context_id": contextID,
					"provider":   v.CallContext.Provider,
					"direction":  v.CallContext.Direction,
					"error":      err.Error(),
				},
			})
		}
	}()

	durationMs := time.Since(startTime).Milliseconds()
	_ = v.Observer.Record(ctx, scope, observability.RecordEvent{
		Component: observability.ComponentCall,
		Event:     observability.CallEnded,
		Attributes: observability.Attributes{
			"context_id":  contextID,
			"provider":    v.CallContext.Provider,
			"direction":   v.CallContext.Direction,
			"reason":      reason,
			"status":      status,
			"duration_ms": fmt.Sprintf("%d", durationMs),
		},
	}, observability.RecordMetric{
		Metrics: []*protos.Metric{
			{
				Name:        observability.MetricCallStatus,
				Value:       status,
				Description: reason,
			},
			{
				Name:        observability.MetricCallDurationMs,
				Value:       fmt.Sprintf("%d", durationMs),
				Description: "Call duration in milliseconds",
			},
		},
	})

	if status == "FAILED" {
		return &PipelineResult{Error: fmt.Errorf("%s", reason)}
	}
	return &PipelineResult{}
}
