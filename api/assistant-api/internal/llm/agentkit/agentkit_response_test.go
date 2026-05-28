package internal_llm_agentkit

import (
	"context"
	"testing"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/protos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleResponse_AllTypes(t *testing.T) {
	tests := []struct {
		name     string
		resp     *protos.TalkOutput
		wantFunc func(t *testing.T, pkts []internal_type.Packet)
	}{
		{
			name: "initialization_ack",
			resp: &protos.TalkOutput{
				Data: &protos.TalkOutput_Initialization{
					Initialization: &protos.ConversationInitialization{
						AssistantConversationId: 42,
					},
				},
			},
			wantFunc: func(t *testing.T, pkts []internal_type.Packet) {
				require.Len(t, pkts, 1)
				ev, ok := pkts[0].(internal_type.ConversationEventPacket)
				require.True(t, ok)
				assert.Equal(t, "agentkit", ev.Name)
				assert.Equal(t, "initialization_ack", ev.Data["type"])
				assert.Equal(t, "42", ev.Data["conversation_id"])
			},
		},
		{
			name: "interruption",
			resp: &protos.TalkOutput{
				Data: &protos.TalkOutput_Interruption{
					Interruption: &protos.ConversationInterruption{Id: "ctx-1"},
				},
			},
			wantFunc: func(t *testing.T, pkts []internal_type.Packet) {
				require.Len(t, pkts, 2)
				ip, ok := pkts[0].(internal_type.InterruptionDetectedPacket)
				require.True(t, ok)
				assert.Equal(t, "ctx-1", ip.ContextID)
				assert.Equal(t, internal_type.InterruptionSourceWord, ip.Source)
				ev, ok := pkts[1].(internal_type.ConversationEventPacket)
				require.True(t, ok)
				assert.Equal(t, "interruption", ev.Data["type"])
			},
		},
		{
			name: "text_delta",
			resp: &protos.TalkOutput{
				Data: &protos.TalkOutput_Assistant{
					Assistant: &protos.ConversationAssistantMessage{
						Id:        "msg-1",
						Completed: false,
						Message:   &protos.ConversationAssistantMessage_Text{Text: "hello "},
					},
				},
			},
			wantFunc: func(t *testing.T, pkts []internal_type.Packet) {
				require.Len(t, pkts, 2)
				delta, ok := pkts[0].(internal_type.LLMResponseDeltaPacket)
				require.True(t, ok)
				assert.Equal(t, "msg-1", delta.ContextID)
				assert.Equal(t, "hello ", delta.Text)
				ev, ok := pkts[1].(internal_type.ConversationEventPacket)
				require.True(t, ok)
				assert.Equal(t, "agentkit", ev.Name)
				assert.Equal(t, "chunk", ev.Data["type"])
				assert.Equal(t, "hello ", ev.Data["text"])
				assert.Equal(t, "6", ev.Data["response_char_count"])
			},
		},
		{
			name: "text_completed",
			resp: &protos.TalkOutput{
				Data: &protos.TalkOutput_Assistant{
					Assistant: &protos.ConversationAssistantMessage{
						Id:        "msg-2",
						Completed: true,
						Message:   &protos.ConversationAssistantMessage_Text{Text: "world"},
					},
				},
			},
			wantFunc: func(t *testing.T, pkts []internal_type.Packet) {
				require.Len(t, pkts, 2)
				done, ok := pkts[0].(internal_type.LLMResponseDonePacket)
				require.True(t, ok)
				assert.Equal(t, "msg-2", done.ContextID)
				assert.Equal(t, "world", done.Text)
				ev, ok := pkts[1].(internal_type.ConversationEventPacket)
				require.True(t, ok)
				assert.Equal(t, "completed", ev.Data["type"])
				assert.Equal(t, "5", ev.Data["response_char_count"])
			},
		},
		{
			name: "audio_noop",
			resp: &protos.TalkOutput{
				Data: &protos.TalkOutput_Assistant{
					Assistant: &protos.ConversationAssistantMessage{
						Id:      "msg-3",
						Message: &protos.ConversationAssistantMessage_Audio{Audio: []byte{0x01}},
					},
				},
			},
			wantFunc: func(t *testing.T, pkts []internal_type.Packet) {
				assert.Empty(t, pkts)
			},
		},
		{
			name: "tool_call",
			resp: &protos.TalkOutput{
				Data: &protos.TalkOutput_ToolCall{
					ToolCall: &protos.ConversationToolCall{
						Id:     "tc-1",
						ToolId: "tool-42",
						Name:   "get_weather",
					},
				},
			},
			wantFunc: func(t *testing.T, pkts []internal_type.Packet) {
				require.Len(t, pkts, 1)
				tc, ok := pkts[0].(internal_type.LLMToolCallPacket)
				require.True(t, ok)
				assert.Equal(t, "tool-42", tc.ToolID)
				assert.Equal(t, "get_weather", tc.Name)
			},
		},
		{
			name: "tool_call_result",
			resp: &protos.TalkOutput{
				Data: &protos.TalkOutput_ToolCallResult{
					ToolCallResult: &protos.ConversationToolCallResult{
						Id:     "tr-1",
						ToolId: "tool-42",
						Name:   "get_weather",
						Action: protos.ToolCallAction_TOOL_CALL_ACTION_UNSPECIFIED,
						Result: map[string]string{"ok": "true"},
					},
				},
			},
			wantFunc: func(t *testing.T, pkts []internal_type.Packet) {
				require.Len(t, pkts, 2)

				toolResult, ok := pkts[0].(internal_type.LLMToolResultPacket)
				require.True(t, ok)
				assert.Equal(t, "tr-1", toolResult.ContextID)
				assert.Equal(t, "tool-42", toolResult.ToolID)
				assert.Equal(t, "get_weather", toolResult.Name)
				assert.Equal(t, map[string]string{"ok": "true"}, toolResult.Result)

				ev, ok := pkts[1].(internal_type.ConversationEventPacket)
				require.True(t, ok)
				assert.Equal(t, "tool", ev.Name)
				assert.Equal(t, "tool_result", ev.Data["type"])
				assert.Equal(t, "tool-42", ev.Data["tool_id"])
			},
		},
		{
			name: "error",
			resp: &protos.TalkOutput{
				Data: &protos.TalkOutput_Error{
					Error: &protos.Error{
						ErrorCode:    500,
						ErrorMessage: "agent crashed",
					},
				},
			},
			wantFunc: func(t *testing.T, pkts []internal_type.Packet) {
				require.Len(t, pkts, 3)
				errPkt, ok := pkts[0].(internal_type.LLMErrorPacket)
				require.True(t, ok)
				assert.Contains(t, errPkt.Error.Error(), "agent crashed")

				ev, ok := pkts[1].(internal_type.ConversationEventPacket)
				require.True(t, ok)
				assert.Equal(t, "error", ev.Data["type"])
				assert.Equal(t, "500", ev.Data["code"])

				dir, ok := pkts[2].(internal_type.LLMToolCallPacket)
				require.True(t, ok)
				assert.Equal(t, protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION, dir.Action)
			},
		},
		{
			name: "tool_call_with_action",
			resp: &protos.TalkOutput{
				Data: &protos.TalkOutput_ToolCall{
					ToolCall: &protos.ConversationToolCall{
						Id:     "d-1",
						Action: protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION,
					},
				},
			},
			wantFunc: func(t *testing.T, pkts []internal_type.Packet) {
				require.Len(t, pkts, 1)
				tc, ok := pkts[0].(internal_type.LLMToolCallPacket)
				require.True(t, ok)
				assert.Equal(t, "d-1", tc.ContextID)
				assert.Equal(t, protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION, tc.Action)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestExecutor()
			comm, collector := newTestComm()
			e.handleResponse(context.Background(), comm, tt.resp)
			tt.wantFunc(t, collector.all())
		})
	}
}
