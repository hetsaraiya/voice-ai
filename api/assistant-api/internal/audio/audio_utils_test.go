// Copyright (c) 2023-2026 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_audio

import (
	"encoding/binary"
	"testing"

	"github.com/rapidaai/protos"
	"github.com/stretchr/testify/assert"
)

func TestLinear16ToInt16_ConvertsSamples(t *testing.T) {
	data := make([]byte, 6)
	binary.LittleEndian.PutUint16(data[0:2], 0x8000)
	binary.LittleEndian.PutUint16(data[2:4], uint16(int16(0)))
	binary.LittleEndian.PutUint16(data[4:6], uint16(int16(32767)))

	got := Linear16ToInt16(data)
	assert.Equal(t, []int16{-32768, 0, 32767}, got)
}

func TestLinear16ToInt16_OddLengthIgnoresTrailingByte(t *testing.T) {
	data := []byte{0x01, 0x00, 0xFF}
	got := Linear16ToInt16(data)
	assert.Equal(t, []int16{1}, got)
}

// ---------------------------------------------------------------------------
// BytesPerSample
// ---------------------------------------------------------------------------

func TestBytesPerSample_LINEAR16(t *testing.T) {
	assert.Equal(t, 2, BytesPerSample(protos.AudioConfig_LINEAR16))
}

func TestBytesPerSample_MuLaw8(t *testing.T) {
	assert.Equal(t, 1, BytesPerSample(protos.AudioConfig_MuLaw8))
}

func TestBytesPerSample_UnsupportedFormat(t *testing.T) {
	assert.Equal(t, 0, BytesPerSample(protos.AudioConfig_AudioFormat(99)))
}

// ---------------------------------------------------------------------------
// BytesPerMs
// ---------------------------------------------------------------------------

func TestBytesPerMs_NilConfig(t *testing.T) {
	assert.Equal(t, 0, BytesPerMs(nil))
}

func TestBytesPerMs_Linear16khzMono(t *testing.T) {
	cfg := NewLinear16khzMonoAudioConfig() // 16000 Hz, LINEAR16, 1ch
	// 16000 * 2 * 1 / 1000 = 32
	assert.Equal(t, 32, BytesPerMs(cfg))
}

func TestBytesPerMs_Mulaw8khzMono(t *testing.T) {
	cfg := NewMulaw8khzMonoAudioConfig() // 8000 Hz, MuLaw8, 1ch
	// 8000 * 1 * 1 / 1000 = 8
	assert.Equal(t, 8, BytesPerMs(cfg))
}

func TestBytesPerMs_Linear48khzMono(t *testing.T) {
	cfg := NewLinear48khzMonoAudioConfig() // 48000 Hz, LINEAR16, 1ch
	// 48000 * 2 * 1 / 1000 = 96
	assert.Equal(t, 96, BytesPerMs(cfg))
}

// ---------------------------------------------------------------------------
// BytesPerSecond
// ---------------------------------------------------------------------------

func TestBytesPerSecond_NilConfig(t *testing.T) {
	assert.Equal(t, 0, BytesPerSecond(nil))
}

func TestBytesPerSecond_Linear16khzMono(t *testing.T) {
	cfg := NewLinear16khzMonoAudioConfig()
	// 16000 * 2 * 1 = 32000
	assert.Equal(t, 32000, BytesPerSecond(cfg))
}

func TestBytesPerSecond_Mulaw8khzMono(t *testing.T) {
	cfg := NewMulaw8khzMonoAudioConfig()
	// 8000 * 1 * 1 = 8000
	assert.Equal(t, 8000, BytesPerSecond(cfg))
}

// ---------------------------------------------------------------------------
// FrameSize
// ---------------------------------------------------------------------------

func TestFrameSize_NilConfig(t *testing.T) {
	assert.Equal(t, 0, FrameSize(nil))
}

func TestFrameSize_Linear16Mono(t *testing.T) {
	cfg := NewLinear16khzMonoAudioConfig()
	// 2 * 1 = 2
	assert.Equal(t, 2, FrameSize(cfg))
}

func TestFrameSize_MuLaw8Mono(t *testing.T) {
	cfg := NewMulaw8khzMonoAudioConfig()
	// 1 * 1 = 1
	assert.Equal(t, 1, FrameSize(cfg))
}

func TestFrameSize_Linear16Stereo(t *testing.T) {
	cfg := &protos.AudioConfig{
		SampleRate:  16000,
		AudioFormat: protos.AudioConfig_LINEAR16,
		Channels:    2,
	}
	// 2 * 2 = 4
	assert.Equal(t, 4, FrameSize(cfg))
}

// ---------------------------------------------------------------------------
// GetAudioInfo
// ---------------------------------------------------------------------------

func TestGetAudioInfo_Linear16_1sec(t *testing.T) {
	cfg := NewLinear16khzMonoAudioConfig()
	data := make([]byte, 32000) // 1 second of 16kHz mono LINEAR16
	info := GetAudioInfo(data, cfg)

	assert.Equal(t, uint32(16000), info.SampleRate)
	assert.Equal(t, protos.AudioConfig_LINEAR16, info.Format)
	assert.Equal(t, uint32(1), info.Channels)
	assert.Equal(t, 16000, info.SamplesPerChannel)
	assert.Equal(t, 2, info.BytesPerSample)
	assert.Equal(t, 32000, info.TotalBytes)
	assert.InDelta(t, 1000.0, info.DurationMs, 0.01) // 1 second = 1000 ms
}

func TestGetAudioInfo_MuLaw8_1sec(t *testing.T) {
	cfg := NewMulaw8khzMonoAudioConfig()
	data := make([]byte, 8000) // 1 second of 8kHz mono MuLaw8
	info := GetAudioInfo(data, cfg)

	assert.Equal(t, uint32(8000), info.SampleRate)
	assert.Equal(t, protos.AudioConfig_MuLaw8, info.Format)
	assert.Equal(t, uint32(1), info.Channels)
	assert.Equal(t, 8000, info.SamplesPerChannel)
	assert.Equal(t, 1, info.BytesPerSample)
	assert.Equal(t, 8000, info.TotalBytes)
	assert.InDelta(t, 1000.0, info.DurationMs, 0.01)
}

func TestGetAudioInfo_Linear16_20ms(t *testing.T) {
	cfg := NewLinear16khzMonoAudioConfig()
	// 20ms frame: 16000 * 2 * 1 / 1000 * 20 = 640 bytes
	data := make([]byte, 640)
	info := GetAudioInfo(data, cfg)

	assert.Equal(t, 320, info.SamplesPerChannel)
	assert.InDelta(t, 20.0, info.DurationMs, 0.01)
}

func TestGetAudioInfo_Linear16_SubMs(t *testing.T) {
	cfg := NewLinear16khzMonoAudioConfig()
	// 10 samples = 20 bytes → 10/16000 * 1000 = 0.625 ms
	data := make([]byte, 20)
	info := GetAudioInfo(data, cfg)

	assert.Equal(t, 10, info.SamplesPerChannel)
	assert.InDelta(t, 0.625, info.DurationMs, 0.001)
}

func TestGetAudioInfo_Stereo(t *testing.T) {
	cfg := &protos.AudioConfig{
		SampleRate:  16000,
		AudioFormat: protos.AudioConfig_LINEAR16,
		Channels:    2,
	}
	// 1 second stereo: 16000 * 2 * 2 = 64000 bytes
	data := make([]byte, 64000)
	info := GetAudioInfo(data, cfg)

	assert.Equal(t, 16000, info.SamplesPerChannel)
	assert.InDelta(t, 1000.0, info.DurationMs, 0.01)
}

func TestGetAudioInfo_EmptyData(t *testing.T) {
	cfg := NewLinear16khzMonoAudioConfig()
	data := make([]byte, 0)
	info := GetAudioInfo(data, cfg)

	assert.Equal(t, 0, info.SamplesPerChannel)
	assert.Equal(t, 0, info.TotalBytes)
	assert.Equal(t, 0.0, info.DurationMs)
}

func TestGetAudioInfo_2sec(t *testing.T) {
	cfg := NewLinear16khzMonoAudioConfig()
	data := make([]byte, 64000) // 2 seconds
	info := GetAudioInfo(data, cfg)

	assert.Equal(t, 32000, info.SamplesPerChannel)
	assert.InDelta(t, 2000.0, info.DurationMs, 0.01)
}

// ---------------------------------------------------------------------------
// String representation
// ---------------------------------------------------------------------------

func TestGetAudioInfo_String(t *testing.T) {
	cfg := NewLinear16khzMonoAudioConfig()
	data := make([]byte, 32000) // 1 second
	info := GetAudioInfo(data, cfg)

	s := info.String()
	assert.Contains(t, s, "Linear16")
	assert.Contains(t, s, "16000")
	assert.Contains(t, s, "Mono")
	assert.Contains(t, s, "ms")
}
