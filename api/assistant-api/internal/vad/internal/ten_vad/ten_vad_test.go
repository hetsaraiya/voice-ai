// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_ten_vad

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/rapidaai/api/assistant-api/internal/observability"
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

func newTenVADOrSkip(t *testing.T, threshold float64, cb func(ctx context.Context, pkt ...internal_type.Packet) error) *TenVAD {
	logger, _ := commons.NewApplicationLogger()
	opts := newTestOptions(t, threshold)
	vad, err := NewTenVAD(t.Context(), logger, cb, opts)
	if err != nil {
		t.Skipf("ten_vad library not available: %v", err)
	}
	tv := vad.(*TenVAD)
	t.Cleanup(func() { _ = tv.Close(context.Background()) })
	return tv
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

func TestNewTenVAD_DefaultThreshold(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newTenVADOrSkip(t, -1, callback)

	assert.NotNil(t, vad.detector)
}

func TestTenVAD_Name(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newTenVADOrSkip(t, 0.5, callback)

	assert.Equal(t, "ten_vad", vad.Name())
}

func TestNewTenVAD_EmitsInitializationObservability(t *testing.T) {
	var packets []internal_type.Packet
	callback := func(_ context.Context, pkt ...internal_type.Packet) error {
		packets = append(packets, pkt...)
		return nil
	}

	_ = newTenVADOrSkip(t, 0.5, callback)

	var hasInitMetric bool
	var hasInitLogWithOptions bool
	for _, packet := range packets {
		switch typed := packet.(type) {
		case internal_type.ObservabilityMetricRecordPacket:
			if typed.Scope == internal_type.ObservabilityRecordScopeConversation &&
				len(typed.Record.Metrics) == 1 &&
				typed.Record.Metrics[0].Name == observability.MetricVADInitLatencyMs &&
				typed.Record.Attributes["provider"] == vadName {
				hasInitMetric = true
			}
		case internal_type.ObservabilityLogRecordPacket:
			if typed.Scope == internal_type.ObservabilityRecordScopeConversation &&
				typed.Record.Level == observability.LevelInfo &&
				typed.Record.Message == "ten_vad: initialization completed" &&
				typed.Record.Attributes["component"] == observability.ComponentVAD.String() &&
				typed.Record.Attributes["provider"] == vadName &&
				typed.Record.Attributes["options"] != "" {
				hasInitLogWithOptions = true
			}
		}
	}

	assert.True(t, hasInitMetric, "expected VAD init latency metric")
	assert.True(t, hasInitLogWithOptions, "expected VAD init log with options")
}

func TestTenVAD_Process_Silence_NoCallback(t *testing.T) {
	detectionFired := false
	callback := func(_ context.Context, pkts ...internal_type.Packet) error {
		for _, p := range pkts {
			if _, ok := p.(internal_type.InterruptionDetectedPacket); ok {
				detectionFired = true
			}
		}
		return nil
	}

	vad := newTenVADOrSkip(t, 0.5, callback)

	err := vad.Execute(context.Background(), generateSilence(16000))
	require.NoError(t, err)
	assert.False(t, detectionFired, "silence should not trigger a speech detection event")
}

func TestTenVAD_Process_Speech_AllowsCallback(t *testing.T) {
	var result internal_type.InterruptionDetectedPacket
	callback := func(ctx context.Context, pkt ...internal_type.Packet) error {
		if len(pkt) > 0 {
			if interruption, ok := pkt[0].(internal_type.InterruptionDetectedPacket); ok {
				result = interruption
			}
		}
		return nil
	}

	vad := newTenVADOrSkip(t, 0.2, callback)

	err := vad.Execute(context.Background(), generateSineWave(16000, 440, 0.9))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, result.EndAt, result.StartAt)
}

func TestTenVAD_Process_CorruptedData(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newTenVADOrSkip(t, 0.5, callback)

	corrupted := make([]byte, 999) // Odd length
	err := vad.Execute(context.Background(), internal_type.UserAudioReceivedPacket{Audio: corrupted})
	_ = err // Accept error or nil; should not panic
}

func TestTenVAD_Process_VerySmallChunks(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newTenVADOrSkip(t, 0.5, callback)

	sizes := []int{1, 2, 5, 10, 20}
	for _, size := range sizes {
		size := size
		t.Run(fmt.Sprintf("%d_samples", size), func(t *testing.T) {
			err := vad.Execute(context.Background(), generateSilence(size))
			_ = err
		})
	}
}

func TestTenVAD_Process_Concurrent(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newTenVADOrSkip(t, 0.5, callback)

	var wg sync.WaitGroup
	const workers = 8
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_ = vad.Execute(context.Background(), generateSilence(1600))
		}()
	}
	wg.Wait()
}

func TestTenVAD_Close_Idempotent(t *testing.T) {
	logger, _ := commons.NewApplicationLogger()
	callback := func(context.Context, ...internal_type.Packet) error { return nil }
	opts := newTestOptions(t, 0.5)

	vad, err := NewTenVAD(t.Context(), logger, callback, opts)
	if err != nil {
		t.Skipf("ten_vad library not available: %v", err)
	}

	require.NoError(t, vad.Close(context.Background()))
	err = vad.Close(context.Background())
	_ = err
}

func TestTenVAD_Close_EmitsDurationUsageAndClosedEvent(t *testing.T) {
	var packets []internal_type.Packet
	logger, _ := commons.NewApplicationLogger()
	callback := func(_ context.Context, pkt ...internal_type.Packet) error {
		packets = append(packets, pkt...)
		return nil
	}
	opts := newTestOptions(t, 0.5)

	vad, err := NewTenVAD(t.Context(), logger, callback, opts)
	if err != nil {
		t.Skipf("ten_vad library not available: %v", err)
	}

	require.NoError(t, vad.Close(context.Background()))

	var hasDurationUsage bool
	var hasClosedEvent bool
	for _, packet := range packets {
		switch typed := packet.(type) {
		case internal_type.ObservabilityUsageRecordPacket:
			if typed.Scope == internal_type.ObservabilityRecordScopeConversation &&
				typed.Record.Component == observability.ComponentName(observability.UsageConversationVADDuration) &&
				typed.Record.Provider == vadName &&
				typed.Record.Duration > 0 {
				hasDurationUsage = true
			}
		case internal_type.ObservabilityEventRecordPacket:
			if typed.Scope == internal_type.ObservabilityRecordScopeConversation &&
				typed.Record.Component == observability.ComponentVAD &&
				typed.Record.Event == observability.VADClosed &&
				typed.Record.Attributes["provider"] == vadName {
				hasClosedEvent = true
			}
		}
	}

	assert.True(t, hasDurationUsage, "expected VAD duration usage after Close")
	assert.True(t, hasClosedEvent, "expected VAD closed event after Close")
}

func TestTenVAD_Process_NoisePatterns(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newTenVADOrSkip(t, 0.5, callback)

	err := vad.Execute(context.Background(), generateNoise(16000))
	require.NoError(t, err)
}

func TestTenVAD_Process_MaxAmplitude(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newTenVADOrSkip(t, 0.5, callback)

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

	err := vad.Execute(context.Background(), internal_type.UserAudioReceivedPacket{Audio: data})
	require.NoError(t, err)
}

func TestTenVAD_Process_RepeatedCalls(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newTenVADOrSkip(t, 0.5, callback)

	chunk := generateSilence(1600)
	for i := 0; i < 50; i++ {
		err := vad.Execute(context.Background(), chunk)
		require.NoError(t, err)
	}
}

func TestTenVAD_StatefulProcessing(t *testing.T) {
	var calls int
	callback := func(context.Context, ...internal_type.Packet) error {
		calls++
		return nil
	}

	vad := newTenVADOrSkip(t, 0.3, callback)

	for i := 0; i < 10; i++ {
		err := vad.Execute(context.Background(), generateSineWave(1600, 440, 0.8))
		require.NoError(t, err)
	}

	assert.GreaterOrEqual(t, calls, 0)
}

func TestTenVAD_Process_80msChunk(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }

	vad := newTenVADOrSkip(t, 0.5, callback)

	// 80ms at 16kHz = 1280 samples — production chunk size
	err := vad.Execute(context.Background(), generateSilence(1280))
	require.NoError(t, err)
}

func TestTenVAD_Process_PartialFrameCarry_NoDrop(t *testing.T) {
	callback := func(context.Context, ...internal_type.Packet) error { return nil }
	vad := newTenVADOrSkip(t, 0.5, callback)

	err := vad.Execute(context.Background(), generateSilence(128))
	require.NoError(t, err)
	assert.Equal(t, 0, vad.currSample)
	assert.Equal(t, 128, len(vad.pending))

	err = vad.Execute(context.Background(), generateSilence(200))
	require.NoError(t, err)
	assert.Equal(t, 256, vad.currSample)
	assert.Equal(t, 72, len(vad.pending))
}

func TestTenVAD_NotifyInterruption_SetsEvent(t *testing.T) {
	var got internal_type.InterruptionDetectedPacket
	callback := func(_ context.Context, pkts ...internal_type.Packet) error {
		for _, p := range pkts {
			if ip, ok := p.(internal_type.InterruptionDetectedPacket); ok {
				got = ip
			}
		}
		return nil
	}

	v := &TenVAD{onPacket: callback}
	v.notifyInterruption(context.Background(), "ctx-test", internal_type.InterruptionEventEnd, 3.25, 1)

	assert.Equal(t, "ctx-test", got.ContextID)
	assert.Equal(t, internal_type.InterruptionSourceVad, got.Source)
	assert.Equal(t, internal_type.InterruptionEventEnd, got.Event)
	assert.Equal(t, 3.25, got.StartAt)
	assert.Equal(t, 3.25, got.EndAt)
}
