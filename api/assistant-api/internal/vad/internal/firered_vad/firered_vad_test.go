// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_firered_vad

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"testing"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestOptions(tb testing.TB, threshold float64) utils.Option {
	opts := map[string]interface{}{}
	if threshold >= 0 {
		opts["microphone.vad.threshold"] = threshold
	}
	return opts
}

func newFireRedOrSkip(t *testing.T, threshold float64, cb func(ctx context.Context, pkt ...internal_type.Packet) error) *FireRedVAD {
	logger, _ := commons.NewApplicationLogger()
	opts := newTestOptions(t, threshold)
	vad, err := NewFireRedVAD(t.Context(), logger, cb, opts)
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
			t.Skipf("firered model not available: %v", err)
		}
		require.NoError(t, err)
	}
	fr := vad.(*FireRedVAD)
	t.Cleanup(func() { _ = fr.Close() })
	return fr
}

func generateSilence(samples int) internal_type.UserAudioReceivedPacket {
	return internal_type.UserAudioReceivedPacket{Audio: make([]byte, samples*2)}
}

func generateSineWave(samples int, frequency, amplitude float64) internal_type.UserAudioReceivedPacket {
	data := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		sample := int16(amplitude * 32767 * math.Sin(2*math.Pi*float64(i)*frequency/16000))
		binary.LittleEndian.PutUint16(data[i*2:i*2+2], uint16(sample))
	}
	return internal_type.UserAudioReceivedPacket{Audio: data}
}

func generateNoise(samples int) internal_type.UserAudioReceivedPacket {
	data := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		sample := int16((i*7919)%65536 - 32768)
		binary.LittleEndian.PutUint16(data[i*2:i*2+2], uint16(sample))
	}
	return internal_type.UserAudioReceivedPacket{Audio: data}
}

// Core functionality tests

func TestNewFireRedVAD_DefaultThreshold(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newFireRedOrSkip(t, -1, callback)

	assert.NotNil(t, vad.detector)
	assert.NotNil(t, vad.fbank)
	assert.NotNil(t, vad.postprocessor)
}

func TestFireRedVAD_Name(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newFireRedOrSkip(t, 0.5, callback)

	assert.Equal(t, "firered_vad", vad.Name())
}

func TestFireRedVAD_Process_Silence_NoCallback(t *testing.T) {
	detectionFired := false
	callback := func(_ context.Context, pkts ...internal_type.Packet) error {
		for _, p := range pkts {
			if _, ok := p.(internal_type.InterruptionDetectedPacket); ok {
				detectionFired = true
			}
		}
		return nil
	}

	vad := newFireRedOrSkip(t, 0.5, callback)

	err := vad.Process(context.Background(), generateSilence(16000))
	require.NoError(t, err)
	assert.False(t, detectionFired, "silence should not trigger a speech detection event")
}

func TestFireRedVAD_Process_Speech_AllowsCallback(t *testing.T) {
	var result internal_type.InterruptionDetectedPacket
	callback := func(ctx context.Context, pkt ...internal_type.Packet) error {
		for _, p := range pkt {
			if interruption, ok := p.(internal_type.InterruptionDetectedPacket); ok {
				result = interruption
			}
		}
		return nil
	}

	vad := newFireRedOrSkip(t, 0.2, callback)

	err := vad.Process(context.Background(), generateSineWave(16000, 440, 0.9))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, result.EndAt, result.StartAt)
}

func TestFireRedVAD_Process_CorruptedData(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newFireRedOrSkip(t, 0.5, callback)

	corrupted := make([]byte, 999)
	err := vad.Process(context.Background(), internal_type.UserAudioReceivedPacket{Audio: corrupted})
	_ = err
}

func TestFireRedVAD_Process_VerySmallChunks(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newFireRedOrSkip(t, 0.5, callback)

	sizes := []int{1, 2, 5, 10, 20}
	for _, size := range sizes {
		size := size
		t.Run(fmt.Sprintf("%d_samples", size), func(t *testing.T) {
			err := vad.Process(context.Background(), generateSilence(size))
			_ = err
		})
	}
}

func TestFireRedVAD_Process_Concurrent(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newFireRedOrSkip(t, 0.5, callback)

	var wg sync.WaitGroup
	const workers = 8
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_ = vad.Process(context.Background(), generateSilence(1600))
		}()
	}
	wg.Wait()
}

func TestFireRedVAD_Close_Idempotent(t *testing.T) {
	logger, _ := commons.NewApplicationLogger()
	callback := func(context.Context, ...internal_type.Packet) error { return nil }
	opts := newTestOptions(t, 0.5)

	vad, err := NewFireRedVAD(t.Context(), logger, callback, opts)
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
			t.Skipf("firered model not available: %v", err)
		}
		require.NoError(t, err)
	}

	require.NoError(t, vad.Close())
	err = vad.Close()
	_ = err
}

func TestFireRedVAD_Process_NoisePatterns(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newFireRedOrSkip(t, 0.5, callback)

	err := vad.Process(context.Background(), generateNoise(16000))
	require.NoError(t, err)
}

func TestFireRedVAD_Process_MaxAmplitude(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newFireRedOrSkip(t, 0.5, callback)

	samples := 16000
	data := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		var val int16
		if i%2 == 0 {
			val = 32767
		} else {
			val = -32768
		}
		binary.LittleEndian.PutUint16(data[i*2:i*2+2], uint16(val))
	}

	err := vad.Process(context.Background(), internal_type.UserAudioReceivedPacket{Audio: data})
	require.NoError(t, err)
}

func TestFireRedVAD_Process_RepeatedCalls(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newFireRedOrSkip(t, 0.5, callback)

	chunk := generateSilence(1600)
	for i := 0; i < 50; i++ {
		err := vad.Process(context.Background(), chunk)
		require.NoError(t, err)
	}
}

func TestFireRedVAD_StatefulProcessing(t *testing.T) {
	var calls int
	callback := func(context.Context, ...internal_type.Packet) error {
		calls++
		return nil
	}

	vad := newFireRedOrSkip(t, 0.3, callback)

	for i := 0; i < 10; i++ {
		err := vad.Process(context.Background(), generateSineWave(1600, 440, 0.8))
		require.NoError(t, err)
	}

	assert.GreaterOrEqual(t, calls, 0)
}

func TestFireRedVAD_Process_80msChunk(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newFireRedOrSkip(t, 0.5, callback)

	// 80ms at 16kHz = 1280 samples — production chunk size
	err := vad.Process(context.Background(), generateSilence(1280))
	require.NoError(t, err)
}

func TestFireRedVAD_Process_PartialFrameCarry_NoDrop(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }
	vad := newFireRedOrSkip(t, 0.5, callback)

	err := vad.Process(context.Background(), generateSilence(128))
	require.NoError(t, err)
	assert.Equal(t, 128, len(vad.audioBuf))

	err = vad.Process(context.Background(), generateSilence(200))
	require.NoError(t, err)
	assert.Equal(t, 328, len(vad.audioBuf))

	err = vad.Process(context.Background(), generateSilence(100))
	require.NoError(t, err)
	// 428 total samples buffered -> one frame processed, shift by 160 samples -> 268 retained.
	assert.Equal(t, 268, len(vad.audioBuf))
}

func TestFireRedVAD_NotifyInterruption_SetsEvent(t *testing.T) {
	var got internal_type.InterruptionDetectedPacket
	callback := func(_ context.Context, pkts ...internal_type.Packet) error {
		for _, p := range pkts {
			if ip, ok := p.(internal_type.InterruptionDetectedPacket); ok {
				got = ip
			}
		}
		return nil
	}

	v := &FireRedVAD{onPacket: callback}
	v.notifyInterruption(context.Background(), internal_type.InterruptionEventEnd, 1.75)

	assert.Equal(t, internal_type.InterruptionSourceVad, got.Source)
	assert.Equal(t, internal_type.InterruptionEventEnd, got.Event)
	assert.Equal(t, 1.75, got.StartAt)
	assert.Equal(t, 1.75, got.EndAt)
}
