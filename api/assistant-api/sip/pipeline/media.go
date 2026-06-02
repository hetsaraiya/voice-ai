// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package sip_pipeline

import (
	"context"
	"errors"
	"fmt"
	"time"

	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	obs "github.com/rapidaai/api/assistant-api/internal/observe"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
)

type preparedSession struct {
	stage    sip_infra.SessionEstablishedPipeline
	setup    *CallSetupResult
	observer *obs.ConversationObserver
	runtime  PreparedCallRuntime
}

type sessionPreparationError struct {
	reason sip_infra.LifecycleReason
	err    error
}

func (e *sessionPreparationError) Error() string {
	return e.err.Error()
}

func (e *sessionPreparationError) Unwrap() error {
	return e.err
}

func newSessionPreparationError(reason sip_infra.LifecycleReason, err error) *sessionPreparationError {
	return &sessionPreparationError{reason: reason, err: err}
}

func (d *Dispatcher) handleSessionEstablished(ctx context.Context, v sip_infra.SessionEstablishedPipeline) {
	prepared, err := d.prepareSession(ctx, v)
	if err != nil {
		d.logger.Error("Pipeline: session preparation failed", "call_id", v.ID, "error", err)
		d.endCall(v.Session, sessionPreparationReason(err))
		return
	}
	d.startPreparedSession(ctx, prepared)
}

func (d *Dispatcher) PrepareSession(ctx context.Context, v sip_infra.SessionEstablishedPipeline) error {
	prepared, err := d.prepareSession(ctx, v)
	if err != nil {
		return err
	}
	d.preparedMu.Lock()
	d.preparedSessions[v.ID] = prepared
	d.preparedMu.Unlock()
	return nil
}

func (d *Dispatcher) StartPreparedSession(ctx context.Context, v sip_infra.SessionEstablishedPipeline) error {
	prepared := d.popPreparedSession(v.ID)
	if prepared == nil {
		return fmt.Errorf("prepared SIP session not found for call %s", v.ID)
	}
	d.startPreparedSession(ctx, prepared)
	return nil
}

func (d *Dispatcher) DiscardPreparedSession(ctx context.Context, callID string) {
	prepared := d.popPreparedSession(callID)
	if prepared == nil {
		return
	}
	prepared.Close(ctx)
}

func (d *Dispatcher) popPreparedSession(callID string) *preparedSession {
	d.preparedMu.Lock()
	defer d.preparedMu.Unlock()
	prepared := d.preparedSessions[callID]
	delete(d.preparedSessions, callID)
	return prepared
}

func (d *Dispatcher) prepareSession(ctx context.Context, v sip_infra.SessionEstablishedPipeline) (*preparedSession, error) {
	d.logger.Infow("Pipeline: SessionEstablished",
		"call_id", v.ID,
		"direction", v.Direction,
		"assistant_id", v.AssistantID,
		"conversation_id", v.ConversationID)

	if d.onCallSetup == nil || d.onCallStart == nil {
		d.logger.Error("Pipeline: callbacks not configured", "call_id", v.ID)
		return nil, newSessionPreparationError(
			sip_infra.LifecycleReasonPipelineCallbacksMissing,
			fmt.Errorf("pipeline callbacks not configured"),
		)
	}

	conversationID := v.ConversationID
	if conversationID == 0 {
		if d.onCreateConversation == nil {
			d.logger.Error("Pipeline: onCreateConversation not configured", "call_id", v.ID)
			return nil, newSessionPreparationError(
				sip_infra.LifecycleReasonPipelineConversationMissing,
				fmt.Errorf("pipeline conversation callback not configured"),
			)
		}
		var err error
		conversationID, err = d.onCreateConversation(ctx, v.Auth, v.AssistantID, v.FromURI, string(v.Direction))
		if err != nil {
			d.logger.Error("Pipeline: create conversation failed", "call_id", v.ID, "error", err)
			return nil, newSessionPreparationError(sip_infra.LifecycleReasonPipelineConversationFailed, err)
		}
		v.Session.SetConversationID(conversationID)
	}

	var cc *callcontext.CallContext
	if d.onEnsureCallContext != nil {
		ensured, err := d.onEnsureCallContext(ctx, v.Session, v.Auth, v.AssistantID, conversationID, v.Direction, v.FromURI, v.ToURI)
		if err != nil {
			d.logger.Warnw("Pipeline: ensure call context failed", "call_id", v.ID, "error", err)
		}
		cc = ensured
	}

	setup, err := d.onCallSetup(ctx, v.Session, v.Auth, v.AssistantID, conversationID, cc)
	if err != nil {
		d.logger.Error("Pipeline: call setup failed", "call_id", v.ID, "error", err)
		return nil, newSessionPreparationError(sip_infra.LifecycleReasonPipelineSetupFailed, err)
	}

	var observer *obs.ConversationObserver
	if d.onCreateObserver != nil {
		observer = d.onCreateObserver(ctx, setup, v.Auth)
	}

	if observer != nil {
		codec := ""
		sampleRate := ""
		if negotiated := v.Session.GetNegotiatedCodec(); negotiated != nil {
			codec = negotiated.Name
			sampleRate = fmt.Sprintf("%d", negotiated.ClockRate)
		}
		// Identity keys flow through ConversationInitialization.Metadata.
		// provider_call_id is emitted here as well because the SIP Call-ID
		// is only known at this stage and isn't required for prompts.
		observer.EmitMetadata(ctx, obs.ClientMetadata(
			"", "", "", "",
			v.ID, "",
			codec, sampleRate,
		))
	}
	var runtime PreparedCallRuntime
	if v.Direction == sip_infra.CallDirectionInbound && d.onPrepareCallRuntime != nil {
		var err error
		runtime, err = d.onPrepareCallRuntime(ctx, v, setup, observer)
		if err != nil {
			if observer != nil {
				observer.Shutdown(ctx)
			}
			d.logger.Error("Pipeline: runtime preparation failed", "call_id", v.ID, "error", err)
			return nil, newSessionPreparationError(sip_infra.LifecycleReasonPipelineSetupFailed, err)
		}
	}
	return &preparedSession{stage: v, setup: setup, observer: observer, runtime: runtime}, nil
}

func (d *Dispatcher) startPreparedSession(ctx context.Context, prepared *preparedSession) {
	v := prepared.stage
	setup := prepared.setup
	observer := prepared.observer
	go func() {
		startTime := time.Now()
		reason := "talk_completed"
		status := "COMPLETED"
		if observer != nil {
			observer.EmitEvent(ctx, obs.ComponentTelephony, map[string]string{
				obs.DataType:      obs.EventCallStarted,
				obs.DataProvider:  "sip",
				obs.DataDirection: string(v.Direction),
			})
		}
		defer func() {
			if r := recover(); r != nil {
				reason = fmt.Sprintf("panic: %v", r)
				status = "FAILED"
				d.logger.Error("Pipeline: onCallStart panicked", "call_id", v.ID, "panic", r)
			}

			if observer != nil {
				observer.EmitEvent(ctx, obs.ComponentTelephony, map[string]string{
					obs.DataType:      obs.EventCallEnded,
					obs.DataProvider:  "sip",
					obs.DataDirection: string(v.Direction),
					obs.DataReason:    reason,
				})
				observer.EmitMetric(ctx, obs.CallStatusMetric(status, reason))
				observer.Shutdown(ctx)
			}
			if d.onCallEnd != nil {
				d.onCallEnd(v.ID)
			}

			d.OnPipeline(ctx, sip_infra.CallEndedPipeline{
				ID:       v.ID,
				Duration: time.Since(startTime),
				Reason:   reason,
			})
		}()
		if prepared.runtime != nil {
			if err := prepared.runtime.Start(ctx); err != nil {
				reason = err.Error()
				status = "FAILED"
			}
		} else if err := d.onCallStart(ctx, v.Session, setup, v.VaultCredential, v.Config, string(v.Direction)); err != nil {
			reason = err.Error()
			status = "FAILED"
		}

		// Check if the call ended due to a bridge transfer — emit transfer events
		if observer != nil {
			if targetVal, ok := v.Session.GetMetadata(sip_infra.MetadataBridgeTransferTarget); ok {
				if target, ok := targetVal.(string); ok && target != "" {
					transferStatus := "failed"
					if statusVal, ok := v.Session.GetMetadata(sip_infra.MetadataBridgeTransferStatus); ok {
						if s, ok := statusVal.(string); ok {
							transferStatus = s
						}
					}
					reason = "transfer_" + transferStatus
					d.logger.Infow("Pipeline: bridge transfer",
						"call_id", v.ID, "target", target, "status", transferStatus)
					observer.EmitEvent(ctx, obs.ComponentTelephony, map[string]string{
						obs.DataType:      obs.EventTransferRequested,
						obs.DataProvider:  "sip",
						obs.DataDirection: string(v.Direction),
						obs.DataTo:        target,
						obs.DataReason:    transferStatus,
					})
				}
			}
		}
	}()
}

func (p *preparedSession) Close(ctx context.Context) {
	if p == nil {
		return
	}
	if p.runtime != nil {
		p.runtime.Close(ctx)
	}
	if p.observer != nil {
		p.observer.Shutdown(ctx)
	}
}

func sessionPreparationReason(err error) sip_infra.LifecycleReason {
	var preparationErr *sessionPreparationError
	if errors.As(err, &preparationErr) {
		return preparationErr.reason
	}
	return sip_infra.LifecycleReasonPipelineSetupFailed
}
