// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package core

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_inbound "github.com/rapidaai/api/assistant-api/sip/internal/inbound"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/protos"
)

type inboundCall struct {
	server      *Server
	request     *sip.Request
	transaction sip.ServerTransaction

	identity       inboundInviteIdentity
	mediaOffer     inboundMediaOffer
	resolvedConfig inboundResolvedConfig

	session          *Session
	dialogSession    *sipgo.DialogServerSession
	rtpHandler       *RTPHandler
	allocatedRTPPort int
	localRTPPort     int
	externalIP       string
	setupPhase       InboundSetupPhase
	setupTimings     InboundSetupTimings
	answerPolicy     InboundAnswerPolicy
	answerController *inboundAnswerController
	rtpStarted       bool
}

func newInboundCall(server *Server, request *sip.Request, transaction sip.ServerTransaction) *inboundCall {
	return &inboundCall{
		server:      server,
		request:     request,
		transaction: transaction,
	}
}

func (inboundCall *inboundCall) run() {
	if err := inboundCall.loadIdentity(); err != nil {
		inboundCall.server.logger.Errorw("Invalid inbound INVITE", "error", err)
		inboundCall.sendFinalResponse(400)
		return
	}
	inboundCall.recordPhase(InboundSetupPhaseInviteReceived, LifecycleReasonInboundInviteReceived)

	server := inboundCall.server
	callID := inboundCall.identity.callID

	server.logger.Infow("Received INVITE",
		"call_id", callID,
		"from", inboundCall.identity.fromURI,
		"to", inboundCall.identity.toURI)
	server.setPendingInvite(callID, inboundCall.request, inboundCall.transaction)
	defer inboundCall.finish()

	if inboundCall.routeReInvite() {
		return
	}

	if inboundCall.cancelIfRequested(LifecycleReasonInviteCancelled) {
		return
	}
	if err := inboundCall.parseMediaOffer(); err != nil {
		inboundCall.failSetupError(400, internal_inbound.FailureMedia, LifecycleReasonInboundInviteFailed, err)
		return
	}
	if inboundCall.cancelIfRequested(LifecycleReasonInviteCancelled) {
		return
	}
	if err := inboundCall.resolveConfig(); err != nil {
		if errors.Is(err, ErrInboundInviteCancelled) {
			inboundCall.cancelIfRequested(LifecycleReasonInviteCancelled)
			return
		}
		inboundCall.failSetupError(500, internal_inbound.FailureConfig, LifecycleReasonInboundInviteFailed, err)
		return
	}
	if inboundCall.cancelIfRequested(LifecycleReasonInviteCancelled) {
		return
	}
	if err := inboundCall.createSession(); err != nil {
		inboundCall.failSetup(500, internal_inbound.FailureSetup, LifecycleReasonInboundInviteFailed, err)
		return
	}

	server.registerSession(inboundCall.session, callID)
	if inboundCall.cancelIfRequested(LifecycleReasonInviteCancelled) {
		return
	}
	if err := inboundCall.createDialog(); err != nil {
		inboundCall.failSetup(500, internal_inbound.FailureDialog, LifecycleReasonInboundInviteFailed, err)
		return
	}
	inboundCall.createAnswerController()
	if err := inboundCall.answerController.SendTrying(); err != nil {
		inboundCall.failSetup(500, internal_inbound.FailureDialog, LifecycleReasonInboundInviteFailed, err)
		return
	}
	server.TransitionCall(inboundCall.session, CallStateRinging, LifecycleReasonInboundInviteRinging)
	if err := inboundCall.answerController.SendRinging(); err != nil {
		inboundCall.failSetup(500, internal_inbound.FailureDialog, LifecycleReasonInboundInviteFailed, err)
		return
	}

	if err := inboundCall.setupRTP(); err != nil {
		inboundCall.failSetup(503, internal_inbound.FailureRTP, LifecycleReasonInboundInviteFailed, err)
		return
	}
	if inboundCall.cancelIfRequested(LifecycleReasonInviteCancelledBeforeAnswer) {
		return
	}
	if err := inboundCall.prepareApplication(); err != nil {
		inboundCall.failSetup(503, internal_inbound.FailureSetup, LifecycleReasonPipelineSetupFailed, err)
		return
	}
	if inboundCall.cancelIfRequested(LifecycleReasonInviteCancelledBeforeAnswer) {
		return
	}
	if err := inboundCall.answerController.WaitUntilAnswerReady(server.ctx); err != nil {
		inboundCall.failSetup(408, internal_inbound.FailureSetup, LifecycleReasonInboundAnswerPolicyTimeout, err)
		return
	}
	if inboundCall.cancelIfRequested(LifecycleReasonInviteCancelledBeforeAnswer) {
		return
	}
	if err := inboundCall.startRTP(); err != nil {
		inboundCall.failSetup(503, internal_inbound.FailureRTP, LifecycleReasonInboundInviteFailed, err)
		return
	}
	if err := inboundCall.answer(); err != nil {
		if errors.Is(err, ErrInboundInviteCancelled) {
			return
		}
		if errors.Is(err, ErrInboundACKTimeout) {
			inboundCall.failSetup(408, internal_inbound.FailureDialog, LifecycleReasonInboundACKTimeout, err)
			return
		}
		inboundCall.failSetup(500, internal_inbound.FailureDialog, LifecycleReasonInboundInviteFailed, err)
		return
	}
	if inboundCall.rtpStarted {
		inboundCall.recordPhase(InboundSetupPhaseMediaFlowing, LifecycleReasonInboundMediaFlowing)
	}

	inboundCall.dispatchInvite()
}

func (inboundCall *inboundCall) loadIdentity() error {
	request := inboundCall.request
	if request == nil {
		return fmt.Errorf("request is nil")
	}
	callIDHeader := request.CallID()
	if callIDHeader == nil || callIDHeader.Value() == "" {
		return fmt.Errorf("Call-ID header is required")
	}
	fromHeader := request.From()
	if fromHeader == nil {
		return fmt.Errorf("From header is required")
	}
	toHeader := request.To()
	if toHeader == nil {
		return fmt.Errorf("To header is required")
	}
	inboundCall.identity = inboundInviteIdentity{
		callID:  callIDHeader.Value(),
		fromURI: fromHeader.Address.String(),
		toURI:   toHeader.Address.String(),
	}
	return nil
}

func (inboundCall *inboundCall) routeReInvite() bool {
	server := inboundCall.server
	callID := inboundCall.identity.callID

	server.mu.RLock()
	existingSession, isReInvite := server.sessions[callID]
	server.mu.RUnlock()

	if !isReInvite || existingSession == nil {
		return false
	}

	info := existingSession.GetInfo()
	server.logger.Infow("Routing as re-INVITE for existing session",
		"call_id", callID,
		"direction", info.Direction,
		"state", info.State)
	server.handleReInvite(inboundCall.request, inboundCall.transaction, existingSession)
	return true
}

func (inboundCall *inboundCall) parseMediaOffer() error {
	server := inboundCall.server
	callID := inboundCall.identity.callID

	mediaOffer, err := parseInboundSDPMediaOffer(
		server,
		inboundCall.request,
		callID,
		"inbound INVITE",
		LifecycleReasonInboundInviteFailed,
		false,
	)
	if err != nil {
		return err
	}

	inboundCall.mediaOffer = mediaOffer
	server.logger.Debugw("Parsed inbound SDP",
		"call_id", callID,
		"remote_rtp_ip", mediaOffer.sdpInfo.ConnectionIP,
		"remote_rtp_port", mediaOffer.sdpInfo.AudioPort,
		"codec", mediaOffer.negotiatedCodec.Name)
	return nil
}

func parseInboundSDPMediaOffer(
	server *Server,
	request *sip.Request,
	callID string,
	requestName string,
	reason LifecycleReason,
	allowDisabledRTPAddress bool,
) (inboundMediaOffer, error) {
	if err := validateSDPContentType(request, requestName, reason); err != nil {
		return inboundMediaOffer{}, err
	}

	sdpInfo, err := server.ParseSDP(request.Body())
	if err != nil {
		return inboundMediaOffer{}, fmt.Errorf("%w: %v", ErrSDPParseFailed, err)
	}
	if err := validateInboundRemoteMedia(sdpInfo, allowDisabledRTPAddress, reason); err != nil {
		return inboundMediaOffer{}, err
	}
	negotiatedCodec := firstSupportedAudioCodec(sdpInfo.PayloadTypes)
	if negotiatedCodec == nil {
		err := fmt.Errorf("%w: %s payload types %v", ErrCodecNotSupported, requestName, sdpInfo.PayloadTypes)
		return inboundMediaOffer{}, newInboundSetupError(488, internal_inbound.FailureMedia, reason, err)
	}

	server.logger.Debugw("Parsed SIP SDP media offer",
		"call_id", callID,
		"request", requestName,
		"remote_rtp_ip", sdpInfo.ConnectionIP,
		"remote_rtp_port", sdpInfo.AudioPort,
		"codec", negotiatedCodec.Name,
		"is_hold", sdpInfo.IsHold())

	return inboundMediaOffer{
		sdpInfo:         sdpInfo,
		negotiatedCodec: negotiatedCodec,
	}, nil
}

func validateSDPContentType(request *sip.Request, requestName string, reason LifecycleReason) error {
	if len(request.Body()) == 0 {
		return nil
	}
	contentTypeHeader := request.GetHeader("Content-Type")
	if contentTypeHeader == nil {
		err := fmt.Errorf("%w: %s missing Content-Type", ErrSDPParseFailed, requestName)
		return newInboundSetupError(415, internal_inbound.FailureMedia, reason, err)
	}
	contentType := strings.ToLower(strings.TrimSpace(contentTypeHeader.Value()))
	if semicolon := strings.Index(contentType, ";"); semicolon >= 0 {
		contentType = strings.TrimSpace(contentType[:semicolon])
	}
	if contentType != internal_inbound.SDPContentType {
		err := fmt.Errorf("%w: unsupported %s media type %q", ErrSDPParseFailed, requestName, contentTypeHeader.Value())
		return newInboundSetupError(415, internal_inbound.FailureMedia, reason, err)
	}
	return nil
}

func validateInboundRemoteMedia(sdpInfo *SDPMediaInfo, allowDisabledRTPAddress bool, reason LifecycleReason) error {
	if sdpInfo == nil {
		return fmt.Errorf("%w: inbound offer missing SDP media", ErrSDPParseFailed)
	}
	if strings.TrimSpace(sdpInfo.ConnectionIP) == "" {
		return fmt.Errorf("%w: inbound offer missing RTP address", ErrSDPParseFailed)
	}
	remoteIP := net.ParseIP(sdpInfo.ConnectionIP)
	if remoteIP == nil {
		return fmt.Errorf("%w: inbound offer invalid RTP address %q", ErrSDPParseFailed, sdpInfo.ConnectionIP)
	}
	if remoteIP.IsUnspecified() && !allowDisabledRTPAddress {
		err := fmt.Errorf("%w: inbound offer disabled RTP address %q", ErrSDPParseFailed, sdpInfo.ConnectionIP)
		return newInboundSetupError(488, internal_inbound.FailureMedia, reason, err)
	}
	if sdpInfo.AudioPort <= 0 || sdpInfo.AudioPort > 65535 {
		return fmt.Errorf("%w: inbound offer invalid RTP port %d", ErrSDPParseFailed, sdpInfo.AudioPort)
	}
	if len(sdpInfo.PayloadTypes) == 0 {
		err := fmt.Errorf("%w: inbound offer has no RTP payload types", ErrCodecNotSupported)
		return newInboundSetupError(488, internal_inbound.FailureMedia, reason, err)
	}
	return nil
}

func (inboundCall *inboundCall) resolveConfig() error {
	server := inboundCall.server

	server.mu.RLock()
	resolver := server.configResolver
	server.mu.RUnlock()
	if resolver == nil {
		return fmt.Errorf("no SIP config resolver configured")
	}

	requestContext := &SIPRequestContext{
		Method:  "INVITE",
		CallID:  inboundCall.identity.callID,
		FromURI: inboundCall.identity.fromURI,
		ToURI:   inboundCall.identity.toURI,
		SDPInfo: inboundCall.mediaOffer.sdpInfo,
	}
	inviteResult, err := resolver(requestContext)
	if err != nil {
		return fmt.Errorf("SIP authentication/config resolution failed: %w", err)
	}
	if inviteResult == nil {
		return fmt.Errorf("SIP config resolver returned nil result")
	}
	if server.isInviteCancelled(inboundCall.identity.callID) {
		return ErrInboundInviteCancelled
	}
	if !inviteResult.ShouldAllow {
		rejectCode := inviteResult.RejectCode
		if rejectCode <= 0 {
			rejectCode = 403
		}
		server.logger.Warnw("Call rejected by authentication chain",
			"call_id", inboundCall.identity.callID,
			"status_code", rejectCode,
			"reason", inviteResult.RejectMsg)
		err := fmt.Errorf("%w: rejected by authentication chain", ErrAuthRequired)
		return newInboundSetupError(rejectCode, internal_inbound.FailureAuth, LifecycleReasonInboundInviteFailed, err)
	}
	if inviteResult.Config == nil {
		return fmt.Errorf("no SIP config resolved for inbound call")
	}
	inboundCall.recordPhase(InboundSetupPhaseAuthenticated, LifecycleReasonInboundAuthenticated)

	resolvedConfig := inboundResolvedConfig{
		config: inviteResult.Config,
		extra:  inviteResult.Extra,
	}
	if resolvedConfig.extra == nil {
		resolvedConfig.extra = map[string]interface{}{}
	}
	if auth, ok := resolvedConfig.extra["auth"].(types.SimplePrinciple); ok {
		resolvedConfig.auth = auth
	}
	if vaultCredential, ok := resolvedConfig.extra["vault_credential"].(*protos.VaultCredential); ok {
		resolvedConfig.vaultCredential = vaultCredential
	}
	if assistant, ok := resolvedConfig.extra["assistant"].(*internal_assistant_entity.Assistant); ok {
		resolvedConfig.assistant = assistant
	}
	if resolvedConfig.assistant != nil {
		inboundCall.recordPhase(InboundSetupPhaseRouted, LifecycleReasonInboundRouted)
	}
	if resolvedConfig.config.Server == "" || resolvedConfig.config.Server == "0.0.0.0" {
		resolvedConfig.config.Server = server.listenConfig.GetExternalIP()
	}

	inboundCall.resolvedConfig = resolvedConfig
	inboundCall.answerPolicy = resolvedConfig.config.EffectiveInboundAnswerPolicy(server.effectiveInboundACKTimeout())
	server.logger.Debugw("SIP INVITE authenticated",
		"call_id", inboundCall.identity.callID,
		"assistant_id", requestContext.AssistantID,
		"has_api_key", requestContext.APIKey != "")
	return nil
}

func (inboundCall *inboundCall) createSession() error {
	session, err := NewSession(inboundCall.server.ctx, &SessionConfig{
		Config:          inboundCall.resolvedConfig.config,
		Direction:       CallDirectionInbound,
		CallID:          inboundCall.identity.callID,
		Codec:           inboundCall.mediaOffer.negotiatedCodec,
		Logger:          inboundCall.server.logger,
		Auth:            inboundCall.resolvedConfig.auth,
		Assistant:       inboundCall.resolvedConfig.assistant,
		VaultCredential: inboundCall.resolvedConfig.vaultCredential,
	})
	if err != nil {
		return fmt.Errorf("failed to create inbound session: %w", err)
	}

	for key, value := range inboundCall.resolvedConfig.extra {
		session.SetMetadata(key, value)
	}
	if inboundCall.setupPhase != "" {
		session.SetInboundSetupPhase(inboundCall.setupPhase)
	}
	session.SetInboundSetupTimings(inboundCall.setupTimings)

	inboundCall.session = session
	return nil
}

func (inboundCall *inboundCall) createDialog() error {
	dialogSession, err := inboundCall.server.dialogServerCache.ReadInvite(inboundCall.request, inboundCall.transaction)
	if err != nil {
		return fmt.Errorf("failed to create inbound dialog session: %w", err)
	}
	inboundCall.dialogSession = dialogSession
	inboundCall.session.SetDialogServerSession(dialogSession)
	return nil
}

func (inboundCall *inboundCall) createAnswerController() {
	inboundCall.answerController = newInboundAnswerController(
		inboundCall.server,
		inboundCall.session,
		inboundCall.request,
		inboundCall.transaction,
		inboundCall.dialogSession,
		inboundCall.answerPolicy,
		inboundCall.identity.callID,
	)
}

func (inboundCall *inboundCall) setupRTP() error {
	server := inboundCall.server
	negotiatedCodec := inboundCall.mediaOffer.negotiatedCodec

	rtpPort, err := server.rtpAllocator.Allocate()
	if err != nil {
		return fmt.Errorf("no RTP ports available: %w", err)
	}
	inboundCall.allocatedRTPPort = rtpPort

	rtpHandlerFactory := server.newRTPHandler
	if rtpHandlerFactory == nil {
		rtpHandlerFactory = NewRTPHandler
	}

	rtpHandler, err := rtpHandlerFactory(server.ctx, &RTPConfig{
		LocalIP:     server.listenConfig.GetBindAddress(),
		LocalPort:   rtpPort,
		PayloadType: negotiatedCodec.PayloadType,
		ClockRate:   negotiatedCodec.ClockRate,
		Logger:      server.logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create RTP handler: %w", err)
	}

	rtpHandler.SetRemoteAddr(inboundCall.mediaOffer.sdpInfo.ConnectionIP, inboundCall.mediaOffer.sdpInfo.AudioPort)
	rtpHandler.SetCodec(negotiatedCodec)
	rtpHandler.SetOnFirstPacket(func() {
		if inboundCall.session != nil && inboundCall.session.MarkInboundFirstRTPReceived() {
			inboundCall.server.logger.Infow("Inbound SIP first RTP received",
				"call_id", inboundCall.identity.callID,
				"reason", LifecycleReasonInboundFirstRTPReceived)
		}
	})

	_, localPort := rtpHandler.LocalAddr()
	externalIP := server.listenConfig.GetExternalIP()
	inboundCall.rtpHandler = rtpHandler
	inboundCall.localRTPPort = localPort
	inboundCall.externalIP = externalIP

	inboundCall.session.SetRemoteRTP(inboundCall.mediaOffer.sdpInfo.ConnectionIP, inboundCall.mediaOffer.sdpInfo.AudioPort)
	inboundCall.session.SetLocalRTP(externalIP, localPort)
	inboundCall.session.SetNegotiatedCodec(negotiatedCodec.Name, int(negotiatedCodec.ClockRate))
	inboundCall.session.SetRTPHandler(rtpHandler)
	inboundCall.recordPhase(InboundSetupPhaseMediaAllocated, LifecycleReasonInboundMediaAllocated)
	return nil
}

func (inboundCall *inboundCall) prepareApplication() error {
	inboundCall.server.mu.RLock()
	onApplicationReady := inboundCall.server.onApplicationReady
	inboundCall.server.mu.RUnlock()
	if onApplicationReady == nil {
		inboundCall.recordPhase(InboundSetupPhaseApplicationReady, LifecycleReasonInboundApplicationReady)
		return nil
	}
	if err := onApplicationReady(inboundCall.session, inboundCall.identity.fromURI, inboundCall.identity.toURI); err != nil {
		return fmt.Errorf("application readiness failed: %w", err)
	}
	inboundCall.recordPhase(InboundSetupPhaseApplicationReady, LifecycleReasonInboundApplicationReady)
	return nil
}

func (inboundCall *inboundCall) startRTP() error {
	if inboundCall.rtpHandler == nil {
		return ErrRTPNotInitialized
	}
	if inboundCall.rtpStarted {
		return nil
	}
	inboundCall.rtpHandler.Start()
	inboundCall.rtpStarted = true
	inboundCall.recordPhase(InboundSetupPhaseMediaFlowing, LifecycleReasonInboundMediaFlowing)
	return nil
}

func (inboundCall *inboundCall) answer() error {
	sdpConfig := inboundCall.server.NegotiatedSDPConfig(
		inboundCall.externalIP,
		inboundCall.localRTPPort,
		inboundCall.mediaOffer.negotiatedCodec,
	)
	sdpBody := inboundCall.server.GenerateSDP(sdpConfig)

	if err := inboundCall.answerController.AnswerAndWaitACK(inboundCall.server.ctx, sdpBody); err != nil {
		return err
	}

	inboundCall.server.logger.Infow("SIP call answered",
		"call_id", inboundCall.identity.callID,
		"local_rtp", fmt.Sprintf("%s:%d", inboundCall.externalIP, inboundCall.localRTPPort),
		"remote_rtp", fmt.Sprintf("%s:%d", inboundCall.mediaOffer.sdpInfo.ConnectionIP, inboundCall.mediaOffer.sdpInfo.AudioPort),
		"codec", inboundCall.mediaOffer.negotiatedCodec.Name)
	return nil
}

func (inboundCall *inboundCall) dispatchInvite() {
	inboundCall.server.mu.RLock()
	onInvite := inboundCall.server.onInvite
	inboundCall.server.mu.RUnlock()
	if onInvite == nil {
		return
	}
	if err := onInvite(inboundCall.session, inboundCall.identity.fromURI, inboundCall.identity.toURI); err != nil {
		inboundCall.server.logger.Errorw("INVITE handler failed",
			"error", err,
			"call_id", inboundCall.identity.callID)
		inboundCall.server.notifyError(inboundCall.session, err)
		_ = inboundCall.server.FailInboundCall(inboundCall.session, LifecycleReasonPipelineSetupFailed, err)
	}
}

func (inboundCall *inboundCall) recordPhase(phase InboundSetupPhase, reason LifecycleReason) {
	inboundCall.setupPhase = phase
	timestamp := time.Now()
	inboundCall.markSetupTimestamp(phase, timestamp)
	if inboundCall.session != nil {
		inboundCall.session.SetInboundSetupPhase(phase)
		inboundCall.session.MarkInboundSetupTimestamp(phase, timestamp)
	}
	inboundCall.server.logger.Infow("Inbound SIP setup phase",
		"call_id", inboundCall.identity.callID,
		"phase", phase,
		"reason", reason)
}

func (inboundCall *inboundCall) markSetupTimestamp(phase InboundSetupPhase, at time.Time) {
	switch phase {
	case InboundSetupPhaseInviteReceived:
		inboundCall.setupTimings.InviteReceivedAt = at
	case InboundSetupPhaseTryingSent:
		inboundCall.setupTimings.TryingSentAt = at
	case InboundSetupPhaseRingingSent:
		inboundCall.setupTimings.RingingSentAt = at
	case InboundSetupPhaseAnswered:
		inboundCall.setupTimings.AnsweredAt = at
	case InboundSetupPhaseACKConfirmed:
		inboundCall.setupTimings.ACKConfirmedAt = at
	}
}

func (inboundCall *inboundCall) cancelIfRequested(reason LifecycleReason) bool {
	if !inboundCall.server.isInviteCancelled(inboundCall.identity.callID) {
		return false
	}
	if inboundCall.answerController != nil {
		inboundCall.answerController.CancelBeforeAnswer(reason)
	} else {
		inboundCall.server.terminatePendingInvite(inboundCall.identity.callID, 487)
	}
	if inboundCall.session != nil {
		inboundCall.cleanupApplication()
		_ = inboundCall.server.CancelInboundCall(inboundCall.session, reason)
	}
	return true
}

func (inboundCall *inboundCall) failSetup(statusCode int, failureClass internal_inbound.FailureClass, reason LifecycleReason, err error) {
	callID := inboundCall.identity.callID
	inboundCall.server.logger.Errorw("Inbound INVITE setup failed",
		"call_id", callID,
		"from_uri", inboundCall.identity.fromURI,
		"to_uri", inboundCall.identity.toURI,
		"status_code", statusCode,
		"failure_class", string(failureClass),
		"reason", reason,
		"error", err)

	if inboundCall.rtpHandler != nil {
		inboundCall.rtpHandler.Stop()
	}
	if inboundCall.session == nil {
		inboundCall.releaseUnownedRTPPort()
		inboundCall.server.RejectInboundInvite(
			inboundCall.request,
			inboundCall.transaction,
			callID,
			statusCode,
			failureClass,
			reason,
			err,
		)
		return
	}

	inboundCall.releaseUnownedRTPPort()
	if inboundCall.answerController == nil {
		inboundCall.createAnswerController()
	}
	if !inboundCall.answerController.FinalResponseStarted() {
		inboundCall.answerController.FailBeforeAnswer(statusCode, failureClass, reason, err)
	}
	inboundCall.cleanupApplication()
	_ = inboundCall.server.FailInboundCall(inboundCall.session, reason, err)
}

func (inboundCall *inboundCall) cleanupApplication() {
	inboundCall.server.mu.RLock()
	onApplicationCleanup := inboundCall.server.onApplicationCleanup
	inboundCall.server.mu.RUnlock()
	if onApplicationCleanup != nil && inboundCall.session != nil {
		onApplicationCleanup(inboundCall.session)
	}
}

func (inboundCall *inboundCall) failSetupError(statusCode int, failureClass internal_inbound.FailureClass, reason LifecycleReason, err error) {
	statusCode, failureClass, reason, err = inboundSetupFailureDetails(err, statusCode, failureClass, reason)
	inboundCall.failSetup(statusCode, failureClass, reason, err)
}

func (inboundCall *inboundCall) sendFinalResponse(statusCode int) {
	inboundCall.server.sendResponse(inboundCall.transaction, inboundCall.request, statusCode)
}

func (inboundCall *inboundCall) releaseUnownedRTPPort() {
	if inboundCall.allocatedRTPPort <= 0 {
		return
	}
	if inboundCall.session != nil {
		_, localPort := inboundCall.session.GetLocalRTP()
		if localPort == inboundCall.allocatedRTPPort {
			inboundCall.allocatedRTPPort = 0
			return
		}
	}
	inboundCall.server.rtpAllocator.Release(inboundCall.allocatedRTPPort)
	inboundCall.allocatedRTPPort = 0
}

func (inboundCall *inboundCall) finish() {
	if inboundCall.identity.callID == "" {
		return
	}
	inboundCall.server.clearPendingInvite(inboundCall.identity.callID)
	inboundCall.server.clearInviteCancelled(inboundCall.identity.callID)
}
