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
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	internal_outbound "github.com/rapidaai/api/assistant-api/sip/internal/outbound"
)

// MakeCall initiates an outbound SIP call and registers the dialog for routing.
func (s *Server) MakeCall(ctx context.Context, cfg *Config, toUser, fromUser string, opts MakeCallOptions) (*Session, error) {
	outboundCall, err := s.prepareOutboundCallLeg(ctx, cfg, toUser, fromUser, outboundCallLegOptions{
		purpose:         OutboundLegPurposePrimary,
		makeCallOptions: opts,
	})
	if err != nil {
		return nil, err
	}

	outboundCall.reportStatus(internal_type.ProviderCallStatusUpdate{CallStatus: string(OutboundCallStatusInitiated)})
	outboundCall.start()

	return outboundCall.session, nil
}

type outboundCallLegOptions struct {
	purpose         OutboundLegPurpose
	makeCallOptions MakeCallOptions
	parentCallID    string
	parentContextID string
	parentConvID    uint64
	transferTarget  string
	transferAttempt int
	transferTotal   int
}

func (s *Server) prepareOutboundCallLeg(ctx context.Context, cfg *Config, toUser, fromUser string, opts outboundCallLegOptions) (*outboundCall, error) {
	if s.state.Load() != int32(ServerStateRunning) {
		return nil, fmt.Errorf("SIP server is not running")
	}
	setupContext := ctx
	// Primary outbound calls outlive the API request that dispatches them.
	callLifecycleContext := context.WithoutCancel(ctx)

	request, err := NewOutboundInviteRequest(cfg, toUser, fromUser)
	if err != nil {
		return nil, err
	}

	session, err := s.createAndRegisterOutboundSession(callLifecycleContext, cfg, "", opts.makeCallOptions)
	if err != nil {
		return nil, err
	}
	s.applyOutboundLegMetadata(session, opts)

	invite, err := s.sendOutboundInvite(setupContext, session, request)
	if err != nil {
		reportOutboundSetupFailure(opts.makeCallOptions.CallStatusObserver, session.GetCallID(), err)
		_ = s.FailCall(session, LifecycleReasonOutboundSetupFailure, err)
		return nil, err
	}

	attachOutboundInviteToSession(session, invite)
	outboundCall := newOutboundCall(s, session, invite, request)
	outboundCall.statusObserver = opts.makeCallOptions.CallStatusObserver
	return outboundCall, nil
}

func (s *Server) applyOutboundLegMetadata(session *Session, opts outboundCallLegOptions) {
	if session == nil {
		return
	}
	purpose := opts.purpose
	if purpose == "" {
		purpose = OutboundLegPurposePrimary
	}
	session.SetMetadata(MetadataOutboundLegPurpose, string(purpose))
	if opts.parentCallID != "" {
		session.SetMetadata(MetadataOutboundParentCallID, opts.parentCallID)
	}
	if opts.parentContextID != "" {
		session.SetMetadata(MetadataOutboundParentContextID, opts.parentContextID)
	}
	if opts.parentConvID > 0 {
		session.SetMetadata(MetadataOutboundParentConversationID, opts.parentConvID)
	}
	if opts.transferTarget != "" {
		session.SetMetadata(MetadataOutboundTransferTarget, opts.transferTarget)
	}
	if opts.transferAttempt > 0 {
		session.SetMetadata(MetadataOutboundTransferAttempt, opts.transferAttempt)
	}
	if opts.transferTotal > 0 {
		session.SetMetadata(MetadataOutboundTransferTotal, opts.transferTotal)
	}
}

func (s *Server) createAndRegisterOutboundSession(ctx context.Context, cfg *Config, callID string, opts MakeCallOptions) (*Session, error) {
	session, err := NewSession(ctx, &SessionConfig{
		Config:          cfg,
		Direction:       CallDirectionOutbound,
		CallID:          callID,
		Codec:           &CodecPCMU,
		Logger:          s.logger,
		Auth:            opts.Auth,
		Assistant:       opts.Assistant,
		ConversationID:  opts.ConversationID,
		ContextID:       opts.ContextID,
		VaultCredential: opts.VaultCredential,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create outbound session: %w", err)
	}
	s.registerSession(session, session.GetCallID())
	return session, nil
}

func attachOutboundInviteToSession(session *Session, invite *outboundInvite) {
	session.SetLocalRTP(invite.externalIP, invite.localPort)
	session.SetRTPHandler(invite.rtpHandler)
	session.SetDialogClientSession(invite.dialogSession)
}

// outboundInvite holds the result of sendOutboundInvite with allocated
// resources and dialog needed to complete or clean up an outbound call.
type outboundInvite struct {
	rtpHandler    *RTPHandler
	rtpPort       int
	localPort     int
	externalIP    string
	callID        string
	dialogSession *sipgo.DialogClientSession

	server *Server // back-reference for cleanup
}

// cleanup releases all resources allocated during sendOutboundInvite.
// Safe to call on error paths before the session takes ownership.
func (o *outboundInvite) cleanup() {
	if o.rtpHandler != nil {
		o.rtpHandler.Stop()
	}
	if o.server != nil && o.rtpPort > 0 {
		o.server.rtpAllocator.Release(o.rtpPort)
	}
	if o.dialogSession != nil {
		time.AfterFunc(2*time.Second, func() { o.dialogSession.Close() })
	}
}

func reportOutboundSetupFailure(statusObserver internal_type.ProviderCallStatusReporter, callID string, err error) {
	if statusObserver == nil {
		return
	}
	errorMessage := "outbound setup failed"
	if err != nil {
		errorMessage = err.Error()
	}
	statusObserver(internal_type.ProviderCallStatusUpdate{
		ChannelUUID:      callID,
		CallStatus:       string(OutboundCallStatusFailed),
		ErrorMessage:     errorMessage,
		FailureClass:     "setup",
		FailureReason:    "outbound setup failed",
		DisconnectReason: LifecycleReasonOutboundSetupFailure.String(),
	})
}

// sendOutboundInvite allocates RTP, builds INVITE headers, and sends INVITE.
func (s *Server) sendOutboundInvite(ctx context.Context, session *Session, request OutboundInviteRequest) (*outboundInvite, error) {
	if session == nil {
		return nil, fmt.Errorf("outbound session is nil")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}

	rtpPort, err := s.rtpAllocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("no RTP ports available: %w", err)
	}

	rtpHandlerFactory := s.newRTPHandler
	if rtpHandlerFactory == nil {
		rtpHandlerFactory = NewRTPHandler
	}

	rtpBindIP := s.listenConfig.GetBindAddress()
	rtpHandler, err := rtpHandlerFactory(context.Background(), &RTPConfig{
		LocalIP:     rtpBindIP,
		LocalPort:   rtpPort,
		PayloadType: CodecPCMU.PayloadType,
		ClockRate:   CodecPCMU.ClockRate,
		Logger:      s.logger,
	})
	if err != nil {
		s.rtpAllocator.Release(rtpPort)
		return nil, fmt.Errorf("failed to create RTP handler: %w", err)
	}

	_, localPort := rtpHandler.LocalAddr()
	externalIP := s.listenConfig.GetExternalIP()
	sdpBody := s.GenerateSDP(DefaultSDPConfig(externalIP, localPort))

	recipient := sip.Uri{
		Scheme: internal_outbound.SIPScheme(internal_outbound.Transport(request.Config.Transport)),
		Host:   request.Config.Address,
		Port:   request.Config.Port,
		User:   request.Identity.ToUser,
	}
	if request.Config.Transport == TransportTLS || request.Config.Transport == TransportTCP {
		if recipient.UriParams == nil {
			recipient.UriParams = sip.NewParams()
		}
		recipient.UriParams.Add("transport", string(request.Config.Transport))
	}

	inviteHeaders, err := internal_outbound.BuildInviteHeaders(outboundInviteRequestToSignaling(request))
	if err != nil {
		rtpHandler.Stop()
		s.rtpAllocator.Release(rtpPort)
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	callIDHeader := sip.CallIDHeader(session.GetCallID())
	inviteHeaders = append(inviteHeaders, &callIDHeader)

	dialogSession, err := s.dialogClientCache.Invite(ctx, recipient, []byte(sdpBody), inviteHeaders...)
	if err != nil {
		rtpHandler.Stop()
		s.rtpAllocator.Release(rtpPort)
		return nil, fmt.Errorf("failed to send INVITE: %w", err)
	}

	return &outboundInvite{
		rtpHandler:    rtpHandler,
		rtpPort:       rtpPort,
		localPort:     localPort,
		externalIP:    externalIP,
		callID:        session.GetCallID(),
		dialogSession: dialogSession,
		server:        s,
	}, nil
}

func outboundInviteRequestToSignaling(request OutboundInviteRequest) internal_outbound.InviteRequest {
	return internal_outbound.InviteRequest{
		Config: internal_outbound.Config{
			Address:   request.Config.Address,
			Port:      request.Config.Port,
			Transport: internal_outbound.Transport(request.Config.Transport),
			Domain:    request.Config.Domain,
			Headers:   request.Config.Headers,
		},
		Identity: internal_outbound.Identity{
			ToUser:   request.Identity.ToUser,
			FromUser: request.Identity.FromUser,
		},
	}
}

type answeredCall struct {
	rtpHandler      *RTPHandler
	negotiatedCodec *Codec
	remoteIP        string
	remotePort      int
}

func (s *Server) acceptOutboundAnswer(invite *outboundInvite) (*answeredCall, error) {
	if invite.dialogSession == nil || invite.dialogSession.InviteResponse == nil {
		return nil, fmt.Errorf("%w: outbound 200 OK response is missing", ErrSDPParseFailed)
	}

	body := invite.dialogSession.InviteResponse.Body()
	if len(body) == 0 {
		return nil, fmt.Errorf("%w: outbound 200 OK SDP body is missing", ErrSDPParseFailed)
	}

	if s.logger != nil {
		s.logger.Debugw("Outbound call 200 OK SDP answer",
			"call_id", invite.callID,
			"sdp_body", string(body))
	}

	sdpInfo, err := s.ParseSDP(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSDPParseFailed, err)
	}
	if sdpInfo.ConnectionIP == "" || sdpInfo.AudioPort <= 0 {
		return nil, fmt.Errorf("%w: outbound answer missing RTP address", ErrSDPParseFailed)
	}

	negotiatedCodec := firstSupportedAudioCodec(sdpInfo.PayloadTypes)
	if negotiatedCodec == nil {
		return nil, fmt.Errorf("%w: outbound answer payload types %v", ErrCodecNotSupported, sdpInfo.PayloadTypes)
	}

	invite.rtpHandler.SetRemoteAddr(sdpInfo.ConnectionIP, sdpInfo.AudioPort)
	invite.rtpHandler.SetCodec(negotiatedCodec)

	return &answeredCall{
		rtpHandler:      invite.rtpHandler,
		negotiatedCodec: negotiatedCodec,
		remoteIP:        sdpInfo.ConnectionIP,
		remotePort:      sdpInfo.AudioPort,
	}, nil
}

func firstSupportedAudioCodec(payloadTypes []uint8) *Codec {
	for _, payloadType := range payloadTypes {
		if payloadType == CodecTelephoneEvent.PayloadType {
			continue
		}
		for _, codec := range SupportedCodecs {
			if codec.PayloadType == payloadType {
				supportedCodec := codec
				return &supportedCodec
			}
		}
	}
	return nil
}
