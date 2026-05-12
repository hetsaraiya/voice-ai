// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_livekit

import (
	"context"
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
	eosName = "livekitEndOfSpeech"

	optKeyThreshold       = "microphone.eos.threshold"
	optKeyQuickTimeout    = "microphone.eos.quick_timeout"
	optKeyExtendedTimeout = "microphone.eos.extended_timeout"
	optKeyFallbackTimeout = "microphone.eos.fallback_timeout"
	optKeyMaxHistory      = "microphone.eos.max_history_turns"

	// Backward-compatible aliases.
	optKeyLegacySilenceTimeout = "microphone.eos.silence_timeout"
	optKeyLegacyTimeout        = "microphone.eos.timeout"

	// defaultThreshold is the English "unlikely_threshold" from LiveKit's
	// languages.json. Probabilities below this → user still speaking.
	defaultThreshold = 0.0289

	// defaultSilenceTimeout (max_endpointing_delay) — used when model predicts
	// user is still speaking (prob < threshold). LiveKit default: 3.0s.
	defaultSilenceTimeout = 3000.0

	// defaultQuickTimeout — short buffer after model says YES before firing.
	defaultQuickTimeout = 250.0

	// defaultMaxHistory matches LiveKit's MAX_HISTORY_TURNS = 6.
	defaultMaxHistory = 6.0

	// defaultFallbackMs is the silence timeout for interim STT and inference failures.
	defaultFallbackMs = 500.0
)

// SpeechSegment represents accumulated speech with metadata.
type SpeechSegment struct {
	ContextID string
	Committed string // accumulated final transcripts
	Pending   string // latest interim transcript (not yet finalized)
	Timestamp time.Time
	Chunks    []internal_type.SpeechToTextPacket
}

// FullText returns the complete transcript including any pending interim text.
func (s SpeechSegment) FullText() string {
	if s.Pending == "" {
		return s.Committed
	}
	if s.Committed == "" {
		return s.Pending
	}
	return s.Committed + " " + s.Pending
}

// command defines operations for the worker goroutine.
type command struct {
	ctx     context.Context
	timeout time.Duration
	segment SpeechSegment
	fireNow bool
	reset   bool
}

// eosState holds protected state for end-of-speech detection.
type eosState struct {
	segment       SpeechSegment
	callbackFired bool
	generation    uint64
}

// LivekitEOS detects end-of-speech using the LiveKit turn detector model
// with a hybrid approach: ONNX inference determines whether to use a quick
// or extended silence timeout, with fallback to standard silence on failure.
//
// Conversation history is built internally from packets flowing through
// Analyze — user turns are recorded when EOS fires, and assistant turns
// are recorded from LLMResponseDonePacket.
type LivekitEOS struct {
	logger   commons.Logger
	callback func(context.Context, ...internal_type.Packet) error

	// Model-based turn detection
	detector *TurnDetector

	// Conversation history built from packets (protected by mu)
	history []chatMessage

	// Configuration
	threshold      float64
	quickTimeout   time.Duration
	silenceTimeout time.Duration
	fallbackMs     time.Duration
	maxHistory     int

	// Worker orchestration
	cmdCh  chan command
	stopCh chan struct{}

	// State
	mu    sync.RWMutex
	state *eosState
}

// NewLivekitEndOfSpeech creates a new LiveKit model-based end-of-speech detector.
func NewLivekitEndOfSpeech(
	logger commons.Logger,
	onCallback func(context.Context, ...internal_type.Packet) error,
	opts utils.Option,
) (internal_type.EndOfSpeech, error) {
	start := time.Now()

	cfg := TurnDetectorConfig{ModelType: "en"}
	if v, err := opts.GetString("microphone.eos.model"); err == nil && v != "" {
		cfg.ModelType = v
	}
	if v, err := opts.GetString("microphone.eos.livekit.model_path"); err == nil {
		cfg.ModelPath = v
	}
	if v, err := opts.GetString("microphone.eos.livekit.tokenizer_path"); err == nil {
		cfg.TokenizerPath = v
	}

	detector, err := NewTurnDetector(cfg)
	if err != nil {
		return nil, fmt.Errorf("livekit_eos: init turn detector: %w", err)
	}

	eos := &LivekitEOS{
		logger:         logger,
		callback:       onCallback,
		detector:       detector,
		threshold:      defaultThreshold,
		quickTimeout:   time.Duration(defaultQuickTimeout) * time.Millisecond,
		silenceTimeout: time.Duration(defaultSilenceTimeout) * time.Millisecond,
		fallbackMs:     time.Duration(defaultFallbackMs) * time.Millisecond,
		maxHistory:     int(defaultMaxHistory),
		cmdCh:          make(chan command, 32),
		stopCh:         make(chan struct{}),
		state:          &eosState{segment: SpeechSegment{}},
	}

	if v, err := opts.GetFloat64(optKeyThreshold); err == nil {
		eos.threshold = v
	}
	if v, err := opts.GetFloat64(optKeyExtendedTimeout); err == nil {
		eos.silenceTimeout = time.Duration(v) * time.Millisecond
	} else if v, err := opts.GetFloat64(optKeyLegacySilenceTimeout); err == nil {
		eos.silenceTimeout = time.Duration(v) * time.Millisecond
	}
	if v, err := opts.GetFloat64(optKeyQuickTimeout); err == nil {
		eos.quickTimeout = time.Duration(v) * time.Millisecond
	}
	if v, err := opts.GetFloat64(optKeyMaxHistory); err == nil {
		eos.maxHistory = int(v)
	}
	if v, err := opts.GetFloat64(optKeyFallbackTimeout); err == nil {
		eos.fallbackMs = time.Duration(v) * time.Millisecond
	} else if v, err := opts.GetFloat64(optKeyLegacyTimeout); err == nil {
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

// Name returns the component name.
func (eos *LivekitEOS) Name() string {
	return eosName
}

// Analyze processes incoming packets using the hybrid turn detection model.
// In addition to the standard EOS packet types (UserTextReceivedPacket, EndOfSpeechInterruptionPacket,
// SpeechToTextPacket), it also observes LLMResponseDonePacket to build
// conversation history for context-aware turn prediction.
func (eos *LivekitEOS) Analyze(ctx context.Context, pkt internal_type.Packet) error {
	switch p := pkt.(type) {
	case internal_type.EndOfSpeechAudioPacket:
		return nil
	case internal_type.UserTextReceivedPacket:
		if p.Text == "" {
			return nil
		}
		eos.mu.Lock()
		seg := SpeechSegment{ContextID: p.ContextId(), Committed: p.Text, Timestamp: time.Now()}
		eos.state.segment = seg
		eos.mu.Unlock()

		eos.callback(ctx,
			internal_type.InterimEndOfSpeechPacket{Speech: seg.Committed, ContextID: seg.ContextID},
			internal_type.ConversationEventPacket{Name: "eos", Data: map[string]string{"type": "interim", "speech": seg.Committed}},
		)
		eos.send(command{
			ctx:     ctx,
			segment: seg,
			fireNow: true,
		})

	case internal_type.EndOfSpeechInterruptionPacket:
		eos.mu.RLock()
		seg := eos.state.segment
		eos.mu.RUnlock()
		if seg.FullText() == "" {
			return nil
		}
		eos.send(command{ctx: ctx, segment: seg, timeout: eos.silenceTimeout})

	case internal_type.SpeechToTextPacket:
		eos.mu.Lock()
		if p.Interim {
			// Interim: just reset timer, no text accumulation, no interim packet.
			// Matches silence-based behavior.
			seg := eos.state.segment
			eos.mu.Unlock()
			if seg.FullText() == "" {
				return nil
			}
			eos.send(command{ctx: ctx, segment: seg, timeout: eos.fallbackMs})
			return nil
		}

		// Final transcript: accumulate text
		newSeg := SpeechSegment{
			ContextID: p.ContextId(),
			Timestamp: time.Now(),
			Committed: eos.state.segment.Committed,
			Chunks:    append([]internal_type.SpeechToTextPacket(nil), eos.state.segment.Chunks...),
		}
		if newSeg.Committed != "" {
			newSeg.Committed = fmt.Sprintf("%s %s", newSeg.Committed, p.Script)
		} else {
			newSeg.Committed = p.Script
		}
		newSeg.Chunks = append(newSeg.Chunks, p)
		eos.state.segment = newSeg
		fullText := newSeg.FullText()
		eos.mu.Unlock()

		if fullText == "" {
			return nil
		}

		// Emit interim update (same as silence-based on final STT)
		eos.callback(ctx,
			internal_type.InterimEndOfSpeechPacket{Speech: fullText, ContextID: newSeg.ContextID},
			internal_type.ConversationEventPacket{Name: "eos", Data: map[string]string{"type": "interim", "speech": fullText}},
		)

		// Run model inference on accumulated final text.
		// YES (prob >= threshold) → quick_timeout buffer, then fire.
		// NO  (prob <  threshold) → keep accumulating, safety timer as fallback.
		prob := eos.predictEOU(fullText)
		if prob >= eos.threshold {
			eos.send(command{ctx: ctx, segment: newSeg, timeout: eos.quickTimeout})
		} else {
			eos.send(command{ctx: ctx, segment: newSeg, timeout: eos.silenceTimeout})
		}

	case internal_type.LLMResponseDonePacket:
		if p.Text != "" {
			eos.mu.Lock()
			eos.history = append(eos.history, chatMessage{Role: "assistant", Content: p.Text})
			eos.mu.Unlock()
		}
	}

	return nil
}

// predictEOU runs the turn detection model and returns the end-of-utterance
// probability. Returns -1 on failure (caller should treat as "not done").
func (eos *LivekitEOS) predictEOU(currentText string) float64 {
	eos.mu.RLock()
	history := make([]chatMessage, len(eos.history))
	copy(history, eos.history)
	eos.mu.RUnlock()

	chatText := formatChatTemplateFromHistory(history, currentText, eos.maxHistory)
	if chatText == "" {
		return -1
	}

	prob, err := eos.detector.Predict(chatText)
	if err != nil {
		if eos.logger != nil {
			eos.logger.Debugf("livekit_eos: inference failed: %v", err)
		}
		return -1
	}

	if eos.logger != nil {
		eos.logger.Debugf("livekit_eos: P(eou)=%.4f threshold=%.4f text=%q", prob, eos.threshold, currentText)
	}

	return prob
}

// send dispatches a command to the worker.
func (eos *LivekitEOS) send(cmd command) {
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

// worker manages silence detection and callback invocation.
// Same pattern as the silence-based EOS: single goroutine, timer, generation counter.
func (eos *LivekitEOS) worker() {
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

// fire triggers the callback, records the user turn in history, and enqueues reset.
// Matches silence-based behavior: always emits one EndOfSpeechPacket + event, then resets.
func (eos *LivekitEOS) fire(ctx context.Context, seg SpeechSegment) {
	speech := seg.FullText()
	if speech == "" {
		return
	}

	// Record user turn in conversation history
	eos.mu.Lock()
	eos.history = append(eos.history, chatMessage{Role: "user", Content: speech})
	eos.mu.Unlock()

	if ctx.Err() != nil {
		ctx = context.Background()
	}

	wordCount := len(strings.Fields(speech))
	triggerAt := time.Now()
	_ = eos.callback(ctx,
		internal_type.EndOfSpeechPacket{
			Speech:    speech,
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
				"speech":             speech,
				"word_count":         fmt.Sprintf("%d", wordCount),
				"char_count":         fmt.Sprintf("%d", len(speech)),
				"text_to_trigger_ms": fmt.Sprintf("%d", triggerAt.Sub(seg.Timestamp).Milliseconds()),
			},
			Time: triggerAt,
		},
	)

	eos.send(command{reset: true})
}

// Close shuts down the detector and releases ONNX resources.
func (eos *LivekitEOS) Close() error {
	close(eos.stopCh)
	if eos.detector != nil {
		eos.detector.Destroy()
		eos.detector = nil
	}
	return nil
}
