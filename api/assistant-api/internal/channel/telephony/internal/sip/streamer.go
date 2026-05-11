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

	internal_audio "github.com/rapidaai/api/assistant-api/internal/audio"
	internal_ambient "github.com/rapidaai/api/assistant-api/internal/audio/ambient"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	channel_base "github.com/rapidaai/api/assistant-api/internal/channel/base"
	internal_telephony_base "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/base"
	internal_telephony_media "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/media"
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

	session    *sip_infra.Session
	rtpHandler *sip_infra.RTPHandler
	audio      *AudioProcessor
	media      *internal_telephony_media.MediaSession

	transferring        atomic.Bool
	ringbackCancel      context.CancelFunc
	onTransferInitiated func(targets []string, postTransferAction string)
}

func NewStreamer(ctx context.Context,
	logger commons.Logger,
	sipSession *sip_infra.Session,
	cc *callcontext.CallContext,
	vaultCred *protos.VaultCredential,
) (internal_type.Streamer, error) {
	if sipSession == nil {
		return nil, fmt.Errorf("SIP session is required — standalone server mode is not supported")
	}

	s := &Streamer{
		BaseTelephonyStreamer: internal_telephony_base.NewBaseTelephonyStreamer(
			logger, cc, vaultCred,
			internal_telephony_base.WithSourceAudioConfig(internal_audio.NewMulaw8khzMonoAudioConfig()),
			internal_telephony_base.WithBaseOption(channel_base.WithInputAudioConfig(internal_audio.NewLinear16khzMonoAudioConfig())),
		),
	}

	// Client disconnect detection (SIP BYE from peer): push disc to Input,
	// let the talker drive Close via Notify → Send.
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

	// Server watches client context: safety net that forces Close if the
	// talker cannot drive it (mirrors watchCallerContext in the webrtc streamer).
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

	rtpHandler := sipSession.GetRTPHandler()
	if rtpHandler == nil {
		return nil, sip_infra.NewSIPError("NewStreamer", sipSession.GetCallID(), "session has no RTP handler", sip_infra.ErrRTPNotInitialized)
	}

	s.session = sipSession
	s.rtpHandler = rtpHandler
	s.audio = NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtpHandler,
		Resampler:  s.Resampler(),
		PushInput:  s.Input,
		Ringtone:   "ringtone_us",
		Ambient:    resolveAmbientConfig(sipSession),
	})
	s.media = internal_telephony_media.NewMediaSession(s.Ctx, logger, s.audio, nil)
	s.media.SetInputSink(func(audio []byte) {
		s.Input(&protos.ConversationUserMessage{
			Message: &protos.ConversationUserMessage_Audio{Audio: audio},
		})
	})
	s.media.SetEventSink(func(event *protos.ConversationEvent) {
		s.Input(event)
	})

	go s.forwardIncomingAudio()
	s.media.Start()
	go s.audio.RunBridgeRecorder(s.Ctx)
	s.Input(s.CreateConnectionRequest())
	s.emitChannelEvent("connected", nil)
	s.emitChannelEvent("media_started", nil)

	localIP, localPort := rtpHandler.LocalAddr()
	codecName := "PCMU"
	if negotiated := sipSession.GetNegotiatedCodec(); negotiated != nil {
		codecName = negotiated.Name
	}
	logger.Infow("SIP streamer created",
		"call_id", sipSession.GetCallID(),
		"codec", codecName,
		"rtp_port", localPort,
		"local_ip", localIP)

	return s, nil
}

func resolveAmbientConfig(sipSession *sip_infra.Session) *internal_ambient.Config {
	if sipSession == nil {
		return nil
	}
	assistant := sipSession.GetAssistant()
	if assistant == nil || assistant.AssistantPhoneDeployment == nil || assistant.AssistantPhoneDeployment.OutputAudio == nil {
		return nil
	}
	opts := assistant.AssistantPhoneDeployment.OutputAudio.GetOptions()
	cfg, ok := internal_ambient.ParseFromOptions(opts)
	if !ok {
		return nil
	}
	return &cfg
}

func (s *Streamer) forwardIncomingAudio() {
	s.mu.RLock()
	rtpHandler := s.rtpHandler
	s.mu.RUnlock()
	if rtpHandler == nil {
		return
	}
	for {
		select {
		case <-s.Ctx.Done():
			return
		case audioData, ok := <-rtpHandler.AudioIn():
			if !ok {
				return
			}
			if s.audio.ForwardUserAudio(audioData) {
				continue
			}
			// During transfer (ringback playing, bridge not yet set, or teardown race),
			// discard audio instead of sending to the AI pipeline.
			if s.transferring.Load() {
				continue
			}
			if s.media != nil {
				if err := s.media.HandleProviderAudio(audioData); err != nil {
					s.Logger.Debugw("SIP provider audio processing failed", "error", err.Error())
				}
			}
		}
	}
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
		if s.media != nil {
			s.media.HandleInitialization(data)
		}
	case *protos.ConversationAssistantMessage:
		switch content := data.Message.(type) {
		case *protos.ConversationAssistantMessage_Audio:
			if s.media == nil {
				return nil
			}
			return s.media.HandleAssistantAudio(content.Audio, data.GetCompleted())
		}
	case *protos.ConversationInterruption:
		if data.Type == protos.ConversationInterruption_INTERRUPTION_TYPE_WORD {
			if s.media != nil {
				s.media.HandleInterrupt()
			}
		}
	case *protos.ConversationDisconnection:
		s.emitChannelEvent("disconnected", map[string]string{"reason": data.GetType().String()})
		s.endSession() // notify SIP peer via BYE (equivalent of Output(data) in webrtc)
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
	default:
		// s.Logger.Warnw("SIP Send: unknown message type, skipping", "type", fmt.Sprintf("%T", response))
	}
	return nil
}

// =============================================================================
// Transfer
// =============================================================================

func (s *Streamer) EnterTransferMode(targets []string, postTransferAction, ringtoneEnum string) {
	if !s.transferring.CompareAndSwap(false, true) {
		return
	}

	s.mu.RLock()
	session := s.session
	callback := s.onTransferInitiated
	s.mu.RUnlock()

	if session != nil {
		session.SetState(sip_infra.CallStateTransferring)
	}

	ringbackCtx, ringbackCancel := context.WithCancel(s.Ctx)
	s.mu.Lock()
	s.ringbackCancel = ringbackCancel
	audio := s.audio
	s.mu.Unlock()
	go func() {
		if audio != nil {
			audio.SetTransferActive(true)
			audio.ClearOutputBuffer()
			audio.SetRingtone(ringtoneEnum)
			audio.PlayRingback(ringbackCtx)
		}
	}()

	if callback != nil {
		callback(targets, postTransferAction)
	}
}

func (s *Streamer) ExitTransferMode() {
	if !s.transferring.Load() {
		return
	}

	s.mu.RLock()
	cancelFn := s.ringbackCancel
	session := s.session
	s.mu.RUnlock()

	if cancelFn != nil {
		cancelFn()
	}
	if session != nil {
		session.SetState(sip_infra.CallStateConnected)
	}

	s.audio.SetTransferActive(false)
	s.audio.ClearBridgeTarget()
	s.transferring.Store(false)
	s.Logger.Infow("Transfer mode: exited, AI resuming")
}

func (s *Streamer) StopRingback() {
	s.mu.RLock()
	cancelFn := s.ringbackCancel
	s.mu.RUnlock()
	if cancelFn != nil {
		cancelFn()
	}
	s.audio.ClearOutputBuffer()
}

func (s *Streamer) SetBridgeOutRTP(rtp *sip_infra.RTPHandler) {
	s.mu.RLock()
	inCodec := s.rtpHandler.GetCodec()
	s.mu.RUnlock()
	var outCodec *sip_infra.Codec
	if rtp != nil {
		outCodec = rtp.GetCodec()
	}
	s.audio.SetBridgeTarget(rtp, inCodec, outCodec)
}

func (s *Streamer) ClearBridgeTarget() {
	s.audio.ClearBridgeTarget()
}

func (s *Streamer) PushBridgeOperatorAudio(audio []byte) {
	s.audio.PushOperatorAudio(audio)
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
		session.End()
	}
}

// =============================================================================
// Lifecycle
// =============================================================================

func (s *Streamer) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	s.emitChannelEvent("media_stopped", nil)

	s.BaseStreamer.Cancel()
	s.ResetInputBuffer()
	if s.media != nil {
		s.media.Shutdown()
	}

	s.mu.RLock()
	session := s.session
	s.mu.RUnlock()

	if session != nil {
		session.End()
	}

	s.Logger.Infow("SIP streamer closed")
	return nil
}

func (s *Streamer) emitChannelEvent(eventType string, extra map[string]string) {
	data := map[string]string{
		"type":     eventType,
		"provider": "sip",
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
