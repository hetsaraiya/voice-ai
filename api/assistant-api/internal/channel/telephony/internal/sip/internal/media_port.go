// Copyright (c) 2023-2026 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_sip

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	internal_ambient "github.com/rapidaai/api/assistant-api/internal/audio/ambient"
	internal_telephony_media "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/media"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/protos"
)

type MediaPortConfig struct {
	Context    context.Context
	Logger     commons.Logger
	Session    *sip_infra.Session
	RTPHandler *sip_infra.RTPHandler
	Resampler  internal_type.AudioResampler
	StreamSink func(internal_type.Stream)
}

type MediaPort struct {
	logger commons.Logger

	session        *sip_infra.Session
	rtpHandler     *sip_infra.RTPHandler
	audioProcessor *AudioProcessor
	mediaSession   *internal_telephony_media.MediaSession
	streamSink     func(internal_type.Stream)

	ctx    context.Context
	cancel context.CancelFunc

	started        atomic.Bool
	closed         atomic.Bool
	transferActive atomic.Bool

	mu             sync.RWMutex
	ringbackCancel context.CancelFunc
}

func NewMediaPort(config MediaPortConfig) (*MediaPort, error) {
	rtpHandler := config.RTPHandler
	if rtpHandler == nil && config.Session != nil {
		rtpHandler = config.Session.GetRTPHandler()
	}
	if rtpHandler == nil {
		callID := ""
		if config.Session != nil {
			callID = config.Session.GetCallID()
		}
		return nil, sip_infra.NewSIPError("NewMediaPort", callID, "session has no RTP handler", sip_infra.ErrRTPNotInitialized)
	}
	if config.Session == nil {
		return nil, sip_infra.NewSIPError("NewMediaPort", "", "session is required", sip_infra.ErrRTPNotInitialized)
	}
	portContext := config.Context
	if portContext == nil {
		portContext = context.Background()
	}
	ctx, cancel := context.WithCancel(portContext)
	mediaPort := &MediaPort{
		logger:     config.Logger,
		session:    config.Session,
		rtpHandler: rtpHandler,
		streamSink: config.StreamSink,
		ctx:        ctx,
		cancel:     cancel,
	}
	mediaPort.audioProcessor = NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtpHandler,
		Resampler:  config.Resampler,
		PushInput:  config.StreamSink,
		Ringtone:   DefaultRingtone,
		Ambient:    resolveAmbientConfig(config.Session),
	})
	mediaPort.mediaSession = internal_telephony_media.NewMediaSession(internal_telephony_media.MediaSessionConfig{
		Context:     ctx,
		Logger:      config.Logger,
		MediaEngine: mediaPort.audioProcessor,
		StreamSink:  config.StreamSink,
		OutputSink: func(frame internal_telephony_media.AssistantOutputFrame) error {
			return mediaPort.audioProcessor.ConsumeFrame(frame.ProviderAudio)
		},
		EventSink: func(event *protos.ConversationEvent) {
			if event == nil {
				return
			}
			if event.Data == nil {
				event.Data = map[string]string{}
			}
			event.Data["provider"] = Provider
			if mediaPort.streamSink != nil {
				mediaPort.streamSink(event)
			}
		},
	})
	return mediaPort, nil
}

func resolveAmbientConfig(sipSession *sip_infra.Session) *internal_ambient.Config {
	if sipSession == nil {
		return nil
	}
	assistant := sipSession.GetAssistant()
	if assistant == nil || assistant.AssistantPhoneDeployment == nil || assistant.AssistantPhoneDeployment.OutputAudio == nil {
		return nil
	}
	options := assistant.AssistantPhoneDeployment.OutputAudio.GetOptions()
	config, ok := internal_ambient.ParseFromOptions(options)
	if !ok {
		return nil
	}
	return &config
}

func (port *MediaPort) Start() {
	if port == nil || port.closed.Load() {
		return
	}
	if !port.started.CompareAndSwap(false, true) {
		return
	}
	go port.forwardIncomingAudio()
	port.mediaSession.Start()
	go port.audioProcessor.RunBridgeRecorder(port.ctx)
}

func (port *MediaPort) Close() error {
	if port == nil {
		return nil
	}
	if !port.closed.CompareAndSwap(false, true) {
		return nil
	}
	port.cancelRingback(false)
	if port.mediaSession != nil {
		port.mediaSession.Shutdown()
	}
	if port.cancel != nil {
		port.cancel()
	}
	return nil
}

func (port *MediaPort) HandleInitialization(init *protos.ConversationInitialization) {
	if port == nil || port.mediaSession == nil {
		return
	}
	port.mediaSession.HandleInitialization(init)
}

func (port *MediaPort) HandleAssistantAudio(audio []byte, completed bool) error {
	if port == nil || port.mediaSession == nil {
		return nil
	}
	if err := port.mediaSession.HandleAssistantAudio(audio, completed); err != nil {
		return err
	}
	if len(audio) > 0 && port.session != nil && port.session.GetInfo().Direction == sip_infra.CallDirectionInbound && port.session.MarkInboundFirstAssistantAudioSent() && port.logger != nil {
		port.logger.Infow("SIP first assistant audio sent", "call_id", port.session.GetCallID())
	}
	return nil
}

func (port *MediaPort) HandleInterrupt() {
	if port == nil || port.mediaSession == nil {
		return
	}
	port.mediaSession.HandleInterrupt()
}

func (port *MediaPort) EnterTransferMode(ringtone string) bool {
	if port == nil {
		return true
	}
	if !port.transferActive.CompareAndSwap(false, true) {
		return false
	}
	ringbackContext, ringbackCancel := context.WithCancel(port.ctx)
	port.mu.Lock()
	port.ringbackCancel = ringbackCancel
	port.mu.Unlock()
	port.audioProcessor.SetTransferActive(true)
	port.audioProcessor.ClearInputBuffer()
	port.audioProcessor.ClearOutputBuffer()
	port.audioProcessor.SetRingtone(ringtone)
	go port.audioProcessor.PlayRingback(ringbackContext)
	return true
}

func (port *MediaPort) ResumeAssistant() bool {
	if port == nil {
		return true
	}
	if !port.transferActive.CompareAndSwap(true, false) {
		return false
	}
	port.cancelRingback(false)
	port.audioProcessor.SetTransferActive(false)
	port.audioProcessor.DisconnectTransferMedia()
	return true
}

func (port *MediaPort) StopTransferRingback() {
	if port == nil {
		return
	}
	port.cancelRingback(true)
}

func (port *MediaPort) ConnectTransferMedia(target internal_type.SIPRTPBridgeTarget, outputCodecName string) {
	if port == nil || port.audioProcessor == nil {
		return
	}
	inCodec := port.rtpHandler.GetCodec()
	port.audioProcessor.ConnectTransferMedia(target, inCodec, outputCodecName)
}

func (port *MediaPort) DisconnectTransferMedia() {
	if port == nil || port.audioProcessor == nil {
		return
	}
	port.audioProcessor.DisconnectTransferMedia()
}

func (port *MediaPort) RecordTransferOperatorAudio(audio []byte) {
	if port == nil || port.audioProcessor == nil {
		return
	}
	port.audioProcessor.RecordTransferOperatorAudio(audio)
}

func (port *MediaPort) LocalAddr() (string, int) {
	if port == nil || port.rtpHandler == nil {
		return "", 0
	}
	return port.rtpHandler.LocalAddr()
}

func (port *MediaPort) CodecName() string {
	if port == nil || port.rtpHandler == nil || port.rtpHandler.GetCodec() == nil {
		return "PCMU"
	}
	return port.rtpHandler.GetCodec().Name
}

func (port *MediaPort) forwardIncomingAudio() {
	if port == nil || port.rtpHandler == nil {
		return
	}
	audioIn := port.rtpHandler.AudioIn()
	for {
		select {
		case <-port.ctx.Done():
			return
		case audioData, ok := <-audioIn:
			if !ok {
				return
			}
			if port.audioProcessor.ForwardUserAudio(audioData) {
				continue
			}
			if port.transferActive.Load() {
				continue
			}
			if err := port.mediaSession.HandleProviderAudioFrame(internal_telephony_media.ProviderAudioFrame{
				Audio:      audioData,
				ReceivedAt: time.Now(),
			}); err != nil && port.logger != nil {
				port.logger.Debugw("SIP provider audio processing failed", "error", err.Error())
			}
		}
	}
}

func (port *MediaPort) cancelRingback(clearOutput bool) {
	port.mu.Lock()
	cancelFn := port.ringbackCancel
	port.ringbackCancel = nil
	port.mu.Unlock()
	if cancelFn != nil {
		cancelFn()
	}
	if clearOutput && port.audioProcessor != nil {
		port.audioProcessor.ClearOutputBuffer()
	}
}
