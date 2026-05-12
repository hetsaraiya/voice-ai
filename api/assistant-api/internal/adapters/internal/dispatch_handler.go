// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package adapter_internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	adapter_lifecycle "github.com/rapidaai/api/assistant-api/internal/adapters/lifecycle"
	internal_analysis "github.com/rapidaai/api/assistant-api/internal/analysis"
	internal_audio "github.com/rapidaai/api/assistant-api/internal/audio"
	internal_audio_recorder "github.com/rapidaai/api/assistant-api/internal/audio/recorder"
	internal_authentication "github.com/rapidaai/api/assistant-api/internal/authentication"
	internal_condition "github.com/rapidaai/api/assistant-api/internal/condition"
	internal_denoiser "github.com/rapidaai/api/assistant-api/internal/denoiser"
	internal_end_of_speech "github.com/rapidaai/api/assistant-api/internal/end_of_speech"
	internal_conversation_entity "github.com/rapidaai/api/assistant-api/internal/entity/conversations"
	internal_llm "github.com/rapidaai/api/assistant-api/internal/llm"
	observe "github.com/rapidaai/api/assistant-api/internal/observe"
	internal_transformer "github.com/rapidaai/api/assistant-api/internal/transformer"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	internal_vad "github.com/rapidaai/api/assistant-api/internal/vad"
	"github.com/rapidaai/api/assistant-api/internal/variable"
	internal_namespace "github.com/rapidaai/api/assistant-api/internal/variable/namespace"
	internal_webhook "github.com/rapidaai/api/assistant-api/internal/webhook"
	pkg_types "github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type requestorDispatchHandler struct {
	r *genericRequestor
}

func (h requestorDispatchHandler) HandleUserText(ctx context.Context, vl internal_type.UserTextReceivedPacket) {
	if !h.r.canAcceptInput() {
		h.r.logger.Tracef(ctx, "dropping user text: session not ready, state=%s", h.r.getSessionState().String())
		return
	}
	h.HandleInterruptionDetected(ctx, internal_type.InterruptionDetectedPacket{
		ContextID: h.r.GetID(),
		Source:    internal_type.InterruptionSourceWord,
	})

	vl.ContextID = h.r.GetID()
	h.r.OnPacket(ctx,
		internal_type.InterimEndOfSpeechPacket{Speech: vl.Text, ContextID: vl.ContextID},
		internal_type.ConversationEventPacket{Name: "eos", Data: map[string]string{"type": "interim", "speech": vl.Text}},
		internal_type.EndOfSpeechPacket{Speech: vl.Text, ContextID: vl.ContextID},
		internal_type.ConversationEventPacket{
			Name: "eos",
			Data: map[string]string{
				"type":       "detected",
				"provider":   "text_input",
				"context_id": vl.ContextID,
				"speech":     vl.Text,
			},
			Time: time.Now(),
		},
	)
}

func (h requestorDispatchHandler) HandleUserAudio(ctx context.Context, vl internal_type.UserAudioReceivedPacket) {
	if !h.r.canAcceptInput() {
		h.r.logger.Tracef(ctx, "dropping user audio: session not ready, state=%s", h.r.getSessionState().String())
		return
	}
	if h.r.denoiser != nil && !vl.NoiseReduced {
		h.r.OnPacket(ctx, internal_type.DenoiseAudioPacket{ContextID: vl.ContextID, Audio: vl.Audio})
		return
	}
	h.r.OnPacket(ctx,
		internal_type.RecordUserAudioPacket{ContextID: vl.ContextID, Audio: vl.Audio},
		internal_type.VadAudioPacket{ContextID: vl.ContextID, Audio: vl.Audio},
		internal_type.SpeechToTextAudioPacket{ContextID: vl.ContextID, Audio: vl.Audio},
		internal_type.EndOfSpeechAudioPacket{ContextID: vl.ContextID, Audio: vl.Audio},
	)
	// h.callEndOfSpeech(ctx, vl)
}

func (h requestorDispatchHandler) HandleEndOfSpeechAudio(ctx context.Context, vl internal_type.EndOfSpeechAudioPacket) {
	if h.r.endOfSpeech != nil {
		if err := h.r.endOfSpeech.Analyze(ctx, vl); err != nil {
			h.r.logger.Errorf("end of speech analyze error: %v", err)
		}
	}
}

func (h requestorDispatchHandler) HandleSpeechToTextAudio(ctx context.Context, vl internal_type.SpeechToTextAudioPacket) {
	if h.r.speechToTextTransformer != nil {
		if err := h.r.speechToTextTransformer.Transform(ctx, vl); err != nil {
			h.r.logger.Tracef(ctx, "error while transforming input %s and error %s", h.r.speechToTextTransformer.Name(), err.Error())
		}
	}
}

func (h requestorDispatchHandler) HandleDenoise(ctx context.Context, vl internal_type.DenoiseAudioPacket) {
	if err := h.r.denoiser.Denoise(ctx, vl); err != nil {
		h.r.logger.Warnf("denoiser returned unexpected error: %+v", err)
	}
}
func (h requestorDispatchHandler) HandleDenoisedAudio(ctx context.Context, vl internal_type.DenoisedAudioPacket) {
	h.r.OnPacket(ctx, internal_type.UserAudioReceivedPacket{
		ContextID:    vl.ContextID,
		Audio:        vl.Audio,
		NoiseReduced: true,
	})
}

func (h requestorDispatchHandler) HandleVadAudio(ctx context.Context, vl internal_type.VadAudioPacket) {
	if h.r.vad != nil {
		utils.Go(ctx, func() {
			if err := h.r.vad.Process(ctx, internal_type.UserAudioReceivedPacket{ContextID: vl.ContextID, Audio: vl.Audio}); err != nil {
				h.r.logger.Warnf("error while processing with vad %s", err.Error())
			}
		})
	}
}
func (h requestorDispatchHandler) HandleVadSpeechActivity(ctx context.Context, vl internal_type.VadSpeechActivityPacket) {
	if h.r.endOfSpeech != nil {
		utils.Go(ctx, func() {
			if err := h.r.endOfSpeech.Analyze(ctx, vl); err != nil {
				h.r.logger.Errorf("end of speech analyze error: %v", err)
			}
		})
	}
}
func (h requestorDispatchHandler) HandleSpeechToText(ctx context.Context, p internal_type.SpeechToTextPacket) {
	p.ContextID = h.r.GetID()
	if err := h.callEndOfSpeech(ctx, p); err != nil {
		if !p.Interim {
			h.r.OnPacket(ctx, internal_type.EndOfSpeechPacket{
				ContextID: p.ContextID,
				Speech:    p.Script,
				Speechs:   []internal_type.SpeechToTextPacket{p},
			})
		}
	}
}
func (h requestorDispatchHandler) HandleInterimEndOfSpeech(ctx context.Context, p internal_type.InterimEndOfSpeechPacket) {
	h.r.Notify(ctx, &protos.ConversationUserMessage{
		Id:        h.r.GetID(),
		Message:   &protos.ConversationUserMessage_Text{Text: p.Speech},
		Completed: false,
		Time:      timestamppb.New(time.Now()),
	})
}
func (h requestorDispatchHandler) HandleEndOfSpeech(ctx context.Context, p internal_type.EndOfSpeechPacket) {
	if err := h.callInputNormalizer(ctx, p); err != nil {
		h.r.OnPacket(ctx, internal_type.UserInputPacket{
			ContextID: p.ContextID,
			Text:      p.Speech,
		})
	}
}
func (h requestorDispatchHandler) HandleUserInput(ctx context.Context, p internal_type.UserInputPacket) {
	h.r.OnPacket(ctx, internal_type.StopIdleTimeoutPacket{
		ContextID: h.r.GetID(), ResetCount: true,
	})

	if err := h.r.Transition(LLMGenerating); err != nil {
		h.r.logger.Errorf("messaging transition error: %v", err)
	}

	contextID := h.r.GetID()
	p.ContextID = contextID

	if err := h.r.Notify(ctx, &protos.ConversationUserMessage{
		Id:        contextID,
		Message:   &protos.ConversationUserMessage_Text{Text: p.Text},
		Completed: true,
		Time:      timestamppb.New(time.Now()),
	}); err != nil {
		h.r.logger.Tracef(ctx, "might be returning processing the duplicate message so cut it out.")
		return
	}
	h.r.OnPacket(ctx,
		internal_type.MessageCreatePacket{ContextID: contextID, MessageRole: "user", Text: p.Text},
		internal_type.UserMessageMetadataPacket{ContextID: contextID, Metadata: []*protos.Metadata{
			{
				Key:   "language",
				Value: p.Language.Name,
			},
			{
				Key:   "language_code",
				Value: p.Language.ISO639_1,
			}}},
		internal_type.UserMessageMetricPacket{ContextID: contextID, Metrics: []*protos.Metric{{Name: "user_turn", Value: type_enums.CONVERSATION_COMPLETE.String(), Description: "User turn started"}}},
	)

	if h.r.assistantExecutor != nil {
		utils.Go(ctx, func() {
			if err := h.r.assistantExecutor.Execute(ctx, h.r, p); err != nil {
				h.r.OnPacket(ctx, internal_type.LLMErrorPacket{ContextID: contextID, Error: err})
			}
		})
	}
}
func (h requestorDispatchHandler) HandleInterruptionDetected(ctx context.Context, p internal_type.InterruptionDetectedPacket) {
	if p.ContextID == "" {
		p.ContextID = h.r.GetID()
	}
	switch p.Source {
	case internal_type.InterruptionSourceWord:
		h.r.OnPacket(ctx,
			internal_type.StopIdleTimeoutPacket{ContextID: p.ContextID},
			internal_type.EndOfSpeechInterruptionPacket{ContextID: p.ContextID, Source: internal_type.InterruptionSourceWord},
		)
		if err := h.r.Transition(Interrupted); err != nil {
			return
		}
		h.r.OnPacket(ctx,
			internal_type.RecordAssistantAudioPacket{ContextID: p.ContextID, Truncate: true},
			internal_type.TextToSpeechInterruptPacket{ContextID: p.ContextID, StartAt: p.StartAt, EndAt: p.EndAt},
			internal_type.LLMInterruptPacket{ContextID: p.ContextID},
		)
		utils.Go(ctx, func() {
			h.r.Notify(ctx, &protos.ConversationInterruption{
				Type: protos.ConversationInterruption_INTERRUPTION_TYPE_WORD,
				Time: timestamppb.Now(),
			})
		})

	default:
		switch p.Event {
		case internal_type.InterruptionEventStart:
			if p.StartAt < 5 {
				return
			}
			if err := h.r.Transition(Interrupt); err != nil {
				return
			}
			h.r.OnPacket(ctx, internal_type.EndOfSpeechInterruptionPacket{ContextID: p.ContextID, Source: internal_type.InterruptionSourceVad})
			utils.Go(ctx, func() {
				h.r.Notify(ctx, &protos.ConversationInterruption{
					Type: protos.ConversationInterruption_INTERRUPTION_TYPE_VAD,
					Time: timestamppb.Now(),
				})
			})
		case internal_type.InterruptionEventEnd:
			h.r.OnPacket(ctx, internal_type.SpeechToTextInterruptPacket{ContextID: p.ContextID})
		}
	}
}

func (h requestorDispatchHandler) HandleEndOfSpeechInterruption(ctx context.Context, p internal_type.EndOfSpeechInterruptionPacket) {
	if h.r.endOfSpeech != nil {
		if err := h.r.endOfSpeech.Analyze(ctx, p); err != nil {
			h.r.logger.Errorf("end of speech analyze error: %v", err)
		}
	}
}

func (h requestorDispatchHandler) HandleTextToSpeechInterrupt(ctx context.Context, p internal_type.TextToSpeechInterruptPacket) {
	if h.r.textToSpeechTransformer != nil {
		if err := h.r.textToSpeechTransformer.Transform(ctx, p); err != nil {
			h.r.logger.Errorf("tts interrupt: %v", err)
		}
	}
}
func (h requestorDispatchHandler) HandleLLMInterrupt(ctx context.Context, p internal_type.LLMInterruptPacket) {
	if h.r.assistantExecutor != nil {
		if err := h.r.assistantExecutor.Execute(ctx, h.r, p); err != nil {
			h.r.logger.Errorf("llm interrupt: %v", err)
		}
	}
}
func (h requestorDispatchHandler) HandleSpeechToTextInterrupt(ctx context.Context, p internal_type.SpeechToTextInterruptPacket) {
	if h.r.speechToTextTransformer != nil {
		if err := h.r.speechToTextTransformer.Transform(ctx, p); err != nil {
			h.r.logger.Errorf("stt interrupt: %v", err)
		}
	}
}
func (h requestorDispatchHandler) HandleTurnChange(ctx context.Context, p internal_type.TurnChangePacket) {
	if p.ContextID == "" {
		p.ContextID = h.r.GetID()
	}
	if p.Time.IsZero() {
		p.Time = time.Now()
	}

	if h.r.speechToTextTransformer != nil {
		if err := h.r.speechToTextTransformer.Transform(ctx, p); err != nil {
			h.r.logger.Errorf("stt context-change update failed: %v", err)
		}
	}
	if h.r.textToSpeechTransformer != nil {
		if err := h.r.textToSpeechTransformer.Transform(ctx, p); err != nil {
			h.r.logger.Errorf("tts context-change update failed: %v", err)
		}
	}

	h.r.OnPacket(ctx, internal_type.ConversationEventPacket{
		ContextID: p.ContextID,
		Name:      "turn",
		Data: map[string]string{
			"type":           "change",
			"old_context_id": p.PreviousContextID,
			"new_context_id": p.ContextID,
			"reason":         p.Reason,
			"source":         p.Source,
		},
		Time: p.Time,
	})
}
func (h requestorDispatchHandler) HandleLLMResponseDelta(ctx context.Context, p internal_type.LLMResponseDeltaPacket) {
	if p.ContextID != h.r.GetID() {
		h.r.OnPacket(ctx, internal_type.ConversationEventPacket{
			ContextID: p.ContextID,
			Name:      "llm",
			Data:      map[string]string{"type": "discarded", "reason": "stale_context", "current_context": h.r.GetID(), "text": p.Text},
			Time:      time.Now(),
		})
		return
	}
	if err := h.r.Transition(LLMGenerating); err != nil {
		h.r.logger.Errorf("messaging transition error: %v", err)
	}
	if h.r.outputNormalizer != nil {
		h.r.outputNormalizer.Normalize(ctx, p)
	} else {
		h.r.OnPacket(ctx, internal_type.TextToSpeechTextPacket{ContextID: p.ContextID, Text: p.Text})
	}
}
func (h requestorDispatchHandler) HandleLLMResponseDone(ctx context.Context, p internal_type.LLMResponseDonePacket) {
	if p.ContextID != h.r.GetID() {
		h.r.OnPacket(ctx, internal_type.ConversationEventPacket{
			ContextID: p.ContextID,
			Name:      "llm",
			Data:      map[string]string{"type": "discarded", "reason": "stale_context", "packet": "done", "current_context": h.r.GetID(), "text": p.Text},
			Time:      time.Now(),
		})
		return
	}
	h.r.OnPacket(ctx, internal_type.StartIdleTimeoutPacket{ContextID: p.ContextID})
	if err := h.r.Transition(LLMGenerated); err != nil {
		h.r.logger.Errorf("messaging transition error: %v", err)
	}
	h.r.OnPacket(ctx,
		internal_type.MessageCreatePacket{ContextID: p.ContextID, MessageRole: "assistant", Text: p.Text},
		internal_type.AssistantMessageMetricPacket{
			ContextID: p.ContextID,
			Metrics:   []*protos.Metric{{Name: "assistant_turn", Value: type_enums.CONVERSATION_COMPLETE.String(), Description: "LLM response completed"}},
		},
	)
	if h.r.outputNormalizer != nil {
		h.r.outputNormalizer.Normalize(ctx, p)
	} else {
		h.r.OnPacket(ctx, internal_type.TextToSpeechDonePacket{ContextID: p.ContextID, Text: p.Text})
	}
}
func (h requestorDispatchHandler) HandleError(ctx context.Context, p internal_type.ErrorPacket) {
	switch errPkt := p.(type) {
	case internal_type.InitializationFailedPacket:
		if err := h.r.sessionLifecycle.Transition(adapter_lifecycle.EventInitializationFailed); err != nil {
			h.r.logger.Tracef(ctx, "session lifecycle init-failed transition ignored: %v", err)
		}
		h.r.OnPacket(ctx,
			internal_type.InitializeOutboundDispatcherPacket{ContextID: p.ContextId()},
			internal_type.ConversationEventPacket{
				ContextID: p.ContextId(),
				Name:      "session",
				Data:      map[string]string{"type": "error", "message": p.ErrMessage()},
				Time:      time.Now(),
			},
		)

	case internal_type.LLMErrorPacket:
		h.r.OnPacket(ctx,
			internal_type.UserMessageMetricPacket{
				ContextID: p.ContextId(),
				Metrics: []*protos.Metric{{
					Name:        "llm_error",
					Value:       p.ErrMessage(),
					Description: "An error occurred during LLM processing"}},
			},
			internal_type.ConversationEventPacket{
				ContextID: p.ContextId(),
				Name:      "llm",
				Data:      map[string]string{"type": "error", "message": p.ErrMessage()},
				Time:      time.Now(),
			})
		h.r.Transition(LLMGenerated)
	case internal_type.SpeechToTextErrorPacket:
		h.r.OnPacket(ctx,
			internal_type.UserMessageMetricPacket{
				ContextID: p.ContextId(),
				Metrics: []*protos.Metric{{
					Name:        "stt_error",
					Value:       p.ErrMessage(),
					Description: "An error occurred during STT processing"}},
			},
			internal_type.ConversationEventPacket{
				ContextID: p.ContextId(),
				Name:      "stt",
				Data:      map[string]string{"type": "error", "message": p.ErrMessage()},
				Time:      time.Now(),
			})
	case internal_type.TextToSpeechErrorPacket:
		h.r.OnPacket(ctx,
			internal_type.UserMessageMetricPacket{
				ContextID: p.ContextId(),
				Metrics: []*protos.Metric{{
					Name:        "tts_error",
					Value:       p.ErrMessage(),
					Description: "An error occurred during TTS processing"}},
			},
			internal_type.ConversationEventPacket{
				ContextID: p.ContextId(),
				Name:      "tts",
				Data:      map[string]string{"type": "error", "message": p.ErrMessage()},
				Time:      time.Now(),
			})
	case internal_type.ModeSwitchErrorPacket:
		if errPkt.IsRecoverable() {
			if err := h.r.sessionLifecycle.Transition(adapter_lifecycle.EventSwitchFailedRecoverable); err != nil {
				h.r.logger.Tracef(ctx, "session lifecycle switch-failed(recoverable) transition ignored: %v", err)
			}
		} else {
			if err := h.r.sessionLifecycle.Transition(adapter_lifecycle.EventSwitchFailedFatal); err != nil {
				h.r.logger.Tracef(ctx, "session lifecycle switch-failed(fatal) transition ignored: %v", err)
			}
		}
		h.r.OnPacket(ctx, internal_type.ConversationEventPacket{
			ContextID: p.ContextId(),
			Name:      observe.ComponentSession,
			Data: map[string]string{
				observe.DataType: "mode_switch_failed",
				"error_type":     string(errPkt.Type),
				"target_mode":    errPkt.StreamMode.String(),
				"error":          p.ErrMessage(),
			},
			Time: time.Now(),
		})
	}
	if !p.IsRecoverable() {
		var conversationId uint64
		if h.r.Conversation() != nil {
			conversationId = h.r.Conversation().Id
		}
		h.r.OnPacket(ctx,
			internal_type.ExecuteWebhookPacket{
				ContextID: p.ContextId(),
				Event:     utils.ConversationFailed,
			},
			internal_type.ConversationEventPacket{
				ContextID: h.r.GetID(),
				Name:      observe.ComponentSession,
				Data: map[string]string{
					observe.DataType:   observe.EventDisconnectRequested,
					observe.DataReason: protos.ConversationDisconnection_DISCONNECTION_TYPE_ERROR.String()},
				Time: time.Now(),
			},
			internal_type.ConversationMetadataPacket{
				ContextID: h.r.Conversation().Id,
				Metadata: []*protos.Metadata{{
					Key:   "disconnect_reason",
					Value: protos.ConversationDisconnection_DISCONNECTION_TYPE_ERROR.String(),
				}},
			},
		)
		h.r.Notify(ctx,
			&protos.ConversationError{
				AssistantConversationId: conversationId,
				Message:                 p.ErrMessage(),
			},
			&protos.ConversationDisconnection{
				Type: protos.ConversationDisconnection_DISCONNECTION_TYPE_ERROR,
			})
		return
	}
	if h.r.Conversation() != nil {
		_ = h.r.Notify(ctx, &protos.ConversationError{
			AssistantConversationId: h.r.Conversation().Id,
			Message:                 p.ErrMessage(),
		})
		return
	}
	_ = h.r.Notify(ctx, &protos.ConversationError{
		Message: p.ErrMessage(),
	})

}
func (h requestorDispatchHandler) HandleInjectMessage(ctx context.Context, p internal_type.InjectMessagePacket) {
	if err := h.r.Transition(LLMGenerating); err != nil {
		h.r.logger.Errorf("messaging transition error: %v", err)
	}

	if h.r.assistantExecutor != nil {
		utils.Go(ctx, func() {
			if err := h.r.assistantExecutor.Execute(ctx, h.r, p); err != nil {
				h.r.logger.Errorf("assistant executor error: %v", err)
			}
		})
	}

	contextID := h.r.GetID()
	if h.r.outputNormalizer != nil {
		h.r.OnPacket(ctx,
			internal_type.MessageCreatePacket{ContextID: contextID, MessageRole: "assistant", Text: p.Text},
			internal_type.AssistantMessageMetricPacket{
				ContextID: contextID,
				Metrics:   []*protos.Metric{{Name: "assistant_turn", Value: type_enums.CONVERSATION_COMPLETE.String(), Description: "Injected message completed"}},
			},
		)
		h.r.outputNormalizer.Normalize(ctx, internal_type.InjectMessagePacket{ContextID: contextID, Text: p.Text})
		if err := h.r.Transition(LLMGenerated); err != nil {
			h.r.logger.Errorf("messaging transition error: %v", err)
		}
	} else {
		h.r.OnPacket(ctx,
			internal_type.LLMResponseDeltaPacket{ContextID: contextID, Text: p.Text},
			internal_type.LLMResponseDonePacket{ContextID: contextID, Text: p.Text},
		)
	}
}
func (h requestorDispatchHandler) HandleStartIdleTimeout(ctx context.Context, p internal_type.StartIdleTimeoutPacket) {
	if h.r.idleTimeoutTimer != nil {
		h.r.idleTimeoutTimer.Stop()
	}
	behavior, err := h.r.GetBehavior()
	if err != nil {
		return
	}
	if behavior.IdleTimeout == nil || *behavior.IdleTimeout == 0 {
		return
	}

	timeoutDuration := time.Duration(*behavior.IdleTimeout) * time.Second
	h.r.idleTimeoutDeadline = time.Now().Add(timeoutDuration)
	h.r.idleTimeoutTimer = time.AfterFunc(timeoutDuration, func() {
		if err := h.r.onIdleTimeout(ctx); err != nil {
			h.r.logger.Errorf("error while handling idle timeout: %v", err)
		}
	})
}
func (h requestorDispatchHandler) HandleStopIdleTimeout(ctx context.Context, p internal_type.StopIdleTimeoutPacket) {
	if h.r.idleTimeoutTimer != nil {
		h.r.idleTimeoutTimer.Stop()
		h.r.idleTimeoutTimer = nil
	}
	h.r.idleTimeoutDeadline = time.Time{}

	if p.ResetCount {
		h.r.idleTimeoutCount = 0
	}
}
func (h requestorDispatchHandler) HandleTextToSpeechText(ctx context.Context, p internal_type.TextToSpeechTextPacket) {
	if p.ContextID != h.r.GetID() {
		return
	}
	if h.r.textToSpeechTransformer != nil && h.r.GetMode().Audio() {
		if err := h.r.textToSpeechTransformer.Transform(ctx, p); err != nil {
			h.r.logger.Errorf("tts text: failed to send chunk: %v", err)
		}
	}
	h.r.Notify(ctx, &protos.ConversationAssistantMessage{
		Time: timestamppb.Now(), Id: p.ContextID, Completed: false,
		Message: &protos.ConversationAssistantMessage_Text{Text: p.Text},
	})
}
func (h requestorDispatchHandler) HandleTextToSpeechDone(ctx context.Context, p internal_type.TextToSpeechDonePacket) {
	if p.ContextID != h.r.GetID() {
		return
	}

	if h.r.textToSpeechTransformer != nil && h.r.GetMode().Audio() {
		if err := h.r.textToSpeechTransformer.Transform(ctx, p); err != nil {
			h.r.logger.Errorf("tts done: failed to send final: %v", err)
		}
	}
	h.r.Notify(ctx, &protos.ConversationAssistantMessage{
		Time: timestamppb.Now(), Id: p.ContextID, Completed: true,
		Message: &protos.ConversationAssistantMessage_Text{Text: p.Text},
	})
}
func (h requestorDispatchHandler) HandleTextToSpeechAudio(ctx context.Context, p internal_type.TextToSpeechAudioPacket) {
	if h.r.GetMode().Audio() {
		audioInfo := internal_audio.GetAudioInfo(p.AudioChunk, internal_audio.RAPIDA_INTERNAL_AUDIO_CONFIG)
		h.r.extendIdleTimeoutTimer(time.Duration(audioInfo.DurationMs) * time.Millisecond)
	}
	if p.ContextID != h.r.GetID() {
		h.r.OnPacket(ctx,
			internal_type.ConversationEventPacket{
				ContextID: p.ContextID,
				Name:      "tts",
				Data:      map[string]string{"type": "discarded", "reason": "stale_context", "packet": "tts_audio", "current_context": h.r.GetID()},
				Time:      time.Now(),
			},
			internal_type.AssistantMessageMetricPacket{
				ContextID: p.ContextID,
				Metrics:   []*protos.Metric{{Name: "discarded_tts_chunk", Value: "true", Description: fmt.Sprintf("tts end packet discarded due to stale contextID %s", h.r.GetID())}},
			})
		return
	}
	if err := h.r.Notify(ctx, &protos.ConversationAssistantMessage{
		Time:      timestamppb.Now(),
		Id:        p.ContextID,
		Message:   &protos.ConversationAssistantMessage_Audio{Audio: p.AudioChunk},
		Completed: false,
	}); err != nil {
		h.r.logger.Tracef(ctx, "error while outputting chunk to the user: %w", err)
	}
	h.r.OnPacket(ctx, internal_type.RecordAssistantAudioPacket{ContextID: p.ContextID, Audio: p.AudioChunk})
}
func (h requestorDispatchHandler) HandleTextToSpeechEnd(ctx context.Context, p internal_type.TextToSpeechEndPacket) {
	if p.ContextID != h.r.GetID() {
		h.r.OnPacket(ctx,
			internal_type.ConversationEventPacket{
				ContextID: p.ContextID,
				Name:      "tts",
				Data:      map[string]string{"type": "discarded", "reason": "stale_context", "packet": "tts_end", "current_context": h.r.GetID()},
				Time:      time.Now(),
			},
			internal_type.AssistantMessageMetricPacket{
				ContextID: p.ContextID,
				Metrics:   []*protos.Metric{{Name: "discarded_tts", Value: "true", Description: fmt.Sprintf("tts end packet discarded due to stale contextID %s", h.r.GetID())}},
			})
		return
	}
	if err := h.r.Notify(ctx, &protos.ConversationAssistantMessage{
		Time:      timestamppb.Now(),
		Id:        p.ContextID,
		Completed: true,
	}); err != nil {
		h.r.logger.Tracef(ctx, "error while outputting chunk to the user: %w", err)
	}
}
func (h requestorDispatchHandler) HandleLLMToolCall(ctx context.Context, p internal_type.LLMToolCallPacket) {
	req, _ := json.Marshal(p)
	h.r.OnPacket(ctx, internal_type.ConversationEventPacket{
		ContextID: p.ContextID,
		Name:      observe.ComponentTool,
		Data:      map[string]string{observe.DataType: observe.EventToolCallStarted, "name": p.Name, "id": p.ToolID, "action": p.Action.String()},
		Time:      time.Now(),
	}, internal_type.ToolLogCreatePacket{
		ContextID: p.ContextID, ToolID: p.ToolID, Name: p.Name, Request: req,
	},
	)

	if msg, ok := p.Arguments["message"]; ok && msg != "" {
		h.r.OnPacket(ctx,
			internal_type.TextToSpeechInterruptPacket{ContextID: p.ContextID},
			internal_type.InjectMessagePacket{ContextID: p.ContextID, Text: msg})
	}

	if delayStr, ok := p.Arguments["delay"]; ok && delayStr != "" {
		if delayMs, err := strconv.Atoi(delayStr); err == nil && delayMs > 0 {
			time.AfterFunc(time.Duration(delayMs)*time.Millisecond, func() {
				h.r.Notify(ctx, &protos.ConversationToolCall{
					Id: p.ContextID, ToolId: p.ToolID, Name: p.Name,
					Action: p.Action, Args: p.Arguments, Time: timestamppb.Now(),
				})
			})
		}
	} else {
		h.r.Notify(ctx, &protos.ConversationToolCall{
			Id: p.ContextID, ToolId: p.ToolID, Name: p.Name,
			Action: p.Action, Args: p.Arguments, Time: timestamppb.Now(),
		})
	}

	if p.Action != protos.ToolCallAction_TOOL_CALL_ACTION_UNSPECIFIED {
		h.r.OnPacket(ctx, internal_type.StopIdleTimeoutPacket{
			ContextID: h.r.GetID(), ResetCount: true,
		})
		if h.r.maxSessionTimer != nil {
			h.r.maxSessionTimer.Stop()
		}
	}

	if h.r.assistantExecutor != nil {
		utils.Go(ctx, func() {
			if err := h.r.assistantExecutor.Execute(ctx, h.r, p); err != nil {
				h.r.logger.Errorf("assistant executor error: %v", err)
			}
		})
	}
}
func (h requestorDispatchHandler) HandleLLMToolResult(ctx context.Context, p internal_type.LLMToolResultPacket) {
	res, _ := json.Marshal(p)

	h.r.OnPacket(ctx,
		internal_type.ToolLogUpdatePacket{
			ContextID: p.ContextID, ToolID: p.ToolID, Response: res,
		})

	switch p.Action {
	case protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION:
		h.r.OnPacket(ctx,
			internal_type.ConversationEventPacket{
				ContextID: h.r.GetID(),
				Name:      observe.ComponentSession,
				Data: map[string]string{
					observe.DataType:   observe.EventDisconnectRequested,
					observe.DataReason: protos.ConversationDisconnection_DISCONNECTION_TYPE_TOOL.String()},
				Time: time.Now(),
			},
			internal_type.ConversationMetadataPacket{
				ContextID: h.r.Conversation().Id,
				Metadata: []*protos.Metadata{{
					Key:   "disconnect_reason",
					Value: protos.ConversationDisconnection_DISCONNECTION_TYPE_TOOL.String(),
				}},
			},
		)
		h.r.Notify(ctx, &protos.ConversationDisconnection{
			Type: protos.ConversationDisconnection_DISCONNECTION_TYPE_TOOL,
		})
		return
	case protos.ToolCallAction_TOOL_CALL_ACTION_TRANSFER_CONVERSATION:
		if p.Result["next_action"] == "end_call" {
			h.r.OnPacket(ctx,
				internal_type.ConversationEventPacket{
					ContextID: h.r.GetID(),
					Name:      observe.ComponentSession,
					Data: map[string]string{
						observe.DataType:   observe.EventDisconnectRequested,
						observe.DataReason: protos.ConversationDisconnection_DISCONNECTION_TYPE_TOOL.String()},
					Time: time.Now(),
				},
				internal_type.ConversationMetadataPacket{
					ContextID: h.r.Conversation().Id,
					Metadata: []*protos.Metadata{{
						Key:   "disconnect_reason",
						Value: protos.ConversationDisconnection_DISCONNECTION_TYPE_TOOL.String(),
					}},
				},
			)
			h.r.Notify(ctx, &protos.ConversationDisconnection{
				Type: protos.ConversationDisconnection_DISCONNECTION_TYPE_TOOL,
			})
			return
		}
	}

	h.r.OnPacket(
		ctx,
		internal_type.TextToSpeechInterruptPacket{ContextID: p.ContextID},
		internal_type.StartIdleTimeoutPacket{ContextID: p.ContextID},
		internal_type.ConversationEventPacket{
			ContextID: p.ContextID,
			Name:      observe.ComponentTool,
			Data:      map[string]string{observe.DataType: observe.EventToolCallCompleted, "name": p.Name, "id": p.ToolID},
			Time:      time.Now(),
		},
	)
	if h.r.assistantExecutor != nil {
		utils.Go(ctx, func() {
			if err := h.r.assistantExecutor.Execute(ctx, h.r, p); err != nil {
				h.r.logger.Errorf("tool result processing failed: %v", err)
			}
		})
	}
}
func (h requestorDispatchHandler) HandleRecordUserAudio(ctx context.Context, p internal_type.RecordUserAudioPacket) {
	if h.r.recorder != nil {
		if err := h.r.recorder.Record(ctx, p); err != nil {
			h.r.logger.Errorf("recorder error: %v", err)
		}
	}
}
func (h requestorDispatchHandler) HandleRecordAssistantAudio(ctx context.Context, p internal_type.RecordAssistantAudioPacket) {
	if h.r.recorder != nil {
		if err := h.r.recorder.Record(ctx, p); err != nil {
			h.r.logger.Errorf("recorder error: %v", err)
		}
	}
}
func (h requestorDispatchHandler) HandleMessageCreate(ctx context.Context, p internal_type.MessageCreatePacket) {
	if err := h.r.onAddMessage(ctx, p); err != nil {
		h.r.logger.Errorf("Error in onAddMessage: %v", err)
	}
}
func (h requestorDispatchHandler) HandleConversationMetric(ctx context.Context, p internal_type.ConversationMetricPacket) {
	if len(p.Metrics) > 0 {
		_ = h.r.Notify(ctx, &protos.ConversationMetric{
			AssistantConversationId: h.r.Conversation().Id,
			Metrics:                 p.Metrics,
		})
		if h.r.observer != nil {
			h.r.observer.EmitMetric(ctx, p.Metrics)
		}
	}
}
func (h requestorDispatchHandler) HandleConversationMetadata(ctx context.Context, p internal_type.ConversationMetadataPacket) {
	if len(p.Metadata) > 0 {
		for _, item := range p.Metadata {
			if item == nil {
				continue
			}
			h.r.metadata[item.Key] = item.Value
		}
		if err := h.r.onAddMetadata(ctx, p.Metadata...); err != nil {
			h.r.logger.Errorf("Error in onAddMetadata: %v", err)
		}
	}
}
func (h requestorDispatchHandler) HandleUserMessageMetric(ctx context.Context, p internal_type.UserMessageMetricPacket) {
	if len(p.Metrics) > 0 {
		_ = h.r.Notify(ctx, &protos.ConversationMetric{
			AssistantConversationId: h.r.Conversation().Id,
			Metrics:                 p.Metrics,
		})
		if p.ContextID == "" {
			p.ContextID = h.r.GetID()
		}
		if err := h.r.onAddMessageMetric(ctx, "user", p.ContextID, p.Metrics); err != nil {
			h.r.logger.Errorf("Error in onMessageMetric: %v", err)
		}
		if h.r.observer != nil {
			h.r.observer.MetricCollectors().Collect(ctx, observe.MessageMetricRecord{
				MessageID:      p.ContextID,
				ConversationID: fmt.Sprintf("%d", h.r.Conversation().Id),
				Metrics:        p.Metrics,
				Time:           time.Now(),
			})
		}
	}
}
func (h requestorDispatchHandler) HandleAssistantMessageMetric(ctx context.Context, p internal_type.AssistantMessageMetricPacket) {
	if len(p.Metrics) > 0 {
		_ = h.r.Notify(ctx, &protos.ConversationMetric{
			AssistantConversationId: h.r.Conversation().Id,
			Metrics:                 p.Metrics,
		})
		if err := h.r.onAddMessageMetric(ctx, "assistant", p.ContextID, p.Metrics); err != nil {
			h.r.logger.Errorf("Error in onMessageMetric: %v", err)
		}
		if h.r.observer != nil {
			h.r.observer.MetricCollectors().Collect(ctx, observe.MessageMetricRecord{
				MessageID:      p.ContextID,
				ConversationID: fmt.Sprintf("%d", h.r.Conversation().Id),
				Metrics:        p.Metrics,
				Time:           time.Now(),
			})
		}
	}
}
func (h requestorDispatchHandler) HandleUserMessageMetadata(ctx context.Context, p internal_type.UserMessageMetadataPacket) {
	if len(p.Metadata) > 0 {
		_ = h.r.Notify(ctx, &protos.ConversationMetadata{
			AssistantConversationId: h.r.Conversation().Id,
			Metadata:                p.Metadata,
		})
		if p.ContextID == "" {
			p.ContextID = h.r.GetID()
		}
		if err := h.r.onAddMessageMetadata(ctx, "user", p.ContextID, p.Metadata); err != nil {
			h.r.logger.Errorf("Error in onAddMessageMetadata: %v", err)
		}
	}
}
func (h requestorDispatchHandler) HandleAssistantMessageMetadata(ctx context.Context, p internal_type.AssistantMessageMetadataPacket) {
	if len(p.Metadata) > 0 {
		_ = h.r.Notify(ctx, &protos.ConversationMetadata{
			AssistantConversationId: h.r.Conversation().Id,
			Metadata:                p.Metadata,
		})
		if p.ContextID == "" {
			p.ContextID = h.r.GetID()
		}
		if err := h.r.onAddMessageMetadata(ctx, "assistant", p.ContextID, p.Metadata); err != nil {
			h.r.logger.Errorf("Error in onAddMessageMetadata: %v", err)
		}
	}
}
func (h requestorDispatchHandler) HandleToolLogCreate(ctx context.Context, p internal_type.ToolLogCreatePacket) {
	if err := h.r.CreateToolLog(ctx, p.ContextID, p.ToolID, p.Name, type_enums.RECORD_IN_PROGRESS, p.Request); err != nil {
		h.r.logger.Errorf("error logging tool call start: %v", err)
	}
}
func (h requestorDispatchHandler) HandleToolLogUpdate(ctx context.Context, p internal_type.ToolLogUpdatePacket) {
	if err := h.r.UpdateToolLog(ctx, p.ToolID, type_enums.RECORD_COMPLETE, p.Response); err != nil {
		h.r.logger.Errorf("error logging tool call result: %v", err)
	}
}
func (h requestorDispatchHandler) HandleHTTPLogCreate(ctx context.Context, p internal_type.HTTPLogCreatePacket) {
	if err := h.r.CreateHTTPLog(
		ctx,
		p.Source,
		p.SourceRefID,
		p.SourceEvent,
		p.ContextID,
		p.HTTPURL,
		p.HTTPMethod,
		p.ResponseStatus,
		p.TimeTaken,
		p.RetryCount,
		p.Status,
		p.ErrorMessage,
		p.RequestPayload,
		p.ResponsePayload,
	); err != nil {
		h.r.logger.Errorf("error logging http execution: %v", err)
	}
}
func (h requestorDispatchHandler) HandleConversationEvent(ctx context.Context, p internal_type.ConversationEventPacket) {
	contextID := p.ContextID
	if contextID == "" {
		contextID = h.r.GetID()
	}
	if p.Time.IsZero() {
		p.Time = time.Now()
	}
	_ = h.r.Notify(ctx, &protos.ConversationEvent{
		Id:   contextID,
		Name: p.Name,
		Data: p.Data,
		Time: timestamppb.New(p.Time),
	})
	if h.r.observer != nil {
		h.r.observer.EventCollectors().Collect(ctx, observe.EventRecord{
			ConversationID: h.r.observer.Meta().AssistantConversationID,
			MessageID:      contextID,
			Name:           p.Name,
			Data:           p.Data,
			Time:           p.Time,
		})
	}
}
func (h requestorDispatchHandler) HandleInitializeAssistant(ctx context.Context, p internal_type.InitializeAssistantPacket) {
	assistant, err := h.r.GetAssistant(ctx, h.r.Auth(), p.Config.Assistant.AssistantId, p.Config.Assistant.Version)
	if err != nil {
		h.r.logger.Errorf("failed to retrieve assistant configuration: %+v", err)
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageAssistant,
			Error:     err,
		})
		return
	}
	h.r.assistant = assistant
	h.r.OnPacket(ctx, internal_type.InitializeConversationPacket{ContextID: p.ContextID, Config: p.Config})
}
func (h requestorDispatchHandler) HandleInitializeConversation(ctx context.Context, vl internal_type.InitializeConversationPacket) {
	if conversationID := vl.Config.GetAssistantConversationId(); conversationID > 0 {
		err := h.r.ResumeConversation(ctx, h.r.assistant, vl.Config)
		if err != nil {
			h.r.logger.Errorf("failed to resume conversation: %+v", err)
			h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
				ContextID: vl.ContextID,
				Stage:     internal_type.InitializationStageConversation,
				Error:     err,
			})
			return
		}
		h.r.OnPacket(ctx, internal_type.ConversationEventPacket{
			Name: "session",
			Data: map[string]string{
				"type":          "resumed",
				"source":        fmt.Sprintf("%v", h.r.source),
				"identifier":    h.r.identifier(vl.Config),
				"message_count": fmt.Sprintf("%d", len(h.r.GetHistories())),
			},
			Time: time.Now(),
		})

	} else {
		err := h.r.BeginConversation(ctx, h.r.assistant, type_enums.DIRECTION_INBOUND, vl.Config)
		if err != nil {
			h.r.logger.Errorf("failed to begin conversation: %+v", err)
			h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
				ContextID: vl.ContextID,
				Stage:     internal_type.InitializationStageConversation,
				Error:     err,
			})
			return
		}
		h.r.OnPacket(ctx, internal_type.ConversationEventPacket{
			Name: observe.ComponentSession,
			Data: map[string]string{
				observe.DataType: observe.EventConnected,
				"source":         fmt.Sprintf("%v", h.r.source),
				"is_new":         "true",
				"identifier":     h.r.identifier(vl.Config),
			},
			Time: time.Now(),
		})
	}
	h.r.OnPacket(ctx,
		internal_type.InitializeSessionRuntimePacket{ContextID: vl.ContextID, Config: vl.Config},
		internal_type.InitializeTelemetryPacket{ContextID: vl.ContextID})
}
func (h requestorDispatchHandler) HandleInitializeSessionRuntime(ctx context.Context, p internal_type.InitializeSessionRuntimePacket) {
	if rc, err := internal_audio_recorder.GetRecorder(h.r.logger); err != nil {
		h.r.logger.Tracef(ctx, "failed to initialize audio recorder: %+v", err)
	} else {
		h.r.recorder = rc
		h.r.recorder.Start()
		h.r.OnPacket(ctx, internal_type.ConversationEventPacket{
			Name: observe.ComponentRecording,
			Data: map[string]string{observe.DataType: observe.EventRecordingStarted},
			Time: time.Now(),
		})
	}
	for _, analysis := range h.r.assistant.AssistantAnalyses {
		exec, err := internal_analysis.NewExecutor(h.r.logger, ctx, analysis, h.r, h.r)
		if err != nil {
			h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
				ContextID: p.ContextID,
				Stage:     internal_type.InitializationStageAnalysis,
				Error:     err,
			})
			return
		}
		h.r.assistantAnalyses = append(h.r.assistantAnalyses, exec)
	}

	for _, webhook := range h.r.assistant.AssistantWebhooks {
		exec, err := internal_webhook.NewExecutor(h.r.logger, ctx, webhook, h.r, h.r)
		if err != nil {
			h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
				ContextID: p.ContextID,
				Stage:     internal_type.InitializationStageWebhook,
				Error:     err,
			})
			return
		}
		h.r.assistantWebhooks = append(h.r.assistantWebhooks, exec)
	}

	if h.r.assistant.AssistantAuthentication != nil && h.r.IsConditionAllowed(h.r.assistant.AssistantAuthentication.GetOptions(), "authentication.condition") {
		authExec, err := internal_authentication.NewExecutor(h.r.logger, ctx, h.r.assistant.AssistantAuthentication, h.r, h.r)
		if err != nil {
			h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
				ContextID: p.ContextID,
				Stage:     internal_type.InitializationStageAuthentication,
				Error:     err,
			})
			return
		}
		h.r.authenticationExecutor = authExec
	}

	if err := h.r.inputNormalizer.Initialize(ctx, h.r, p.Config); err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageInputNormalizer,
			Error:     err,
		})
		return
	}

	if err := h.r.outputNormalizer.Initialize(ctx, h.r, p.Config); err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageOutputNormalizer,
			Error:     err,
		})
		return
	}

	h.r.OnPacket(ctx,
		internal_type.ConversationMetricPacket{
			ContextID: h.r.Conversation().Id,
			Metrics: []*protos.Metric{{
				Name:        type_enums.CONVERSATION_STATUS.String(),
				Value:       type_enums.CONVERSATION_IN_PROGRESS.String(),
				Description: "Conversation is currently in progress",
			}},
		},
		internal_type.InitializeAuthenticationPacket{
			ContextID: p.ContextID,
			Config:    p.Config,
		},
	)

	if v := h.r.extractClientInformation(ctx); v != nil {
		h.r.OnPacket(ctx, internal_type.ConversationMetadataPacket{
			ContextID: h.r.Conversation().Id,
			Metadata:  v,
		})
	}
}
func (h requestorDispatchHandler) HandleInitializeAuthentication(ctx context.Context, p internal_type.InitializeAuthenticationPacket) {
	if h.r.authenticationExecutor == nil {
		h.r.OnPacket(ctx, internal_type.SessionAuthenticationSucceededPacket{
			ContextID:      p.ContextID,
			Authenticated:  false,
			Initialization: p.Config,
		})
		return
	}
	source := variable.NewCommunicationSource(h.r)
	registry := internal_namespace.NewDefaultRegistry()
	args, err := h.r.authenticationExecutor.Arguments()
	if err != nil {
		h.r.logger.Errorf("failed to get authentication arguments: %v", err)
		return
	}
	h.r.OnPacket(ctx,
		internal_type.ExecuteSessionAuthenticationPacket{
			ContextID:      p.ContextID,
			Arguments:      registry.Apply(args, source, variable.ResolveContext{}),
			Initialization: p.Config,
		}, internal_type.ConversationEventPacket{
			ContextID: p.ContextID,
			Name:      observe.ComponentSession,
			Data:      map[string]string{"type": "authentication_started"},
			Time:      time.Now(),
		})
}
func (h requestorDispatchHandler) HandleExecuteSessionAuthentication(ctx context.Context, p internal_type.ExecuteSessionAuthenticationPacket) {
	if err := h.r.authenticationExecutor.Execute(ctx, p); err != nil {
		h.r.logger.Errorf("authentication executor execute failed: %v", err)
	}
}
func (h requestorDispatchHandler) HandleSessionAuthenticationSucceeded(ctx context.Context, p internal_type.SessionAuthenticationSucceededPacket) {
	if p.Authenticated {
		h.r.applyArguments(p.Arguments)
		h.r.applyMetadata(p.Metadata)
		h.r.applyOptions(p.Options)
	}

	switch p.Initialization.StreamMode {
	case protos.StreamMode_STREAM_MODE_TEXT:
		h.r.SwitchMode(type_enums.TextMode)
		h.r.OnPacket(ctx,
			internal_type.InitializeAssistantExecutorPacket{
				ContextID: p.ContextID,
				Config:    p.Initialization,
			})
	case protos.StreamMode_STREAM_MODE_AUDIO:
		h.r.OnPacket(ctx,
			internal_type.InitializeSpeechToTextPacket{
				ContextID: p.ContextID,
				Config:    p.Initialization,
			},
			internal_type.InitializeTextToSpeechPacket{
				ContextID: p.ContextID,
				Config:    p.Initialization,
			},
			internal_type.InitializeAssistantExecutorPacket{
				ContextID: p.ContextID,
				Config:    p.Initialization,
			},
			internal_type.InitializeVoiceActivityDetectionPacket{
				ContextID: p.ContextID,
				Config:    p.Initialization,
			},
			internal_type.InitializeEndOfSpeechPacket{
				ContextID: p.ContextID,
				Config:    p.Initialization,
			},
			internal_type.InitializeDenoisePacket{
				ContextID: p.ContextID,
				Config:    p.Initialization,
			},
		)
		h.r.SwitchMode(type_enums.AudioMode)
	}

}

func (h requestorDispatchHandler) HandleInitializeAssistantExecutorPacket(ctx context.Context, p internal_type.InitializeAssistantExecutorPacket) {
	assistantExec, err := internal_llm.NewExecutor(h.r.logger, ctx, h.r, p.Config)
	if err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageService,
			Error:     err,
		})
		return
	}
	h.r.assistantExecutor = assistantExec
	h.r.OnPacket(ctx, internal_type.InitializeBehaviorPacket{
		ContextID: p.ContextID,
		Config:    p.Config,
	})
}

func (h requestorDispatchHandler) HandleInitializeSpeechToText(ctx context.Context, p internal_type.InitializeSpeechToTextPacket) {
	cfg, err := h.r.GetSpeechToTextTransformer()
	if err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageSpeechToText,
			Error:     err,
		})
		return
	}
	options := utils.MergeMaps(h.r.options, cfg.GetOptions())
	credentialId, err := options.GetUint64("rapida.credential_id")
	if err != nil {
		h.r.logger.Errorf("unable to find credential from options %+v", err)
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageSpeechToText,
			Error:     err,
		})
		return
	}
	credential, err := h.r.VaultCaller().GetCredential(ctx, h.r.Auth(), credentialId)
	if err != nil {
		h.r.logger.Errorf("Api call to find credential failed %+v", err)
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageSpeechToText,
			Error:     err,
		})
		return
	}
	atransformer, err := internal_transformer.GetSpeechToTextTransformer(
		ctx,
		h.r.logger,
		cfg.AudioProvider,
		credential,
		func(pkt ...internal_type.Packet) error { return h.r.OnPacket(ctx, pkt...) },
		options)
	if err != nil {
		h.r.logger.Errorf("unable to create input audio transformer with error %v", err)
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageSpeechToText,
			Error:     err,
		})
		return
	}
	if err := atransformer.Initialize(); err != nil {
		h.r.logger.Errorf("unable to initialize transformer %v", err)
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageSpeechToText,
			Error:     err,
		})
		return
	}
	h.r.speechToTextTransformer = atransformer
}

func (h requestorDispatchHandler) HandleInitializeDenoise(ctx context.Context, p internal_type.InitializeDenoisePacket) {
	cfg, err := h.r.GetSpeechToTextTransformer()
	if err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageDenoise,
			Error:     err,
		})
		return
	}
	options := utils.MergeMaps(h.r.options, cfg.GetOptions())
	denoise, err := internal_denoiser.GetDenoiser(
		ctx, h.r.logger, internal_audio.RAPIDA_INTERNAL_AUDIO_CONFIG,
		h.r.OnPacket,
		options)
	if err != nil {
		h.r.logger.Errorf("error while initializing denoiser %+v", err)
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageDenoise,
			Error:     err,
		})
		return
	}
	h.r.denoiser = denoise
}
func (h requestorDispatchHandler) HandleInitializeTextToSpeech(ctx context.Context, p internal_type.InitializeTextToSpeechPacket) {
	outputTransformer, err := h.r.GetTextToSpeechTransformer()
	if err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageTextToSpeech,
			Error:     err,
		})
		return
	}
	speakerOpts := utils.MergeMaps(outputTransformer.GetOptions())
	credentialId, err := speakerOpts.GetUint64("rapida.credential_id")
	if err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageTextToSpeech,
			Error:     err,
		})
		return
	}
	credential, err := h.r.VaultCaller().GetCredential(ctx, h.r.Auth(), credentialId)
	if err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageTextToSpeech,
			Error:     err,
		})
		return
	}
	// Use the session ctx (not errgroup's ectx) so the transformer's stream
	// lifecycle is tied to the session, not the short-lived errgroup.
	atransformer, err := internal_transformer.GetTextToSpeechTransformer(
		ctx, h.r.logger,
		outputTransformer.GetName(),
		credential,
		func(pkt ...internal_type.Packet) error { return h.r.OnPacket(ctx, pkt...) },
		speakerOpts)
	if err != nil {
		h.r.logger.Errorf("tts: unable to create transformer %v", err)
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageTextToSpeech,
			Error:     err,
		})
		return
	}
	if err := atransformer.Initialize(); err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageTextToSpeech,
			Error:     err,
		})
		return
	}
	h.r.textToSpeechTransformer = atransformer
	h.r.SwitchMode(type_enums.AudioMode)
}
func (h requestorDispatchHandler) HandleInitializeVoiceActivityDetection(ctx context.Context, p internal_type.InitializeVoiceActivityDetectionPacket) {
	config := p.Config
	if config.StreamMode == protos.StreamMode_STREAM_MODE_AUDIO {
		cfg, err := h.r.GetSpeechToTextTransformer()
		if err != nil {
			h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
				ContextID: p.ContextID,
				Stage:     internal_type.InitializationStageVoiceActivity,
				Error:     err,
			})
			return
		}

		options := utils.MergeMaps(h.r.options, cfg.GetOptions())
		vad, err := internal_vad.GetVAD(ctx, h.r.logger, h.r.OnPacket, options)
		if err != nil {
			h.r.logger.Errorf("error while initializing vad %+v", err)
			h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
				ContextID: p.ContextID,
				Stage:     internal_type.InitializationStageVoiceActivity,
				Error:     err,
			})
			return
		}
		h.r.vad = vad
	}

}
func (h requestorDispatchHandler) HandleInitializeEndOfSpeech(ctx context.Context, p internal_type.InitializeEndOfSpeechPacket) {
	cfg, err := h.r.GetSpeechToTextTransformer()
	if err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageEndOfSpeech,
			Error:     err,
		})
		return
	}
	options := utils.MergeMaps(h.r.options, cfg.GetOptions())
	endOfSpeech, err := internal_end_of_speech.GetEndOfSpeech(ctx,
		h.r.logger,
		h.r.OnPacket,
		options)
	if err != nil {
		return
	}
	h.r.endOfSpeech = endOfSpeech
}
func (h requestorDispatchHandler) HandleInitializeBehavior(ctx context.Context, p internal_type.InitializeBehaviorPacket) {
	h.r.initializeBehavior(ctx)
	event := utils.ConversationResume
	if p.Config.GetAssistantConversationId() == 0 {
		event = utils.ConversationBegin
	}
	h.r.OnPacket(ctx, internal_type.InitializationCompletedPacket{
		ContextID: p.ContextID,
		Config:    p.Config,
		Event:     event,
	})
}

func (h requestorDispatchHandler) HandleModeSwitchRequested(ctx context.Context, p internal_type.ModeSwitchRequestedPacket) {
	switch p.StreamMode {
	case protos.StreamMode_STREAM_MODE_AUDIO:
		if h.r.GetMode().Audio() {
			h.r.Notify(ctx, &protos.ConversationConfiguration{StreamMode: p.StreamMode})
			return
		}
		if !h.r.canSwitchSession() {
			h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
				ContextID:  p.ContextID,
				StreamMode: p.StreamMode,
				Type:       internal_type.ModeSwitchErrorTypePreconditionNotReady,
				Error:      errors.New("mode switch requested while session is not ready"),
			})
			return
		}
		if err := h.r.sessionLifecycle.Transition(adapter_lifecycle.EventSwitchRequested); err != nil {
			h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
				ContextID:  p.ContextID,
				StreamMode: p.StreamMode,
				Type:       internal_type.ModeSwitchErrorTypePreconditionNotReady,
				Error:      err,
			})
			return
		}
		// Kick off the serial init chain. Each handler emits the next packet
		// on success; any failure emits a non-recoverable ModeSwitchErrorPacket
		// which routes through HandleError → OnDisconnect.
		h.r.OnPacket(ctx,
			internal_type.ModeSwitchInitializeSpeechToTextPacket{
				ContextID: p.ContextID, StreamMode: p.StreamMode,
			},
			internal_type.ModeSwitchInitializeVoiceActivityDetectionPacket{ContextID: p.ContextID, StreamMode: p.StreamMode},
			internal_type.ModeSwitchInitializeEndOfSpeechPacket{ContextID: p.ContextID, StreamMode: p.StreamMode},
			internal_type.ModeSwitchInitializeDenoisePacket{ContextID: p.ContextID, StreamMode: p.StreamMode},
			internal_type.ModeSwitchInitializeTextToSpeechPacket{ContextID: p.ContextID, StreamMode: p.StreamMode},
			internal_type.ModeSwitchCompletedPacket{ContextID: p.ContextID, StreamMode: p.StreamMode},
		)
	case protos.StreamMode_STREAM_MODE_TEXT:
		if h.r.GetMode().Text() {
			h.r.Notify(ctx, &protos.ConversationConfiguration{StreamMode: p.StreamMode})
			return
		}
		if !h.r.canSwitchSession() {
			h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
				ContextID:  p.ContextID,
				StreamMode: p.StreamMode,
				Type:       internal_type.ModeSwitchErrorTypePreconditionNotReady,
				Error:      errors.New("mode switch requested while session is not ready"),
			})
			return
		}
		if err := h.r.sessionLifecycle.Transition(adapter_lifecycle.EventSwitchRequested); err != nil {
			h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
				ContextID:  p.ContextID,
				StreamMode: p.StreamMode,
				Type:       internal_type.ModeSwitchErrorTypePreconditionNotReady,
				Error:      err,
			})
			return
		}
		// Fan out the 5 finalize packets in parallel (each is AsyncPacket) and
		// emit ModeSwitchCompleted in the same batch — the client is told it's
		// in text mode immediately while the 5 component closes happen in
		// background goroutines.
		h.r.OnPacket(ctx,
			internal_type.ModeSwitchFinalizeSpeechToTextPacket{ContextID: p.ContextID, StreamMode: p.StreamMode},
			internal_type.ModeSwitchFinalizeTextToSpeechPacket{ContextID: p.ContextID, StreamMode: p.StreamMode},
			internal_type.ModeSwitchFinalizeVoiceActivityDetectionPacket{ContextID: p.ContextID, StreamMode: p.StreamMode},
			internal_type.ModeSwitchFinalizeEndOfSpeechPacket{ContextID: p.ContextID, StreamMode: p.StreamMode},
			internal_type.ModeSwitchFinalizeDenoisePacket{ContextID: p.ContextID, StreamMode: p.StreamMode},
			internal_type.ModeSwitchCompletedPacket{ContextID: p.ContextID, StreamMode: p.StreamMode},
		)
	default:
		err := fmt.Errorf("unsupported mode switch request: %s", p.StreamMode.String())
		h.r.logger.Warnf(err.Error())
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID:  p.ContextID,
			StreamMode: p.StreamMode,
			Type:       internal_type.ModeSwitchErrorTypeUnsupportedMode,
			Error:      err,
		})
	}
}

// Init chain — sequential. Each handler runs sync on the bootstrap goroutine,
// and on success emits the next packet in the chain. On failure: emits
// ModeSwitchErrorPacket (non-recoverable) which routes through HandleError →
// OnDisconnect, tearing down the session.

func (h requestorDispatchHandler) HandleModeSwitchInitializeSpeechToText(ctx context.Context, p internal_type.ModeSwitchInitializeSpeechToTextPacket) {
	cfg, err := h.r.GetSpeechToTextTransformer()
	if err != nil {
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID,
			Type:      internal_type.ModeSwitchErrorTypeInitializeSpeechToText,
			Error:     err,
		})
		return
	}
	options := utils.MergeMaps(h.r.options, cfg.GetOptions())
	credentialId, err := options.GetUint64("rapida.credential_id")
	if err != nil {
		h.r.logger.Errorf("unable to find credential from options %+v", err)
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID,
			Type:      internal_type.ModeSwitchErrorTypeInitializeSpeechToText,
			Error:     err,
		})
		return
	}
	credential, err := h.r.VaultCaller().GetCredential(ctx, h.r.Auth(), credentialId)
	if err != nil {
		h.r.logger.Errorf("Api call to find credential failed %+v", err)
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID,
			Type:      internal_type.ModeSwitchErrorTypeInitializeSpeechToText,
			Error:     err,
		})
		return
	}
	atransformer, err := internal_transformer.GetSpeechToTextTransformer(
		ctx,
		h.r.logger,
		cfg.AudioProvider,
		credential,
		func(pkt ...internal_type.Packet) error { return h.r.OnPacket(ctx, pkt...) },
		options)
	if err != nil {
		h.r.logger.Errorf("unable to create input audio transformer with error %v", err)
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID,
			Type:      internal_type.ModeSwitchErrorTypeInitializeSpeechToText,
			Error:     err,
		})
		return
	}
	if err := atransformer.Initialize(); err != nil {
		h.r.logger.Errorf("unable to initialize transformer %v", err)
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID,
			Type:      internal_type.ModeSwitchErrorTypeInitializeSpeechToText,
			Error:     err,
		})
		return
	}
	h.r.speechToTextTransformer = atransformer
}

func (h requestorDispatchHandler) HandleModeSwitchInitializeTextToSpeech(ctx context.Context, p internal_type.ModeSwitchInitializeTextToSpeechPacket) {
	outputTransformer, err := h.r.GetTextToSpeechTransformer()
	if err != nil {
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID, StreamMode: p.StreamMode,
			Type: internal_type.ModeSwitchErrorTypeInitializeTextToSpeech, Error: err,
		})
		return
	}
	speakerOpts := utils.MergeMaps(outputTransformer.GetOptions())
	credentialId, err := speakerOpts.GetUint64("rapida.credential_id")
	if err != nil {
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID, StreamMode: p.StreamMode,
			Type: internal_type.ModeSwitchErrorTypeInitializeTextToSpeech, Error: err,
		})
		return
	}
	credential, err := h.r.VaultCaller().GetCredential(ctx, h.r.Auth(), credentialId)
	if err != nil {
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID, StreamMode: p.StreamMode,
			Type: internal_type.ModeSwitchErrorTypeInitializeTextToSpeech, Error: err,
		})
		return
	}
	// Use the session ctx (not errgroup's ectx) so the transformer's stream
	// lifecycle is tied to the session, not the short-lived errgroup.
	atransformer, err := internal_transformer.GetTextToSpeechTransformer(
		ctx, h.r.logger,
		outputTransformer.GetName(),
		credential,
		func(pkt ...internal_type.Packet) error { return h.r.OnPacket(ctx, pkt...) },
		speakerOpts)
	if err != nil {
		h.r.logger.Errorf("tts: unable to create transformer %v", err)
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID, StreamMode: p.StreamMode,
			Type: internal_type.ModeSwitchErrorTypeInitializeTextToSpeech, Error: err,
		})
		return
	}
	if err := atransformer.Initialize(); err != nil {
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID, StreamMode: p.StreamMode,
			Type: internal_type.ModeSwitchErrorTypeInitializeTextToSpeech, Error: err,
		})
		return
	}
	h.r.textToSpeechTransformer = atransformer
}

func (h requestorDispatchHandler) HandleModeSwitchInitializeVoiceActivityDetection(ctx context.Context, p internal_type.ModeSwitchInitializeVoiceActivityDetectionPacket) {
	cfg, err := h.r.GetSpeechToTextTransformer()
	if err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageVoiceActivity,
			Error:     err,
		})
		return
	}
	options := utils.MergeMaps(h.r.options, cfg.GetOptions())
	vad, err := internal_vad.GetVAD(ctx, h.r.logger, h.r.OnPacket, options)
	if err != nil {
		h.r.logger.Errorf("error while initializing vad %+v", err)
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageVoiceActivity,
			Error:     err,
		})
	}
	h.r.vad = vad
}

func (h requestorDispatchHandler) HandleModeSwitchInitializeDenoise(ctx context.Context, p internal_type.ModeSwitchInitializeDenoisePacket) {
	cfg, err := h.r.GetSpeechToTextTransformer()
	if err != nil {
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID, StreamMode: p.StreamMode,
			Type: internal_type.ModeSwitchErrorTypeInitializeDenoise, Error: err,
		})
		return
	}

	options := utils.MergeMaps(h.r.args, cfg.GetOptions())
	denoise, err := internal_denoiser.GetDenoiser(ctx, h.r.logger, internal_audio.RAPIDA_INTERNAL_AUDIO_CONFIG,
		func(pctx context.Context, pkt ...internal_type.Packet) error { return h.r.OnPacket(pctx, pkt...) },
		options)
	if err != nil {
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID, StreamMode: p.StreamMode,
			Type: internal_type.ModeSwitchErrorTypeInitializeDenoise, Error: err,
		})
		return
	}
	h.r.denoiser = denoise
}

func (h requestorDispatchHandler) HandleModeSwitchInitializeEndOfSpeech(ctx context.Context, p internal_type.ModeSwitchInitializeEndOfSpeechPacket) {
	cfg, err := h.r.GetSpeechToTextTransformer()
	if err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageEndOfSpeech,
			Error:     err,
		})
		return
	}
	options := utils.MergeMaps(h.r.options, cfg.GetOptions())
	endOfSpeech, err := internal_end_of_speech.GetEndOfSpeech(ctx,
		h.r.logger,
		h.r.OnPacket,
		options)
	if err != nil {
		h.r.logger.Warnf("unable to initialize text analyzer %+v", err)
		return
	}
	h.r.endOfSpeech = endOfSpeech

}

// Finalize handlers — fire-and-forget. Each runs in its own goroutine
// (AsyncPacket). The client has already been confirmed in text mode by the
// time these run. Errors are logged only — no client-facing error packet.

func (h requestorDispatchHandler) HandleModeSwitchFinalizeSpeechToText(ctx context.Context, p internal_type.ModeSwitchFinalizeSpeechToTextPacket) {
	if h.r.speechToTextTransformer != nil {
		if err := h.r.speechToTextTransformer.Close(ctx); err != nil {
			h.r.logger.Warnf("mode-switch finalize speech-to-text: %v", err)
		}
		h.r.speechToTextTransformer = nil
	}
}

func (h requestorDispatchHandler) HandleModeSwitchFinalizeTextToSpeech(ctx context.Context, p internal_type.ModeSwitchFinalizeTextToSpeechPacket) {
	if h.r.textToSpeechTransformer != nil {
		if err := h.r.textToSpeechTransformer.Close(ctx); err != nil {
			h.r.logger.Warnf("mode-switch finalize text-to-speech: %v", err)
		}
		h.r.textToSpeechTransformer = nil
	}
}

func (h requestorDispatchHandler) HandleModeSwitchFinalizeVoiceActivityDetection(ctx context.Context, p internal_type.ModeSwitchFinalizeVoiceActivityDetectionPacket) {
	if h.r.vad != nil {
		if err := h.r.vad.Close(); err != nil {
			h.r.logger.Warnf("mode-switch finalize voice activity detection: %v", err)
		}
		h.r.vad = nil
	}
}

func (h requestorDispatchHandler) HandleModeSwitchFinalizeEndOfSpeech(ctx context.Context, p internal_type.ModeSwitchFinalizeEndOfSpeechPacket) {
	if h.r.endOfSpeech != nil {
		if err := h.r.endOfSpeech.Close(); err != nil {
			h.r.logger.Warnf("cancel end of speech with error %v", err)
		}
		h.r.endOfSpeech = nil
	}
}

func (h requestorDispatchHandler) HandleModeSwitchFinalizeDenoise(ctx context.Context, p internal_type.ModeSwitchFinalizeDenoisePacket) {
	if h.r.denoiser != nil {
		if err := h.r.denoiser.Close(); err != nil {
			h.r.logger.Warnf("mode-switch finalize denoiser: %v", err)
		}
		h.r.denoiser = nil
	}
}

func (h requestorDispatchHandler) HandleModeSwitchCompleted(ctx context.Context, p internal_type.ModeSwitchCompletedPacket) {
	currentMode := h.r.GetMode()
	switch p.StreamMode {
	case protos.StreamMode_STREAM_MODE_AUDIO:
		if !currentMode.Audio() {
			h.r.SwitchMode(type_enums.AudioMode)
		}
	case protos.StreamMode_STREAM_MODE_TEXT:
		if !currentMode.Text() {
			h.r.SwitchMode(type_enums.TextMode)
		}
	default:
		err := fmt.Errorf("mode switch completed with unsupported mode: %s", p.StreamMode.String())
		h.r.logger.Warnf(err.Error())
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID:  p.ContextID,
			StreamMode: p.StreamMode,
			Type:       internal_type.ModeSwitchErrorTypeUnsupportedMode,
			Error:      err,
		})
		return
	}
	if err := h.r.sessionLifecycle.Transition(adapter_lifecycle.EventSwitchCompleted); err != nil {
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID:  p.ContextID,
			StreamMode: p.StreamMode,
			Type:       internal_type.ModeSwitchErrorTypeUnknown,
			Error:      err,
		})
		return
	}

	_ = h.r.Notify(ctx, &protos.ConversationConfiguration{
		StreamMode: p.StreamMode,
	})
}

func (h requestorDispatchHandler) HandleInitializationCompleted(ctx context.Context, p internal_type.InitializationCompletedPacket) {
	if err := h.r.sessionLifecycle.Transition(adapter_lifecycle.EventInitializationCompleted); err != nil {
		h.r.logger.Tracef(ctx, "session lifecycle init-completed transition ignored: %v", err)
	}
	h.r.notifyConfiguration(ctx, p.Config, h.r.assistantConversation)
	h.r.OnPacket(ctx, internal_type.InitializeInboundDispatcherPacket{ContextID: p.ContextID})
	h.r.OnPacket(ctx, internal_type.ConversationEventPacket{
		ContextID: p.ContextID,
		Name:      observe.ComponentSession,
		Data: map[string]string{
			observe.DataType: observe.EventInitialized,
			"event":          p.Event.Get(),
			observe.DataMode: h.r.GetMode().String(),
		},
		Time: time.Now(),
	}, internal_type.ExecuteWebhookPacket{
		ContextID: p.ContextID,
		Event:     p.Event,
	})

}

func (h requestorDispatchHandler) HandleInitializeTelemetry(ctx context.Context, p internal_type.InitializeTelemetryPacket) {
	defer h.r.OnPacket(ctx, internal_type.InitializeOutboundDispatcherPacket{ContextID: p.ContextID})
	h.r.initializeCollectors(ctx)
}

func (h requestorDispatchHandler) HandleInitializeOutboundDispatcher(ctx context.Context, p internal_type.InitializeOutboundDispatcherPacket) {
	h.r.lowStart.Do(func() {
		go h.r.runLowDispatcher(h.r.sessionCtx)
	})
}

func (h requestorDispatchHandler) HandleInitializeInboundDispatcher(ctx context.Context, p internal_type.InitializeInboundDispatcherPacket) {
	h.r.inputStart.Do(func() {
		go h.r.runInputDispatcher(h.r.sessionCtx)
	})
}

func (h requestorDispatchHandler) HandleFinalizeBehavior(ctx context.Context, p internal_type.FinalizeBehaviorPacket) {
	if h.r.idleTimeoutTimer != nil {
		h.r.idleTimeoutTimer.Stop()
	}
	if h.r.maxSessionTimer != nil {
		h.r.maxSessionTimer.Stop()
	}
	h.r.OnPacket(ctx, internal_type.FinalizeEndOfSpeechPacket{ContextID: p.ContextID})
}
func (h requestorDispatchHandler) HandleFinalizeEndOfSpeech(ctx context.Context, p internal_type.FinalizeEndOfSpeechPacket) {
	if h.r.endOfSpeech != nil {
		if err := h.r.endOfSpeech.Close(); err != nil {
			h.r.logger.Tracef(ctx, "failed to close end of speech: %+v", err)
		}
		h.r.endOfSpeech = nil
	}
	h.r.OnPacket(ctx, internal_type.FinalizeVoiceActivityDetectionPacket{ContextID: p.ContextID})
}
func (h requestorDispatchHandler) HandleFinalizeVoiceActivityDetection(ctx context.Context, p internal_type.FinalizeVoiceActivityDetectionPacket) {
	if h.r.vad != nil {
		if err := h.r.vad.Close(); err != nil {
			h.r.logger.Tracef(ctx, "failed to close voice activity detection: %+v", err)
		}
		h.r.vad = nil
	}
	h.r.OnPacket(ctx, internal_type.FinalizeTextToSpeechPacket{ContextID: p.ContextID})
}
func (h requestorDispatchHandler) HandleFinalizeTextToSpeech(ctx context.Context, p internal_type.FinalizeTextToSpeechPacket) {
	if h.r.textToSpeechTransformer != nil {
		if err := h.r.textToSpeechTransformer.Close(ctx); err != nil {
			h.r.logger.Errorf("cancel all output transformer with error %v", err)
		}
		h.r.textToSpeechTransformer = nil
	}
	h.r.OnPacket(ctx, internal_type.FinalizeSpeechToTextPacket{ContextID: p.ContextID})
}
func (h requestorDispatchHandler) HandleFinalizeSpeechToText(ctx context.Context, p internal_type.FinalizeSpeechToTextPacket) {
	if h.r.speechToTextTransformer != nil {
		if err := h.r.speechToTextTransformer.Close(ctx); err != nil {
			h.r.logger.Warnf("mode-switch finalize speech-to-text: %v", err)
		}
		h.r.speechToTextTransformer = nil
	}
	h.r.OnPacket(ctx, internal_type.FinalizeAuthenticationPacket{ContextID: p.ContextID})
}
func (h requestorDispatchHandler) HandleFinalizeAuthentication(ctx context.Context, p internal_type.FinalizeAuthenticationPacket) {
	h.r.OnPacket(ctx, internal_type.FinalizeSessionRuntimePacket{ContextID: p.ContextID})
}
func (h requestorDispatchHandler) HandleFinalizeSessionRuntime(ctx context.Context, p internal_type.FinalizeSessionRuntimePacket) {
	if h.r.outputNormalizer != nil {
		h.r.outputNormalizer.Close(ctx)
		h.r.outputNormalizer = nil
	}

	//
	if h.r.inputNormalizer != nil {
		h.r.inputNormalizer.Close(ctx)
		h.r.inputNormalizer = nil
	}

	//
	if h.r.recorder != nil {
		utils.Go(ctx, func() {
			userAudio, systemAudio, err := h.r.recorder.Persist()
			if err != nil {
				h.r.logger.Tracef(ctx, "failed to persist audio recording: %+v", err)
				return
			}
			if err = h.r.CreateConversationRecording(ctx, userAudio, systemAudio); err != nil {
				h.r.logger.Tracef(ctx, "failed to create conversation recording record: %+v", err)
			}
		})
	}
	// analysis -> webhooks -> finalize conversation
	h.r.OnPacket(ctx,
		internal_type.ExecuteAnalysisPacket{
			ContextID:      p.ContextID,
			ConversationID: h.r.assistantConversation.Id,
			Auth:           h.r.auth,
		},
		internal_type.ExecuteWebhookPacket{
			ContextID: p.ContextID,
			Event:     utils.ConversationCompleted,
		},
		internal_type.FinalizeConversationPacket{ContextID: p.ContextID})

}
func (h requestorDispatchHandler) HandleFinalizeConversation(ctx context.Context, p internal_type.FinalizeConversationPacket) {
	if h.r.observer != nil {
		h.r.observer.EventCollectors().Collect(ctx, observe.EventRecord{
			ConversationID: h.r.observer.Meta().AssistantConversationID,
			MessageID:      h.r.GetID(),
			Name:           observe.ComponentSession,
			Data:           map[string]string{observe.DataType: observe.EventDisconnected, observe.DataMessages: fmt.Sprintf("%d", len(h.r.GetHistories()))},
			Time:           time.Now(),
		})
	}
	h.r.shutdownCollectors(ctx)
	h.r.OnPacket(ctx, internal_type.FinalizeAssistantPacket{ContextID: p.ContextID})
}
func (h requestorDispatchHandler) HandleFinalizeAssistant(ctx context.Context, p internal_type.FinalizeAssistantPacket) {
	if h.r.assistantExecutor != nil {
		if err := h.r.assistantExecutor.Close(ctx); err != nil {
			h.r.logger.Errorf("failed to close assistant executor: %v", err)
		}
	}
	h.r.OnPacket(ctx, internal_type.FinalizationCompletedPacket{ContextID: p.ContextID})
}
func (h requestorDispatchHandler) HandleFinalizationCompleted(ctx context.Context, p internal_type.FinalizationCompletedPacket) {
	if err := h.r.sessionLifecycle.Transition(adapter_lifecycle.EventDisconnectCompleted); err != nil {
		h.r.logger.Tracef(ctx, "session lifecycle disconnect-completed transition ignored: %v", err)
	}
	h.r.cancelSession()
}

func (h requestorDispatchHandler) HandleExecuteAnalysis(ctx context.Context, p internal_type.ExecuteAnalysisPacket) {
	if len(h.r.assistantAnalyses) == 0 {
		return
	}
	source := variable.NewCommunicationSource(h.r)
	registry := internal_namespace.NewDefaultRegistry().With("event", &internal_namespace.EventNamespace{})
	for _, initializedAnalysis := range h.r.assistantAnalyses {
		if !h.r.IsConditionAllowed(initializedAnalysis.Options(), "analysis.condition") {
			arguments, err := initializedAnalysis.Arguments()
			if err != nil {
				h.r.logger.Warnw("failed to get analysis arguments", "name", initializedAnalysis.Name(), "error", err)
				continue
			}
			p.Arguments = registry.Apply(arguments, source, variable.ResolveContext{Event: utils.ConversationCompleted.Get()})
			if err := initializedAnalysis.Execute(ctx, p); err != nil {
				h.r.logger.Warnw("analysis execution failed", "name", initializedAnalysis.Name(), "error", err)
			}
		}

	}
}

func (h requestorDispatchHandler) HandleExecuteWebhook(ctx context.Context, p internal_type.ExecuteWebhookPacket) {
	if len(h.r.assistantWebhooks) == 0 {
		return
	}
	source := variable.NewCommunicationSource(h.r)
	registry := internal_namespace.NewDefaultRegistry().With("event", &internal_namespace.EventNamespace{})
	for _, initializedWebhook := range h.r.assistantWebhooks {
		if h.r.IsConditionAllowed(initializedWebhook.Options(), "webhook.condition") {
			arguments, err := initializedWebhook.Arguments()
			if err != nil {
				h.r.logger.Warnw("failed to get webhook arguments", "webhookID", initializedWebhook.Name(), "error", err)
				continue
			}
			p.Arguments = registry.Apply(arguments, source, variable.ResolveContext{Event: p.Event.Get()})
			if err := initializedWebhook.Execute(ctx, p); err != nil {
				h.r.logger.Warnw("webhook execution failed", "webhookID", initializedWebhook.Name(), "error", err)
			}
		}
	}
}

func (h requestorDispatchHandler) callEndOfSpeech(ctx context.Context, vl internal_type.Packet) error {
	if h.r.endOfSpeech != nil {
		utils.Go(ctx, func() {
			if err := h.r.endOfSpeech.Analyze(ctx, vl); err != nil {
				h.r.logger.Errorf("end of speech analyze error: %v", err)
			}
		})
		return nil
	}
	return errors.New("end of speech analyzer not configured")
}

func (h requestorDispatchHandler) callInputNormalizer(ctx context.Context, vl internal_type.EndOfSpeechPacket) error {
	if h.r.inputNormalizer == nil {
		return errors.New("input inputNormalizer not configured")
	}
	if err := h.r.inputNormalizer.Normalize(ctx, vl); err != nil {
		h.r.logger.Errorf("input inputNormalizer error: %v", err)
		return err
	}
	return nil
}

func (r *genericRequestor) notifyConfiguration(ctx context.Context, config *protos.ConversationInitialization, conversation *internal_conversation_entity.AssistantConversation) {
	options := config.GetOptions()
	mergedOptions := map[string]interface{}{}
	if base, err := utils.AnyMapToInterfaceMap(config.GetOptions()); err == nil {
		mergedOptions = base
	}

	if outputAudio, err := r.GetTextToSpeechTransformer(); err == nil && outputAudio != nil {
		outputOpts := outputAudio.GetOptions()
		if ambient, err := outputOpts.GetString("speaker.ambient"); err == nil && ambient != "" {
			mergedOptions["speaker.ambient"] = ambient
		}
		if volume, err := outputOpts.GetString("speaker.ambient_volume"); err == nil && volume != "" {
			mergedOptions["speaker.ambient_volume"] = volume
		} else if volumeNum, err := outputOpts.GetUint64("speaker.ambient_volume"); err == nil {
			mergedOptions["speaker.ambient_volume"] = volumeNum
		}
	}

	if len(mergedOptions) > 0 {
		if anyMap, err := utils.InterfaceMapToAnyMap(mergedOptions); err == nil {
			options = anyMap
		}
	}
	if err := r.Notify(ctx, &protos.ConversationInitialization{
		AssistantConversationId: conversation.Id,
		Assistant: &protos.AssistantDefinition{
			AssistantId: r.assistant.Id,
			Version:     utils.GetVersionString(r.assistant.AssistantProviderId),
		},
		Args:         config.GetArgs(),
		Metadata:     config.GetMetadata(),
		Options:      options,
		StreamMode:   config.GetStreamMode(),
		UserIdentity: config.GetUserIdentity(),
		Time:         timestamppb.Now(),
	}); err != nil {
		r.logger.Errorf("failed to send configuration notification: %v", err)
	}
}

func (r *genericRequestor) extractClientInformation(ctx context.Context) []*protos.Metadata {
	var metadata []*protos.Metadata
	clientInfo := pkg_types.GetClientInfoFromGrpcContext(ctx)
	if clientInfo == nil {
		return nil
	}

	if clientInfo.Timezone != "" {
		metadata = append(metadata, &protos.Metadata{Key: "client.timezone", Value: clientInfo.Timezone})
	}
	if clientInfo.Platform != "" {
		metadata = append(metadata, &protos.Metadata{Key: "client.platform", Value: clientInfo.Platform})
	}
	if clientInfo.Language != "" {
		metadata = append(metadata, &protos.Metadata{Key: "client.language", Value: clientInfo.Language})
	}
	if clientInfo.UserAgent != "" {
		metadata = append(metadata, &protos.Metadata{Key: "client.user_agent", Value: clientInfo.UserAgent})
	}
	if clientInfo.Referrer != "" {
		metadata = append(metadata, &protos.Metadata{Key: "client.referrer", Value: clientInfo.Referrer})
	}
	if clientInfo.ConnectionType != "" {
		metadata = append(metadata, &protos.Metadata{Key: "client.connection_type", Value: clientInfo.ConnectionType})
	}
	if clientInfo.Latitude != 0 || clientInfo.Longitude != 0 {
		metadata = append(metadata,
			&protos.Metadata{Key: "client.latitude", Value: fmt.Sprintf("%f", clientInfo.Latitude)},
			&protos.Metadata{Key: "client.longitude", Value: fmt.Sprintf("%f", clientInfo.Longitude)},
		)
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func (r *genericRequestor) IsConditionAllowed(opts utils.Option, key string) bool {
	raw, err := opts.GetString(key)
	if err != nil {
		return true
	}
	parsed, parseErr := internal_condition.Parse(raw)
	if parseErr != nil {
		r.logger.Warnf("invalid %s: %v", key, parseErr)
		return false
	}
	allowed, evalErr := parsed.Run(
		internal_condition.ConditionValue{RuleType: internal_condition.RuleTypeSource, Value: r.GetSource().Get()},
		internal_condition.ConditionValue{RuleType: internal_condition.RuleTypeMode, Value: r.GetMode().String()},
		internal_condition.ConditionValue{RuleType: internal_condition.RuleTypeDirection, Value: r.Conversation().Direction.String()},
	)
	if evalErr != nil {
		r.logger.Warnf("condition eval failed for %s: %v", key, evalErr)
		return false
	}
	return allowed
}
