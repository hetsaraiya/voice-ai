// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_vonage

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

// Vonage audio constants (linear16 16kHz - same as downstream)
const (
	// Standard chunk duration for telephony (20ms)
	ChunkDuration = 20 * time.Millisecond

	// Linear16 16kHz: 32 bytes per ms (16-bit mono, 16000 samples/sec)
	Linear16BytesPerMs = 32

	// Output chunk size: 20ms at 16kHz linear16 = 640 bytes
	OutputChunkSize = Linear16BytesPerMs * 20

	// Input buffer threshold: 60ms at 16kHz linear16 = 1920 bytes
	InputBufferThreshold = Linear16BytesPerMs * 60
)

// AudioChunk represents a processed audio chunk ready for streaming
type AudioChunk struct {
	Data     []byte
	Duration time.Duration
}

// AudioProcessor handles audio for Vonage (linear16 16kHz - no conversion needed)
type AudioProcessor struct {
	logger commons.Logger

	// Resampler for ambient asset conversion when target format differs
	resampler internal_type.AudioResampler

	// Audio config (same for Vonage and downstream)
	audioConfig *protos.AudioConfig // linear16 16kHz

	// Input buffer for accumulating incoming audio
	inputBuffer internal_channel_input.InputBuffer

	// Output buffer for audio to be sent to Vonage
	outputBuffer internal_telephony_output.FrameBuffer

	// Callback for processed input audio (to send to downstream)
	onInputAudio func(audio []byte)

	// Callback for sending audio chunk to Vonage
	onOutputChunk func(chunk *AudioChunk) error

	// Pre-created silence chunk
	silenceChunk *AudioChunk

	ambientMixer internal_ambient.Mixer
	adapter      internal_telephony_output.AudioAdapter

	outputSenderRunning atomic.Bool
	outputHealth        *internal_telephony_output.HealthStats
}

// NewAudioProcessor creates a new Vonage audio processor
func NewAudioProcessor(logger commons.Logger) (*AudioProcessor, error) {
	resampler, err := internal_audio_resampler.GetResampler(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create resampler: %w", err)
	}

	p := &AudioProcessor{
		logger:       logger,
		resampler:    resampler,
		audioConfig:  internal_audio.NewLinear16khzMonoAudioConfig(),
		inputBuffer:  internal_channel_input.NewBytesInputBuffer(InputBufferThreshold * 2),
		outputBuffer: internal_telephony_output.NewBytesFrameBuffer(OutputChunkSize * 8),
		outputHealth: internal_telephony_output.NewHealthStats(),
	}
	p.adapter = newAudioAdapter(OutputChunkSize)

	// Pre-create silence chunk (all zeros for linear16)
	p.silenceChunk = p.createSilenceChunk()
	ambientMixer, err := internal_ambient.NewLoopMixer(internal_ambient.MixerSpec{
		Resampler:         p.resampler,
		TargetAudioConfig: p.audioConfig,
		FrameBytes:        OutputChunkSize,
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

// SetInputAudioCallback sets the callback for processed input audio
func (p *AudioProcessor) SetInputAudioCallback(callback func(audio []byte)) {
	p.onInputAudio = callback
}

// SetOutputChunkCallback sets the callback for sending audio chunks to Vonage
func (p *AudioProcessor) SetOutputChunkCallback(callback func(chunk *AudioChunk) error) {
	p.onOutputChunk = callback
}

// ============================================================================
// Input Audio Processing (from Vonage linear16 16kHz -> downstream - no conversion)
// ============================================================================

// ProcessInputAudio buffers incoming linear16 16kHz audio (no conversion needed)
func (p *AudioProcessor) ProcessInputAudio(audio []byte) error {
	if len(audio) == 0 {
		return nil
	}

	// No conversion needed - Vonage uses same format as downstream
	p.bufferAndSendInput(audio)
	return nil
}

// bufferAndSendInput buffers input audio and sends when threshold is reached
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

// ============================================================================
// Output Audio Processing (from downstream linear16 16kHz -> Vonage - no conversion)
// ============================================================================

// ProcessOutputAudio buffers outgoing linear16 16kHz audio (no conversion needed)
func (p *AudioProcessor) ProcessOutputAudio(audio []byte) error {
	if len(audio) == 0 {
		return nil
	}

	converted, err := p.adapter.ConvertOutput(audio)
	if err != nil {
		return err
	}
	p.outputBuffer.Write(converted)

	return nil
}

// Complete flushes buffered trailing bytes by padding to a full frame.
func (p *AudioProcessor) Complete() {
	p.outputBuffer.Complete(p.adapter.FrameSize(), p.adapter.SilenceByte())
}

// GetNextChunk retrieves the next audio chunk from the output buffer
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

// createSilenceChunk creates a linear16 silence chunk (all zeros)
func (p *AudioProcessor) createSilenceChunk() *AudioChunk {
	return &AudioChunk{
		Data:     make([]byte, p.adapter.FrameSize()),
		Duration: ChunkDuration,
	}
}

// RunOutputSender continuously sends audio chunks at consistent 20ms intervals
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

// ClearOutputBuffer clears the output audio buffer
func (p *AudioProcessor) ClearOutputBuffer() {
	p.outputBuffer.Clear()
}
