// Copyright (c) 2023-2026 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_audio

import (
	"encoding/binary"
	"math"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/protos"
	"github.com/zaf/g711"
)

// Linear16ToInt16 converts signed 16-bit little-endian PCM bytes to int16 samples.
// If data length is odd, the trailing byte is ignored.
func Linear16ToInt16(data []byte) []int16 {
	numSamples := len(data) / 2
	samples := make([]int16, numSamples)
	for i := 0; i < numSamples; i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2]))
	}
	return samples
}

// BytesPerSample returns the number of bytes per audio sample for the given
// audio format. Returns 0 for unsupported formats.
func BytesPerSample(format protos.AudioConfig_AudioFormat) int {
	switch format {
	case protos.AudioConfig_LINEAR16:
		return 2
	case protos.AudioConfig_MuLaw8:
		return 1
	default:
		return 0
	}
}

// BytesPerMs computes the byte rate per millisecond for the given audio config.
// Formula: sampleRate × bytesPerSample × channels / 1000.
// Returns 0 if cfg is nil or the format is unsupported.
func BytesPerMs(cfg *protos.AudioConfig) int {
	if cfg == nil {
		return 0
	}
	return int(cfg.GetSampleRate()) * BytesPerSample(cfg.GetAudioFormat()) * int(cfg.GetChannels()) / 1000
}

// BytesPerSecond computes the byte rate per second for the given audio config.
// Formula: sampleRate × bytesPerSample × channels.
// Returns 0 if cfg is nil or the format is unsupported.
func BytesPerSecond(cfg *protos.AudioConfig) int {
	if cfg == nil {
		return 0
	}
	return int(cfg.GetSampleRate()) * BytesPerSample(cfg.GetAudioFormat()) * int(cfg.GetChannels())
}

// FrameSize returns the number of bytes in a single audio frame (all channels)
// for the given audio config. Returns 0 if cfg is nil or the format is unsupported.
func FrameSize(cfg *protos.AudioConfig) int {
	if cfg == nil {
		return 0
	}
	return BytesPerSample(cfg.GetAudioFormat()) * int(cfg.GetChannels())
}

// AlawToUlaw converts A-law (PCMA) encoded audio to µ-law (PCMU).
func AlawToUlaw(data []byte) []byte {
	return g711.Alaw2Ulaw(data)
}

// UlawToAlaw converts µ-law (PCMU) encoded audio to A-law (PCMA).
func UlawToAlaw(data []byte) []byte {
	return g711.EncodeAlaw(g711.DecodeUlaw(data))
}

// EncodeUlawSample encodes a single 16-bit PCM sample to µ-law.
func EncodeUlawSample(sample int16) byte {
	return g711.EncodeUlawFrame(sample)
}

// GenerateRingbackMulawFrame generates a single 20ms frame of ringback tone as
// 8kHz µ-law (160 bytes). Intended for direct RTP injection — no resampling needed.
func GenerateRingbackMulawFrame(sampleOffset int) ([]byte, int) {
	const (
		sampleRate      = 8000
		frameMs         = 20
		toneHz          = 425
		amplitude       = 8000.0
		onDurationMs    = 1000
		cycleDurationMs = 4000
	)

	samplesPerFrame := sampleRate * frameMs / 1000
	onSamples := sampleRate * onDurationMs / 1000
	cycleSamples := sampleRate * cycleDurationMs / 1000

	frame := make([]byte, samplesPerFrame)
	for i := 0; i < samplesPerFrame; i++ {
		pos := (sampleOffset + i) % cycleSamples
		var sample int16
		if pos < onSamples {
			sample = int16(amplitude * math.Sin(2*math.Pi*float64(toneHz)*float64(pos)/float64(sampleRate)))
		}
		frame[i] = EncodeUlawSample(sample)
	}

	return frame, sampleOffset + samplesPerFrame
}

// GetAudioInfo returns detailed information about raw audio data based on
// the provided audio config. The returned AudioInfo.DurationMs contains the
// audio duration in milliseconds for sub-second granularity.
func GetAudioInfo(data []byte, config *protos.AudioConfig) internal_type.AudioInfo {
	bps := BytesPerSample(config.GetAudioFormat())
	channels := int(config.GetChannels())

	var samplesPerChannel int
	if bps > 0 && channels > 0 {
		samplesPerChannel = len(data) / (bps * channels)
	}

	durationMs := float64(samplesPerChannel) / float64(config.GetSampleRate()) * 1000.0

	return internal_type.AudioInfo{
		SampleRate:        config.GetSampleRate(),
		Format:            config.GetAudioFormat(),
		Channels:          config.GetChannels(),
		SamplesPerChannel: samplesPerChannel,
		BytesPerSample:    bps,
		TotalBytes:        len(data),
		DurationMs:        durationMs,
	}
}
