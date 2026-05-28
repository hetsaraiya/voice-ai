// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_llm_agentkit

import (
	"context"
	"fmt"
	"strings"
	"time"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
)

func (e *agentkitExecutor) handleResponse(ctx context.Context, comm internal_type.Communication, resp *protos.TalkOutput) {
	switch data := resp.GetData().(type) {
	case *protos.TalkOutput_Initialization:
		comm.OnPacket(ctx, internal_type.ConversationEventPacket{
			Name: "agentkit", Time: time.Now(),
			Data: map[string]string{
				"type":            "initialization_ack",
				"conversation_id": fmt.Sprintf("%d", data.Initialization.GetAssistantConversationId()),
			},
		})

	case *protos.TalkOutput_Interruption:
		if !e.isCurrentContext(data.Interruption.GetId()) {
			return
		}
		comm.OnPacket(ctx,
			internal_type.InterruptionDetectedPacket{ContextID: data.Interruption.Id, Source: internal_type.InterruptionSourceWord},
			internal_type.ConversationEventPacket{
				ContextID: data.Interruption.Id, Name: "agentkit", Time: time.Now(),
				Data: map[string]string{"type": "interruption", "source": "word"},
			},
		)

	case *protos.TalkOutput_Assistant:
		if !e.isCurrentContext(data.Assistant.GetId()) {
			return
		}
		contextID := data.Assistant.GetId()
		switch msg := data.Assistant.GetMessage().(type) {
		case *protos.ConversationAssistantMessage_Text:
			if data.Assistant.GetCompleted() {
				comm.OnPacket(ctx,
					internal_type.LLMResponseDonePacket{ContextID: contextID, Text: msg.Text},
					internal_type.ConversationEventPacket{
						ContextID: contextID, Name: "agentkit", Time: time.Now(),
						Data: map[string]string{
							"type": "completed", "text": msg.Text,
							"response_char_count": fmt.Sprintf("%d", len(msg.Text)),
						},
					},
				)
			} else {
				comm.OnPacket(ctx,
					internal_type.LLMResponseDeltaPacket{ContextID: contextID, Text: msg.Text},
					internal_type.ConversationEventPacket{
						ContextID: contextID, Name: "agentkit", Time: time.Now(),
						Data: map[string]string{
							"type": "chunk", "text": msg.Text,
							"response_char_count": fmt.Sprintf("%d", len(msg.Text)),
						},
					},
				)
			}
		}

	case *protos.TalkOutput_ToolCall:
		if !e.isCurrentContext(data.ToolCall.GetId()) {
			return
		}
		comm.OnPacket(ctx, internal_type.LLMToolCallPacket{
			ContextID: data.ToolCall.GetId(), ToolID: data.ToolCall.GetToolId(),
			Name: data.ToolCall.GetName(), Action: data.ToolCall.GetAction(), Arguments: data.ToolCall.GetArgs(),
		})

	case *protos.TalkOutput_ToolCallResult:
		if !e.isCurrentContext(data.ToolCallResult.GetId()) {
			return
		}
		comm.OnPacket(ctx,
			internal_type.LLMToolResultPacket{
				ToolID:    data.ToolCallResult.GetToolId(),
				Name:      data.ToolCallResult.GetName(),
				ContextID: data.ToolCallResult.GetId(),
				Action:    data.ToolCallResult.GetAction(),
				Result:    data.ToolCallResult.GetResult(),
			},
			internal_type.ConversationEventPacket{
				ContextID: data.ToolCallResult.GetId(), Name: "tool", Time: time.Now(),
				Data: map[string]string{
					"type": "tool_result", "tool_id": data.ToolCallResult.GetToolId(),
					"name": data.ToolCallResult.GetName(), "action": data.ToolCallResult.GetAction().String(),
				},
			})

	case *protos.TalkOutput_Error:
		contextID := e.getActiveContextID()
		e.logger.Errorf("AgentKit agent error: code=%d message=%s", data.Error.GetErrorCode(), data.Error.GetErrorMessage())
		comm.OnPacket(ctx,
			internal_type.LLMErrorPacket{
				ContextID: contextID,
				Error:     fmt.Errorf("agentkit error %d: %s", data.Error.GetErrorCode(), data.Error.GetErrorMessage()),
				Type:      internal_type.LLMSystemPanic,
			},
			internal_type.ConversationEventPacket{
				ContextID: contextID, Name: "agentkit", Time: time.Now(),
				Data: map[string]string{
					"type": "error", "error": data.Error.GetErrorMessage(),
					"code": fmt.Sprintf("%d", data.Error.GetErrorCode()),
				},
			},
			internal_type.LLMToolCallPacket{
				ContextID: contextID,
				Name:      "end_conversation",
				Action:    protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION,
				Arguments: map[string]string{"reason": data.Error.GetErrorMessage()},
			},
		)
	}
}

func (e *agentkitExecutor) isCurrentContext(id string) bool {
	clean := strings.TrimSpace(id)
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	if e.activeContextID == "" {
		return true
	}
	if !validator.NotBlank(clean) {
		return true
	}
	return clean == e.activeContextID
}

func (e *agentkitExecutor) getActiveContextID() string {
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	return e.activeContextID
}
