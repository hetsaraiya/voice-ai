// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_sip

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	internal_audio "github.com/rapidaai/api/assistant-api/internal/audio"
	internal_ambient "github.com/rapidaai/api/assistant-api/internal/audio/ambient"
	internal_channel_input "github.com/rapidaai/api/assistant-api/internal/channel/input"
	internal_telephony_output "github.com/rapidaai/api/assistant-api/internal/channel/output"
	internal_telephony_media "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/media"
	"github.com/rapidaai/api/assistant-api/internal/observe"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	"github.com/rapidaai/protos"
	"github.com/zaf/g711"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type bridgeState struct {
	outputTarget internal_type.SIPRTPBridgeTarget
	transcode    func([]byte) []byte
}

// AudioProcessor owns SIP RTP codec conversion, buffering, pacing, bridge audio,
// and ringback generation.
type AudioProcessor struct {
	resampler  internal_type.AudioResampler
	rtpHandler *sip_infra.RTPHandler
	pushInput  func(internal_type.Stream)

	inputBuffer        internal_channel_input.InputBuffer
	outputBuffer       internal_telephony_output.FrameBuffer
	bridgeOutputBuffer internal_telephony_output.FrameBuffer

	// bridgeMu orders ForwardUserAudio with DisconnectTransferMedia so outbound RTP is
	// not closed while a bridge send is in flight.
	bridgeMu         sync.Mutex
	bridge           atomic.Pointer[bridgeState]
	bridgeUserCh     chan []byte
	bridgeOperatorCh chan []byte

	ringtoneMu sync.RWMutex
	ringtone   []byte

	ambientMixer internal_ambient.Mixer

	outputHealth        *internal_telephony_output.HealthStats
	droppedOutputFrames atomic.Uint64
	droppedBridgeFrames atomic.Uint64
	transferActive      atomic.Bool
}

func NewAudioProcessor(cfg AudioProcessorConfig) *AudioProcessor {
	p := &AudioProcessor{
		resampler:          cfg.Resampler,
		rtpHandler:         cfg.RTPHandler,
		pushInput:          cfg.PushInput,
		inputBuffer:        internal_channel_input.NewBytesInputBuffer(InputBufferThreshold * 2),
		outputBuffer:       internal_telephony_output.NewBytesFrameBuffer(MulawFrameSize * 8),
		bridgeOutputBuffer: internal_telephony_output.NewBytesFrameBuffer(BridgeOutputFrameSize * 8),
		bridgeUserCh:       make(chan []byte, AudioChannelSize),
		bridgeOperatorCh:   make(chan []byte, AudioChannelSize),
		outputHealth:       internal_telephony_output.NewHealthStats(),
	}
	p.SetRingtone(cfg.Ringtone)
	ambientMixer, err := internal_ambient.NewLoopMixer(internal_ambient.MixerSpec{
		Resampler:         cfg.Resampler,
		TargetAudioConfig: Linear8kConfig,
		FrameBytes:        MulawFrameSize * 2,
	})
	if err == nil {
		p.ambientMixer = ambientMixer
		if cfg.Ambient != nil {
			_ = p.ambientMixer.Configure(*cfg.Ambient)
		}
	}
	return p
}

func (p *AudioProcessor) currentCodec() *sip_infra.Codec {
	if p.rtpHandler == nil {
		return &sip_infra.CodecPCMU
	}
	codec := p.rtpHandler.GetCodec()
	if codec == nil {
		return &sip_infra.CodecPCMU
	}
	return codec
}

func (p *AudioProcessor) SetRingtone(name string) {
	audio := LoadRingtoneBytes(name)
	p.ringtoneMu.Lock()
	p.ringtone = audio
	p.ringtoneMu.Unlock()
}

func (p *AudioProcessor) ConfigureAmbient(cfg internal_ambient.Config) error {
	if p.ambientMixer == nil {
		return nil
	}
	return p.ambientMixer.Configure(cfg)
}

func (p *AudioProcessor) ringtoneBytes() []byte {
	p.ringtoneMu.RLock()
	defer p.ringtoneMu.RUnlock()
	return p.ringtone
}

func (p *AudioProcessor) convertInputAudio(audioData []byte) ([]byte, error) {
	codec := p.currentCodec()
	if codec.Name == sip_infra.CodecPCMA.Name {
		audioData = internal_audio.AlawToUlaw(audioData)
	}
	resampled, err := p.resampler.Resample(audioData, Mulaw8kConfig, Rapida16kConfig)
	if err != nil {
		return nil, err
	}
	return resampled, nil
}

func (p *AudioProcessor) convertOutputAudio(audioData []byte) ([]byte, error) {
	convertedAudio, err := p.resampler.Resample(audioData, Rapida16kConfig, Mulaw8kConfig)
	if err != nil {
		return nil, err
	}
	if p.currentCodec().Name == sip_infra.CodecPCMA.Name {
		convertedAudio = internal_audio.UlawToAlaw(convertedAudio)
	}
	return convertedAudio, nil
}

func (p *AudioProcessor) ProcessProviderAudioFrame(frame internal_telephony_media.ProviderAudioFrame) (internal_telephony_media.InputAudioFrame, error) {
	inputFrame := internal_telephony_media.InputAudioFrame{
		ReceivedAt: frame.ReceivedAt,
	}
	if len(frame.Audio) == 0 {
		return inputFrame, nil
	}
	resampled, err := p.convertInputAudio(frame.Audio)
	if err != nil {
		return inputFrame, fmt.Errorf("%w: %w", ErrProviderAudioConversionFailed, err)
	}
	if len(resampled) == 0 {
		return inputFrame, nil
	}
	inputFrame.BridgeAudio = resampled
	p.inputBuffer.Write(resampled)
	if pipelineAudio, ok := p.inputBuffer.DrainIfReady(InputBufferThreshold); ok {
		inputFrame.PipelineAudio = pipelineAudio
	}
	return inputFrame, nil
}

func (p *AudioProcessor) ProcessAssistantAudio(audio []byte, completed bool) error {
	if p.bridge.Load() != nil || p.transferActive.Load() {
		return nil
	}
	if len(audio) > 0 {
		convertedAudio, err := p.convertOutputAudio(audio)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrAssistantAudioConversionFailed, err)
		}
		p.outputBuffer.Write(convertedAudio)
		p.bridgeOutputBuffer.Write(audio)
	}
	if completed {
		p.Complete()
	}
	return nil
}

func (p *AudioProcessor) Complete() {
	p.outputBuffer.Complete(MulawFrameSize, MulawSilenceByte)
	p.bridgeOutputBuffer.Complete(BridgeOutputFrameSize, 0)
}

func (p *AudioProcessor) getNextChunk() []byte {
	chunk, ok := p.outputBuffer.Next(MulawFrameSize)
	if !ok {
		return nil
	}
	return chunk
}

func (p *AudioProcessor) ClearOutputBuffer() {
	p.outputBuffer.Clear()
	p.bridgeOutputBuffer.Clear()
	p.rtpHandler.FlushAudioOut()
}

func (p *AudioProcessor) ClearInputBuffer() {
	p.inputBuffer.Clear()
}

func (p *AudioProcessor) OutputHealthSnapshot() internal_telephony_output.HealthSnapshot {
	if p.outputHealth == nil {
		return internal_telephony_output.HealthSnapshot{}
	}
	return p.outputHealth.Snapshot()
}

func (p *AudioProcessor) OnTickHealth(event internal_telephony_output.TickHealth) {
	if p.outputHealth != nil {
		p.outputHealth.OnTickHealth(event)
	}
}

func (p *AudioProcessor) OutputFrameDuration() time.Duration {
	return ChunkDuration
}

func (p *AudioProcessor) applyAmbient(chunk []byte) []byte {
	if p.ambientMixer == nil {
		return chunk
	}
	codec := p.currentCodec()
	var primaryPCM []byte
	if len(chunk) > 0 {
		switch codec.Name {
		case sip_infra.CodecPCMA.Name:
			primaryPCM = g711.DecodeAlaw(chunk)
		default:
			primaryPCM = g711.DecodeUlaw(chunk)
		}
	}
	mixedPCM, err := p.ambientMixer.Mix(primaryPCM)
	if err != nil || len(mixedPCM) == 0 {
		return chunk
	}
	switch codec.Name {
	case sip_infra.CodecPCMA.Name:
		return g711.EncodeAlaw(mixedPCM)
	default:
		return g711.EncodeUlaw(mixedPCM)
	}
}

func (p *AudioProcessor) NextOutputFrame() (internal_telephony_media.AssistantOutputFrame, bool) {
	if p.transferActive.Load() {
		return internal_telephony_media.AssistantOutputFrame{}, false
	}
	providerAudio := p.getNextChunk()
	if len(providerAudio) == 0 {
		return internal_telephony_media.AssistantOutputFrame{}, false
	}
	bridgeAudio, _ := p.bridgeOutputBuffer.Next(BridgeOutputFrameSize)
	return internal_telephony_media.AssistantOutputFrame{
		ProviderAudio: p.applyAmbient(providerAudio),
		BridgeAudio:   bridgeAudio,
	}, true
}

func (p *AudioProcessor) IdleOutputFrame() (internal_telephony_media.AssistantOutputFrame, bool) {
	if p.transferActive.Load() {
		return internal_telephony_media.AssistantOutputFrame{}, false
	}
	frame := p.applyAmbient(nil)
	if len(frame) == 0 {
		return internal_telephony_media.AssistantOutputFrame{}, false
	}
	return internal_telephony_media.AssistantOutputFrame{ProviderAudio: frame, Idle: true}, true
}

func (p *AudioProcessor) SetTransferActive(active bool) {
	p.transferActive.Store(active)
}

func (p *AudioProcessor) ConsumeFrame(frame []byte) error {
	select {
	case p.rtpHandler.AudioOut() <- frame:
		return nil
	default:
		dropped := p.droppedOutputFrames.Add(1)
		return fmt.Errorf("%w: dropped_frames_total=%d", ErrRTPOutputQueueFull, dropped)
	}
}

func (p *AudioProcessor) IsBridgeActive() bool {
	return p.bridge.Load() != nil
}

func (p *AudioProcessor) ConnectTransferMedia(target internal_type.SIPRTPBridgeTarget, inputCodec *sip_infra.Codec, outputCodecName string) {
	if target == nil {
		return
	}
	state := &bridgeState{outputTarget: target}
	if inputCodec != nil && outputCodecName != "" && inputCodec.Name != outputCodecName {
		if inputCodec.Name == sip_infra.CodecPCMA.Name && outputCodecName == sip_infra.CodecPCMU.Name {
			state.transcode = internal_audio.AlawToUlaw
		} else if inputCodec.Name == sip_infra.CodecPCMU.Name && outputCodecName == sip_infra.CodecPCMA.Name {
			state.transcode = internal_audio.UlawToAlaw
		}
	}
	p.bridgeMu.Lock()
	p.bridge.Store(state)
	p.bridgeMu.Unlock()
}

// DisconnectTransferMedia returns only after in-flight bridge sends finish.
func (p *AudioProcessor) DisconnectTransferMedia() {
	p.bridgeMu.Lock()
	p.bridge.Store(nil)
	p.bridgeMu.Unlock()
}

// ForwardUserAudio routes caller audio to the bridge target and recorder.
func (p *AudioProcessor) ForwardUserAudio(audioData []byte) bool {
	p.bridgeMu.Lock()
	defer p.bridgeMu.Unlock()
	state := p.bridge.Load()
	if state == nil {
		return false
	}
	originalAudio := audioData
	if state.transcode != nil {
		audioData = state.transcode(audioData)
	}
	bridgeFrameDelivered := false
	select {
	case state.outputTarget.AudioOut() <- audioData:
		bridgeFrameDelivered = true
	default:
	}
	if !bridgeFrameDelivered {
		dropped := p.droppedBridgeFrames.Add(1)
		if p.pushInput != nil && (dropped == 1 || dropped%100 == 0) {
			p.pushInput(&protos.ConversationEvent{
				Name: observe.ComponentTelephony,
				Data: map[string]string{
					"type":                 "output_send_error",
					"provider":             Provider,
					"reason":               "bridge_audio_out_full",
					"dropped_frames_total": fmt.Sprintf("%d", dropped),
				},
				Time: timestamppb.Now(),
			})
		}
		return true
	}
	select {
	case p.bridgeUserCh <- originalAudio:
	default:
	}
	return true
}

// RecordTransferOperatorAudio queues transfer target audio for recording.
func (p *AudioProcessor) RecordTransferOperatorAudio(audio []byte) {
	select {
	case p.bridgeOperatorCh <- audio:
	default:
	}
}

// RunBridgeRecorder pushes bridge audio into the Talk pipeline.
func (p *AudioProcessor) RunBridgeRecorder(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case audio := <-p.bridgeUserCh:
			if resampled, err := p.resampler.Resample(audio, Mulaw8kConfig, Rapida16kConfig); err == nil {
				p.pushInput(&protos.ConversationBridgeUserAudio{
					Audio: resampled,
					Time:  timestamppb.Now(),
				})
			}
		case audio := <-p.bridgeOperatorCh:
			if resampled, err := p.resampler.Resample(audio, Mulaw8kConfig, Rapida16kConfig); err == nil {
				p.pushInput(&protos.ConversationBridgeOperatorAudio{
					Audio: resampled,
					Time:  timestamppb.Now(),
				})
			}
		}
	}
}

// PlayRingback writes ringback frames directly to RTP.
func (p *AudioProcessor) PlayRingback(ctx context.Context) {
	codec := p.currentCodec()
	ticker := time.NewTicker(ChunkDuration)
	defer ticker.Stop()

	ringtone := p.ringtoneBytes()
	useFile := len(ringtone) >= MulawFrameSize
	fileOffset := 0
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var frame []byte
			if useFile {
				if len(ringtone) >= MulawFrameSize {
					end := fileOffset + MulawFrameSize
					if end > len(ringtone) {
						fileOffset = 0
						end = MulawFrameSize
					}
					if end <= len(ringtone) {
						frame = ringtone[fileOffset:end]
						fileOffset = end
					}
				}
				if len(frame) == 0 {
					frame, offset = internal_audio.GenerateRingbackMulawFrame(offset)
				}
			} else {
				frame, offset = internal_audio.GenerateRingbackMulawFrame(offset)
			}
			if codec.Name == sip_infra.CodecPCMA.Name {
				frame = internal_audio.UlawToAlaw(frame)
			}
			select {
			case p.rtpHandler.AudioOut() <- frame:
			case <-ctx.Done():
				return
			default:
			}
		}
	}
}
