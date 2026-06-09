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

	"github.com/rapidaai/api/assistant-api/internal/observability"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	"github.com/rapidaai/protos"
)

type preparedSession struct {
	stage    sip_infra.SessionEstablishedPipeline
	setup    *CallSetupResult
	observer observability.Recorder
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

	conversationID := v.ConversationID
	if conversationID == 0 {
		var err error
		conversationID, err = d.createConversation(ctx, v)
		if err != nil {
			d.logger.Error("Pipeline: create conversation failed", "call_id", v.ID, "error", err)
			return nil, newSessionPreparationError(sip_infra.LifecycleReasonPipelineConversationFailed, err)
		}
		v.Session.SetConversationID(conversationID)
	}

	cc, err := d.ensureCallContext(ctx, v, conversationID)
	if err != nil {
		d.logger.Warnw("Pipeline: ensure call context failed", "call_id", v.ID, "error", err)
	}

	setup, err := d.setupCall(ctx, v, conversationID, cc)
	if err != nil {
		d.logger.Error("Pipeline: call setup failed", "call_id", v.ID, "error", err)
		return nil, newSessionPreparationError(sip_infra.LifecycleReasonPipelineSetupFailed, err)
	}

	observer := d.createObserver(ctx, setup, v.Auth)

	codec := ""
	sampleRate := ""
	if negotiated := v.Session.GetNegotiatedCodec(); negotiated != nil {
		codec = negotiated.Name
		sampleRate = fmt.Sprintf("%d", negotiated.ClockRate)
	}
	// Identity keys flow through ConversationInitialization.Metadata.
	// provider_call_id is emitted here as well because the SIP Call-ID
	// is only known at this stage and isn't required for prompts.
	scope := observability.ConversationScope{
		AssistantScope: observability.AssistantScope{AssistantID: setup.AssistantID},
		ConversationID: setup.ConversationID,
	}
	_ = observer.Record(
		ctx,
		scope,
		observability.RecordMetadata{
			Metadata: observability.ClientMetadata("", "", string(v.Direction), "sip", v.ID, "", codec, sampleRate),
		},
		observability.RecordEvent{
			Component: observability.ComponentCall,
			Event:     observability.CallSessionConnected,
			Attributes: observability.Attributes{
				"provider":   "sip",
				"direction":  string(v.Direction),
				"call_id":    v.ID,
				"context_id": v.ID,
			},
		},
		observability.RecordMetric{
			Metrics: []*protos.Metric{{
				Name:        observability.MetricCallStatus,
				Value:       observability.MetricCallStatusInProgress,
				Description: "SIP session connected",
			}},
		},
	)
	var runtime PreparedCallRuntime
	if v.Direction == sip_infra.CallDirectionInbound {
		var err error
		preparedRuntime, err := d.prepareSIPCallRuntime(ctx, v.Session, setup, observer, v.VaultCredential, v.Config, string(v.Direction))
		if err != nil {
			_ = observer.Close(ctx)
			d.logger.Error("Pipeline: runtime preparation failed", "call_id", v.ID, "error", err)
			return nil, newSessionPreparationError(sip_infra.LifecycleReasonPipelineSetupFailed, err)
		}
		if err := preparedRuntime.StartBeforeAnswer(ctx, inboundRuntimeReadyTimeout(v.Config)); err != nil {
			preparedRuntime.Close(ctx)
			_ = observer.Close(ctx)
			d.logger.Error("Pipeline: runtime pre-answer start failed", "call_id", v.ID, "error", err)
			return nil, newSessionPreparationError(sip_infra.LifecycleReasonPipelineSetupFailed, err)
		}
		runtime = preparedRuntime
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
		status := observability.MetricCallStatusComplete
		scope := observability.ConversationScope{
			AssistantScope: observability.AssistantScope{AssistantID: setup.AssistantID},
			ConversationID: setup.ConversationID,
		}
		_ = observer.Record(
			ctx,
			scope,
			observability.RecordEvent{
				Component: observability.ComponentCall,
				Event:     observability.CallStarted,
				Attributes: observability.Attributes{
					"provider":  "sip",
					"direction": string(v.Direction),
					"call_id":   v.ID,
				},
			},
			observability.RecordMetric{
				Metrics: []*protos.Metric{{
					Name:        observability.MetricCallStatus,
					Value:       observability.MetricCallStatusInProgress,
					Description: "SIP call started",
				}},
			},
		)
		defer func() {
			var panicRecord observability.RecordLog
			hasPanic := false
			if r := recover(); r != nil {
				reason = fmt.Sprintf("panic: %v", r)
				status = observability.MetricCallStatusFailed
				d.logger.Error("Pipeline: onCallStart panicked", "call_id", v.ID, "panic", r)
				hasPanic = true
				panicRecord = observability.RecordLog{
					Level:   observability.LevelError,
					Message: "SIP pipeline call start panicked",
					Attributes: observability.Attributes{
						"provider":  "sip",
						"direction": string(v.Direction),
						"call_id":   v.ID,
						"panic":     fmt.Sprintf("%v", r),
					},
				}
			}

			durationMs := time.Since(startTime).Milliseconds()
			event := observability.CallEnded
			if status == observability.MetricCallStatusFailed {
				event = observability.CallFailed
			}
			eventRecord := observability.RecordEvent{
				Component: observability.ComponentCall,
				Event:     event,
				Attributes: observability.Attributes{
					"provider":    "sip",
					"direction":   string(v.Direction),
					"call_id":     v.ID,
					"reason":      reason,
					"status":      status,
					"duration_ms": fmt.Sprintf("%d", durationMs),
				},
			}
			metricRecord := observability.RecordMetric{
				Metrics: []*protos.Metric{{
					Name:        observability.MetricCallStatus,
					Value:       status,
					Description: reason,
				}, {
					Name:        observability.MetricCallDurationMs,
					Value:       fmt.Sprintf("%d", durationMs),
					Description: "SIP call duration in milliseconds",
				}},
			}
			if hasPanic {
				_ = observer.Record(ctx, scope, panicRecord, eventRecord, metricRecord)
			} else {
				_ = observer.Record(ctx, scope, eventRecord, metricRecord)
			}
			_ = observer.Close(ctx)
			d.endCall(v.Session, sip_infra.LifecycleReasonPipelineCallEnd)
			d.OnPipeline(ctx, sip_infra.CallEndedPipeline{
				ID:       v.ID,
				Duration: time.Since(startTime),
				Reason:   reason,
			})
		}()
		if prepared.runtime != nil {
			if err := prepared.runtime.Start(ctx); err != nil {
				reason = err.Error()
				status = observability.MetricCallStatusFailed
			}
		} else {
			if v.Session.GetInfo().Direction == sip_infra.CallDirectionOutbound {
				defer func() {
					if !v.Session.IsEnded() {
						state := v.Session.GetState()
						if state == sip_infra.CallStateTransferring || state == sip_infra.CallStateBridgeConnected {
							return
						}
						d.endCall(v.Session, sip_infra.LifecycleReasonPipelineTalkCompleted)
					}
				}()
			}
			runtime, err := d.prepareSIPCallRuntime(ctx, v.Session, setup, observer, v.VaultCredential, v.Config, string(v.Direction))
			if err != nil {
				reason = err.Error()
				status = observability.MetricCallStatusFailed
			} else if err := runtime.Start(ctx); err != nil {
				reason = err.Error()
				status = observability.MetricCallStatusFailed
			}
		}

		// Check if the call ended due to a bridge transfer — emit transfer events
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
				_ = observer.Record(ctx, scope, observability.RecordEvent{
					Component: observability.ComponentSIP,
					Event:     observability.SIPTransferRequested,
					Attributes: observability.Attributes{
						"provider":  "sip",
						"direction": string(v.Direction),
						"call_id":   v.ID,
						"target":    target,
						"reason":    transferStatus,
					},
				})
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
	_ = p.observer.Close(ctx)
}

func sessionPreparationReason(err error) sip_infra.LifecycleReason {
	var preparationErr *sessionPreparationError
	if errors.As(err, &preparationErr) {
		return preparationErr.reason
	}
	return sip_infra.LifecycleReasonPipelineSetupFailed
}
