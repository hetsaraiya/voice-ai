// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_denoiser_rnnoise

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger(t testing.TB) commons.Logger {
	logger, err := commons.NewApplicationLogger(
		commons.EnableConsole(true),
		commons.EnableFile(false),
		commons.Name("rnnoise-denoiser-test"),
		commons.Level("error"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = logger.Sync() })
	return logger
}

func captureDenoisedAudio(pkts []internal_type.Packet) (internal_type.DenoisedAudioPacket, bool) {
	for _, pkt := range pkts {
		if out, ok := pkt.(internal_type.DenoisedAudioPacket); ok {
			return out, true
		}
	}
	return internal_type.DenoisedAudioPacket{}, false
}

func hasDenoiseErrorEvent(pkts []internal_type.Packet) bool {
	for _, pkt := range pkts {
		event, ok := pkt.(internal_type.ObservabilityEventRecordPacket)
		if !ok || event.Record.Component != observability.ComponentDenoise {
			continue
		}
		if event.Record.Event == observability.DenoiseError {
			return true
		}
	}
	return false
}

func generatePCM16Sine(samples int) []byte {
	data := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		sample := int16(math.Sin(float64(i)*2*math.Pi/120.0) * 28000)
		binary.LittleEndian.PutUint16(data[i*2:i*2+2], uint16(sample))
	}
	return data
}

func TestRNNoise_ProcessAudioUsesPCMScaleBoundary(t *testing.T) {
	rn, err := NewRNNoise()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rn.Close()) })

	input := make([]float32, frameSize*4)
	for i := 0; i < len(input); i++ {
		input[i] = float32(math.Sin(float64(i)*2*math.Pi/120.0) * 0.85)
	}

	confidence, output, err := rn.ProcessAudio(input)
	require.NoError(t, err)

	var outputEnergy float64
	var outputMaxAbsoluteValue float64
	for _, sample := range output {
		outputEnergy += float64(sample) * float64(sample)
		if absoluteValue := math.Abs(float64(sample)); absoluteValue > outputMaxAbsoluteValue {
			outputMaxAbsoluteValue = absoluteValue
		}
	}
	outputRMS := math.Sqrt(outputEnergy / float64(len(output)))

	assert.Len(t, output, len(input))
	assert.Greater(t, confidence, 0.5, "expected RNNoise to see PCM-amplitude samples, not near-silence")
	assert.Greater(t, outputRMS, 0.25, "expected denoised audio to remain audible")
	assert.LessOrEqual(t, outputMaxAbsoluteValue, 1.0, "wrapper must return normalized samples")
}

func TestRnnoiseDenoiser_PreservesLengthOnFirstChunk(t *testing.T) {
	logger := testLogger(t)
	var packets []internal_type.Packet

	denoiser, err := NewRnnoiseDenoiser(
		t.Context(),
		logger,
		func(_ context.Context, pkt ...internal_type.Packet) error {
			packets = append(packets, pkt...)
			return nil
		},
		nil,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, denoiser.Close(t.Context())) })

	packets = nil                   // discard constructor telemetry
	input := generatePCM16Sine(960) // 60ms at 16kHz

	err = denoiser.Execute(t.Context(), internal_type.DenoiseAudioPacket{
		ContextID: "ctx-first",
		Audio:     input,
	})
	require.NoError(t, err)

	output, ok := captureDenoisedAudio(packets)
	require.True(t, ok, "expected denoised audio packet")
	assert.Len(t, output.Audio, len(input))
	assert.False(t, hasDenoiseErrorEvent(packets), "unexpected denoise error event")
}

func TestRnnoiseDenoiser_EmitsNonSilentAudio(t *testing.T) {
	logger := testLogger(t)
	var packets []internal_type.Packet

	denoiser, err := NewRnnoiseDenoiser(
		t.Context(),
		logger,
		func(_ context.Context, pkt ...internal_type.Packet) error {
			packets = append(packets, pkt...)
			return nil
		},
		nil,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, denoiser.Close(t.Context())) })

	input := generatePCM16Sine(960) // 60ms at 16kHz

	err = denoiser.Execute(t.Context(), internal_type.DenoiseAudioPacket{
		ContextID: "ctx-audible",
		Audio:     input,
	})
	require.NoError(t, err)

	output, ok := captureDenoisedAudio(packets)
	require.True(t, ok, "expected denoised audio packet")

	var outputEnergy float64
	zeroSamples := 0
	for i := 0; i < len(output.Audio)/2; i++ {
		sample := int16(binary.LittleEndian.Uint16(output.Audio[i*2 : i*2+2]))
		outputEnergy += float64(sample) * float64(sample)
		if sample == 0 {
			zeroSamples++
		}
	}
	outputRMS := math.Sqrt(outputEnergy / float64(len(output.Audio)/2))
	outputZeroRatio := float64(zeroSamples) / float64(len(output.Audio)/2)

	assert.Len(t, output.Audio, len(input))
	assert.Greater(t, outputRMS, 1000.0, "expected denoiser output to remain audible")
	assert.Less(t, outputZeroRatio, 0.5, "expected denoiser output not to be mostly silence")
	assert.False(t, hasDenoiseErrorEvent(packets), "unexpected denoise error event")
}

func TestRnnoiseDenoiser_PreservesLengthAcrossCalls(t *testing.T) {
	logger := testLogger(t)
	var packets []internal_type.Packet

	denoiser, err := NewRnnoiseDenoiser(
		t.Context(),
		logger,
		func(_ context.Context, pkt ...internal_type.Packet) error {
			packets = append(packets, pkt...)
			return nil
		},
		nil,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, denoiser.Close(t.Context())) })

	chunks := []struct {
		name  string
		audio []byte
		ctxID string
	}{
		{name: "first", audio: generatePCM16Sine(960), ctxID: "ctx-1"},
		{name: "second", audio: generatePCM16Sine(960), ctxID: "ctx-2"},
	}

	for _, chunk := range chunks {
		packets = nil
		err = denoiser.Execute(t.Context(), internal_type.DenoiseAudioPacket{
			ContextID: chunk.ctxID,
			Audio:     chunk.audio,
		})
		require.NoError(t, err, chunk.name)

		output, ok := captureDenoisedAudio(packets)
		require.True(t, ok, "expected denoised audio packet for %s", chunk.name)
		assert.Len(t, output.Audio, len(chunk.audio), chunk.name)
		assert.False(t, hasDenoiseErrorEvent(packets), chunk.name)
	}
}
