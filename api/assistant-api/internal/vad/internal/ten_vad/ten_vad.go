// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_ten_vad

import (
	"context"
	"fmt"
	"sync"
	"time"

	internal_audio "github.com/rapidaai/api/assistant-api/internal/audio"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/utils"
)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const (
	vadName = "ten_vad"

	// TEN VAD processes fixed-size frames. hop_size=256 = 16ms at 16kHz.
	defaultHopSize = 256

	// Default speech detection threshold [0.0, 1.0]
	defaultThreshold = 0.5

	// Default durations — aligned with FireRedVAD defaults
	// (20 frames × 10 ms = 200 ms silence, 8 frames × 10 ms = 80 ms pad)
	defaultMinSilenceDurationMs = 200
	defaultSpeechPadMs          = 80
)

// -----------------------------------------------------------------------------
// TenVAD - Voice Activity Detection using TEN Framework
// -----------------------------------------------------------------------------

// TenVAD implements the Vad interface using the TEN VAD library.
// It provides frame-level speech probability scores with low latency.
//
// Input audio must be 16 kHz LINEAR16 mono (the platform's internal format).
// NOT safe for concurrent use — the wrapper serializes with a mutex.
type TenVAD struct {
	logger   commons.Logger
	onPacket func(ctx context.Context, pkt ...internal_type.Packet) error

	// TEN VAD detector instance
	detector *Detector

	// Thread-safety
	mu           sync.RWMutex
	isTerminated bool

	// Frame-level state for segment tracking
	currSample int
	triggered  bool
	tempEnd    int
	pending    []int16

	// Configuration
	hopSize              int
	threshold            float32
	minSilenceDurationMs int
	speechPadMs          int
}

// NewTenVAD creates a new TenVAD instance.
// Input audio must be 16 kHz LINEAR16 mono.
func NewTenVAD(
	ctx context.Context,
	logger commons.Logger,
	onPacket func(ctx context.Context, pkt ...internal_type.Packet) error,
	options utils.Option,
) (internal_type.Vad, error) {
	start := time.Now()

	hopSize := defaultHopSize
	threshold := resolveThreshold(options)

	detector, err := NewDetector(hopSize, float32(threshold))
	if err != nil {
		return nil, fmt.Errorf("failed to create ten_vad detector: %w", err)
	}

	tv := &TenVAD{
		logger:               logger,
		onPacket:             onPacket,
		detector:             detector,
		hopSize:              hopSize,
		threshold:            float32(threshold),
		minSilenceDurationMs: resolveMinSilenceDurationMs(options),
		speechPadMs:          resolveSpeechPadMs(options),
		isTerminated:         false,
	}

	// Auto-close on context cancellation
	go func() {
		<-ctx.Done()
		tv.Close()
	}()

	if onPacket != nil {
		_ = onPacket(ctx, internal_type.ConversationEventPacket{
			Name: "vad",
			Data: map[string]string{
				"type":     "initialized",
				"provider": vadName,
				"init_ms":  fmt.Sprintf("%d", time.Since(start).Milliseconds()),
			},
			Time: time.Now(),
		})
	}

	return tv, nil
}

// Name returns the identifier for this VAD implementation.
func (t *TenVAD) Name() string {
	return vadName
}

// Process analyzes an audio packet for voice activity.
// The packet must contain 16 kHz LINEAR16 mono audio.
func (t *TenVAD) Process(ctx context.Context, pkt internal_type.UserAudioReceivedPacket) error {
	if !t.isActive() {
		return nil
	}

	// Convert bytes to int16 samples
	samples := internal_audio.Linear16ToInt16(pkt.Audio)
	// Process frame-by-frame under lock
	segments, err := t.processFrames(samples)
	if err != nil {
		return err
	}

	hasSpeechStart := false
	hasSpeechEnd := false
	var speechStartAt, speechEndAt float64
	for _, seg := range segments {
		if seg.startAt >= 0 && (!hasSpeechStart || seg.startAt < speechStartAt) {
			speechStartAt = seg.startAt
			hasSpeechStart = true
		}
		if seg.endAt >= 0 && (!hasSpeechEnd || seg.endAt > speechEndAt) {
			speechEndAt = seg.endAt
			hasSpeechEnd = true
		}
	}

	// Emit a heartbeat while the user is actively speaking so the EOS
	// silence timer keeps extending during sustained speech.
	t.mu.RLock()
	isSpeaking := t.triggered
	t.mu.RUnlock()
	if isSpeaking && t.onPacket != nil {
		_ = t.onPacket(ctx,
			internal_type.VadSpeechActivityPacket{},
		)
	}

	// Emit explicit interruption lifecycle events from VAD transitions.
	if hasSpeechStart {
		t.notifyInterruption(ctx, internal_type.InterruptionEventStart, speechStartAt, len(segments))
	}
	if hasSpeechEnd {
		t.notifyInterruption(ctx, internal_type.InterruptionEventEnd, speechEndAt, len(segments))
	}

	return nil
}

// Close releases the TEN VAD resources.
func (t *TenVAD) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isTerminated {
		return nil
	}
	t.isTerminated = true

	if t.detector != nil {
		t.detector.Close()
		t.detector = nil
	}

	return nil
}

// -----------------------------------------------------------------------------
// Private Methods
// -----------------------------------------------------------------------------

func (t *TenVAD) isActive() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return !t.isTerminated && t.detector != nil
}

// segment represents a detected speech region.
type segment struct {
	// startAt/endAt use -1 as "unset" sentinel so valid timestamp 0 is preserved.
	startAt float64
	endAt   float64
}

// processFrames slides a hop-sized window across the samples, calling
// the TEN VAD detector for each frame. Tracks speech onset/offset
// with the same hysteresis logic as Silero VAD for consistency.
func (t *TenVAD) processFrames(samples []int16) ([]segment, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isTerminated || t.detector == nil {
		return nil, nil
	}

	sampleRate := 16000
	minSilenceSamples := t.minSilenceDurationMs * sampleRate / 1000
	speechPadSamples := t.speechPadMs * sampleRate / 1000

	input := samples
	if len(t.pending) > 0 {
		combined := make([]int16, 0, len(t.pending)+len(samples))
		combined = append(combined, t.pending...)
		combined = append(combined, samples...)
		input = combined
	}

	fullSamples := (len(input) / t.hopSize) * t.hopSize
	if fullSamples == 0 {
		t.pending = append(t.pending[:0], input...)
		return nil, nil
	}

	if fullSamples < len(input) {
		t.pending = append(t.pending[:0], input[fullSamples:]...)
	} else {
		t.pending = t.pending[:0]
	}

	var segments []segment

	for i := 0; i < fullSamples; i += t.hopSize {
		frame := input[i : i+t.hopSize]

		probability, _, err := t.detector.Process(frame)
		if err != nil {
			return nil, fmt.Errorf("ten_vad process failed: %w", err)
		}

		t.currSample += t.hopSize

		// Speech resumes during silence measurement
		if probability >= t.threshold && t.tempEnd != 0 {
			t.tempEnd = 0
		}

		// Speech onset
		if probability >= t.threshold && !t.triggered {
			t.triggered = true
			speechStartAt := float64(t.currSample-t.hopSize-speechPadSamples) / float64(sampleRate)
			if speechStartAt < 0 {
				speechStartAt = 0
			}
			segments = append(segments, segment{startAt: speechStartAt, endAt: -1})
		}

		// Speech offset (with hysteresis)
		if probability < (t.threshold-0.15) && t.triggered {
			if t.tempEnd == 0 {
				t.tempEnd = t.currSample
			}

			if t.currSample-t.tempEnd < minSilenceSamples {
				continue
			}

			speechEndAt := float64(t.tempEnd+speechPadSamples) / float64(sampleRate)
			t.tempEnd = 0
			t.triggered = false

			// Speech started in a previous call — onset already reported
			if len(segments) == 0 {
				segments = append(segments, segment{startAt: -1, endAt: speechEndAt})
				continue
			}
			segments[len(segments)-1].endAt = speechEndAt
		}
	}

	return segments, nil
}

func (t *TenVAD) notifyInterruption(ctx context.Context, event internal_type.InterruptionEvent, at float64, segmentCount int) {
	if t.onPacket != nil {
		_ = t.onPacket(ctx,
			internal_type.InterruptionDetectedPacket{
				Source:  internal_type.InterruptionSourceVad,
				Event:   event,
				StartAt: at,
				EndAt:   at,
			},
			internal_type.ConversationEventPacket{
				Name: "vad",
				Data: map[string]string{
					"type":          "detected",
					"event":         string(event),
					"start_at":      fmt.Sprintf("%f", at),
					"end_at":        fmt.Sprintf("%f", at),
					"segment_count": fmt.Sprintf("%d", segmentCount),
				},
			},
		)
	}
}

func resolveThreshold(options utils.Option) float64 {
	if options == nil {
		return defaultThreshold
	}
	if threshold, err := options.GetFloat64("microphone.vad.threshold"); err == nil {
		return threshold
	}
	return defaultThreshold
}

// resolveMinSilenceDurationMs extracts min silence duration from options.
// The option key uses frame count (consistent with FireRedVAD config);
// each frame is 10 ms, so we multiply by 10 to get milliseconds.
func resolveMinSilenceDurationMs(options utils.Option) int {
	if options == nil {
		return defaultMinSilenceDurationMs
	}
	if v, err := options.GetFloat64("microphone.vad.min_silence_frame"); err == nil {
		return int(v) * 10
	}
	return defaultMinSilenceDurationMs
}

// resolveSpeechPadMs extracts speech pad duration from options.
// The option key uses frame count (consistent with FireRedVAD config);
// each frame is 10 ms, so we multiply by 10 to get milliseconds.
func resolveSpeechPadMs(options utils.Option) int {
	if options == nil {
		return defaultSpeechPadMs
	}
	if v, err := options.GetFloat64("microphone.vad.min_speech_frame"); err == nil {
		return int(v) * 10
	}
	return defaultSpeechPadMs
}
