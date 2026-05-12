// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_firered_vad

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	vadName          = "firered_vad"
	envModelPathKey  = "FIRERED_VAD_MODEL_PATH"
	defaultModelFile = "models/fireredvad_stream_vad_with_cache.onnx"
)

// -----------------------------------------------------------------------------
// FireRedVAD — Voice Activity Detection using FireRedVAD DFSMN model
// -----------------------------------------------------------------------------

// FireRedVAD implements the Vad interface using the FireRedVAD ONNX streaming
// model. It performs Kaldi-compatible fbank feature extraction, CMVN
// normalisation, ONNX inference, and postprocessing on incoming 16 kHz
// LINEAR16 mono audio.
type FireRedVAD struct {
	logger   commons.Logger
	onPacket func(ctx context.Context, pkt ...internal_type.Packet) error

	detector      *Detector
	fbank         *FbankExtractor
	postprocessor *Postprocessor

	// Audio sample buffer for frame extraction
	audioBuf []int16

	mu           sync.RWMutex
	isTerminated bool
}

// NewFireRedVAD creates a new FireRedVAD instance.
// Input audio must be 16 kHz LINEAR16 mono — the platform's internal format.
func NewFireRedVAD(
	ctx context.Context,
	logger commons.Logger,
	onPacket func(ctx context.Context, pkt ...internal_type.Packet) error,
	options utils.Option,
) (internal_type.Vad, error) {
	start := time.Now()

	modelPath := resolveModelPath()
	detector, err := NewDetector(modelPath)
	if err != nil {
		return nil, fmt.Errorf("firered_vad: failed to create detector: %w", err)
	}

	ppCfg := resolvePostprocessorConfig(options)

	vad := &FireRedVAD{
		logger:        logger,
		onPacket:      onPacket,
		detector:      detector,
		fbank:         NewFbankExtractor(),
		postprocessor: NewPostprocessor(ppCfg),
		audioBuf:      make([]int16, 0, frameLenSample*2),
		isTerminated:  false,
	}

	go func() {
		<-ctx.Done()
		_ = vad.Close()
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

	return vad, nil
}

// -----------------------------------------------------------------------------
// Public Interface Methods
// -----------------------------------------------------------------------------

func (v *FireRedVAD) Name() string {
	return vadName
}

// Process analyses an audio packet for voice activity.
// The packet must contain 16 kHz LINEAR16 mono audio.
func (v *FireRedVAD) Process(ctx context.Context, pkt internal_type.UserAudioReceivedPacket) error {
	if !v.isActive() {
		return nil
	}

	// Convert LINEAR16 bytes to int16 samples
	samples := internal_audio.Linear16ToInt16(pkt.Audio)

	// Append to buffer
	v.mu.Lock()
	v.audioBuf = append(v.audioBuf, samples...)

	// Process complete frames (400 samples each, 160-sample shift)
	hasSpeech := false
	var speechStartAt, speechEndAt float64
	hasSpeechStart := false
	hasSpeechEnd := false

	for len(v.audioBuf) >= frameLenSample {
		frame := v.audioBuf[:frameLenSample]

		// Extract fbank features for this frame
		var feat [featDim]float32
		v.fbank.Extract(frame, feat[:])

		// Apply CMVN normalisation
		applyCMVN(feat[:])

		// Run ONNX inference
		if v.isTerminated || v.detector == nil {
			v.mu.Unlock()
			return nil
		}
		prob, err := v.detector.Infer(feat[:])
		if err != nil {
			v.mu.Unlock()
			return fmt.Errorf("firered_vad: inference failed: %w", err)
		}

		// Postprocess
		result := v.postprocessor.ProcessFrame(prob)

		if result.IsSpeechStart {
			startAt := float64(result.SpeechStartFrame-1) / float64(framesPerSecond)
			if startAt < 0 {
				startAt = 0
			}
			if !hasSpeechStart || startAt < speechStartAt {
				speechStartAt = startAt
			}
			hasSpeechStart = true
		}
		if result.IsSpeechEnd {
			endAt := float64(result.SpeechEndFrame-1) / float64(framesPerSecond)
			if endAt < 0 {
				endAt = 0
			}
			if !hasSpeechEnd || endAt > speechEndAt {
				speechEndAt = endAt
			}
			hasSpeechEnd = true
		}

		// Only treat as speech when the postprocessor has confirmed onset
		// (past MinSpeechFrame). Frames in statePossibleSpeech are
		// unconfirmed and likely noise — skip them.
		if v.postprocessor.InSpeech() {
			hasSpeech = true
		}

		// Shift by frameShiftSamp (160 samples)
		v.audioBuf = v.audioBuf[frameShiftSamp:]
	}
	v.mu.Unlock()

	// Emit a heartbeat while in confirmed speech so the EOS silence
	// timer keeps extending during sustained speech.
	if hasSpeech && v.onPacket != nil {
		_ = v.onPacket(ctx,
			internal_type.VadSpeechActivityPacket{},
		)
	}

	// Emit explicit interruption lifecycle events from VAD transitions.
	if hasSpeechStart {
		v.notifyInterruption(ctx, internal_type.InterruptionEventStart, speechStartAt)
	}
	if hasSpeechEnd {
		v.notifyInterruption(ctx, internal_type.InterruptionEventEnd, speechEndAt)
	}

	return nil
}

// Close terminates the VAD and releases all resources.
func (v *FireRedVAD) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.isTerminated {
		return nil
	}
	v.isTerminated = true

	if v.detector != nil {
		v.detector.Destroy()
		v.detector = nil
	}

	return nil
}

// -----------------------------------------------------------------------------
// Private Methods
// -----------------------------------------------------------------------------

func resolveModelPath() string {
	if envPath := os.Getenv(envModelPathKey); envPath != "" {
		return envPath
	}
	_, currentFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(currentFile), defaultModelFile)
}

func resolvePostprocessorConfig(options utils.Option) PostprocessorConfig {
	cfg := DefaultPostprocessorConfig()
	if options == nil {
		return cfg
	}
	if v, err := options.GetFloat64("microphone.vad.threshold"); err == nil {
		cfg.SpeechThreshold = float32(v)
	}
	if v, err := options.GetFloat64("microphone.vad.min_silence_frame"); err == nil {
		cfg.MinSilenceFrame = int(v)
	}
	if v, err := options.GetFloat64("microphone.vad.min_speech_frame"); err == nil {
		cfg.MinSpeechFrame = int(v)
	}
	return cfg
}

func (v *FireRedVAD) isActive() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return !v.isTerminated && v.detector != nil
}

func (v *FireRedVAD) notifyInterruption(ctx context.Context, event internal_type.InterruptionEvent, at float64) {
	if v.onPacket == nil {
		return
	}
	v.onPacket(ctx,
		internal_type.InterruptionDetectedPacket{
			Source:  internal_type.InterruptionSourceVad,
			Event:   event,
			StartAt: at,
			EndAt:   at,
		},
		internal_type.ConversationEventPacket{
			Name: "vad",
			Data: map[string]string{
				"type":     "detected",
				"event":    string(event),
				"start_at": fmt.Sprintf("%f", at),
				"end_at":   fmt.Sprintf("%f", at),
			},
		},
	)
}
