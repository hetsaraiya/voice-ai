// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_llm_agentkit

import (
	"context"
	"fmt"
	"time"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (e *agentkitExecutor) Execute(ctx context.Context, comm internal_type.Communication, pctk internal_type.Packet) error {
	switch p := pctk.(type) {
	case internal_type.UserInputPacket:
		return e.Run(ctx, comm, UserTurnPipeline{Packet: p})
	case internal_type.UserTextReceivedPacket:
		return e.Run(ctx, comm, UserTextPipeline{Packet: p})
	case internal_type.InjectMessagePacket:
		return e.Run(ctx, comm, InjectMessagePipeline{Packet: p})
	case internal_type.LLMInterruptPacket:
		return e.Run(ctx, comm, InterruptionPipeline{Packet: p})
	default:
		return fmt.Errorf("unsupported packet type: %T", pctk)
	}
}

func (e *agentkitExecutor) Run(ctx context.Context, comm internal_type.Communication, p AgentPipeline) error {
	switch v := p.(type) {
	case UserTurnPipeline:
		return e.handleUserTurn(ctx, comm, v.Packet.ContextID, v.Packet.Text)
	case UserTextPipeline:
		return e.handleUserTurn(ctx, comm, v.Packet.ContextID, v.Packet.Text)
	case InjectMessagePipeline:
		// no-op: external agent manages its own history
		return nil
	case InterruptionPipeline:
		e.stateMu.Lock()
		e.activeContextID = ""
		e.stateMu.Unlock()
		return nil
	case ResponsePipeline:
		e.handleResponse(ctx, comm, v.Response)
		return nil
	default:
		return fmt.Errorf("unknown pipeline type: %T", p)
	}
}

func (e *agentkitExecutor) handleUserTurn(ctx context.Context, comm internal_type.Communication, contextID, text string) error {
	if !validator.NotBlank(contextID) {
		return nil
	}
	e.stateMu.Lock()
	e.activeContextID = contextID
	e.requestStartedAt = time.Now()
	e.stateMu.Unlock()

	comm.OnPacket(ctx,
		internal_type.ObservabilityEventRecordPacket{
			ContextID: contextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.NewMessageRecord(contextID, observability.ComponentLLM, observability.LLMStarted, observability.MessageRoleAssistant, observability.Attributes{
				"provider":         e.Name(),
				"context_id":       contextID,
				"input_char_count": fmt.Sprintf("%d", len(text)),
			}),
		},
		internal_type.ObservabilityLogRecordPacket{
			ContextID: contextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelDebug,
				Message: "agentkit request started",
				Attributes: observability.Attributes{
					"component":        observability.ComponentLLM.String(),
					"operation":        "execute",
					"provider":         e.Name(),
					"context_id":       contextID,
					"input_char_count": fmt.Sprintf("%d", len(text)),
				},
				OccurredAt: time.Now(),
			},
		},
		internal_type.ObservabilityMetricRecordPacket{
			ContextID: contextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordMetric{
				Attributes: observability.Attributes{"provider": e.Name()},
				Metrics: []*protos.Metric{{
					Name:        "llm_input_char_count",
					Value:       fmt.Sprintf("%d", len(text)),
					Description: "Input character count sent to AgentKit",
				}},
			},
		},
	)
	if err := e.send(&protos.TalkInput{
		Request: &protos.TalkInput_Message{
			Message: &protos.ConversationUserMessage{
				Message:   &protos.ConversationUserMessage_Text{Text: text},
				Id:        contextID,
				Completed: true,
				Time:      timestamppb.Now(),
			},
		},
	}); err != nil {
		return err
	}
	return nil
}
