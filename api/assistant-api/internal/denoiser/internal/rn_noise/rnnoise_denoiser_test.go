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
		event, ok := pkt.(internal_type.ConversationEventPacket)
		if !ok || event.Name != "denoise" {
			continue
		}
		if event.Data["type"] == "error" {
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
	assert.True(t, output.NoiseReduced)
	assert.Len(t, output.Audio, len(input))
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
		assert.True(t, output.NoiseReduced, chunk.name)
		assert.Len(t, output.Audio, len(chunk.audio), chunk.name)
		assert.False(t, hasDenoiseErrorEvent(packets), chunk.name)
	}
}
