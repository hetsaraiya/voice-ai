// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package core

import (
	"context"
	"fmt"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	internal_inbound "github.com/rapidaai/api/assistant-api/sip/internal/inbound"
)

type inboundAnswerController struct {
	server      *Server
	session     *Session
	request     *sip.Request
	transaction sip.ServerTransaction
	dialog      *sipgo.DialogServerSession
	policy      InboundAnswerPolicy
	callID      string

	finalResponseStarted bool
}

func newInboundAnswerController(
	server *Server,
	session *Session,
	request *sip.Request,
	transaction sip.ServerTransaction,
	dialog *sipgo.DialogServerSession,
	policy InboundAnswerPolicy,
	callID string,
) *inboundAnswerController {
	return &inboundAnswerController{
		server:      server,
		session:     session,
		request:     request,
		transaction: transaction,
		dialog:      dialog,
		policy:      policy,
		callID:      callID,
	}
}

func (controller *inboundAnswerController) SendTrying() error {
	if controller.dialog == nil {
		return fmt.Errorf("inbound dialog session is required before trying")
	}
	if err := controller.dialog.Respond(100, "Trying", nil); err != nil {
		return fmt.Errorf("failed to send 100 Trying: %w", err)
	}
	controller.recordPhase(InboundSetupPhaseTryingSent, LifecycleReasonInboundInviteReceived)
	return nil
}

func (controller *inboundAnswerController) SendRinging() error {
	if controller.dialog == nil {
		return fmt.Errorf("inbound dialog session is required before ringing")
	}
	if err := controller.dialog.Respond(180, "Ringing", nil); err != nil {
		return fmt.Errorf("failed to send 180 Ringing: %w", err)
	}
	controller.recordPhase(InboundSetupPhaseRingingSent, LifecycleReasonInboundInviteRinging)
	return nil
}

func (controller *inboundAnswerController) WaitUntilAnswerReady(ctx context.Context) error {
	policy := controller.policy
	if policy.Mode == "" {
		policy = DefaultInboundAnswerPolicy()
	}
	if !policy.Mode.IsValid() {
		return fmt.Errorf("%w: invalid inbound answer mode %q", ErrInvalidConfig, policy.Mode)
	}

	switch policy.Mode {
	case InboundAnswerModeImmediate, InboundAnswerModeAssistantReady:
	case InboundAnswerModeAfterMinRingDuration:
		if err := controller.waitForMinimumRing(ctx, policy.MinRingDuration); err != nil {
			return err
		}
	case InboundAnswerModeBeforeMaxRingDuration:
		if err := controller.failIfMaximumRingExceeded(policy.MaxRingDuration); err != nil {
			return err
		}
	}
	if policy.RequireAssistantAudioReady {
		if err := controller.waitForAssistantAudioReady(ctx, policy.AssistantAudioReadyTimeout); err != nil {
			return err
		}
	}
	controller.recordPhase(InboundSetupPhaseAnswerReady, LifecycleReasonInboundAnswerPolicyReady)
	return nil
}

func (controller *inboundAnswerController) AnswerAndWaitACK(ctx context.Context, sdpBody string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	controller.finalResponseStarted = true
	if !controller.server.beginPendingInviteFinalResponse(controller.callID) {
		return ErrInboundInviteCancelled
	}
	controller.recordPhase(InboundSetupPhaseAnswered, LifecycleReasonInboundInviteAnswered)
	if err := controller.server.sendSDPResponseAndWaitACK(
		controller.transaction,
		controller.request,
		controller.session,
		sdpBody,
		LifecycleReasonInboundInviteACKReceived,
		controller.policy.ACKTimeout,
	); err != nil {
		if err == ErrInboundACKTimeout {
			return fmt.Errorf("%w: initial INVITE ACK not received", ErrInboundACKTimeout)
		}
		return fmt.Errorf("failed to send inbound 200 OK: %w", err)
	}
	return nil
}

func (controller *inboundAnswerController) CancelBeforeAnswer(reason LifecycleReason) bool {
	if controller == nil {
		return false
	}
	terminated := controller.server.terminatePendingInvite(controller.callID, 487)
	if terminated {
		controller.finalResponseStarted = true
	}
	return terminated
}

func (controller *inboundAnswerController) FailBeforeAnswer(statusCode int, failureClass internal_inbound.FailureClass, reason LifecycleReason, err error) {
	if controller == nil {
		return
	}
	if controller.finalResponseStarted {
		return
	}
	if controller.session == nil {
		controller.server.RejectInboundInvite(controller.request, controller.transaction, controller.callID, statusCode, failureClass, reason, err)
		controller.finalResponseStarted = true
		return
	}
	controller.sendSessionFinalResponse(statusCode)
}

func (controller *inboundAnswerController) FinalResponseStarted() bool {
	return controller != nil && controller.finalResponseStarted
}

func (controller *inboundAnswerController) waitForMinimumRing(ctx context.Context, minRingDuration time.Duration) error {
	timings := controller.session.GetInboundSetupTimings()
	if minRingDuration <= 0 || timings.RingingSentAt.IsZero() {
		return nil
	}
	if remaining := minRingDuration - time.Since(timings.RingingSentAt); remaining > 0 {
		timer := time.NewTimer(remaining)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-controller.server.ctx.Done():
			return controller.server.ctx.Err()
		case <-controller.session.Context().Done():
			return controller.session.Context().Err()
		case <-timer.C:
		}
	}
	return nil
}

func (controller *inboundAnswerController) failIfMaximumRingExceeded(maxRingDuration time.Duration) error {
	timings := controller.session.GetInboundSetupTimings()
	if maxRingDuration <= 0 || timings.RingingSentAt.IsZero() {
		return nil
	}
	if time.Since(timings.RingingSentAt) <= maxRingDuration {
		return nil
	}
	return fmt.Errorf("%w: maximum inbound ring duration exceeded", ErrInboundAnswerPolicyTimeout)
}

func (controller *inboundAnswerController) waitForAssistantAudioReady(ctx context.Context, timeout time.Duration) error {
	if controller.session == nil {
		return fmt.Errorf("%w: inbound session is not available", ErrInboundAnswerPolicyTimeout)
	}
	if !controller.session.GetInboundSetupTimings().FirstAssistantAudioReadyAt.IsZero() {
		return nil
	}
	if timeout <= 0 {
		return fmt.Errorf("%w: assistant audio readiness required before answer", ErrInboundAnswerPolicyTimeout)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-controller.server.ctx.Done():
			return controller.server.ctx.Err()
		case <-controller.session.Context().Done():
			return controller.session.Context().Err()
		case <-timer.C:
			return fmt.Errorf("%w: assistant audio readiness timeout", ErrInboundAnswerPolicyTimeout)
		case <-ticker.C:
			if !controller.session.GetInboundSetupTimings().FirstAssistantAudioReadyAt.IsZero() {
				return nil
			}
		}
	}
}

func (controller *inboundAnswerController) sendSessionFinalResponse(statusCode int) {
	if controller.finalResponseStarted {
		return
	}
	if controller.dialog == nil || controller.dialog.InviteRequest == nil {
		controller.server.logger.Errorw("Inbound session final response skipped without dialog ownership",
			"call_id", controller.callID,
			"status_code", statusCode)
		controller.finalResponseStarted = true
		return
	}

	response := sip.NewResponseFromRequest(controller.dialog.InviteRequest, statusCode, "", nil)
	if response.Contact() == nil {
		contactHeader := buildSIPContactHeader(controller.server.listenConfig)
		response.AppendHeader(&contactHeader)
	}
	controller.dialog.InviteResponse = response
	if err := controller.transaction.Respond(response); err != nil {
		controller.server.logger.Errorw("Failed to send inbound dialog final response",
			"error", err,
			"call_id", controller.callID,
			"status_code", statusCode)
	}
	controller.finalResponseStarted = true
}

func (controller *inboundAnswerController) recordPhase(phase InboundSetupPhase, reason LifecycleReason) {
	timestamp := time.Now()
	if controller.session != nil {
		controller.session.SetInboundSetupPhase(phase)
		controller.session.MarkInboundSetupTimestamp(phase, timestamp)
	}
	controller.server.logger.Infow("Inbound SIP setup phase",
		"call_id", controller.callID,
		"phase", phase,
		"reason", reason)
}
