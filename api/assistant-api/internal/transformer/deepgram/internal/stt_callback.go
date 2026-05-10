// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package deepgram_internal

import (
	"fmt"
	"strings"
	"time"

	msginterfaces "github.com/deepgram/deepgram-go-sdk/v3/pkg/api/listen/v1/websocket/interfaces"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

// Implement the LiveMessageCallback interface
type deepgramSttCallback struct {
	logger      commons.Logger
	onPacket    func(pkt ...internal_type.Packet) error
	options     utils.Option
	swapStarted func() time.Time
	contextID   func() string
}

func NewDeepgramSttCallback(
	logger commons.Logger,
	onPacket func(pkt ...internal_type.Packet) error,
	options utils.Option,
	swapStarted func() time.Time,
	contextID func() string,
) msginterfaces.LiveMessageCallback {
	return &deepgramSttCallback{
		logger:      logger,
		onPacket:    onPacket,
		options:     options,
		swapStarted: swapStarted,
		contextID:   contextID,
	}
}

// Handle when the WebSocket is opened
func (d *deepgramSttCallback) Open(or *msginterfaces.OpenResponse) error {
	return nil
}

// Handle incoming transcription messages from Deepgram
func (d *deepgramSttCallback) Message(mr *msginterfaces.MessageResponse) error {
	for _, alternative := range mr.Channel.Alternatives {
		if alternative.Transcript == "" {
			continue
		}
		lang := d.GetMostUsedLanguage(alternative.Languages)
		confStr := fmt.Sprintf("%.4f", alternative.Confidence)

		if v, err := d.options.GetFloat64("listen.threshold"); err == nil {
			if alternative.Confidence < v {
				// confidence below threshold, emit event and skip stt processing
				ctxID := d.contextID()
				d.onPacket(
					internal_type.ConversationEventPacket{
						ContextID: ctxID,
						Name:      "stt",
						Data: map[string]string{
							"type":       "low_confidence",
							"script":     alternative.Transcript,
							"confidence": confStr,
							"threshold":  fmt.Sprintf("%.4f", v),
						},
						Time: time.Now(),
					},
				)
				return nil
			}
		}

		if mr.IsFinal {
			now := time.Now()
			var latencyMs int64
			if started := d.swapStarted(); !started.IsZero() {
				latencyMs = now.Sub(started).Milliseconds()
			}
			wordCount := len(strings.Fields(alternative.Transcript))
			ctxID := d.contextID()
			d.onPacket(
				internal_type.InterruptionDetectedPacket{ContextID: ctxID, Source: "word"},
				internal_type.SpeechToTextPacket{
					ContextID:  ctxID,
					Script:     alternative.Transcript,
					Confidence: alternative.Confidence,
					Language:   lang,
					Interim:    false,
				},
				internal_type.ConversationEventPacket{
					ContextID: ctxID,
					Name:      "stt",
					Data: map[string]string{
						"type":       "completed",
						"script":     alternative.Transcript,
						"confidence": confStr,
						"language":   lang,
						"word_count": fmt.Sprintf("%d", wordCount),
						"char_count": fmt.Sprintf("%d", len(alternative.Transcript)),
					},
					Time: now,
				},
				internal_type.UserMessageMetricPacket{
					ContextID: ctxID,
					Metrics:   []*protos.Metric{{Name: "stt_latency_ms", Value: fmt.Sprintf("%d", latencyMs)}},
				},
			)
		} else {
			// Non-final interim transcript
			ctxID := d.contextID()
			d.onPacket(
				internal_type.InterruptionDetectedPacket{ContextID: ctxID, Source: "word"},
				internal_type.SpeechToTextPacket{
					ContextID:  ctxID,
					Script:     alternative.Transcript,
					Confidence: alternative.Confidence,
					Language:   lang,
					Interim:    true,
				},
				internal_type.ConversationEventPacket{
					ContextID: ctxID,
					Name:      "stt",
					Data: map[string]string{
						"type":       "interim",
						"script":     alternative.Transcript,
						"confidence": confStr,
					},
					Time: time.Now(),
				},
			)
		}
		return nil
	}
	return nil
}

// Handle utterance end event - this signals the end of a sentence
func (d *deepgramSttCallback) UtteranceEnd(ur *msginterfaces.UtteranceEndResponse) error {
	return nil
}

// Handle metadata (optional, can be left empty)
func (d *deepgramSttCallback) Metadata(md *msginterfaces.MetadataResponse) error {
	return nil
}

// Handle speech started event — no-op; timing is driven by Transform() via startedAtNano.
func (d *deepgramSttCallback) SpeechStarted(ssr *msginterfaces.SpeechStartedResponse) error {
	return nil
}

// Handle when the WebSocket is closed
func (d *deepgramSttCallback) Close(cr *msginterfaces.CloseResponse) error {
	// d.logger.Debugf("Deepgram WebSocket closed")
	return nil
}

// Handle errors from Deepgram
func (d *deepgramSttCallback) Error(er *msginterfaces.ErrorResponse) error {
	ctxID := d.contextID()
	d.onPacket(internal_type.ConversationEventPacket{
		ContextID: ctxID,
		Name:      "stt",
		Data:      map[string]string{"type": "error", "error": er.ErrMsg},
		Time:      time.Now(),
	})
	return nil
}

// Handle unhandled events (optional, can be left empty)
func (d *deepgramSttCallback) UnhandledEvent(byData []byte) error {
	d.logger.Errorf("UnhandledEvent %+v", byData)
	return nil
}

func (d *deepgramSttCallback) GetMostUsedLanguage(languages []string) string {
	if len(languages) == 0 {
		return "en"
	}

	languageCount := make(map[string]int)
	for _, lang := range languages {
		languageCount[lang]++
	}

	mostUsedLang := ""
	maxCount := 0
	for lang, count := range languageCount {
		if count > maxCount {
			maxCount = count
			mostUsedLang = lang
		}
	}
	return mostUsedLang
}
