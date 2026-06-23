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
	"sync"
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
	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_transformer "github.com/rapidaai/api/assistant-api/internal/transformer"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	internal_vad "github.com/rapidaai/api/assistant-api/internal/vad"
	"github.com/rapidaai/api/assistant-api/internal/variable"
	internal_namespace "github.com/rapidaai/api/assistant-api/internal/variable/namespace"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type requestorDispatchHandler struct {
	r *genericRequestor
}

func (h requestorDispatchHandler) HandleUserText(ctx context.Context, vl internal_type.UserTextReceivedPacket) {
	h.HandleInterruptionDetected(ctx, internal_type.InterruptionDetectedPacket{
		ContextID: h.r.GetID(),
		Source:    internal_type.InterruptionSourceWord,
	})

	vl.ContextID = h.r.GetID()
	h.r.OnPacket(ctx,
		internal_type.InterimEndOfSpeechPacket{Speech: vl.Text, ContextID: vl.ContextID},
		internal_type.ObservabilityEventRecordPacket{
			ContextID: vl.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeUserMessage,
			Record:    observability.NewMessageRecord(vl.ContextID, observability.ComponentEOS, observability.EOSStarted, observability.MessageRoleUser, observability.Attributes{"speech": vl.Text}),
		},
		internal_type.EndOfSpeechPacket{Speech: vl.Text, ContextID: vl.ContextID},
		internal_type.ObservabilityEventRecordPacket{
			ContextID: vl.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeUserMessage,
			Record: observability.NewMessageRecord(vl.ContextID, observability.ComponentEOS, observability.EOSCompleted, observability.MessageRoleUser, observability.Attributes{
				"provider":   "text_input",
				"context_id": vl.ContextID,
				"speech":     vl.Text,
			}),
		},
	)
}

func (h requestorDispatchHandler) HandleUserAudio(ctx context.Context, vl internal_type.UserAudioReceivedPacket) {
	if h.r.denoiserExecutor != nil {
		h.r.OnPacket(ctx,
			internal_type.DenoiseAudioPacket{ContextID: vl.ContextID, Audio: vl.Audio},
		)
		return
	}
	if h.r.vadExecutor != nil {
		h.r.OnPacket(ctx,
			internal_type.VadAudioPacket{ContextID: vl.ContextID, Audio: vl.Audio})
	}
	h.r.OnPacket(ctx,
		internal_type.SpeechToTextAudioPacket{ContextID: vl.ContextID, Audio: vl.Audio},
		internal_type.EndOfSpeechAudioPacket{ContextID: vl.ContextID, Audio: vl.Audio},
	)
}

func (h requestorDispatchHandler) HandleEndOfSpeechAudio(ctx context.Context, vl internal_type.EndOfSpeechAudioPacket) {
	if h.r.endOfSpeechExecutor != nil {
		_ = h.r.endOfSpeechExecutor.Execute(ctx, vl)
	}
}

func (h requestorDispatchHandler) HandleSpeechToTextAudio(ctx context.Context, vl internal_type.SpeechToTextAudioPacket) {
	if h.r.speechToTextTransformer != nil {
		_ = h.r.speechToTextTransformer.Transform(ctx, vl)
	}
}

func (h requestorDispatchHandler) HandleDenoise(ctx context.Context, vl internal_type.DenoiseAudioPacket) {
	_ = h.r.denoiserExecutor.Execute(ctx, vl)
}

func (h requestorDispatchHandler) HandleDenoisedAudio(ctx context.Context, vl internal_type.DenoisedAudioPacket) {
	if h.r.vadExecutor != nil {
		h.r.OnPacket(ctx,
			internal_type.VadAudioPacket{ContextID: vl.ContextID, Audio: vl.Audio})
	}
	h.r.OnPacket(ctx,
		internal_type.SpeechToTextAudioPacket{ContextID: vl.ContextID, Audio: vl.Audio},
		internal_type.EndOfSpeechAudioPacket{ContextID: vl.ContextID, Audio: vl.Audio},
	)
}

func (h requestorDispatchHandler) HandleVadAudio(ctx context.Context, vl internal_type.VadAudioPacket) {
	_ = h.r.vadExecutor.Execute(ctx, internal_type.UserAudioReceivedPacket{ContextID: vl.ContextID, Audio: vl.Audio})
}
func (h requestorDispatchHandler) HandleVadSpeechActivity(ctx context.Context, vl internal_type.VadSpeechActivityPacket) {
	if h.r.endOfSpeechExecutor != nil {
		_ = h.r.endOfSpeechExecutor.Execute(ctx, vl)
	}
}
func (h requestorDispatchHandler) HandleSpeechToText(ctx context.Context, p internal_type.SpeechToTextPacket) {
	p.ContextID = h.r.GetID()
	if h.r.endOfSpeechExecutor != nil {
		_ = h.r.endOfSpeechExecutor.Execute(ctx, p)
		return
	}
	// just a fallback to trigger the end of speech event in case endOfSpeechExecutor is not configured.
	if !p.Interim {
		h.r.OnPacket(ctx, internal_type.EndOfSpeechPacket{
			ContextID: p.ContextID,
			Speech:    p.Script,
			Speechs:   []internal_type.SpeechToTextPacket{p},
		})
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
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: h.r.GetID(),
			Scope:     internal_type.ObservabilityRecordScopeUserMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Message state transition failed; check target_state and current turn state",
				Attributes: observability.Attributes{
					"component":    observability.ComponentTurn.String(),
					"operation":    "transition",
					"context_id":   h.r.GetID(),
					"message_role": string(observability.MessageRoleUser),
					"target_state": LLMGenerating.String(),
					"error":        err.Error(),
					"error_type":   fmt.Sprintf("%T", err),
				},
			},
		})
	}

	contextID := h.r.GetID()
	p.ContextID = contextID
	h.r.OnPacket(ctx,
		internal_type.MessageCreatePacket{ContextID: contextID, MessageRole: "user", Text: p.Text},
		internal_type.ObservabilityMetadataRecordPacket{
			ContextID: contextID,
			Scope:     internal_type.ObservabilityRecordScopeUserMessage,
			Record: observability.NewMessageMetadataRecord(
				contextID,
				observability.MessageRoleUser,
				[]*protos.Metadata{
					{
						Key:   "language",
						Value: p.Language.Name,
					},
					{
						Key:   "language_code",
						Value: p.Language.ISO639_1,
					},
				},
			),
		},
		internal_type.ObservabilityMetricRecordPacket{
			ContextID: contextID,
			Scope:     internal_type.ObservabilityRecordScopeUserMessage,
			Record: observability.NewMessageMetricRecord(
				contextID,
				observability.MessageRoleUser,
				[]*protos.Metric{{
					Name:        "user_turn",
					Value:       type_enums.CONVERSATION_COMPLETE.String(),
					Description: "User turn completed and ready for assistant response generation",
				}},
			),
		},
	)

	if h.r.assistantExecutor != nil {
		utils.Go(ctx, func() {
			if err := h.r.assistantExecutor.Execute(ctx, h.r, p); err != nil {
				h.r.OnPacket(ctx, internal_type.LLMErrorPacket{ContextID: contextID, Error: err})
			}
		})
	}
	if err := h.r.Notify(ctx, &protos.ConversationUserMessage{
		Id:        contextID,
		Message:   &protos.ConversationUserMessage_Text{Text: p.Text},
		Completed: true,
		Time:      timestamppb.New(time.Now()),
	}); err != nil {
		return
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
			h.r.Transition(Interrupt)
			h.r.OnPacket(
				ctx,
				internal_type.EndOfSpeechInterruptionPacket{ContextID: p.ContextID, Source: internal_type.InterruptionSourceVad},
				internal_type.SpeechToTextStartPacket{ContextID: p.ContextID},
			)
			//
			utils.Go(ctx, func() {
				h.r.Notify(ctx, &protos.ConversationInterruption{
					Type: protos.ConversationInterruption_INTERRUPTION_TYPE_VAD,
					Time: timestamppb.Now(),
				})
			})
		case internal_type.InterruptionEventEnd:
			h.r.OnPacket(ctx, internal_type.SpeechToTextEndPacket{ContextID: p.ContextID})
		}
	}
}

func (h requestorDispatchHandler) HandleEndOfSpeechInterruption(ctx context.Context, p internal_type.EndOfSpeechInterruptionPacket) {
	if h.r.endOfSpeechExecutor != nil {
		if err := h.r.endOfSpeechExecutor.Execute(ctx, p); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeUserMessage,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "End-of-speech analysis failed; user turn completion may be delayed",
					Attributes: observability.Attributes{
						"component":    observability.ComponentEOS.String(),
						"operation":    "execute",
						"packet":       "EndOfSpeechInterruptionPacket",
						"context_id":   p.ContextID,
						"message_role": string(observability.MessageRoleUser),
						"source":       "interruption",
						"error":        err.Error(),
						"error_type":   fmt.Sprintf("%T", err),
					},
				},
			})
		}
	}
}

func (h requestorDispatchHandler) HandleTextToSpeechInterrupt(ctx context.Context, p internal_type.TextToSpeechInterruptPacket) {
	if h.r.textToSpeechTransformer != nil {
		if err := h.r.textToSpeechTransformer.Transform(ctx, p); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "TTS interrupt failed; assistant audio may continue after interruption",
					Attributes: observability.Attributes{
						"component":    observability.ComponentTTS.String(),
						"operation":    "interrupt",
						"packet":       "TextToSpeechInterruptPacket",
						"context_id":   p.ContextID,
						"message_role": string(observability.MessageRoleAssistant),
						"error":        err.Error(),
						"error_type":   fmt.Sprintf("%T", err),
					},
				},
			})
		}
	}
}
func (h requestorDispatchHandler) HandleLLMInterrupt(ctx context.Context, p internal_type.LLMInterruptPacket) {
	if h.r.assistantExecutor != nil {
		if err := h.r.assistantExecutor.Execute(ctx, h.r, p); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "LLM interrupt failed; generation may continue after user interruption",
					Attributes: observability.Attributes{
						"component":    observability.ComponentLLM.String(),
						"operation":    "interrupt",
						"packet":       "LLMInterruptPacket",
						"context_id":   p.ContextID,
						"message_role": string(observability.MessageRoleAssistant),
						"error":        err.Error(),
						"error_type":   fmt.Sprintf("%T", err),
					},
				},
			})
		}
	}
}

func (h requestorDispatchHandler) HandleDispatchPolicy(ctx context.Context, p internal_type.DispatchPolicyPacket) {
	h.r.dispatchRoute.ApplyPolicy(p.Policy)
}

func (h requestorDispatchHandler) HandleSpeechToTextStart(ctx context.Context, p internal_type.SpeechToTextStartPacket) {
	if h.r.speechToTextTransformer != nil {
		if err := h.r.speechToTextTransformer.Transform(ctx, p); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeUserMessage,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "STT start notification failed; provider may miss the speech boundary",
					Attributes: observability.Attributes{
						"component":    observability.ComponentSTT.String(),
						"operation":    "start",
						"packet":       "SpeechToTextStartPacket",
						"context_id":   p.ContextID,
						"message_role": string(observability.MessageRoleUser),
						"error":        err.Error(),
						"error_type":   fmt.Sprintf("%T", err),
					},
				},
			})
		}
	}
}

func (h requestorDispatchHandler) HandleSpeechToTextEnd(ctx context.Context, p internal_type.SpeechToTextEndPacket) {
	if h.r.speechToTextTransformer != nil {
		if err := h.r.speechToTextTransformer.Transform(ctx, p); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeUserMessage,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "STT end notification failed; buffered speech may not flush",
					Attributes: observability.Attributes{
						"component":    observability.ComponentSTT.String(),
						"operation":    "end",
						"packet":       "SpeechToTextEndPacket",
						"context_id":   p.ContextID,
						"message_role": string(observability.MessageRoleUser),
						"error":        err.Error(),
						"error_type":   fmt.Sprintf("%T", err),
					},
				},
			})
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
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Turn context update failed; downstream packets may use a stale context",
					Attributes: observability.Attributes{
						"component":  observability.ComponentSTT.String(),
						"operation":  "turn_change",
						"packet":     "TurnChangePacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
	}
	if h.r.textToSpeechTransformer != nil {
		if err := h.r.textToSpeechTransformer.Transform(ctx, p); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Turn context update failed; downstream packets may use a stale context",
					Attributes: observability.Attributes{
						"component":  observability.ComponentTTS.String(),
						"operation":  "turn_change",
						"packet":     "TurnChangePacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
	}

	h.r.OnPacket(ctx, internal_type.ObservabilityEventRecordPacket{
		ContextID: p.ContextID,
		Scope:     internal_type.ObservabilityRecordScopeConversation,
		Record: observability.RecordEvent{
			OccurredAt: p.Time,
			Component:  observability.ComponentTurn,
			Event:      observability.TurnChange,
			Attributes: observability.Attributes{
				"old_context_id": p.PreviousContextID,
				"new_context_id": p.ContextID,
				"reason":         p.Reason,
				"source":         p.Source,
			},
		},
	})
}
func (h requestorDispatchHandler) HandleLLMResponseDelta(ctx context.Context, p internal_type.LLMResponseDeltaPacket) {
	if p.ContextID != h.r.GetID() {
		h.r.OnPacket(ctx, internal_type.ObservabilityEventRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.NewMessageRecord(p.ContextID, observability.ComponentLLM, observability.LLMDiscarded, observability.MessageRoleAssistant, observability.Attributes{
				"reason":          "stale_context",
				"current_context": h.r.GetID(),
				"text":            p.Text,
			}),
		})
		return
	}
	if err := h.r.Transition(LLMGenerating); err != nil {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Message state transition failed; check target_state and current turn state",
				Attributes: observability.Attributes{
					"component":    observability.ComponentLLM.String(),
					"operation":    "transition",
					"packet":       "LLMResponseDeltaPacket",
					"context_id":   p.ContextID,
					"message_role": string(observability.MessageRoleAssistant),
					"target_state": LLMGenerating.String(),
					"error":        err.Error(),
					"error_type":   fmt.Sprintf("%T", err),
				},
			},
		})
	}
	if h.r.outputNormalizer != nil {
		h.r.outputNormalizer.Normalize(ctx, p)
	} else {
		h.r.OnPacket(ctx, internal_type.TextToSpeechTextPacket{ContextID: p.ContextID, Text: p.Text})
	}
}
func (h requestorDispatchHandler) HandleLLMResponseDone(ctx context.Context, p internal_type.LLMResponseDonePacket) {
	if p.ContextID != h.r.GetID() {
		h.r.OnPacket(ctx, internal_type.ObservabilityEventRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.NewMessageRecord(p.ContextID, observability.ComponentLLM, observability.LLMDiscarded, observability.MessageRoleAssistant, observability.Attributes{
				"reason":          "stale_context",
				"packet":          "done",
				"current_context": h.r.GetID(),
				"text":            p.Text,
			}),
		})
		return
	}
	if h.r.endOfSpeechExecutor != nil {
		if err := h.r.endOfSpeechExecutor.Execute(ctx, p); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeUserMessage,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "End-of-speech analysis failed; user turn completion may be delayed",
					Attributes: observability.Attributes{
						"component":    observability.ComponentEOS.String(),
						"operation":    "execute",
						"packet":       "LLMResponseDonePacket",
						"context_id":   p.ContextID,
						"message_role": string(observability.MessageRoleUser),
						"error":        err.Error(),
						"error_type":   fmt.Sprintf("%T", err),
					},
				},
			})
		}
	}
	h.r.OnPacket(ctx, internal_type.StartIdleTimeoutPacket{ContextID: p.ContextID})
	if err := h.r.Transition(LLMGenerated); err != nil {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Message state transition failed; check target_state and current turn state",
				Attributes: observability.Attributes{
					"component":    observability.ComponentLLM.String(),
					"operation":    "transition",
					"packet":       "LLMResponseDonePacket",
					"context_id":   p.ContextID,
					"message_role": string(observability.MessageRoleAssistant),
					"target_state": LLMGenerated.String(),
					"error":        err.Error(),
					"error_type":   fmt.Sprintf("%T", err),
				},
			},
		})
	}
	h.r.OnPacket(ctx,
		internal_type.MessageCreatePacket{ContextID: p.ContextID, MessageRole: "assistant", Text: p.Text},
		internal_type.ObservabilityMetricRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.NewMessageMetricRecord(
				p.ContextID,
				observability.MessageRoleAssistant,
				[]*protos.Metric{{Name: "assistant_turn", Value: type_enums.CONVERSATION_COMPLETE.String(), Description: "LLM response completed"}},
			),
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
		_ = h.r.sessionLifecycle.Transition(adapter_lifecycle.EventInitializationFailed)
		h.r.OnPacket(ctx,
			internal_type.ObservabilityEventRecordPacket{
				ContextID: p.ContextId(),
				Scope:     internal_type.ObservabilityRecordScopeAssistant,
				Record: observability.NewConversationEventRecord(observability.ConversationError, observability.Attributes{
					"message": p.ErrMessage(),
				}),
			},
		)

	case internal_type.LLMErrorPacket:
		h.r.OnPacket(ctx,
			internal_type.ObservabilityMetricRecordPacket{
				ContextID: p.ContextId(),
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.NewMessageMetricRecord(
					p.ContextId(),
					observability.MessageRoleAssistant,
					[]*protos.Metric{{
						Name:        "llm_error",
						Value:       p.ErrMessage(),
						Description: "An error occurred during LLM processing"}},
				),
			},
			internal_type.ObservabilityEventRecordPacket{
				ContextID: p.ContextId(),
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.NewMessageRecord(p.ContextId(), observability.ComponentLLM, observability.LLMError, observability.MessageRoleAssistant, observability.Attributes{
					"message": p.ErrMessage(),
				}),
			})
		h.r.Transition(LLMGenerated)
	case internal_type.SpeechToTextErrorPacket:
		h.r.OnPacket(ctx,
			internal_type.ObservabilityMetricRecordPacket{
				ContextID: p.ContextId(),
				Scope:     internal_type.ObservabilityRecordScopeUserMessage,
				Record: observability.NewMessageMetricRecord(
					p.ContextId(),
					observability.MessageRoleUser,
					[]*protos.Metric{{
						Name:        "stt_error",
						Value:       p.ErrMessage(),
						Description: "An error occurred during STT processing"}},
				),
			},
			internal_type.ObservabilityEventRecordPacket{
				ContextID: p.ContextId(),
				Scope:     internal_type.ObservabilityRecordScopeUserMessage,
				Record: observability.NewMessageRecord(p.ContextId(), observability.ComponentSTT, observability.STTError, observability.MessageRoleUser, observability.Attributes{
					"message": p.ErrMessage(),
				}),
			})
	case internal_type.TextToSpeechErrorPacket:
		h.r.OnPacket(ctx,
			internal_type.ObservabilityMetricRecordPacket{
				ContextID: p.ContextId(),
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.NewMessageMetricRecord(
					p.ContextId(),
					observability.MessageRoleAssistant,
					[]*protos.Metric{{
						Name:        "tts_error",
						Value:       p.ErrMessage(),
						Description: "An error occurred during TTS processing"}},
				),
			},
			internal_type.ObservabilityEventRecordPacket{
				ContextID: p.ContextId(),
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.NewMessageRecord(p.ContextId(), observability.ComponentTTS, observability.TTSError, observability.MessageRoleAssistant, observability.Attributes{
					"message": p.ErrMessage(),
				}),
			})
		h.r.OnPacket(ctx,
			internal_type.DispatchPolicyPacket{
				ContextID: p.ContextId(),
				Policy: internal_type.DispatchPolicy{
					Target: internal_type.PacketNameUserAudioReceived,
					Action: internal_type.DispatchActionPassthrough,
				},
			},
			internal_type.DispatchPolicyPacket{
				ContextID: p.ContextId(),
				Policy: internal_type.DispatchPolicy{
					Target: internal_type.PacketNameUserTextReceived,
					Action: internal_type.DispatchActionPassthrough,
				},
			},
			internal_type.DispatchPolicyPacket{
				ContextID: p.ContextId(),
				Policy: internal_type.DispatchPolicy{
					Target: internal_type.PacketNameInterruptionDetected,
					Action: internal_type.DispatchActionPassthrough,
				},
			},
		)
	case internal_type.ModeSwitchErrorPacket:
		if errPkt.IsRecoverable() {
			_ = h.r.sessionLifecycle.Transition(adapter_lifecycle.EventSwitchFailedRecoverable)
		} else {
			_ = h.r.sessionLifecycle.Transition(adapter_lifecycle.EventSwitchFailedFatal)
		}
		h.r.OnPacket(ctx, internal_type.ObservabilityEventRecordPacket{
			ContextID: p.ContextId(),
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.NewConversationEventRecord(observability.ConversationModeSwitchFailed, observability.Attributes{
				"error_type":  string(errPkt.Type),
				"target_mode": errPkt.StreamMode.String(),
				"error":       p.ErrMessage(),
			}),
		})
	}
	if !p.IsRecoverable() {
		var conversationId uint64
		if h.r.Conversation() != nil {
			conversationId = h.r.Conversation().Id
		}
		webhookScope := internal_type.ObservabilityRecordScopeAssistant
		if h.r.Conversation() != nil {
			webhookScope = internal_type.ObservabilityRecordScopeConversation
		}
		h.r.OnPacket(ctx,
			internal_type.ObservabilityWebhookRecordPacket{
				ContextID: p.ContextId(),
				Scope:     webhookScope,
				Record: observability.RecordWebhook{
					Event: observability.ConversationError,
					Payload: map[string]interface{}{
						"reason":  protos.ConversationDisconnection_DISCONNECTION_TYPE_ERROR.String(),
						"message": p.ErrMessage(),
					},
				},
			},
			internal_type.ObservabilityEventRecordPacket{
				ContextID: h.r.GetID(),
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.NewConversationEventRecord(observability.ConversationError, observability.Attributes{
					"reason": protos.ConversationDisconnection_DISCONNECTION_TYPE_ERROR.String(),
				}),
			},
			internal_type.ObservabilityMetadataRecordPacket{
				ContextID: h.r.GetID(),
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.NewConversationMetadataRecord([]*protos.Metadata{{
					Key:   "disconnect_reason",
					Value: protos.ConversationDisconnection_DISCONNECTION_TYPE_ERROR.String(),
				}}),
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
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Message state transition failed; check target_state and current turn state",
				Attributes: observability.Attributes{
					"component":    observability.ComponentLLM.String(),
					"operation":    "transition",
					"packet":       "InjectMessagePacket",
					"context_id":   h.r.GetID(),
					"message_role": string(observability.MessageRoleAssistant),
					"target_state": LLMGenerating.String(),
					"error":        err.Error(),
					"error_type":   fmt.Sprintf("%T", err),
				},
			},
		})
	}

	if h.r.assistantExecutor != nil {
		utils.Go(ctx, func() {
			if err := h.r.assistantExecutor.Execute(ctx, h.r, p); err != nil {
				h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
					ContextID: h.r.GetID(),
					Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
					Record: observability.RecordLog{
						Level:   observability.LevelError,
						Message: "Assistant executor failed during response generation",
						Attributes: observability.Attributes{
							"component":    observability.ComponentLLM.String(),
							"operation":    "execute",
							"packet":       "InjectMessagePacket",
							"context_id":   h.r.GetID(),
							"message_role": string(observability.MessageRoleAssistant),
							"text_length":  fmt.Sprintf("%d", len(p.Text)),
							"error":        err.Error(),
							"error_type":   fmt.Sprintf("%T", err),
						},
					},
				})
			}
		})
	}

	contextID := h.r.GetID()
	if h.r.outputNormalizer != nil {
		h.r.OnPacket(ctx,
			internal_type.MessageCreatePacket{ContextID: contextID, MessageRole: "assistant", Text: p.Text},
			internal_type.ObservabilityMetricRecordPacket{
				ContextID: contextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.NewMessageMetricRecord(
					contextID,
					observability.MessageRoleAssistant,
					[]*protos.Metric{{Name: "assistant_turn", Value: type_enums.CONVERSATION_COMPLETE.String(), Description: "Injected message completed"}},
				),
			},
		)
		h.r.outputNormalizer.Normalize(ctx, internal_type.InjectMessagePacket{ContextID: contextID, Text: p.Text})
		if err := h.r.Transition(LLMGenerated); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: contextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Message state transition failed; check target_state and current turn state",
					Attributes: observability.Attributes{
						"component":    observability.ComponentLLM.String(),
						"operation":    "transition",
						"packet":       "InjectMessagePacket",
						"context_id":   contextID,
						"message_role": string(observability.MessageRoleAssistant),
						"target_state": LLMGenerated.String(),
						"error":        err.Error(),
						"error_type":   fmt.Sprintf("%T", err),
					},
				},
			})
		}
	} else {
		h.r.OnPacket(ctx,
			internal_type.LLMResponseDeltaPacket{ContextID: contextID, Text: p.Text},
			internal_type.LLMResponseDonePacket{ContextID: contextID, Text: p.Text},
		)
	}
}
func (h requestorDispatchHandler) HandleStartIdleTimeout(ctx context.Context, p internal_type.StartIdleTimeoutPacket) {
	behavior, err := h.r.GetBehavior()
	if err != nil {
		return
	}
	if behavior.IdleTimeout == nil || *behavior.IdleTimeout == 0 {
		return
	}

	timeoutDuration := time.Duration(*behavior.IdleTimeout) * time.Second
	if h.r.idleTimeoutWatchdog != nil {
		h.r.idleTimeoutWatchdog.Start(p.ContextID, timeoutDuration)
	}
}
func (h requestorDispatchHandler) HandleStopIdleTimeout(ctx context.Context, p internal_type.StopIdleTimeoutPacket) {
	if h.r.idleTimeoutWatchdog != nil {
		h.r.idleTimeoutWatchdog.Stop(p.ResetCount)
	}
}

func (h requestorDispatchHandler) HandleIdleTimeoutExpired(ctx context.Context, p internal_type.IdleTimeoutExpiredPacket) {
	if err := h.r.onIdleTimeout(ctx); err != nil {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Idle timeout handling failed; conversation may not end as configured",
				Attributes: observability.Attributes{
					"component":  observability.ComponentConversation.String(),
					"operation":  "idle_timeout",
					"packet":     "IdleTimeoutExpiredPacket",
					"context_id": p.ContextID,
					"count":      fmt.Sprintf("%d", p.Count),
					"error":      err.Error(),
					"error_type": fmt.Sprintf("%T", err),
				},
			},
		})
	}
}

func (h requestorDispatchHandler) HandleTextToSpeechText(ctx context.Context, p internal_type.TextToSpeechTextPacket) {
	if p.ContextID != h.r.GetID() {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Skipped TTS for stale context; assistant text belongs to an older turn",
				Attributes: observability.Attributes{
					"component":          observability.ComponentTTS.String(),
					"operation":          "discard",
					"packet":             "TextToSpeechTextPacket",
					"context_id":         p.ContextID,
					"current_context_id": h.r.GetID(),
					"message_role":       string(observability.MessageRoleAssistant),
					"text_length":        fmt.Sprintf("%d", len(p.Text)),
					"error":              "stale context, current context is " + h.r.GetID(),
				},
			},
		})
		return
	}

	if h.r.textToSpeechTransformer != nil && h.r.GetMode().Audio() {
		if h.r.ttsCompletionWatchdog != nil {
			h.r.ttsCompletionWatchdog.StartFromText(p.ContextID, p.Text)
		}
		if err := h.r.textToSpeechTransformer.Transform(ctx, p); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "TTS transform failed; assistant audio was not generated",
					Attributes: observability.Attributes{
						"component":    observability.ComponentTTS.String(),
						"operation":    "transform",
						"packet":       "TextToSpeechTextPacket",
						"context_id":   p.ContextID,
						"message_role": string(observability.MessageRoleAssistant),
						"text_length":  fmt.Sprintf("%d", len(p.Text)),
						"error":        err.Error(),
						"error_type":   fmt.Sprintf("%T", err),
					},
				},
			})
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
		if h.r.ttsCompletionWatchdog != nil {
			h.r.ttsCompletionWatchdog.StartFromText(p.ContextID, p.Text)
		}
		if err := h.r.textToSpeechTransformer.Transform(ctx, p); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "TTS transform failed; assistant audio was not generated",
					Attributes: observability.Attributes{
						"component":    observability.ComponentTTS.String(),
						"operation":    "transform",
						"packet":       "TextToSpeechDonePacket",
						"context_id":   p.ContextID,
						"message_role": string(observability.MessageRoleAssistant),
						"text_length":  fmt.Sprintf("%d", len(p.Text)),
						"error":        err.Error(),
						"error_type":   fmt.Sprintf("%T", err),
					},
				},
			})
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
		audioDuration := time.Duration(audioInfo.DurationMs) * time.Millisecond
		if h.r.idleTimeoutWatchdog != nil {
			h.r.idleTimeoutWatchdog.Extend(p.ContextID, audioDuration)
		}
		if h.r.ttsCompletionWatchdog != nil {
			h.r.ttsCompletionWatchdog.Extend(p.ContextID, audioDuration)
		}
	}
	if p.ContextID != h.r.GetID() {
		h.r.OnPacket(ctx,
			internal_type.ObservabilityEventRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.NewMessageRecord(p.ContextID, observability.ComponentTTS, observability.TTSDiscarded, observability.MessageRoleAssistant, observability.Attributes{
					"reason":          "stale_context",
					"packet":          "tts_audio",
					"current_context": h.r.GetID(),
				}),
			},
			internal_type.ObservabilityMetricRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.NewMessageMetricRecord(
					p.ContextID,
					observability.MessageRoleAssistant,
					[]*protos.Metric{{Name: "discarded_tts_chunk", Value: "true", Description: fmt.Sprintf("tts end packet discarded due to stale contextID %s", h.r.GetID())}},
				),
			})
		return
	}
	if err := h.r.Notify(ctx, &protos.ConversationAssistantMessage{
		Time:      timestamppb.Now(),
		Id:        p.ContextID,
		Message:   &protos.ConversationAssistantMessage_Audio{Audio: p.AudioChunk},
		Completed: false,
	}); err != nil {
		return
	}
}
func (h requestorDispatchHandler) HandleTextToSpeechEnd(ctx context.Context, p internal_type.TextToSpeechEndPacket) {
	if h.r.ttsCompletionWatchdog != nil {
		h.r.ttsCompletionWatchdog.Complete(p.ContextID)
	}
	if p.ContextID != h.r.GetID() {
		h.r.OnPacket(ctx,
			internal_type.ObservabilityEventRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.NewMessageRecord(p.ContextID, observability.ComponentTTS, observability.TTSDiscarded, observability.MessageRoleAssistant, observability.Attributes{
					"reason":          "stale_context",
					"packet":          "tts_end",
					"current_context": h.r.GetID(),
				}),
			},
			internal_type.ObservabilityMetricRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.NewMessageMetricRecord(
					p.ContextID,
					observability.MessageRoleAssistant,
					[]*protos.Metric{{Name: "discarded_tts", Value: "true", Description: fmt.Sprintf("tts end packet discarded due to stale contextID %s", h.r.GetID())}},
				),
			})
		return
	}
	if err := h.r.Notify(ctx, &protos.ConversationAssistantMessage{
		Time:      timestamppb.Now(),
		Id:        p.ContextID,
		Completed: true,
	}); err != nil {
		return
	}
	h.r.OnPacket(ctx,
		internal_type.DispatchPolicyPacket{
			ContextID: p.ContextID,
			Policy: internal_type.DispatchPolicy{
				Target: internal_type.PacketNameUserAudioReceived,
				Action: internal_type.DispatchActionPassthrough,
			},
		},
		internal_type.DispatchPolicyPacket{
			ContextID: p.ContextID,
			Policy: internal_type.DispatchPolicy{
				Target: internal_type.PacketNameUserTextReceived,
				Action: internal_type.DispatchActionPassthrough,
			},
		},
		internal_type.DispatchPolicyPacket{
			ContextID: p.ContextID,
			Policy: internal_type.DispatchPolicy{
				Target: internal_type.PacketNameInterruptionDetected,
				Action: internal_type.DispatchActionPassthrough,
			},
		},
	)
}
func (h requestorDispatchHandler) HandleLLMToolCall(ctx context.Context, p internal_type.LLMToolCallPacket) {
	req, _ := json.Marshal(p)
	h.r.OnPacket(
		ctx,
		internal_type.ObservabilityEventRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.NewMessageRecord(p.ContextID, observability.ComponentTool, observability.ToolCallStarted, observability.MessageRoleAssistant, observability.Attributes{
				"name":   p.Name,
				"id":     p.ToolID,
				"action": p.Action.String(),
			}),
		},
		internal_type.ToolLogCreatePacket{
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
				h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
					ContextID: p.ContextID,
					Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
					Record: observability.RecordLog{
						Level:   observability.LevelError,
						Message: "Tool call execution failed; tool result will be marked failed",
						Attributes: observability.Attributes{
							"component":    observability.ComponentTool.String(),
							"operation":    "execute",
							"packet":       "LLMToolCallPacket",
							"context_id":   p.ContextID,
							"message_role": string(observability.MessageRoleAssistant),
							"tool_id":      p.ToolID,
							"name":         p.Name,
							"action":       p.Action.String(),
							"error":        err.Error(),
							"error_type":   fmt.Sprintf("%T", err),
						},
					},
				})
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
			internal_type.ObservabilityEventRecordPacket{
				ContextID: h.r.GetID(),
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.NewConversationEventRecord(observability.ConversationCompleted, observability.Attributes{
					"reason": protos.ConversationDisconnection_DISCONNECTION_TYPE_TOOL.String(),
				}),
			},
			internal_type.ObservabilityMetadataRecordPacket{
				ContextID: h.r.GetID(),
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.NewConversationMetadataRecord([]*protos.Metadata{{
					Key:   "disconnect_reason",
					Value: protos.ConversationDisconnection_DISCONNECTION_TYPE_TOOL.String(),
				}}),
			},
		)
		h.r.Notify(ctx, &protos.ConversationDisconnection{
			Type: protos.ConversationDisconnection_DISCONNECTION_TYPE_TOOL,
		})
		return
	case protos.ToolCallAction_TOOL_CALL_ACTION_TRANSFER_CONVERSATION:
		if p.Result["next_action"] == "end_call" {
			h.r.OnPacket(ctx,
				internal_type.ObservabilityEventRecordPacket{
					ContextID: h.r.GetID(),
					Scope:     internal_type.ObservabilityRecordScopeConversation,
					Record: observability.NewConversationEventRecord(observability.ConversationCompleted, observability.Attributes{
						"reason": protos.ConversationDisconnection_DISCONNECTION_TYPE_TOOL.String(),
					}),
				},
				internal_type.ObservabilityMetadataRecordPacket{
					ContextID: h.r.GetID(),
					Scope:     internal_type.ObservabilityRecordScopeConversation,
					Record: observability.NewConversationMetadataRecord([]*protos.Metadata{{
						Key:   "disconnect_reason",
						Value: protos.ConversationDisconnection_DISCONNECTION_TYPE_TOOL.String(),
					}}),
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
		internal_type.ObservabilityEventRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.NewMessageRecord(p.ContextID, observability.ComponentTool, observability.ToolCallCompleted, observability.MessageRoleAssistant, observability.Attributes{
				"name": p.Name,
				"id":   p.ToolID,
			}),
		},
	)
	if h.r.assistantExecutor != nil {
		utils.Go(ctx, func() {
			if err := h.r.assistantExecutor.Execute(ctx, h.r, p); err != nil {
				h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
					ContextID: p.ContextID,
					Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
					Record: observability.RecordLog{
						Level:   observability.LevelError,
						Message: "Tool result processing failed; assistant context may miss tool output",
						Attributes: observability.Attributes{
							"component":    observability.ComponentTool.String(),
							"operation":    "process_result",
							"packet":       "LLMToolResultPacket",
							"context_id":   p.ContextID,
							"message_role": string(observability.MessageRoleAssistant),
							"tool_id":      p.ToolID,
							"name":         p.Name,
							"action":       p.Action.String(),
							"error":        err.Error(),
							"error_type":   fmt.Sprintf("%T", err),
						},
					},
				})
			}
		})
	}
}
func (h requestorDispatchHandler) HandleRecordUserAudio(ctx context.Context, p internal_type.RecordUserAudioPacket) {
	if h.r.conversationRecordingExecutor != nil {
		if err := h.r.conversationRecordingExecutor.Execute(ctx, p); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "User audio recording failed; conversation recording may be incomplete",
					Attributes: observability.Attributes{
						"component":   observability.ComponentRecording.String(),
						"operation":   "record_user_audio",
						"packet":      "RecordUserAudioPacket",
						"context_id":  p.ContextID,
						"audio_bytes": fmt.Sprintf("%d", len(p.Audio)),
						"error":       err.Error(),
						"error_type":  fmt.Sprintf("%T", err),
					},
				},
			})
		}
	}
}
func (h requestorDispatchHandler) HandleRecordAssistantAudio(ctx context.Context, p internal_type.RecordAssistantAudioPacket) {
	if h.r.conversationRecordingExecutor != nil {
		if err := h.r.conversationRecordingExecutor.Execute(ctx, p); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Assistant audio recording failed; conversation recording may be incomplete",
					Attributes: observability.Attributes{
						"component":   observability.ComponentRecording.String(),
						"operation":   "record_assistant_audio",
						"packet":      "RecordAssistantAudioPacket",
						"context_id":  p.ContextID,
						"audio_bytes": fmt.Sprintf("%d", len(p.Audio)),
						"error":       err.Error(),
						"error_type":  fmt.Sprintf("%T", err),
					},
				},
			})
		}
	}
}
func (h requestorDispatchHandler) HandleConversationRecordingCompleted(ctx context.Context, p internal_type.ConversationRecordingCompletedPacket) {
	h.r.OnPacket(ctx, internal_type.ObservabilityEventRecordPacket{
		ContextID: p.ContextID,
		Scope:     internal_type.ObservabilityRecordScopeConversation,
		Record:    observability.NewConversationEventRecord(observability.RecordingCompleted, nil),
	})
	if err := h.r.CreateConversationRecording(ctx, p.Audio.UserAudio, p.Audio.AssistantAudio, p.Audio.MixedAudio); err != nil {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Conversation recording persistence failed; recording artifact may be missing",
				Attributes: observability.Attributes{
					"component":             observability.ComponentRecording.String(),
					"operation":             "persist_recording",
					"packet":                "ConversationRecordingCompletedPacket",
					"context_id":            p.ContextID,
					"user_audio_bytes":      fmt.Sprintf("%d", len(p.Audio.UserAudio)),
					"assistant_audio_bytes": fmt.Sprintf("%d", len(p.Audio.AssistantAudio)),
					"mixed_audio_bytes":     fmt.Sprintf("%d", len(p.Audio.MixedAudio)),
					"error":                 err.Error(),
					"error_type":            fmt.Sprintf("%T", err),
				},
			},
		})
	}
}
func (h requestorDispatchHandler) HandleMessageCreate(ctx context.Context, p internal_type.MessageCreatePacket) {
	if err := h.r.onAddMessage(ctx, p); err != nil {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Message persistence failed; conversation transcript may be incomplete",
				Attributes: observability.Attributes{
					"component":    observability.ComponentSTT.String(),
					"operation":    "persist_message",
					"packet":       "MessageCreatePacket",
					"context_id":   p.ContextID,
					"message_id":   p.ContextID,
					"message_role": p.MessageRole,
					"text_length":  fmt.Sprintf("%d", len(p.Text)),
					"error":        err.Error(),
					"error_type":   fmt.Sprintf("%T", err),
				},
			},
		})
	}
}

func (h requestorDispatchHandler) HandleObservabilityRecordPacket(ctx context.Context, p internal_type.ObservabilityRecordPacket) {
	if h.r.observabilityRecorder != nil {
		observabilityRecord := p.GetRecord()
		switch p.GetScope() {
		case internal_type.ObservabilityRecordScopeAssistant:
			if h.r.Assistant() == nil {
				h.r.logger.Errorw("observability assistant scope failed to resolve", "record", p)
				return
			}
			if err := h.r.observabilityRecorder.Record(ctx, observability.AssistantScope{
				AssistantID: h.r.Assistant().Id,
			}, observabilityRecord); err != nil {
				h.r.logger.Errorw("observability record failed to persist", "error", err, "record", p)
			}
		case internal_type.ObservabilityRecordScopeConversation:
			if h.r.Assistant() == nil || h.r.Conversation() == nil {
				h.r.logger.Errorw("observability conversation scope failed to resolve", "record", p)
				return
			}
			if err := h.r.observabilityRecorder.Record(ctx, observability.ConversationScope{
				AssistantScope: observability.AssistantScope{
					AssistantID: h.r.Assistant().Id,
				},
				ConversationID: h.r.Conversation().Id,
			}, observabilityRecord); err != nil {
				h.r.logger.Errorw("observability record failed to persist", "error", err, "record", p)
			}
		case internal_type.ObservabilityRecordScopeUserMessage:
			if h.r.Assistant() == nil || h.r.Conversation() == nil {
				h.r.logger.Errorw("observability message scope failed to resolve", "record", p)
				return
			}
			if err := h.r.observabilityRecorder.Record(ctx, observability.MessageScope{
				ConversationScope: observability.ConversationScope{
					AssistantScope: observability.AssistantScope{
						AssistantID: h.r.Assistant().Id,
					},
					ConversationID: h.r.Conversation().Id,
				},
				MessageID: p.ContextId(),
				Role:      observability.MessageRoleUser,
			}, observabilityRecord); err != nil {
				h.r.logger.Errorw("observability record failed to persist", "error", err, "record", p)
			}
		case internal_type.ObservabilityRecordScopeAssistantMessage:
			if h.r.Assistant() == nil || h.r.Conversation() == nil {
				h.r.logger.Errorw("observability message scope failed to resolve", "record", p)
				return
			}
			if err := h.r.observabilityRecorder.Record(ctx, observability.MessageScope{
				ConversationScope: observability.ConversationScope{
					AssistantScope: observability.AssistantScope{
						AssistantID: h.r.Assistant().Id,
					},
					ConversationID: h.r.Conversation().Id,
				},
				MessageID: p.ContextId(),
				Role:      observability.MessageRoleAssistant,
			}, observabilityRecord); err != nil {
				h.r.logger.Errorw("observability record failed to persist", "error", err, "record", p)
			}
		default:
			h.r.logger.Errorw("observability scope is unsupported", "record", p, "scope", p.GetScope())
			return
		}

	}
}

func (h requestorDispatchHandler) HandleToolLogCreate(ctx context.Context, p internal_type.ToolLogCreatePacket) {
	if !validator.NotBlank(p.ToolID) {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Skipped tool log creation because tool_id is empty",
				Attributes: observability.Attributes{
					"component":    observability.ComponentTool.String(),
					"operation":    "create_tool_log",
					"packet":       "ToolLogCreatePacket",
					"context_id":   p.ContextID,
					"message_role": string(observability.MessageRoleAssistant),
					"reason":       "empty_tool_id",
					"name":         p.Name,
				},
			},
		})
		return
	}
	if err := h.r.observabilityRecorder.Record(ctx, observability.MessageScope{
		ConversationScope: observability.ConversationScope{
			AssistantScope: observability.AssistantScope{
				AssistantID: h.r.assistant.Id,
			},
			ConversationID: h.r.assistantConversation.Id,
		},
		MessageID: p.ContextID,
		Role:      observability.MessageRoleAssistant,
	}, observability.RecordToolLog{
		Operation:      observability.ToolLogOperationCreate,
		ToolCallID:     p.ToolID,
		ToolName:       p.Name,
		Status:         type_enums.RECORD_IN_PROGRESS,
		RequestPayload: p.Request,
	}); err != nil {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Tool log creation failed; tool execution trace may be incomplete",
				Attributes: observability.Attributes{
					"component":    observability.ComponentTool.String(),
					"operation":    "create_tool_log",
					"packet":       "ToolLogCreatePacket",
					"context_id":   p.ContextID,
					"message_role": string(observability.MessageRoleAssistant),
					"tool_id":      p.ToolID,
					"name":         p.Name,
					"error":        err.Error(),
					"error_type":   fmt.Sprintf("%T", err),
				},
			},
		})
	}
}
func (h requestorDispatchHandler) HandleToolLogUpdate(ctx context.Context, p internal_type.ToolLogUpdatePacket) {
	if !validator.NotBlank(p.ToolID) {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Skipped tool log update because tool_id is empty",
				Attributes: observability.Attributes{
					"component":    observability.ComponentTool.String(),
					"operation":    "update_tool_log",
					"packet":       "ToolLogUpdatePacket",
					"context_id":   p.ContextID,
					"message_role": string(observability.MessageRoleAssistant),
					"reason":       "empty_tool_id",
				},
			},
		})
		return
	}
	if err := h.r.observabilityRecorder.Record(ctx, observability.MessageScope{
		ConversationScope: observability.ConversationScope{
			AssistantScope: observability.AssistantScope{
				AssistantID: h.r.assistant.Id,
			},
			ConversationID: h.r.assistantConversation.Id,
		},
		MessageID: p.ContextID,
		Role:      observability.MessageRoleAssistant,
	}, observability.RecordToolLog{
		Operation:       observability.ToolLogOperationUpdate,
		ToolCallID:      p.ToolID,
		Status:          type_enums.RECORD_COMPLETE,
		ResponsePayload: p.Response,
	}); err != nil {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Tool log update failed; tool execution trace may be stale",
				Attributes: observability.Attributes{
					"component":    observability.ComponentTool.String(),
					"operation":    "update_tool_log",
					"packet":       "ToolLogUpdatePacket",
					"context_id":   p.ContextID,
					"message_role": string(observability.MessageRoleAssistant),
					"tool_id":      p.ToolID,
					"error":        err.Error(),
					"error_type":   fmt.Sprintf("%T", err),
				},
			},
		})
	}
}
func (h requestorDispatchHandler) HandleHTTPLogCreate(ctx context.Context, p internal_type.HTTPLogCreatePacket) {
	if err := h.r.observabilityRecorder.Record(ctx, observability.ConversationScope{
		AssistantScope: observability.AssistantScope{
			AssistantID: h.r.assistant.Id,
		},
		ConversationID: h.r.assistantConversation.Id,
	}, observability.RecordRequestLog{
		Source:          p.Source,
		SourceRefID:     p.SourceRefID,
		SourceEvent:     p.SourceEvent,
		ContextID:       p.ContextID,
		HTTPURL:         p.HTTPURL,
		HTTPMethod:      p.HTTPMethod,
		ResponseStatus:  p.ResponseStatus,
		TimeTaken:       p.TimeTaken,
		RetryCount:      p.RetryCount,
		Status:          p.Status,
		ErrorMessage:    p.ErrorMessage,
		RequestPayload:  p.RequestPayload,
		ResponsePayload: p.ResponsePayload,
	}); err != nil {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Request log persistence failed; request trace may be incomplete",
				Attributes: observability.Attributes{
					"component":     observability.ComponentLog.String(),
					"operation":     "persist_request_log",
					"packet":        "HTTPLogCreatePacket",
					"context_id":    p.ContextID,
					"source":        p.Source,
					"source_ref_id": fmt.Sprintf("%d", p.SourceRefID),
					"source_event":  p.SourceEvent,
					"http_method":   p.HTTPMethod,
					"http_url":      p.HTTPURL,
					"status":        p.Status.String(),
					"error":         err.Error(),
					"error_type":    fmt.Sprintf("%T", err),
				},
			},
		})
	}
}

func (h requestorDispatchHandler) HandleInitializeAssistant(ctx context.Context, p internal_type.InitializeAssistantPacket) {
	assistant, err := h.r.GetAssistant(ctx, h.r.Auth(), p.Config.Assistant.AssistantId, p.Config.Assistant.Version)
	if err != nil {
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
			h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
				ContextID: vl.ContextID,
				Stage:     internal_type.InitializationStageConversation,
				Error:     err,
			})
			return
		}
		h.r.OnPacket(ctx, internal_type.ObservabilityEventRecordPacket{
			ContextID: vl.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.NewConversationEventRecord(observability.ConversationResume, observability.Attributes{
				"source":        fmt.Sprintf("%v", h.r.source),
				"identifier":    h.r.identifier(vl.Config),
				"message_count": fmt.Sprintf("%d", len(h.r.GetHistories())),
			}),
		})

	} else {
		err := h.r.BeginConversation(ctx, h.r.assistant, type_enums.DIRECTION_INBOUND, vl.Config)
		if err != nil {
			h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
				ContextID: vl.ContextID,
				Stage:     internal_type.InitializationStageConversation,
				Error:     err,
			})
			return
		}
		h.r.OnPacket(ctx, internal_type.ObservabilityEventRecordPacket{
			ContextID: vl.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.NewConversationEventRecord(observability.ConversationBegin, observability.Attributes{
				"source":     fmt.Sprintf("%v", h.r.source),
				"is_new":     "true",
				"identifier": h.r.identifier(vl.Config),
			}),
		})
	}
	h.r.OnPacket(ctx,
		internal_type.InitializeSessionRuntimePacket{ContextID: vl.ContextID, Config: vl.Config})
}
func (h requestorDispatchHandler) HandleInitializeSessionRuntime(ctx context.Context, p internal_type.InitializeSessionRuntimePacket) {
	recordingExecutor, err := internal_audio_recorder.GetConversationRecordingExecutor(p.ContextID, h.r.OnPacket)
	if err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageRecording,
			Error:     err,
		})
		return
	}
	h.r.conversationRecordingExecutor = recordingExecutor
	h.r.OnPacket(ctx, internal_type.ObservabilityEventRecordPacket{
		ContextID: p.ContextID,
		Scope:     internal_type.ObservabilityRecordScopeConversation,
		Record:    observability.NewConversationEventRecord(observability.RecordingStarted, nil),
	})
	for _, analysis := range h.r.assistant.AssistantAnalyses {
		exec, err := internal_analysis.NewExecutor(h.r.logger, ctx, analysis, h.r)
		if err != nil {
			h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
				ContextID: p.ContextID,
				Stage:     internal_type.InitializationStageAnalysis,
				Error:     err,
			})
			continue
		}
		h.r.assistantAnalyseExecutors = append(h.r.assistantAnalyseExecutors, exec)
	}

	if h.r.assistant.AssistantAuthentication != nil {
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

	// start accepting input as conversation is inilize properly
	go h.r.runLowDispatcher(h.r.sessionCtx)
	if h.r.metrics == nil {
		h.r.metrics = make(map[string]*protos.Metric)
	}
	h.r.metrics[type_enums.CONVERSATION_STATUS.String()] = &protos.Metric{
		Name:        type_enums.CONVERSATION_STATUS.String(),
		Value:       type_enums.CONVERSATION_IN_PROGRESS.String(),
		Description: "Conversation is currently in progress",
	}
	h.r.OnPacket(ctx,
		internal_type.ObservabilityMetricRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.NewConversationMetricRecord([]*protos.Metric{{
				Name:        type_enums.CONVERSATION_STATUS.String(),
				Value:       type_enums.CONVERSATION_IN_PROGRESS.String(),
				Description: "Conversation is currently in progress",
			}}),
		},
		internal_type.InitializeAuthenticationPacket{
			ContextID: p.ContextID,
			Config:    p.Config,
		},
	)

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

	// check if the authentication condition is satisfied before executing authentication, if not satisfied return authentication failed immediately
	if !h.r.IsConditionAllowed(h.r.authenticationExecutor.Options(), "authentication.condition") {
		h.r.OnPacket(ctx, internal_type.SessionAuthenticationSucceededPacket{
			ContextID:      p.ContextID,
			Authenticated:  false,
			Initialization: p.Config,
		})
		return
	}

	// for authentication if something is wrong
	args, err := h.r.authenticationExecutor.Arguments()
	if err != nil {
		h.r.OnPacket(ctx,
			internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Authentication argument resolution failed; authentication cannot run",
					Attributes: observability.Attributes{
						"component":  observability.ComponentConversation.String(),
						"operation":  "build_authentication_arguments",
						"packet":     "InitializeSessionAuthenticationPacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			},
			internal_type.SessionAuthenticationSucceededPacket{
				ContextID:      p.ContextID,
				Authenticated:  false,
				Initialization: p.Config,
			})
		return
	}
	h.r.OnPacket(ctx,
		internal_type.ObservabilityEventRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record:    observability.NewConversationEventRecord(observability.ConversationAuthenticationStarted, nil),
		},
	)
	output, err := h.r.authenticationExecutor.Execute(ctx, internal_type.AuthenticationInput{
		ContextID:      p.ContextID,
		Arguments:      registry.Apply(args, source, variable.ResolveContext{}),
		Initialization: p.Config,
	})
	if err != nil {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Authentication execution failed; session may be rejected",
				Attributes: observability.Attributes{
					"component":  observability.ComponentConversation.String(),
					"operation":  "execute_authentication",
					"packet":     "InitializeAuthenticationPacket",
					"context_id": p.ContextID,
					"error":      err.Error(),
					"error_type": fmt.Sprintf("%T", err),
				},
			},
		})
		h.r.OnPacket(ctx, internal_type.SessionAuthenticationFailedPacket{
			ContextID:      p.ContextID,
			Error:          fmt.Errorf("authentication: execution failed: %w", err),
			Initialization: p.Config,
		})
		return
	}
	h.r.OnPacket(ctx, internal_type.SessionAuthenticationSucceededPacket{
		ContextID:      p.ContextID,
		Authenticated:  output.Authenticated,
		Arguments:      output.Arguments,
		Metadata:       output.Metadata,
		Options:        output.Options,
		Initialization: p.Config,
	})
}
func (h requestorDispatchHandler) HandleSessionAuthenticationSucceeded(ctx context.Context, p internal_type.SessionAuthenticationSucceededPacket) {
	if p.Authenticated {
		h.r.applyArguments(p.Arguments)
		h.r.applyMetadata(p.Metadata)
		h.r.applyOptions(p.Options)
	}

	conversationConfigurationObj := &protos.ConversationInitialization{
		AssistantConversationId: h.r.assistantConversation.Id,
		Assistant: &protos.AssistantDefinition{
			AssistantId: h.r.assistant.Id,
			Version:     utils.GetVersionString(h.r.assistant.AssistantProviderId),
		},
		StreamMode:   p.Initialization.GetStreamMode(),
		UserIdentity: p.Initialization.GetUserIdentity(),
		Time:         timestamppb.Now(),
	}
	options := h.r.GetOptions()
	if outputAudio, err := h.r.GetTextToSpeechTransformer(); err == nil && outputAudio != nil {
		if ambient, _ := outputAudio.GetOptions().GetString("speaker.ambient"); ambient != "" {
			options["speaker.ambient"] = ambient
		}
		if volume, _ := outputAudio.GetOptions().GetString("speaker.ambient_volume"); volume != "" {
			options["speaker.ambient_volume"] = volume
		}
	}
	if anyArgMap, err := utils.InterfaceMapToAnyMap(h.r.GetArgs()); err == nil {
		conversationConfigurationObj.Args = anyArgMap
	}
	if anyMetaMap, err := utils.InterfaceMapToAnyMap(h.r.GetMetadata()); err == nil {
		conversationConfigurationObj.Metadata = anyMetaMap
	}
	if anyOptionMap, err := utils.InterfaceMapToAnyMap(options); err == nil {
		conversationConfigurationObj.Options = anyOptionMap
	}

	switch p.Initialization.StreamMode {
	case protos.StreamMode_STREAM_MODE_TEXT:
		h.r.SwitchMode(type_enums.TextMode)
		h.r.OnPacket(ctx,
			internal_type.InitializeAssistantExecutorPacket{
				ContextID: p.ContextID,
				Config:    conversationConfigurationObj,
			}, internal_type.InitializeBehaviorPacket{
				ContextID: p.ContextID,
				Config:    conversationConfigurationObj,
			},
			internal_type.InitializationCompletedPacket{
				ContextID: p.ContextID,
				Config:    conversationConfigurationObj,
			})

	case protos.StreamMode_STREAM_MODE_AUDIO:
		h.r.OnPacket(ctx,
			internal_type.InitializeSpeechToTextPacket{
				ContextID: p.ContextID,
				Config:    conversationConfigurationObj,
			},
			internal_type.InitializeTextToSpeechPacket{
				ContextID: p.ContextID,
				Config:    conversationConfigurationObj,
			},
			internal_type.InitializeAssistantExecutorPacket{
				ContextID: p.ContextID,
				Config:    conversationConfigurationObj,
			},
			internal_type.InitializeVoiceActivityDetectionPacket{
				ContextID: p.ContextID,
				Config:    conversationConfigurationObj,
			},
			internal_type.InitializeEndOfSpeechPacket{
				ContextID: p.ContextID,
				Config:    conversationConfigurationObj,
			},
			internal_type.InitializeDenoisePacket{
				ContextID: p.ContextID,
				Config:    conversationConfigurationObj,
			},
			internal_type.InitializeBehaviorPacket{
				ContextID: p.ContextID,
				Config:    conversationConfigurationObj,
			},
			internal_type.InitializationCompletedPacket{
				ContextID: p.ContextID,
				Config:    conversationConfigurationObj,
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
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageSpeechToText,
			Error:     err,
		})
		return
	}
	credential, err := h.r.VaultCaller().GetCredential(ctx, h.r.Auth(), credentialId)
	if err != nil {
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
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageSpeechToText,
			Error:     err,
		})
		return
	}
	if err := atransformer.Initialize(); err != nil {
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
		ctx, h.r.logger,
		h.r.OnPacket,
		options)
	if err != nil {
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageDenoise,
			Error:     err,
		})
		return
	}
	h.r.denoiserExecutor = denoise
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
	speakerOpts := utils.MergeMaps(h.r.options, outputTransformer.GetOptions())
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
	atransformer, err := internal_transformer.GetTextToSpeechTransformer(
		ctx, h.r.logger,
		outputTransformer.GetName(),
		credential,
		func(pkt ...internal_type.Packet) error { return h.r.OnPacket(ctx, pkt...) },
		speakerOpts)
	if err != nil {
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
			h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
				ContextID: p.ContextID,
				Stage:     internal_type.InitializationStageVoiceActivity,
				Error:     err,
			})
			return
		}
		h.r.vadExecutor = vad
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
		h.r.OnPacket(ctx, internal_type.InitializationFailedPacket{
			ContextID: p.ContextID,
			Stage:     internal_type.InitializationStageEndOfSpeech,
			Error:     err,
		})
		return
	}
	h.r.endOfSpeechExecutor = endOfSpeech
}
func (h requestorDispatchHandler) HandleInitializeBehavior(ctx context.Context, p internal_type.InitializeBehaviorPacket) {
	behavior, err := h.r.GetBehavior()
	if err != nil {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Behavior initialization failed; greeting or timeout behavior may be unavailable",
				Attributes: observability.Attributes{
					"component":  observability.ComponentConversation.String(),
					"operation":  "initialize_behavior",
					"packet":     "InitializeBehaviorPacket",
					"context_id": p.ContextID,
					"error":      err.Error(),
					"error_type": fmt.Sprintf("%T", err),
				},
			},
		})
		return
	}
	h.r.initializeGreeting(ctx, behavior)
	h.r.initializeIdleTimeout(ctx, behavior)
	h.r.initializeMaxSessionDuration(ctx, behavior)
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
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID,
			Type:      internal_type.ModeSwitchErrorTypeInitializeSpeechToText,
			Error:     err,
		})
		return
	}
	credential, err := h.r.VaultCaller().GetCredential(ctx, h.r.Auth(), credentialId)
	if err != nil {
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
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID: p.ContextID,
			Type:      internal_type.ModeSwitchErrorTypeInitializeSpeechToText,
			Error:     err,
		})
		return
	}
	if err := atransformer.Initialize(); err != nil {
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
	speakerOpts := utils.MergeMaps(h.r.options, outputTransformer.GetOptions())
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
	atransformer, err := internal_transformer.GetTextToSpeechTransformer(
		ctx, h.r.logger,
		outputTransformer.GetName(),
		credential,
		func(pkt ...internal_type.Packet) error { return h.r.OnPacket(ctx, pkt...) },
		speakerOpts)
	if err != nil {
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
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID:  p.ContextID,
			StreamMode: p.StreamMode,
			Type:       internal_type.ModeSwitchErrorTypeInitializeVoiceActivityDetection,
			Error:      err,
		})
		return
	}
	options := utils.MergeMaps(h.r.options, cfg.GetOptions())
	vad, err := internal_vad.GetVAD(ctx, h.r.logger, h.r.OnPacket, options)
	if err != nil {
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID:  p.ContextID,
			StreamMode: p.StreamMode,
			Type:       internal_type.ModeSwitchErrorTypeInitializeVoiceActivityDetection,
			Error:      err,
		})
		return
	}
	h.r.vadExecutor = vad
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

	options := utils.MergeMaps(h.r.options, cfg.GetOptions())
	denoise, err := internal_denoiser.GetDenoiser(ctx, h.r.logger,
		func(pctx context.Context, pkt ...internal_type.Packet) error { return h.r.OnPacket(pctx, pkt...) },
		options)
	if err != nil {
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID:  p.ContextID,
			StreamMode: p.StreamMode,
			Type:       internal_type.ModeSwitchErrorTypeInitializeDenoise, Error: err,
		})
		return
	}
	h.r.denoiserExecutor = denoise
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
		h.r.OnPacket(ctx, internal_type.ModeSwitchErrorPacket{
			ContextID:  p.ContextID,
			StreamMode: p.StreamMode,
			Type:       internal_type.ModeSwitchErrorTypeInitializeEndOfSpeech,
			Error:      err,
		})
		return
	}
	h.r.endOfSpeechExecutor = endOfSpeech

}

// Finalize handlers — fire-and-forget. Each runs in its own goroutine
// (AsyncPacket). The client has already been confirmed in text mode by the
// time these run. Errors are logged only — no client-facing error packet.

func (h requestorDispatchHandler) HandleModeSwitchFinalizeSpeechToText(ctx context.Context, p internal_type.ModeSwitchFinalizeSpeechToTextPacket) {
	if h.r.speechToTextTransformer != nil {
		if err := h.r.speechToTextTransformer.Close(ctx); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Mode switch finalization failed; previous media component may still be active",
					Attributes: observability.Attributes{
						"component":  observability.ComponentSTT.String(),
						"operation":  "finalize",
						"packet":     "ModeSwitchFinalizeSpeechToTextPacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
		h.r.speechToTextTransformer = nil
	}
}

func (h requestorDispatchHandler) HandleModeSwitchFinalizeTextToSpeech(ctx context.Context, p internal_type.ModeSwitchFinalizeTextToSpeechPacket) {
	if h.r.textToSpeechTransformer != nil {
		if err := h.r.textToSpeechTransformer.Close(ctx); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Mode switch finalization failed; previous media component may still be active",
					Attributes: observability.Attributes{
						"component":  observability.ComponentTTS.String(),
						"operation":  "finalize",
						"packet":     "ModeSwitchFinalizeTextToSpeechPacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
		h.r.textToSpeechTransformer = nil
	}
}

func (h requestorDispatchHandler) HandleModeSwitchFinalizeVoiceActivityDetection(ctx context.Context, p internal_type.ModeSwitchFinalizeVoiceActivityDetectionPacket) {
	if h.r.vadExecutor != nil {
		if err := h.r.vadExecutor.Close(ctx); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Mode switch finalization failed; previous media component may still be active",
					Attributes: observability.Attributes{
						"component":  observability.ComponentVAD.String(),
						"operation":  "finalize",
						"packet":     "ModeSwitchFinalizeVoiceActivityDetectionPacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
		h.r.vadExecutor = nil
	}
}

func (h requestorDispatchHandler) HandleModeSwitchFinalizeEndOfSpeech(ctx context.Context, p internal_type.ModeSwitchFinalizeEndOfSpeechPacket) {
	if h.r.endOfSpeechExecutor != nil {
		if err := h.r.endOfSpeechExecutor.Close(ctx); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Mode switch finalization failed; previous media component may still be active",
					Attributes: observability.Attributes{
						"component":  observability.ComponentEOS.String(),
						"operation":  "finalize",
						"packet":     "ModeSwitchFinalizeEndOfSpeechPacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
		h.r.endOfSpeechExecutor = nil
	}
}

func (h requestorDispatchHandler) HandleModeSwitchFinalizeDenoise(ctx context.Context, p internal_type.ModeSwitchFinalizeDenoisePacket) {
	if h.r.denoiserExecutor != nil {
		if err := h.r.denoiserExecutor.Close(ctx); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Mode switch finalization failed; previous media component may still be active",
					Attributes: observability.Attributes{
						"component":  observability.ComponentDenoise.String(),
						"operation":  "finalize",
						"packet":     "ModeSwitchFinalizeDenoisePacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
		h.r.denoiserExecutor = nil
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
	_ = h.r.sessionLifecycle.Transition(adapter_lifecycle.EventInitializationCompleted)
	h.r.OnNotifyAssistantConfiguration(ctx, p.Config, h.r.assistantConversation)
	h.r.OnPacket(ctx, internal_type.InitializeInboundDispatcherPacket{ContextID: p.ContextID})

	event := utils.ConversationResume
	webhookEvent := observability.ConversationResume
	webhookData := map[string]interface{}{
		"source":        fmt.Sprintf("%v", h.r.source),
		"identifier":    h.r.identifier(p.Config),
		"message_count": fmt.Sprintf("%d", len(h.r.GetHistories())),
	}
	if p.Config.GetAssistantConversationId() == 0 {
		event = utils.ConversationBegin
		webhookEvent = observability.ConversationBegin
		webhookData = map[string]interface{}{
			"source":     fmt.Sprintf("%v", h.r.source),
			"is_new":     "true",
			"identifier": h.r.identifier(p.Config),
		}
	}
	h.r.OnPacket(ctx, internal_type.ObservabilityEventRecordPacket{
		ContextID: p.ContextID,
		Scope:     internal_type.ObservabilityRecordScopeConversation,
		Record: observability.NewConversationEventRecord(observability.ConversationInitialized, observability.Attributes{
			"event": event.Get(),
			"mode":  h.r.GetMode().String(),
		}),
	}, internal_type.ObservabilityWebhookRecordPacket{
		ContextID: p.ContextID,
		Scope:     internal_type.ObservabilityRecordScopeConversation,
		Record: observability.RecordWebhook{
			Event:   webhookEvent,
			Payload: webhookData,
		},
	})

}

func (h requestorDispatchHandler) HandleInitializeInboundDispatcher(ctx context.Context, p internal_type.InitializeInboundDispatcherPacket) {
	go h.r.runInputDispatcher(h.r.sessionCtx)
}

func (h requestorDispatchHandler) HandleFinalizeBehavior(ctx context.Context, p internal_type.FinalizeBehaviorPacket) {
	if h.r.idleTimeoutWatchdog != nil {
		h.r.idleTimeoutWatchdog.Cancel()
	}
	if h.r.ttsCompletionWatchdog != nil {
		h.r.ttsCompletionWatchdog.Cancel()
	}
	if h.r.maxSessionTimer != nil {
		h.r.maxSessionTimer.Stop()
	}
	h.r.OnPacket(ctx, internal_type.FinalizeEndOfSpeechPacket{ContextID: p.ContextID})
}
func (h requestorDispatchHandler) HandleFinalizeEndOfSpeech(ctx context.Context, p internal_type.FinalizeEndOfSpeechPacket) {
	if h.r.endOfSpeechExecutor != nil {
		if err := h.r.endOfSpeechExecutor.Close(ctx); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "End-of-speech executor close failed; shutdown may leave resources open",
					Attributes: observability.Attributes{
						"component":  observability.ComponentEOS.String(),
						"operation":  "finalize",
						"packet":     "FinalizeEndOfSpeechPacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
		h.r.endOfSpeechExecutor = nil
	}
	h.r.OnPacket(ctx, internal_type.FinalizeVoiceActivityDetectionPacket{ContextID: p.ContextID})
}

func (h requestorDispatchHandler) HandleFinalizeVoiceActivityDetection(ctx context.Context, p internal_type.FinalizeVoiceActivityDetectionPacket) {
	if h.r.vadExecutor != nil {
		if err := h.r.vadExecutor.Close(ctx); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "VAD executor close failed; shutdown may leave resources open",
					Attributes: observability.Attributes{
						"component":  observability.ComponentVAD.String(),
						"operation":  "finalize",
						"packet":     "FinalizeVoiceActivityDetectionPacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
		h.r.vadExecutor = nil
	}
	h.r.OnPacket(ctx, internal_type.FinalizeTextToSpeechPacket{ContextID: p.ContextID})
}

func (h requestorDispatchHandler) HandleFinalizeTextToSpeech(ctx context.Context, p internal_type.FinalizeTextToSpeechPacket) {
	if h.r.textToSpeechTransformer != nil {
		if err := h.r.textToSpeechTransformer.Close(ctx); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "TTS transformer close failed; shutdown may leave resources open",
					Attributes: observability.Attributes{
						"component":  observability.ComponentTTS.String(),
						"operation":  "finalize",
						"packet":     "FinalizeTextToSpeechPacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
		h.r.textToSpeechTransformer = nil
	}
	h.r.OnPacket(ctx, internal_type.FinalizeSpeechToTextPacket{ContextID: p.ContextID})
}

func (h requestorDispatchHandler) HandleFinalizeSpeechToText(ctx context.Context, p internal_type.FinalizeSpeechToTextPacket) {
	if h.r.speechToTextTransformer != nil {
		if err := h.r.speechToTextTransformer.Close(ctx); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "STT transformer close failed; shutdown may leave resources open",
					Attributes: observability.Attributes{
						"component":  observability.ComponentSTT.String(),
						"operation":  "finalize",
						"packet":     "FinalizeSpeechToTextPacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
		h.r.speechToTextTransformer = nil
	}
	h.r.OnPacket(ctx, internal_type.FinalizeAuthenticationPacket{ContextID: p.ContextID})
}
func (h requestorDispatchHandler) HandleFinalizeAuthentication(ctx context.Context, p internal_type.FinalizeAuthenticationPacket) {
	h.r.OnPacket(ctx, internal_type.FinalizeSessionRuntimePacket{ContextID: p.ContextID})
}
func (h requestorDispatchHandler) HandleFinalizeSessionRuntime(ctx context.Context, p internal_type.FinalizeSessionRuntimePacket) {
	var closeGroup sync.WaitGroup
	if outputNormalizer := h.r.outputNormalizer; outputNormalizer != nil {
		closeGroup.Add(1)
		utils.Go(ctx, func() {
			defer closeGroup.Done()
			outputNormalizer.Close(ctx)
			h.r.outputNormalizer = nil
		})
	}
	//
	if inputNormalizer := h.r.inputNormalizer; inputNormalizer != nil {
		closeGroup.Add(1)
		utils.Go(ctx, func() {
			defer closeGroup.Done()
			inputNormalizer.Close(ctx)
			h.r.inputNormalizer = nil
		})
	}
	//
	if conversationRecordingExecutor := h.r.conversationRecordingExecutor; conversationRecordingExecutor != nil {
		closeGroup.Add(1)
		utils.Go(ctx, func() {
			defer closeGroup.Done()
			if err := conversationRecordingExecutor.Close(ctx); err != nil {
				h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
					ContextID: p.ContextID,
					Scope:     internal_type.ObservabilityRecordScopeConversation,
					Record: observability.RecordLog{
						Level:   observability.LevelError,
						Message: "Recorded audio persistence failed; recording artifact may be incomplete",
						Attributes: observability.Attributes{
							"component":  observability.ComponentRecording.String(),
							"operation":  "finalize",
							"packet":     "FinalizeSessionRuntimePacket",
							"context_id": p.ContextID,
							"error":      err.Error(),
							"error_type": fmt.Sprintf("%T", err),
						},
					},
				})
			}
			h.r.conversationRecordingExecutor = nil
		})
	}
	closeGroup.Wait()
	h.r.OnPacket(ctx, internal_type.FinalizeConversationPacket{ContextID: p.ContextID})

}
func (h requestorDispatchHandler) HandleFinalizeConversation(ctx context.Context, p internal_type.FinalizeConversationPacket) {
	if len(h.r.assistantAnalyseExecutors) > 0 {
		conversationID := uint64(0)
		if h.r.Conversation() != nil {
			conversationID = h.r.Conversation().Id
		}
		source := variable.NewCommunicationSource(h.r)
		registry := internal_namespace.NewDefaultRegistry().With("event", &internal_namespace.EventNamespace{})
		for _, initializedAnalysis := range h.r.assistantAnalyseExecutors {
			if h.r.IsConditionAllowed(initializedAnalysis.Options(), "analysis.condition") {
				arguments, err := initializedAnalysis.Arguments()
				if err != nil {
					h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
						ContextID: p.ContextID,
						Scope:     internal_type.ObservabilityRecordScopeConversation,
						Record: observability.RecordLog{
							Level:   observability.LevelError,
							Message: "Analysis argument resolution failed; analysis cannot run",
							Attributes: observability.Attributes{
								"component":  observability.ComponentConversation.String(),
								"operation":  "build_analysis_arguments",
								"packet":     "FinalizeConversationPacket",
								"context_id": p.ContextID,
								"name":       initializedAnalysis.Name(),
								"event":      utils.ConversationCompleted.Get(),
								"error":      err.Error(),
								"error_type": fmt.Sprintf("%T", err),
							},
						},
					})
					continue
				}
				input := internal_type.AnalysisInput{
					ContextID:      p.ContextID,
					Arguments:      registry.Apply(arguments, source, variable.ResolveContext{Event: utils.ConversationCompleted.Get()}),
					ConversationID: conversationID,
					Auth:           h.r.auth,
				}
				output, err := initializedAnalysis.Execute(ctx, input)
				if err != nil {
					h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
						ContextID: p.ContextID,
						Scope:     internal_type.ObservabilityRecordScopeConversation,
						Record: observability.RecordLog{
							Level:   observability.LevelError,
							Message: "Analysis execution failed; post-call analysis may be missing",
							Attributes: observability.Attributes{
								"component":  observability.ComponentConversation.String(),
								"operation":  "execute_analysis",
								"packet":     "FinalizeConversationPacket",
								"context_id": p.ContextID,
								"name":       initializedAnalysis.Name(),
								"event":      utils.ConversationCompleted.Get(),
								"error":      err.Error(),
								"error_type": fmt.Sprintf("%T", err),
							},
						},
					})
					continue
				}
				metadata := output.Metadata
				if metadata != nil && metadata.GetKey() != "" {
					if h.r.metadata == nil {
						h.r.metadata = make(map[string]interface{})
					}
					h.r.metadata[metadata.GetKey()] = metadata.GetValue()
					metadataContextID := p.ContextID
					if conversationID != 0 {
						metadataContextID = fmt.Sprintf("%d", conversationID)
					}
					h.r.OnPacket(ctx, internal_type.ObservabilityMetadataRecordPacket{
						ContextID: metadataContextID,
						Scope:     internal_type.ObservabilityRecordScopeConversation,
						Record:    observability.NewConversationMetadataRecord([]*protos.Metadata{metadata}),
					})
				}
			}
		}
	}
	if h.r.metrics == nil {
		h.r.metrics = make(map[string]*protos.Metric)
	}
	h.r.metrics[type_enums.CONVERSATION_STATUS.String()] = &protos.Metric{
		Name:        type_enums.CONVERSATION_STATUS.String(),
		Value:       type_enums.CONVERSATION_COMPLETE.String(),
		Description: "Status of current conversation",
	}
	if h.r.observabilityRecorder != nil && h.r.Assistant() != nil && h.r.Conversation() != nil {
		messagesPayload := make([]map[string]interface{}, 0, len(h.r.GetHistories()))
		for _, message := range h.r.GetHistories() {
			if message == nil {
				continue
			}
			messagesPayload = append(messagesPayload, map[string]interface{}{
				"id":      message.ContextId(),
				"role":    message.Role(),
				"content": message.Content(),
			})
		}
		metadataPayload := make(map[string]interface{}, len(h.r.metadata))
		for key, value := range h.r.metadata {
			metadataPayload[key] = value
		}
		metricsPayload := make([]map[string]interface{}, 0, len(h.r.metrics))
		for _, metric := range h.r.metrics {
			if metric == nil {
				continue
			}
			metricsPayload = append(metricsPayload, map[string]interface{}{
				"name":        metric.GetName(),
				"value":       metric.GetValue(),
				"description": metric.GetDescription(),
			})
		}
		if err := h.r.observabilityRecorder.Record(ctx, observability.ConversationScope{
			AssistantScope: observability.AssistantScope{
				AssistantID: h.r.Assistant().Id,
			},
			ConversationID: h.r.Conversation().Id,
		}, observability.RecordWebhook{
			Event:     observability.ConversationCompleted,
			ContextID: p.ContextID,
			Payload: map[string]interface{}{
				"reason":   "conversation_completed",
				"status":   "completed",
				"messages": messagesPayload,
				"metadata": metadataPayload,
				"metrics":  metricsPayload,
			},
		}); err != nil {
			h.r.logger.Errorw("observability completed webhook failed to record", "error", err, "context_id", p.ContextID)
		}
	}

	// running analysis to close
	for _, analysis := range h.r.assistantAnalyseExecutors {
		if err := analysis.Close(ctx); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Analysis executor close failed; shutdown may leave resources open",
					Attributes: observability.Attributes{
						"component":  observability.ComponentConversation.String(),
						"operation":  "finalize",
						"packet":     "FinalizeConversationPacket",
						"context_id": p.ContextID,
						"name":       analysis.Name(),
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
	}

	// assistant executor cleanup
	if h.r.authenticationExecutor != nil {
		if err := h.r.authenticationExecutor.Close(ctx); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Authentication executor close failed; shutdown may leave resources open",
					Attributes: observability.Attributes{
						"component":  observability.ComponentConversation.String(),
						"operation":  "finalize",
						"packet":     "FinalizeConversationPacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
	}

	// trigger finalize
	h.r.OnPacket(ctx, internal_type.FinalizeAssistantPacket{ContextID: p.ContextID})
}
func (h requestorDispatchHandler) HandleFinalizeAssistant(ctx context.Context, p internal_type.FinalizeAssistantPacket) {
	if h.r.assistantExecutor != nil {
		if err := h.r.assistantExecutor.Close(ctx); err != nil {
			h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Assistant executor close failed; shutdown may leave resources open",
					Attributes: observability.Attributes{
						"component":  observability.ComponentLLM.String(),
						"operation":  "finalize",
						"packet":     "FinalizeAssistantPacket",
						"context_id": p.ContextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
	}
	h.r.OnPacket(ctx, internal_type.FinalizationCompletedPacket{ContextID: p.ContextID})
}

func (h requestorDispatchHandler) HandleFinalizationCompleted(ctx context.Context, p internal_type.FinalizationCompletedPacket) {
	_ = h.r.sessionLifecycle.Transition(adapter_lifecycle.EventDisconnectCompleted)
	h.r.cancelSession()
}

func (h requestorDispatchHandler) callInputNormalizer(ctx context.Context, vl internal_type.EndOfSpeechPacket) error {
	if h.r.inputNormalizer == nil {
		return errors.New("input inputNormalizer not configured")
	}
	if err := h.r.inputNormalizer.Normalize(ctx, vl); err != nil {
		h.r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: vl.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeUserMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Input normalization failed; raw user text will be used",
				Attributes: observability.Attributes{
					"component":    observability.ComponentSTT.String(),
					"operation":    "normalize_input",
					"packet":       "EndOfSpeechPacket",
					"context_id":   vl.ContextID,
					"message_role": string(observability.MessageRoleUser),
					"text_length":  fmt.Sprintf("%d", len(vl.Speech)),
					"error":        err.Error(),
					"error_type":   fmt.Sprintf("%T", err),
				},
			},
		})
		return err
	}
	return nil
}

func (r *genericRequestor) OnNotifyAssistantConfiguration(ctx context.Context, config *protos.ConversationInitialization, conversation *internal_conversation_entity.AssistantConversation) {
	utils.Go(ctx, func() {
		if err := r.Notify(ctx, config); err != nil {
			r.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: r.GetID(),
				Scope:     internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "Configuration notification failed; streamer may use stale settings",
					Attributes: observability.Attributes{
						"component":  observability.ComponentConversation.String(),
						"operation":  "notify_configuration",
						"context_id": r.GetID(),
						"mode":       r.GetMode().String(),
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					},
				},
			})
		}
	})
}

func (r *genericRequestor) IsConditionAllowed(opts utils.Option, key string) bool {
	raw, err := opts.GetString(key)
	if err != nil {
		return true
	}
	parsed, parseErr := internal_condition.Parse(raw)
	if parseErr != nil {
		return false
	}
	allowed, evalErr := parsed.Run(
		internal_condition.ConditionValue{RuleType: internal_condition.RuleTypeSource, Value: r.GetSource().Get()},
		internal_condition.ConditionValue{RuleType: internal_condition.RuleTypeMode, Value: r.GetMode().String()},
		internal_condition.ConditionValue{RuleType: internal_condition.RuleTypeDirection, Value: r.Conversation().Direction.String()},
	)
	if evalErr != nil {
		return false
	}
	return allowed
}
