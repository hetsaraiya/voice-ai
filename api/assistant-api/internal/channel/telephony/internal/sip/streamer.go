// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_sip_telephony

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	internal_telephony_base "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/base"
	internal_sip "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/sip/internal"
	"github.com/rapidaai/api/assistant-api/internal/observe"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/protos"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Streamer struct {
	internal_telephony_base.BaseTelephonyStreamer

	mu     sync.RWMutex
	closed atomic.Bool

	session   *sip_infra.Session
	lifecycle sip_infra.LifecycleController
	mediaPort *internal_sip.MediaPort

	onTransferInitiated func(targets []string, postTransferAction string)
}

func NewStreamer(ctx context.Context,
	logger commons.Logger,
	sipSession *sip_infra.Session,
	lifecycle sip_infra.LifecycleController,
	cc *callcontext.CallContext,
	vaultCred *protos.VaultCredential,
) (internal_type.Streamer, error) {
	if sipSession == nil {
		return nil, fmt.Errorf("SIP session is required; standalone server mode is not supported")
	}
	if lifecycle == nil {
		return nil, fmt.Errorf("SIP lifecycle controller is required")
	}

	s := &Streamer{
		BaseTelephonyStreamer: internal_telephony_base.NewBaseTelephonyStreamer(
			logger, cc, vaultCred,
		),
	}

	// Peer BYE is reported to Talk; MediaPort owns bridge teardown safety.
	go func() {
		select {
		case <-sipSession.ByeReceived():
			s.Logger.Infow("SIP streamer: user BYE received")
			s.emitChannelEvent("disconnected", map[string]string{"reason": "bye_received"})
			if msg := s.Disconnect(protos.ConversationDisconnection_DISCONNECTION_TYPE_USER); msg != nil {
				s.Input(msg)
			}
		case <-s.Ctx.Done():
		}
	}()

	// Context cancellation is a safety net when Talk cannot drive teardown.
	go func() {
		select {
		case <-sipSession.Context().Done():
			s.Logger.Infow("SIP streamer: session context cancelled, closing")
		case <-ctx.Done():
			s.Logger.Infow("SIP streamer: caller context cancelled, closing")
		case <-s.Ctx.Done():
			return
		}
		if msg := s.Disconnect(protos.ConversationDisconnection_DISCONNECTION_TYPE_USER); msg != nil {
			s.Input(msg)
		}
		s.Close()
	}()

	s.session = sipSession
	s.lifecycle = lifecycle
	mediaPort, err := internal_sip.NewMediaPort(internal_sip.MediaPortConfig{
		Context:    s.Ctx,
		Logger:     logger,
		Session:    sipSession,
		Resampler:  s.Resampler(),
		StreamSink: s.Input,
	})
	if err != nil {
		return nil, err
	}
	s.mediaPort = mediaPort
	s.mediaPort.Start()
	s.Input(s.CreateConnectionRequest())
	s.emitChannelEvent("connected", nil)
	s.emitChannelEvent("media_started", nil)

	localIP, localPort := mediaPort.LocalAddr()
	logger.Infow("SIP streamer created",
		"call_id", sipSession.GetCallID(),
		"codec", mediaPort.CodecName(),
		"rtp_port", localPort,
		"local_ip", localIP)

	return s, nil
}

func (s *Streamer) Context() context.Context {
	return s.Ctx
}

func (s *Streamer) Send(response internal_type.Stream) error {
	if s.closed.Load() {
		return sip_infra.ErrSessionClosed
	}
	switch data := response.(type) {
	case *protos.ConversationInitialization:
		if s.mediaPort != nil {
			s.mediaPort.HandleInitialization(data)
		}
	case *protos.ConversationAssistantMessage:
		switch content := data.Message.(type) {
		case *protos.ConversationAssistantMessage_Audio:
			if s.mediaPort == nil {
				return nil
			}
			return s.mediaPort.HandleAssistantAudio(content.Audio, data.GetCompleted())
		}
	case *protos.ConversationInterruption:
		if data.Type == protos.ConversationInterruption_INTERRUPTION_TYPE_WORD {
			if s.mediaPort != nil {
				s.mediaPort.HandleInterrupt()
			}
		}
	case *protos.ConversationDisconnection:
		_ = s.Disconnect(data.GetType())
		s.emitChannelEvent("disconnected", map[string]string{"reason": data.GetType().String()})
		s.endSession()
		s.Close()
	case *protos.ConversationToolCall:
		switch data.GetAction() {
		case protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION:
			s.PushToolCallResult(data.GetId(), data.GetToolId(), data.GetName(), data.GetAction(), map[string]string{
				"status": "completed",
			})
		case protos.ToolCallAction_TOOL_CALL_ACTION_TRANSFER_CONVERSATION:
			raw := data.GetArgs()["transfer_to"]
			if raw == "" {
				s.Logger.Warnw("Transfer tool call missing 'transfer_to' target")
				s.PushToolCallResult(data.GetId(), data.GetToolId(), data.GetName(), data.GetAction(), map[string]string{
					"status": "failed", "reason": "missing transfer target",
				})
				return nil
			}
			targets := s.SplitTransferTargets(raw)
			postTransferAction := data.GetArgs()["post_transfer_action"]
			ringtone := data.GetArgs()["ringtone"]
			s.mu.RLock()
			if s.session != nil {
				s.session.SetMetadata(sip_infra.MetadataBridgeTransferTarget, strings.Join(targets, commons.SEPARATOR))
				s.session.SetMetadata("tool_id", data.GetToolId())
				s.session.SetMetadata("tool_context_id", data.GetId())
			}
			s.mu.RUnlock()
			s.EnterTransferMode(targets, postTransferAction, ringtone)
			return nil
		}
	}
	return nil
}

func (s *Streamer) EnterTransferMode(targets []string, postTransferAction, ringtoneEnum string) {
	if s.mediaPort != nil && !s.mediaPort.EnterTransferMode(ringtoneEnum) {
		return
	}

	s.mu.RLock()
	session := s.session
	callback := s.onTransferInitiated
	s.mu.RUnlock()

	if session != nil {
		s.transitionCall(session, sip_infra.CallStateTransferring, sip_infra.LifecycleReasonTransferModeStarted)
	}

	if callback != nil {
		callback(targets, postTransferAction)
	}
}

func (s *Streamer) ExitTransferMode() {
	if s.mediaPort != nil && !s.mediaPort.ExitTransferMode() {
		return
	}

	s.mu.RLock()
	session := s.session
	s.mu.RUnlock()

	if session != nil {
		s.transitionCall(session, sip_infra.CallStateConnected, sip_infra.LifecycleReasonTransferModeEnded)
	}

	s.Logger.Infow("Transfer mode: exited, AI resuming")
}

func (s *Streamer) StopRingback() {
	if s.mediaPort != nil {
		s.mediaPort.StopRingback()
	}
}

func (s *Streamer) SetBridgeOutRTP(rtp *sip_infra.RTPHandler) {
	if s.mediaPort != nil {
		s.mediaPort.SetBridgeTarget(rtp)
	}
}

func (s *Streamer) ClearBridgeTarget() {
	if s.mediaPort != nil {
		s.mediaPort.ClearBridgeTarget()
	}
}

func (s *Streamer) PushBridgeOperatorAudio(audio []byte) {
	if s.mediaPort != nil {
		s.mediaPort.PushBridgeOperatorAudio(audio)
	}
}

func (s *Streamer) PushToolCallResult(contextID, toolID, toolName string, action protos.ToolCallAction, result map[string]string) {
	s.Input(&protos.ConversationToolCallResult{
		Id:     contextID,
		ToolId: toolID,
		Name:   toolName,
		Action: action,
		Result: result,
	})
}

func (s *Streamer) SetOnTransferInitiated(fn func(targets []string, postTransferAction string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onTransferInitiated = fn
}

func (s *Streamer) endSession() {
	s.mu.RLock()
	session := s.session
	s.mu.RUnlock()
	if session != nil {
		s.endCall(session, sip_infra.LifecycleReasonStreamerEndSession)
	}
}

func (s *Streamer) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	s.emitChannelEvent("media_stopped", nil)

	if s.mediaPort != nil {
		if err := s.mediaPort.Close(); err != nil {
			s.Logger.Warnw("SIP media port close failed", "error", err)
		}
	}
	s.BaseStreamer.Cancel()

	s.mu.RLock()
	session := s.session
	s.mu.RUnlock()

	if session != nil {
		s.endCall(session, sip_infra.LifecycleReasonStreamerClosed)
	}

	s.Logger.Infow("SIP streamer closed")
	return nil
}

func (s *Streamer) transitionCall(session *sip_infra.Session, next sip_infra.CallState, reason sip_infra.LifecycleReason) {
	if s.lifecycle == nil {
		s.Logger.Warnw("SIP lifecycle transition skipped: controller unavailable",
			"call_id", session.GetCallID(),
			"to", next,
			"reason", reason)
		return
	}
	s.lifecycle.TransitionCall(session, next, reason)
}

func (s *Streamer) endCall(session *sip_infra.Session, reason sip_infra.LifecycleReason) {
	if s.lifecycle == nil {
		s.Logger.Warnw("SIP lifecycle end skipped: controller unavailable",
			"call_id", session.GetCallID(),
			"reason", reason)
		return
	}
	if err := s.lifecycle.EndCallWithReason(session, reason); err != nil {
		s.Logger.Warnw("SIP lifecycle end failed",
			"call_id", session.GetCallID(),
			"reason", reason,
			"error", err)
	}
}

func (s *Streamer) emitChannelEvent(eventType string, extra map[string]string) {
	data := map[string]string{
		"type":     eventType,
		"provider": internal_sip.Provider,
	}
	for k, v := range extra {
		data[k] = v
	}
	s.Input(&protos.ConversationEvent{
		Name: observe.ComponentTelephony,
		Data: data,
		Time: timestamppb.Now(),
	})
}
