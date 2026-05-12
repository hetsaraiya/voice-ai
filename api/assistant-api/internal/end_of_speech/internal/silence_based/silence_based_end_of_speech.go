// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapidsilenceEOS.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapidsilenceEOS.ai for commercial usage.
package internal_silence_based

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

// SpeechSegment represents accumulated speech with metadata
type SpeechSegment struct {
	ContextID string
	Text      string
	Chunks    []internal_type.SpeechToTextPacket
	Timestamp time.Time
}

// command defines operations for the worker goroutine
type command struct {
	ctx     context.Context
	timeout time.Duration
	segment SpeechSegment
	fireNow bool
	reset   bool
}

// SilenceBasedEOS detects end-of-speech based on silence duration
type SilenceBasedEOS struct {
	logger         commons.Logger
	callback       func(context.Context, ...internal_type.Packet) error
	silenceTimeout time.Duration

	// worker orchestration
	cmdCh  chan command
	stopCh chan struct{}

	// state
	mu    sync.RWMutex
	state *eosState
}

// eosState holds protected state for end-of-speech detection
type eosState struct {
	segment       SpeechSegment
	callbackFired bool
	generation    uint64
}

// NewSilenceBasedEOS creates a new silence-based end-of-speech detector
func NewSilenceBasedEndOfSpeech(logger commons.Logger, callback func(context.Context, ...internal_type.Packet) error, opts utils.Option,
) (internal_type.EndOfSpeech, error) {
	start := time.Now()
	threshold := 1000 * time.Millisecond
	if v, err := opts.GetFloat64("microphone.eos.timeout"); err == nil {
		threshold = time.Duration(v) * time.Millisecond
	}
	eos := &SilenceBasedEOS{
		logger:         logger,
		callback:       callback,
		silenceTimeout: threshold,
		cmdCh:          make(chan command, 32),
		stopCh:         make(chan struct{}),
		state: &eosState{
			segment: SpeechSegment{},
		},
	}

	go eos.worker()

	if callback != nil {
		_ = callback(context.Background(), internal_type.ConversationEventPacket{
			Name: "eos",
			Data: map[string]string{
				"type":     "initialized",
				"provider": eos.Name(),
				"init_ms":  fmt.Sprintf("%d", time.Since(start).Milliseconds()),
			},
			Time: time.Now(),
		})
	}

	return eos, nil
}

// Name returns the component name
func (eos *SilenceBasedEOS) Name() string {
	return "silenceBasedEndOfSpeech"
}

// Analyze processes incoming speech packets
func (eos *SilenceBasedEOS) Analyze(ctx context.Context, pkt internal_type.Packet) error {
	switch p := pkt.(type) {
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
		eos.send(command{
			ctx:     ctx,
			segment: seg,
			timeout: eos.silenceTimeout,
		})

	case internal_type.VadSpeechActivityPacket:
		eos.mu.RLock()
		seg := eos.state.segment
		eos.mu.RUnlock()

		if seg.Text == "" {
			return nil
		}
		eos.send(command{
			ctx:     ctx,
			segment: seg,
			timeout: eos.silenceTimeout,
		})

	case internal_type.SpeechToTextPacket:
		eos.mu.Lock()
		if p.Interim {
			seg := eos.state.segment
			eos.mu.Unlock()
			// ignore interim with no text
			if seg.Text == "" {
				return nil
			}
			//
			eos.send(command{
				ctx:     ctx,
				segment: seg,
				timeout: eos.silenceTimeout,
			})

			return nil
		}

		newSeg := SpeechSegment{
			ContextID: p.ContextId(),
			Timestamp: time.Now(),
			Text:      eos.state.segment.Text,
			Chunks:    append([]internal_type.SpeechToTextPacket(nil), eos.state.segment.Chunks...),
		}
		if newSeg.Text != "" {
			newSeg.Text = fmt.Sprintf("%s %s", eos.state.segment.Text, p.Script)
		} else {
			newSeg.Text = p.Script
		}
		newSeg.Chunks = append(newSeg.Chunks, p)
		eos.state.segment = newSeg
		eos.mu.Unlock()

		// let the client know about interim speech
		eos.callback(ctx,
			internal_type.InterimEndOfSpeechPacket{Speech: newSeg.Text, ContextID: newSeg.ContextID},
			internal_type.ConversationEventPacket{
				Name: "eos",
				Data: map[string]string{"type": "interim", "speech": newSeg.Text},
			},
		)

		// trigger the command to reset timer
		eos.send(command{
			ctx:     ctx,
			segment: newSeg,
			timeout: eos.silenceTimeout,
		})

	}

	return nil
}

// send dispatches a command to the worker
func (eos *SilenceBasedEOS) send(cmd command) {
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

// worker manages silence detection and callback invocation
func (eos *SilenceBasedEOS) worker() {
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

			// handle reset
			if cmd.reset {
				eos.state.callbackFired = false
				eos.state.generation++
				eos.state.segment = SpeechSegment{}
				eos.mu.Unlock()
				continue
			}

			// drop if callback pending
			if eos.state.callbackFired {
				eos.mu.Unlock()
				continue
			}

			// immediate fire
			if cmd.fireNow {
				eos.state.callbackFired = true
				seg := eos.state.segment
				cbCtx := cmd.ctx
				cleanup()
				eos.mu.Unlock()
				eos.fire(cbCtx, seg)
				continue
			}

			// schedule timer
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
			// stale timer check
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

// fire triggers the callback and enqueues reset
func (eos *SilenceBasedEOS) fire(ctx context.Context, seg SpeechSegment) {
	if seg.Text == "" {
		return
	}

	// Use a detached context for the callback. The original context from the
	// Analyze call is typically cancelled by the time the silence timer fires,
	// which would silently prevent the end-of-speech callback from ever being
	// invoked. Since EOS detection is inherently asynchronous (timer-based),
	// the callback must not depend on the original request context's lifetime.
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

// Close shuts down the detector
func (eos *SilenceBasedEOS) Close() error {
	close(eos.stopCh)
	return nil
}
