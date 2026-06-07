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

	"github.com/rapidaai/api/assistant-api/internal/observability"
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

	// defaultFallbackTimeout is the silence timeout for interim STT and inference failures.
	defaultFallbackTimeout = 500.0
)

type speechSegment struct {
	ContextID string
	Committed string // accumulated final transcripts
	Pending   string // latest interim transcript (not yet finalized)
	// Timestamp tracks the last text-bearing update for this segment.
	Timestamp time.Time
	Chunks    []internal_type.SpeechToTextPacket
}

func (segment speechSegment) FullText() string {
	if segment.Pending == "" {
		return segment.Committed
	}
	if segment.Committed == "" {
		return segment.Pending
	}
	if strings.HasSuffix(segment.Committed, " ") || strings.HasPrefix(segment.Pending, " ") {
		return segment.Committed + segment.Pending
	}
	return segment.Committed + " " + segment.Pending
}

type workerCommand struct {
	ctx             context.Context
	timeout         time.Duration
	segment         speechSegment
	confidence      float64
	fireImmediately bool
}

type endOfSpeechState struct {
	segment       speechSegment
	confidence    float64
	callbackFired bool
	generation    uint64
}

type turnPredictor interface {
	Predict(string) (float64, error)
}

// livekitEndOfSpeech detects end-of-speech using the LiveKit turn detector model
// with a hybrid approach: ONNX inference determines whether to use a quick
// or extended silence timeout, with fallback to standard silence on failure.
//
// Conversation history is built internally from packets flowing through
// Execute — user turns are recorded when EOS fires, and assistant turns
// are recorded from LLMResponseDonePacket.
type livekitEndOfSpeech struct {
	logger   commons.Logger
	onPacket func(context.Context, ...internal_type.Packet) error
	opts     utils.Option

	// Model-based turn detection
	predictor   turnPredictor
	predictorMu sync.Mutex

	// Conversation history built from packets (protected by mu)
	history []chatMessage

	// Configuration
	threshold       float64
	quickTimeout    time.Duration
	silenceTimeout  time.Duration
	fallbackTimeout time.Duration
	maxHistory      int

	// Worker orchestration
	commandCh chan workerCommand
	stopCh    chan struct{}

	// State
	mu    sync.RWMutex
	state *endOfSpeechState
}

func NewLivekitEndOfSpeech(
	logger commons.Logger,
	onPacket func(context.Context, ...internal_type.Packet) error,
	opts utils.Option,
) (internal_type.EndOfSpeechExecutor, error) {
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

	endOfSpeech := &livekitEndOfSpeech{
		logger:          logger,
		onPacket:        onPacket,
		opts:            opts,
		predictor:       detector,
		threshold:       defaultThreshold,
		quickTimeout:    time.Duration(defaultQuickTimeout) * time.Millisecond,
		silenceTimeout:  time.Duration(defaultSilenceTimeout) * time.Millisecond,
		fallbackTimeout: time.Duration(defaultFallbackTimeout) * time.Millisecond,
		maxHistory:      int(defaultMaxHistory),
		commandCh:       make(chan workerCommand, 32),
		stopCh:          make(chan struct{}),
		state:           &endOfSpeechState{segment: speechSegment{}},
	}

	if v, err := opts.GetFloat64(optKeyThreshold); err == nil {
		endOfSpeech.threshold = v
	}
	if v, err := opts.GetFloat64(optKeyExtendedTimeout); err == nil {
		endOfSpeech.silenceTimeout = time.Duration(v) * time.Millisecond
	} else if v, err := opts.GetFloat64(optKeyLegacySilenceTimeout); err == nil {
		endOfSpeech.silenceTimeout = time.Duration(v) * time.Millisecond
	}
	if v, err := opts.GetFloat64(optKeyQuickTimeout); err == nil {
		endOfSpeech.quickTimeout = time.Duration(v) * time.Millisecond
	}
	if v, err := opts.GetFloat64(optKeyMaxHistory); err == nil {
		endOfSpeech.maxHistory = int(v)
	}
	if v, err := opts.GetFloat64(optKeyFallbackTimeout); err == nil {
		endOfSpeech.fallbackTimeout = time.Duration(v) * time.Millisecond
	} else if v, err := opts.GetFloat64(optKeyLegacyTimeout); err == nil {
		endOfSpeech.fallbackTimeout = time.Duration(v) * time.Millisecond
	}

	go endOfSpeech.worker()
	if onPacket != nil {
		_ = onPacket(context.Background(),
			internal_type.ObservabilityEventRecordPacket{
				Scope: internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordEvent{
					Component: observability.ComponentEOS,
					Event:     observability.EOSStarted,
					Attributes: observability.Attributes{
						"provider":            eosName,
						"init_ms":             fmt.Sprintf("%d", time.Since(start).Milliseconds()),
						"threshold":           fmt.Sprintf("%.4f", endOfSpeech.threshold),
						"quick_timeout_ms":    fmt.Sprintf("%d", endOfSpeech.quickTimeout.Milliseconds()),
						"extended_timeout_ms": fmt.Sprintf("%d", endOfSpeech.silenceTimeout.Milliseconds()),
						"fallback_timeout_ms": fmt.Sprintf("%d", endOfSpeech.fallbackTimeout.Milliseconds()),
						"max_history":         fmt.Sprintf("%d", endOfSpeech.maxHistory),
					},
				},
			},
			internal_type.ObservabilityLogRecordPacket{
				Scope: internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelDebug,
					Message: "eos initialized",
					Attributes: observability.Attributes{
						"component":           observability.ComponentEOS.String(),
						"operation":           "initialize",
						"provider":            eosName,
						"init_ms":             fmt.Sprintf("%d", time.Since(start).Milliseconds()),
						"threshold":           fmt.Sprintf("%.4f", endOfSpeech.threshold),
						"quick_timeout_ms":    fmt.Sprintf("%d", endOfSpeech.quickTimeout.Milliseconds()),
						"extended_timeout_ms": fmt.Sprintf("%d", endOfSpeech.silenceTimeout.Milliseconds()),
						"fallback_timeout_ms": fmt.Sprintf("%d", endOfSpeech.fallbackTimeout.Milliseconds()),
						"max_history":         fmt.Sprintf("%d", endOfSpeech.maxHistory),
					},
				},
			},
		)
	}

	return endOfSpeech, nil
}

func (endOfSpeech *livekitEndOfSpeech) Name() string {
	return eosName
}

func (endOfSpeech *livekitEndOfSpeech) Options() utils.Option {
	return endOfSpeech.opts
}

func (endOfSpeech *livekitEndOfSpeech) Arguments() (map[string]string, error) {
	return map[string]string{}, nil
}

func (endOfSpeech *livekitEndOfSpeech) Execute(ctx context.Context, packet internal_type.Packet) error {
	switch packet := packet.(type) {
	case internal_type.EndOfSpeechAudioPacket:
		return nil
	case internal_type.UserTextReceivedPacket:
		if packet.Text == "" {
			return nil
		}
		endOfSpeech.mu.Lock()
		segment := speechSegment{ContextID: packet.ContextId(), Committed: packet.Text, Timestamp: time.Now()}
		endOfSpeech.state.segment = segment
		endOfSpeech.state.confidence = 0
		endOfSpeech.mu.Unlock()

		packets := []internal_type.Packet{internal_type.InterimEndOfSpeechPacket{
			Speech:    segment.Committed,
			ContextID: segment.ContextID,
		}}
		_ = endOfSpeech.onPacket(ctx, packets...)
		endOfSpeech.enqueueCommand(workerCommand{
			ctx:             ctx,
			segment:         segment,
			fireImmediately: true,
		})

	case internal_type.EndOfSpeechInterruptionPacket:
		endOfSpeech.mu.RLock()
		segment := endOfSpeech.state.segment
		confidence := endOfSpeech.state.confidence
		endOfSpeech.mu.RUnlock()
		if segment.FullText() == "" {
			return nil
		}
		endOfSpeech.enqueueCommand(workerCommand{
			ctx:        ctx,
			segment:    segment,
			confidence: confidence,
			timeout:    endOfSpeech.silenceTimeout,
		})

	case internal_type.VadSpeechActivityPacket:
		endOfSpeech.mu.RLock()
		segment := endOfSpeech.state.segment
		confidence := endOfSpeech.state.confidence
		endOfSpeech.mu.RUnlock()
		if segment.FullText() == "" {
			return nil
		}
		endOfSpeech.enqueueCommand(workerCommand{
			ctx:        ctx,
			segment:    segment,
			confidence: confidence,
			timeout:    endOfSpeech.silenceTimeout,
		})

	case internal_type.SpeechToTextPacket:
		endOfSpeech.mu.Lock()
		if packet.Interim {
			// Interim: just reset timer, no text accumulation, no interim packet.
			// Matches silence-based behavior.
			segment := endOfSpeech.state.segment
			confidence := endOfSpeech.state.confidence
			endOfSpeech.mu.Unlock()
			if segment.FullText() == "" {
				return nil
			}
			endOfSpeech.enqueueCommand(workerCommand{
				ctx:        ctx,
				segment:    segment,
				confidence: confidence,
				timeout:    endOfSpeech.fallbackTimeout,
			})
			return nil
		}

		// Final transcript: accumulate text
		segment := speechSegment{
			ContextID: packet.ContextId(),
			Timestamp: time.Now(),
			Committed: endOfSpeech.state.segment.Committed,
			Chunks:    append([]internal_type.SpeechToTextPacket(nil), endOfSpeech.state.segment.Chunks...),
		}
		if segment.Committed != "" {
			segment.Committed += packet.GetConcat()
			segment.Committed += packet.Script
		} else {
			segment.Committed = packet.Script
		}
		segment.Chunks = append(segment.Chunks, packet)
		endOfSpeech.state.segment = segment
		endOfSpeech.state.confidence = 0
		fullText := segment.FullText()
		endOfSpeech.mu.Unlock()

		if fullText == "" {
			return nil
		}

		// Emit interim update (same as silence-based on final STT)
		packets := []internal_type.Packet{internal_type.InterimEndOfSpeechPacket{
			Speech:    fullText,
			ContextID: segment.ContextID,
		}}
		_ = endOfSpeech.onPacket(ctx, packets...)

		// Run model inference on accumulated final text.
		// YES (prob >= threshold) → quick_timeout buffer, then fire.
		// NO  (prob <  threshold) → keep accumulating, safety timer as fallback.
		probability := endOfSpeech.predictEOU(fullText)
		command := workerCommand{
			ctx:     ctx,
			segment: segment,
		}
		if probability < 0 {
			command.timeout = endOfSpeech.fallbackTimeout
		} else {
			command.confidence = probability
			endOfSpeech.mu.Lock()
			endOfSpeech.state.confidence = probability
			endOfSpeech.mu.Unlock()
			if probability >= endOfSpeech.threshold {
				command.timeout = endOfSpeech.quickTimeout
			} else {
				command.timeout = endOfSpeech.silenceTimeout
			}
		}
		endOfSpeech.enqueueCommand(command)

	case internal_type.LLMResponseDonePacket:
		if packet.Text != "" {
			endOfSpeech.mu.Lock()
			endOfSpeech.history = append(endOfSpeech.history, chatMessage{Role: "assistant", Content: packet.Text})
			endOfSpeech.mu.Unlock()
		}
	}

	return nil
}

// predictEOU runs the turn detection model and returns the end-of-utterance
// probability. Returns -1 on failure (caller should treat as "not done").
func (endOfSpeech *livekitEndOfSpeech) predictEOU(currentText string) float64 {
	endOfSpeech.mu.RLock()
	history := make([]chatMessage, len(endOfSpeech.history))
	copy(history, endOfSpeech.history)
	endOfSpeech.mu.RUnlock()

	chatText := formatChatTemplateFromHistory(history, currentText, endOfSpeech.maxHistory)
	if chatText == "" {
		return -1
	}

	endOfSpeech.predictorMu.Lock()
	defer endOfSpeech.predictorMu.Unlock()

	if endOfSpeech.predictor == nil {
		return -1
	}

	probability, err := endOfSpeech.predictor.Predict(chatText)
	if err != nil {
		if endOfSpeech.logger != nil {
			endOfSpeech.logger.Debugf("livekit_eos: inference failed: %v", err)
		}
		return -1
	}

	if endOfSpeech.logger != nil {
		endOfSpeech.logger.Debugf("livekit_eos: P(eou)=%.4f threshold=%.4f text=%q", probability, endOfSpeech.threshold, currentText)
	}

	return probability
}

func (endOfSpeech *livekitEndOfSpeech) enqueueCommand(command workerCommand) {
	select {
	case <-endOfSpeech.stopCh:
		return
	default:
	}

	select {
	case endOfSpeech.commandCh <- command:
	case <-endOfSpeech.stopCh:
	}
}

func (endOfSpeech *livekitEndOfSpeech) worker() {
	var (
		timer          *time.Timer
		timerCh        <-chan time.Time
		timerArmedAt   time.Time
		generation     uint64
		currentCommand workerCommand
	)

	stopTimer := func() {
		if timer != nil {
			timer.Stop()
			timer = nil
			timerCh = nil
		}
		timerArmedAt = time.Time{}
	}
	resetState := func() {
		endOfSpeech.state.callbackFired = false
		endOfSpeech.state.generation++
		endOfSpeech.state.segment = speechSegment{}
		endOfSpeech.state.confidence = 0
	}

	for {
		select {
		case <-endOfSpeech.stopCh:
			stopTimer()
			return

		case command := <-endOfSpeech.commandCh:
			endOfSpeech.mu.Lock()

			if endOfSpeech.state.callbackFired {
				endOfSpeech.mu.Unlock()
				continue
			}

			if command.fireImmediately {
				endOfSpeech.state.callbackFired = true
				currentCommand = command
				stopTimer()
				endOfSpeech.mu.Unlock()
				endOfSpeech.fire(currentCommand, time.Now())
				endOfSpeech.mu.Lock()
				resetState()
				endOfSpeech.mu.Unlock()
				continue
			}

			generation = endOfSpeech.state.generation + 1
			endOfSpeech.state.generation = generation
			currentCommand = command
			stopTimer()
			timerArmedAt = time.Now()
			timer = time.NewTimer(command.timeout)
			timerCh = timer.C
			endOfSpeech.mu.Unlock()

		case <-timerCh:
			endOfSpeech.mu.Lock()
			if endOfSpeech.state.callbackFired || generation != endOfSpeech.state.generation {
				endOfSpeech.mu.Unlock()
				continue
			}

			endOfSpeech.state.callbackFired = true
			command := currentCommand
			armedAt := timerArmedAt
			stopTimer()
			endOfSpeech.mu.Unlock()
			endOfSpeech.fire(command, armedAt)
			endOfSpeech.mu.Lock()
			resetState()
			endOfSpeech.mu.Unlock()
		}
	}
}

func (endOfSpeech *livekitEndOfSpeech) fire(command workerCommand, timerArmedAt time.Time) {
	ctx := command.ctx
	segment := command.segment
	confidence := command.confidence
	speech := segment.FullText()
	if speech == "" {
		return
	}

	// Record user turn in conversation history
	endOfSpeech.mu.Lock()
	endOfSpeech.history = append(endOfSpeech.history, chatMessage{Role: "user", Content: speech})
	endOfSpeech.mu.Unlock()

	if confidence < 0 {
		confidence = 0
	}
	if ctx != nil && ctx.Err() != nil {
		ctx = context.Background()
	}

	wordCount := len(strings.Fields(speech))
	triggerAt := time.Now()
	textToTriggerMs := triggerAt.Sub(segment.Timestamp).Milliseconds()
	waitToTriggerMs := textToTriggerMs
	if !timerArmedAt.IsZero() {
		waitToTriggerMs = triggerAt.Sub(timerArmedAt).Milliseconds()
	}
	_ = endOfSpeech.onPacket(ctx,
		internal_type.EndOfSpeechPacket{
			Speech:    speech,
			ContextID: segment.ContextID,
			Speechs:   append([]internal_type.SpeechToTextPacket(nil), segment.Chunks...),
		},
		internal_type.ObservabilityEventRecordPacket{
			ContextID:   segment.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordEvent{
				Component:  observability.ComponentEOS,
				Event:      observability.EOSCompleted,
				OccurredAt: triggerAt,
				Attributes: observability.Attributes{
					"provider":           eosName,
					"context_id":         segment.ContextID,
					"speech":             speech,
					"confidence":         fmt.Sprintf("%.4f", confidence),
					"word_count":         fmt.Sprintf("%d", wordCount),
					"char_count":         fmt.Sprintf("%d", len(speech)),
					"text_to_trigger_ms": fmt.Sprintf("%d", textToTriggerMs),
					"wait_to_trigger_ms": fmt.Sprintf("%d", waitToTriggerMs),
				},
			},
		},
		internal_type.ObservabilityMetricRecordPacket{
			ContextID:   segment.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordMetric{
				OccurredAt: triggerAt,
				Metrics: []*protos.Metric{
					{Name: observability.MetricEOSLatencyMs, Value: fmt.Sprintf("%d", waitToTriggerMs)},
					{Name: observability.MetricEOSTextToTriggerMs, Value: fmt.Sprintf("%d", textToTriggerMs)},
					{Name: observability.MetricEOSWordCount, Value: fmt.Sprintf("%d", wordCount)},
					{Name: observability.MetricEOSCharCount, Value: fmt.Sprintf("%d", len(speech))},
					{Name: observability.MetricEOSConfidence, Value: fmt.Sprintf("%.4f", confidence)},
				},
			},
		})
}

func (endOfSpeech *livekitEndOfSpeech) Close(ctx context.Context) error {
	if endOfSpeech.onPacket != nil {
		_ = endOfSpeech.onPacket(ctx,
			internal_type.ObservabilityEventRecordPacket{
				Scope: internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordEvent{
					Component: observability.ComponentEOS,
					Event:     observability.EOSClosed,
					Attributes: observability.Attributes{
						"provider": endOfSpeech.Name(),
					},
				},
			},
			internal_type.ObservabilityLogRecordPacket{
				Scope: internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelDebug,
					Message: "eos closed",
					Attributes: observability.Attributes{
						"component": observability.ComponentEOS.String(),
						"operation": "close",
						"provider":  endOfSpeech.Name(),
					},
				},
			},
		)
	}
	close(endOfSpeech.stopCh)

	endOfSpeech.predictorMu.Lock()
	if predictor, ok := endOfSpeech.predictor.(interface{ Destroy() }); ok {
		predictor.Destroy()
	}
	endOfSpeech.predictor = nil
	endOfSpeech.predictorMu.Unlock()

	return nil
}
