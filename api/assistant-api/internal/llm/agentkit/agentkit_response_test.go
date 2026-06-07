package internal_llm_agentkit

import (
	"context"
	"testing"

	"github.com/rapidaai/api/assistant-api/internal/observability"
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
				log, ok := pkts[0].(internal_type.ObservabilityLogRecordPacket)
				require.True(t, ok)
				assert.Equal(t, observability.LevelDebug, log.Record.Level)
				assert.Equal(t, "initialization_ack", log.Record.Attributes["operation"])
				assert.Equal(t, "42", log.Record.Attributes["conversation_id"])
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
				require.Len(t, pkts, 3)
				ip, ok := pkts[0].(internal_type.InterruptionDetectedPacket)
				require.True(t, ok)
				assert.Equal(t, "ctx-1", ip.ContextID)
				assert.Equal(t, internal_type.InterruptionSourceWord, ip.Source)
				ev, ok := pkts[1].(internal_type.ObservabilityEventRecordPacket)
				require.True(t, ok)
				assert.Equal(t, observability.LLMDiscarded, ev.Record.Event)
				log, ok := pkts[2].(internal_type.ObservabilityLogRecordPacket)
				require.True(t, ok)
				assert.Equal(t, "interrupt", log.Record.Attributes["operation"])
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
				require.Len(t, pkts, 1)
				delta, ok := pkts[0].(internal_type.LLMResponseDeltaPacket)
				require.True(t, ok)
				assert.Equal(t, "msg-1", delta.ContextID)
				assert.Equal(t, "hello ", delta.Text)
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
				require.Len(t, pkts, 3)
				done, ok := pkts[0].(internal_type.LLMResponseDonePacket)
				require.True(t, ok)
				assert.Equal(t, "msg-2", done.ContextID)
				assert.Equal(t, "world", done.Text)
				ev, ok := pkts[1].(internal_type.ObservabilityEventRecordPacket)
				require.True(t, ok)
				assert.Equal(t, observability.LLMCompleted, ev.Record.Event)
				assert.Equal(t, "5", ev.Record.Attributes["response_char_count"])
				metric, ok := pkts[2].(internal_type.ObservabilityMetricRecordPacket)
				require.True(t, ok)
				require.Len(t, metric.Record.Metrics, 1)
				assert.Equal(t, "llm_response_char_count", metric.Record.Metrics[0].Name)
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

				log, ok := pkts[1].(internal_type.ObservabilityLogRecordPacket)
				require.True(t, ok)
				assert.Equal(t, "tool_result", log.Record.Attributes["operation"])
				assert.Equal(t, "tool-42", log.Record.Attributes["tool_id"])
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
				require.Len(t, pkts, 4)
				errPkt, ok := pkts[0].(internal_type.LLMErrorPacket)
				require.True(t, ok)
				assert.Contains(t, errPkt.Error.Error(), "agent crashed")

				ev, ok := pkts[1].(internal_type.ObservabilityEventRecordPacket)
				require.True(t, ok)
				assert.Equal(t, observability.LLMError, ev.Record.Event)
				assert.Equal(t, "500", ev.Record.Attributes["code"])
				log, ok := pkts[2].(internal_type.ObservabilityLogRecordPacket)
				require.True(t, ok)
				assert.Equal(t, observability.LevelError, log.Record.Level)

				dir, ok := pkts[3].(internal_type.LLMToolCallPacket)
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
