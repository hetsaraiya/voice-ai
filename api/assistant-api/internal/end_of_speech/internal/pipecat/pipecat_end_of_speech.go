// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_pipecat

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

const (
	eosName = "pipecatSmartTurnEndOfSpeech"

	optPctThreshold       = "microphone.eos.threshold"
	optPctExtendedTimeout = "microphone.eos.extended_timeout"
	optPctQuickTimeout    = "microphone.eos.quick_timeout"
	optPctFallbackTimeout = "microphone.eos.fallback_timeout"

	// Backward-compatible aliases.
	optPctLegacySilenceTimeout = "microphone.eos.silence_timeout"
	optPctLegacyTimeout        = "microphone.eos.timeout"

	defaultPctThreshold      = 0.5
	defaultPctQuickTimeout   = 250.0
	defaultPctSilenceTimeout = 2000.0
	defaultPctFallbackMs     = 500.0

	maxAudioSamples = whisperMaxSamples
)

// SpeechSegment represents accumulated speech with metadata.
type SpeechSegment struct {
	ContextID string
	Text      string
	Timestamp time.Time
	Chunks    []internal_type.SpeechToTextPacket
}

type command struct {
	ctx     context.Context
	timeout time.Duration
	segment SpeechSegment
	fireNow bool
	reset   bool
}

type eosState struct {
	segment       SpeechSegment
	callbackFired bool
	generation    uint64
}

// PipecatEOS detects end-of-speech using the Pipecat Smart Turn audio model.
type PipecatEOS struct {
	logger   commons.Logger
	callback func(context.Context, ...internal_type.Packet) error

	detector *PipecatDetector
	// Optional test seam for deterministic timing tests.
	// When set, predictEOU uses this callback instead of detector.Predict.
	predictor func([]float32) (float64, error)

	// Configuration
	threshold      float64
	quickTimeout   time.Duration
	silenceTimeout time.Duration
	fallbackMs     time.Duration

	// Rolling audio buffer (protected by mu)
	audioBuf []float32

	// Worker orchestration
	cmdCh  chan command
	stopCh chan struct{}

	// State
	mu    sync.RWMutex
	state *eosState
}

func NewPipecatEndOfSpeech(
	logger commons.Logger,
	onCallback func(context.Context, ...internal_type.Packet) error,
	opts utils.Option,
) (internal_type.EndOfSpeech, error) {
	start := time.Now()

	cfg := PipecatDetectorConfig{}
	if v, err := opts.GetString("microphone.eos.pipecat.model_path"); err == nil {
		cfg.ModelPath = v
	}

	detector, err := NewPipecatDetector(cfg)
	if err != nil {
		return nil, fmt.Errorf("pipecat_eos: init detector: %w", err)
	}

	eos := &PipecatEOS{
		logger:         logger,
		callback:       onCallback,
		detector:       detector,
		threshold:      defaultPctThreshold,
		quickTimeout:   time.Duration(defaultPctQuickTimeout) * time.Millisecond,
		silenceTimeout: time.Duration(defaultPctSilenceTimeout) * time.Millisecond,
		fallbackMs:     time.Duration(defaultPctFallbackMs) * time.Millisecond,
		audioBuf:       make([]float32, 0, maxAudioSamples),
		cmdCh:          make(chan command, 32),
		stopCh:         make(chan struct{}),
		state:          &eosState{segment: SpeechSegment{}},
	}

	if v, err := opts.GetFloat64(optPctThreshold); err == nil {
		eos.threshold = v
	}
	if v, err := opts.GetFloat64(optPctExtendedTimeout); err == nil {
		eos.silenceTimeout = time.Duration(v) * time.Millisecond
	} else if v, err := opts.GetFloat64(optPctLegacySilenceTimeout); err == nil {
		eos.silenceTimeout = time.Duration(v) * time.Millisecond
	}
	if v, err := opts.GetFloat64(optPctQuickTimeout); err == nil {
		eos.quickTimeout = time.Duration(v) * time.Millisecond
	}
	if v, err := opts.GetFloat64(optPctFallbackTimeout); err == nil {
		eos.fallbackMs = time.Duration(v) * time.Millisecond
	} else if v, err := opts.GetFloat64(optPctLegacyTimeout); err == nil {
		eos.fallbackMs = time.Duration(v) * time.Millisecond
	}

	go eos.worker()

	if onCallback != nil {
		_ = onCallback(context.Background(), internal_type.ConversationEventPacket{
			Name: "eos",
			Data: map[string]string{
				"type":     "initialized",
				"provider": eosName,
				"init_ms":  fmt.Sprintf("%d", time.Since(start).Milliseconds()),
			},
			Time: time.Now(),
		})
	}

	return eos, nil
}

func (eos *PipecatEOS) Name() string {
	return eosName
}

// Analyze processes incoming packets. Matches silence-based behavior for
// packet handling: interim STT resets timer only, final STT accumulates
// text and emits InterimEndOfSpeechPacket. Audio packets are accumulated
// in a rolling buffer for model inference.
func (eos *PipecatEOS) Analyze(ctx context.Context, pkt internal_type.Packet) error {
	switch p := pkt.(type) {
	case internal_type.EndOfSpeechAudioPacket:
		eos.appendAudio(p.Audio)
	case internal_type.UserTextReceivedPacket:
		if p.Text == "" {
			return nil
		}
		eos.mu.Lock()
		seg := SpeechSegment{ContextID: p.ContextId(), Text: p.Text, Timestamp: time.Now()}
		eos.state.segment = seg
		eos.mu.Unlock()

		eos.callback(ctx,
			internal_type.InterimEndOfSpeechPacket{Speech: seg.Text, ContextID: seg.ContextID},
			internal_type.ConversationEventPacket{Name: "eos", Data: map[string]string{"type": "interim", "speech": seg.Text}},
		)
		eos.send(command{ctx: ctx, segment: seg, fireNow: true})

	case internal_type.EndOfSpeechInterruptionPacket:
		eos.mu.RLock()
		seg := eos.state.segment
		eos.mu.RUnlock()
		if seg.Text == "" {
			return nil
		}
		eos.send(command{ctx: ctx, segment: seg, timeout: eos.silenceTimeout})

	case internal_type.SpeechToTextPacket:
		eos.mu.Lock()
		if p.Interim {
			// Interim: just reset timer, no text accumulation, no interim packet.
			seg := eos.state.segment
			eos.mu.Unlock()
			if seg.Text == "" {
				return nil
			}
			eos.send(command{ctx: ctx, segment: seg, timeout: eos.fallbackMs})
			return nil
		}

		// Final transcript: accumulate text, fresh segment with packet's ContextID
		newSeg := SpeechSegment{
			ContextID: p.ContextId(),
			Timestamp: time.Now(),
			Text:      eos.state.segment.Text,
			Chunks:    append([]internal_type.SpeechToTextPacket(nil), eos.state.segment.Chunks...),
		}
		if newSeg.Text != "" {
			newSeg.Text = fmt.Sprintf("%s %s", newSeg.Text, p.Script)
		} else {
			newSeg.Text = p.Script
		}
		newSeg.Chunks = append(newSeg.Chunks, p)
		eos.state.segment = newSeg
		eos.mu.Unlock()

		if newSeg.Text == "" {
			return nil
		}

		// Emit interim update (same as silence-based on final STT)
		eos.callback(ctx,
			internal_type.InterimEndOfSpeechPacket{Speech: newSeg.Text, ContextID: newSeg.ContextID},
			internal_type.ConversationEventPacket{Name: "eos", Data: map[string]string{"type": "interim", "speech": newSeg.Text}},
		)

		// Run audio model inference.
		// YES (prob >= threshold) → quick_timeout buffer, then fire.
		// NO  (0 <= prob < threshold) → keep accumulating with silence_timeout.
		// FAIL (prob < 0) → fallback timeout (microphone.eos.fallback_timeout).
		prob := eos.predictEOU()
		switch {
		case prob < 0:
			eos.send(command{ctx: ctx, segment: newSeg, timeout: eos.fallbackMs})
		case prob >= eos.threshold:
			eos.send(command{ctx: ctx, segment: newSeg, timeout: eos.quickTimeout})
		default:
			eos.send(command{ctx: ctx, segment: newSeg, timeout: eos.silenceTimeout})
		}
	}

	return nil
}

// appendAudio converts PCM16 LE bytes to float32 and appends to the rolling buffer.
func (eos *PipecatEOS) appendAudio(pcm16 []byte) {
	if len(pcm16) < 2 {
		return
	}

	nSamples := len(pcm16) / 2
	samples := make([]float32, nSamples)
	for i := 0; i < nSamples; i++ {
		s := int16(binary.LittleEndian.Uint16(pcm16[i*2:]))
		samples[i] = float32(s) / 32768.0
	}

	eos.mu.Lock()
	eos.audioBuf = append(eos.audioBuf, samples...)
	if len(eos.audioBuf) > maxAudioSamples {
		excess := len(eos.audioBuf) - maxAudioSamples
		eos.audioBuf = eos.audioBuf[excess:]
	}
	eos.mu.Unlock()
}

// predictEOU runs the audio model and returns the turn-completion probability.
// Returns -1 on failure (caller should treat as "not done").
func (eos *PipecatEOS) predictEOU() float64 {
	eos.mu.RLock()
	audio := make([]float32, len(eos.audioBuf))
	copy(audio, eos.audioBuf)
	eos.mu.RUnlock()

	if len(audio) == 0 {
		if eos.logger != nil {
			eos.logger.Debugf("pipecat_eos: inference skipped: empty audio buffer")
		}
		return -1
	}

	if eos.predictor == nil && eos.detector == nil {
		if eos.logger != nil {
			eos.logger.Debugf("pipecat_eos: inference skipped: detector unavailable")
		}
		return -1
	}

	var (
		prob float64
		err  error
	)
	if eos.predictor != nil {
		prob, err = eos.predictor(audio)
	} else {
		prob, err = eos.detector.Predict(audio)
	}
	if err != nil {
		if eos.logger != nil {
			eos.logger.Debugf("pipecat_eos: inference failed: %v", err)
		}
		return -1
	}

	if eos.logger != nil {
		eos.logger.Debugf("pipecat_eos: P(complete)=%.4f threshold=%.4f audio_samples=%d", prob, eos.threshold, len(audio))
	}

	return prob
}

func (eos *PipecatEOS) send(cmd command) {
	select {
	case <-eos.stopCh:
		return
	default:
	}

	select {
	case eos.cmdCh <- cmd:
	default:
		go func() {
			select {
			case eos.cmdCh <- cmd:
			case <-eos.stopCh:
			}
		}()
	}
}

func (eos *PipecatEOS) worker() {
	var (
		timer   *time.Timer
		timerC  <-chan time.Time
		gen     uint64
		ctx     context.Context
		segment SpeechSegment
	)

	cleanup := func() {
		if timer != nil {
			timer.Stop()
			timer = nil
			timerC = nil
		}
	}

	for {
		select {
		case <-eos.stopCh:
			cleanup()
			return

		case cmd := <-eos.cmdCh:
			eos.mu.Lock()

			if cmd.reset {
				eos.state.callbackFired = false
				eos.state.generation++
				eos.state.segment = SpeechSegment{}
				eos.audioBuf = eos.audioBuf[:0]
				eos.mu.Unlock()
				continue
			}

			if eos.state.callbackFired {
				eos.mu.Unlock()
				continue
			}

			if cmd.fireNow {
				eos.state.callbackFired = true
				seg := eos.state.segment
				cbCtx := cmd.ctx
				cleanup()
				eos.mu.Unlock()
				eos.fire(cbCtx, seg)
				continue
			}

			gen = eos.state.generation + 1
			eos.state.generation = gen
			ctx = cmd.ctx
			segment = cmd.segment
			cleanup()
			timer = time.NewTimer(cmd.timeout)
			timerC = timer.C
			eos.mu.Unlock()

		case <-timerC:
			eos.mu.Lock()
			if eos.state.callbackFired || gen != eos.state.generation {
				eos.mu.Unlock()
				continue
			}

			eos.state.callbackFired = true
			seg := segment
			cbCtx := ctx
			cleanup()
			eos.mu.Unlock()
			eos.fire(cbCtx, seg)
		}
	}
}

// fire triggers the callback and enqueues reset.
// Always emits one EndOfSpeechPacket — matches silence-based behavior.
func (eos *PipecatEOS) fire(ctx context.Context, seg SpeechSegment) {
	if seg.Text == "" {
		return
	}

	if ctx.Err() != nil {
		ctx = context.Background()
	}

	wordCount := len(strings.Fields(seg.Text))
	triggerAt := time.Now()
	_ = eos.callback(ctx,
		internal_type.EndOfSpeechPacket{
			Speech:    seg.Text,
			ContextID: seg.ContextID,
			Speechs:   append([]internal_type.SpeechToTextPacket(nil), seg.Chunks...),
		},
		internal_type.UserMessageMetricPacket{
			ContextID: seg.ContextID,
			Metrics: []*protos.Metric{{
				Name:  "eos_latency_ms",
				Value: fmt.Sprintf("%d", triggerAt.Sub(seg.Timestamp).Milliseconds()),
			}},
		},
		internal_type.ConversationEventPacket{
			Name: "eos",
			Data: map[string]string{
				"type":               "detected",
				"provider":           eosName,
				"context_id":         seg.ContextID,
				"speech":             seg.Text,
				"word_count":         fmt.Sprintf("%d", wordCount),
				"char_count":         fmt.Sprintf("%d", len(seg.Text)),
				"text_to_trigger_ms": fmt.Sprintf("%d", triggerAt.Sub(seg.Timestamp).Milliseconds()),
			},
			Time: triggerAt,
		},
	)

	eos.send(command{reset: true})
}

func (eos *PipecatEOS) Close() error {
	close(eos.stopCh)
	if eos.detector != nil {
		eos.detector.Destroy()
		eos.detector = nil
	}
	return nil
}
