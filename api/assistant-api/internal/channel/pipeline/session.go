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

	obs "github.com/rapidaai/api/assistant-api/internal/observe"
)

// runSession handles the full session lifecycle for AudioSocket, WebSocket, and
// telephony channels. Follows the same standard as SIP media.go:
//
//	resolve → streamer → talker → observer → call_started → Talk() → call_ended + metric + shutdown
func (d *Dispatcher) runSession(ctx context.Context, v SessionConnectedPipeline) *PipelineResult {
	startTime := time.Now()
	d.logger.Infow("Pipeline: SessionConnected", "call_id", v.ID)

	contextID := v.ContextID
	if contextID == "" {
		contextID = v.ID
	}

	// --- Resolve session ---
	if d.onResolveSession == nil {
		return &PipelineResult{Error: ErrCallbackNotConfigured}
	}
	cc, vc, err := d.onResolveSession(ctx, v.ContextID)
	if err != nil {
		d.logger.Errorw("Pipeline: session resolution failed", "call_id", v.ID, "error", err)
		return &PipelineResult{Error: err}
	}

	// --- Create streamer ---
	if d.onCreateStreamer == nil {
		return &PipelineResult{Error: ErrCallbackNotConfigured}
	}
	streamer, err := d.onCreateStreamer(ctx, cc, vc, v.WebSocket, v.Conn, v.Reader, v.Writer)
	if err != nil {
		d.logger.Errorw("Pipeline: streamer creation failed", "call_id", v.ID, "error", err)
		return &PipelineResult{Error: err}
	}

	// --- Create talker ---
	if d.onCreateTalker == nil {
		return &PipelineResult{Error: ErrCallbackNotConfigured}
	}
	talker, err := d.onCreateTalker(ctx, streamer)
	if err != nil {
		d.logger.Errorw("Pipeline: talker creation failed", "call_id", v.ID, "error", err)
		return &PipelineResult{Error: err}
	}

	auth := cc.ToAuth()

	// --- Create observer ---
	var observer *obs.ConversationObserver
	if d.onCreateObserver != nil {
		observer = d.onCreateObserver(ctx, contextID, auth, cc.AssistantID, cc.ConversationID)
	}

	if observer != nil {
		clientPhone := cc.CallerNumber
		assistantPhone := cc.FromNumber
		if cc.Direction == "outbound" {
			clientPhone = cc.CallerNumber
			assistantPhone = cc.FromNumber
		}
		observer.EmitMetadata(ctx, obs.ClientMetadata(
			clientPhone, assistantPhone, cc.Direction, cc.Provider,
			cc.ChannelUUID, contextID, "", "", // codec/sampleRate set by streamer
		))
		observer.EmitEvent(ctx, obs.ComponentTelephony, map[string]string{
			obs.DataContextID: contextID,
			obs.DataType:      obs.EventCallStarted,
			obs.DataProvider:  cc.Provider,
			obs.DataDirection: cc.Direction,
		})
	}

	// --- Run Talk with panic recovery ---
	reason := "talk_completed"
	status := "COMPLETED"

	func() {
		defer func() {
			if r := recover(); r != nil {
				reason = fmt.Sprintf("panic: %v", r)
				status = "FAILED"
				d.logger.Errorw("Pipeline: Talk panicked", "call_id", v.ID, "panic", r)
			}
		}()

		if d.onRunTalk == nil {
			reason = "callback_not_configured"
			status = "FAILED"
			return
		}
		if err := d.onRunTalk(ctx, talker, auth); err != nil {
			reason = fmt.Sprintf("talk_error: %v", err)
			status = "FAILED"
		}
	}()

	// --- Cleanup: observer, complete ---
	if observer != nil {
		observer.EmitEvent(ctx, obs.ComponentTelephony, map[string]string{
			obs.DataType:      obs.EventCallEnded,
			obs.DataProvider:  cc.Provider,
			obs.DataDirection: cc.Direction,
			obs.DataReason:    reason,
		})
		observer.EmitMetric(ctx, obs.CallStatusMetric(status, reason))
		observer.Shutdown(ctx)
	}

	if d.onCompleteSession != nil {
		d.onCompleteSession(ctx, contextID)
	}

	d.logger.Infow("Pipeline: CallEnded",
		"call_id", v.ID,
		"duration", fmt.Sprintf("%dms", time.Since(startTime).Milliseconds()),
		"reason", reason,
		"status", status)

	if status == "FAILED" {
		return &PipelineResult{Error: fmt.Errorf("%s", reason)}
	}
	return &PipelineResult{}
}

func (d *Dispatcher) handleModeSwitch(ctx context.Context, v ModeSwitchPipeline) {
	d.logger.Infow("Pipeline: ModeSwitch", "call_id", v.ID, "from", v.From, "to", v.To)
}
