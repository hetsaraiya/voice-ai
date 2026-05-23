// Copyright (c) 2023-2025 RapidaAI
// Author: RapidaAI Team <team@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_telnyx

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	internal_audio "github.com/rapidaai/api/assistant-api/internal/audio"
	internal_ambient "github.com/rapidaai/api/assistant-api/internal/audio/ambient"
	internal_audio_resampler "github.com/rapidaai/api/assistant-api/internal/audio/resampler"
	internal_channel_input "github.com/rapidaai/api/assistant-api/internal/channel/input"
	internal_telephony_output "github.com/rapidaai/api/assistant-api/internal/channel/output"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/protos"
)

const (
	ChunkDuration = 20 * time.Millisecond

	MulawBytesPerMs = 8
	OutputChunkSize = MulawBytesPerMs * 20

	InputBufferThreshold = 32 * 60
	MulawSilence         = 0xFF
)

// AudioProcessor handles audio conversion for Telnyx PCMU 8kHz streams.
type AudioProcessor struct {
	logger commons.Logger

	resampler internal_type.AudioResampler

	telnyxConfig     *protos.AudioConfig
	downstreamConfig *protos.AudioConfig

	inputBuffer  internal_channel_input.InputBuffer
	outputBuffer internal_telephony_output.FrameBuffer

	onInputAudio  func(audio []byte)
	onOutputChunk func(chunk *AudioChunk) error

	silenceChunk *AudioChunk

	ambientMixer internal_ambient.Mixer
	adapter      internal_telephony_output.AudioAdapter

	outputSenderRunning atomic.Bool
	outputHealth        *internal_telephony_output.HealthStats
}

func NewAudioProcessor(logger commons.Logger) (*AudioProcessor, error) {
	resampler, err := internal_audio_resampler.GetResampler(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create resampler: %w", err)
	}

	p := &AudioProcessor{
		logger:           logger,
		resampler:        resampler,
		telnyxConfig:     internal_audio.NewMulaw8khzMonoAudioConfig(),
		downstreamConfig: internal_audio.NewLinear16khzMonoAudioConfig(),
		inputBuffer:      internal_channel_input.NewBytesInputBuffer(InputBufferThreshold * 2),
		outputBuffer:     internal_telephony_output.NewBytesFrameBuffer(OutputChunkSize * 8),
		outputHealth:     internal_telephony_output.NewHealthStats(),
	}
	p.adapter = newAudioAdapter(p.resampler, p.downstreamConfig, p.telnyxConfig, OutputChunkSize, MulawSilence)
	p.silenceChunk = p.createSilenceChunk()

	ambientMixer, err := internal_ambient.NewLoopMixer(internal_ambient.MixerSpec{
		Resampler:         p.resampler,
		TargetAudioConfig: internal_audio.NewLinear8khzMonoAudioConfig(),
		FrameBytes:        OutputChunkSize * 2,
	})
	if err == nil {
		p.ambientMixer = ambientMixer
	}

	return p, nil
}

func (p *AudioProcessor) ConfigureAmbient(cfg internal_ambient.Config) error {
	if p.ambientMixer == nil {
		return nil
	}
	return p.ambientMixer.Configure(cfg)
}

func (p *AudioProcessor) SetInputAudioCallback(callback func(audio []byte)) {
	p.onInputAudio = callback
}

func (p *AudioProcessor) SetOutputChunkCallback(callback func(chunk *AudioChunk) error) {
	p.onOutputChunk = callback
}

func (p *AudioProcessor) ProcessInputAudio(audio []byte) error {
	if len(audio) == 0 {
		return nil
	}

	converted, err := p.resampler.Resample(audio, p.telnyxConfig, p.downstreamConfig)
	if err != nil {
		return fmt.Errorf("audio conversion to 16kHz linear16 failed: %w", err)
	}

	p.bufferAndSendInput(converted)
	return nil
}

func (p *AudioProcessor) bufferAndSendInput(audio []byte) {
	p.inputBuffer.Write(audio)
	audioData, ok := p.inputBuffer.DrainIfReady(InputBufferThreshold)
	if !ok {
		return
	}

	if p.onInputAudio != nil {
		p.onInputAudio(audioData)
	}
}

func (p *AudioProcessor) ProcessOutputAudio(audio []byte) error {
	if len(audio) == 0 {
		return nil
	}

	converted, err := p.adapter.ConvertOutput(audio)
	if err != nil {
		return fmt.Errorf("audio conversion to mulaw 8kHz failed: %w", err)
	}

	p.outputBuffer.Write(converted)
	return nil
}

func (p *AudioProcessor) Complete() {
	p.outputBuffer.Complete(p.adapter.FrameSize(), p.adapter.SilenceByte())
}

func (p *AudioProcessor) GetNextChunk() *AudioChunk {
	chunk, ok := p.outputBuffer.Next(p.adapter.FrameSize())
	if !ok {
		return nil
	}

	return &AudioChunk{
		Data:     chunk,
		Duration: ChunkDuration,
	}
}

func (p *AudioProcessor) createSilenceChunk() *AudioChunk {
	chunk := make([]byte, p.adapter.FrameSize())
	for i := range chunk {
		chunk[i] = p.adapter.SilenceByte()
	}
	return &AudioChunk{
		Data:     chunk,
		Duration: ChunkDuration,
	}
}

func (p *AudioProcessor) RunOutputSender(ctx context.Context) {
	if p.onOutputChunk == nil {
		p.logger.Error("RunOutputSender called without output callback set")
		return
	}
	if !p.outputSenderRunning.CompareAndSwap(false, true) {
		return
	}
	defer p.outputSenderRunning.Store(false)
	(&internal_telephony_output.Pacer{
		Logger:        p.logger,
		FrameDuration: ChunkDuration,
		Provider:      p,
		Consumer:      p,
		Health:        p.outputHealth,
	}).Run(ctx)
}

func (p *AudioProcessor) OutputHealthSnapshot() internal_telephony_output.HealthSnapshot {
	if p.outputHealth == nil {
		return internal_telephony_output.HealthSnapshot{}
	}
	return p.outputHealth.Snapshot()
}

func (p *AudioProcessor) applyAmbient(chunk []byte) []byte {
	return p.adapter.MixAmbient(chunk, p.ambientMixer)
}

func (p *AudioProcessor) NextFrame() []byte {
	chunk := p.GetNextChunk()
	if chunk == nil {
		return nil
	}
	return p.applyAmbient(chunk.Data)
}

func (p *AudioProcessor) IdleFrame() []byte {
	frame := p.applyAmbient(nil)
	if len(frame) > 0 {
		return frame
	}
	return append([]byte(nil), p.silenceChunk.Data...)
}

func (p *AudioProcessor) ConsumeFrame(frame []byte) error {
	return p.onOutputChunk(&AudioChunk{
		Data:     frame,
		Duration: ChunkDuration,
	})
}

func (p *AudioProcessor) ClearOutputBuffer() {
	p.outputBuffer.Clear()
}
