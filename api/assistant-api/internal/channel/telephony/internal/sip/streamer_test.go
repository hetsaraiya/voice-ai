package internal_sip_telephony

import (
	"testing"
	"time"

	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	internal_telephony_base "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/base"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/protos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSIPStreamer(t *testing.T) *Streamer {
	t.Helper()
	logger, err := commons.NewApplicationLogger()
	require.NoError(t, err)

	return &Streamer{
		BaseTelephonyStreamer: internal_telephony_base.NewBaseTelephonyStreamer(logger, &callcontext.CallContext{}, nil),
	}
}

func TestSend_EndConversation_PushesToolResult(t *testing.T) {
	s := newTestSIPStreamer(t)

	err := s.Send(&protos.ConversationToolCall{
		Id:     "ctx-1",
		ToolId: "tool-1",
		Name:   "end_conversation",
		Action: protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION,
	})
	require.NoError(t, err)

	select {
	case msg := <-s.CriticalCh:
		result, ok := msg.(*protos.ConversationToolCallResult)
		require.True(t, ok, "expected ConversationToolCallResult, got %T", msg)
		assert.Equal(t, "ctx-1", result.GetId())
		assert.Equal(t, "tool-1", result.GetToolId())
		assert.Equal(t, map[string]string{"status": "completed"}, result.GetResult())
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ConversationToolCallResult")
	}

	// Context should remain open; disconnect is owned by handleToolResult in adapter layer.
	select {
	case <-s.Context().Done():
		t.Fatal("streamer context should remain open; teardown is owned by Talk loop")
	default:
	}
}

// drainLowChForEvent reads from LowCh until it finds a ConversationEvent whose
// "type" matches wantType, or the timeout expires.
func drainLowChForEvent(t *testing.T, s *Streamer, wantType string, timeout time.Duration) *protos.ConversationEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case msg := <-s.LowCh:
			event, ok := msg.(*protos.ConversationEvent)
			if !ok {
				continue
			}
			if event.GetData()["type"] == wantType {
				return event
			}
		case <-deadline:
			return nil
		}
	}
}

func TestSend_ConversationDisconnection_EmitsEventAndClosesStreamer(t *testing.T) {
	s := newTestSIPStreamer(t)

	err := s.Send(&protos.ConversationDisconnection{
		Type: protos.ConversationDisconnection_DISCONNECTION_TYPE_IDLE_TIMEOUT,
	})
	require.NoError(t, err)

	// Server-initiated Send no longer requeues the disconnect onto CriticalCh —
	// the server callsite already knows the reason. The talker exits via the
	// Recv-err path once Close cancels s.Ctx.
	select {
	case msg := <-s.CriticalCh:
		t.Fatalf("server-initiated Send must not push to CriticalCh; got %T", msg)
	default:
	}

	// Verify a "disconnected" channel event carrying the reason was emitted.
	event := drainLowChForEvent(t, s, "disconnected", time.Second)
	require.NotNil(t, event, "expected a 'disconnected' channel event")
	assert.Equal(t, protos.ConversationDisconnection_DISCONNECTION_TYPE_IDLE_TIMEOUT.String(), event.GetData()["reason"])

	// Streamer should be closed: s.Ctx cancelled so Talker.Recv returns EOF.
	select {
	case <-s.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("expected streamer context to be cancelled after disconnect")
	}
}

func TestSend_ConversationDisconnection_PreservesExplicitReason(t *testing.T) {
	s := newTestSIPStreamer(t)

	err := s.Send(&protos.ConversationDisconnection{
		Type: protos.ConversationDisconnection_DISCONNECTION_TYPE_MAX_DURATION,
	})
	require.NoError(t, err)

	event := drainLowChForEvent(t, s, "disconnected", time.Second)
	require.NotNil(t, event, "expected a 'disconnected' channel event")
	assert.Equal(t, protos.ConversationDisconnection_DISCONNECTION_TYPE_MAX_DURATION.String(), event.GetData()["reason"])
}

func TestSend_ConversationDisconnection_SubsequentSendErrors(t *testing.T) {
	s := newTestSIPStreamer(t)

	// First disconnect closes the streamer.
	require.NoError(t, s.Send(&protos.ConversationDisconnection{
		Type: protos.ConversationDisconnection_DISCONNECTION_TYPE_USER,
	}))

	select {
	case <-s.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("expected streamer context to be cancelled")
	}

	// Subsequent Send must error (streamer is closed) without panicking or
	// duplicating cleanup.
	err := s.Send(&protos.ConversationDisconnection{
		Type: protos.ConversationDisconnection_DISCONNECTION_TYPE_USER,
	})
	require.Error(t, err, "Send on closed streamer should return an error")
}

func TestSend_TransferConversation_UsesTransferToKey(t *testing.T) {
	s := newTestSIPStreamer(t)

	var gotTargets []string
	var gotPostTransferAction string
	s.SetOnTransferInitiated(func(targets []string, postTransferAction string) {
		gotTargets = append([]string(nil), targets...)
		gotPostTransferAction = postTransferAction
	})

	err := s.Send(&protos.ConversationToolCall{
		Id:     "ctx-transfer",
		ToolId: "tool-transfer",
		Name:   "transfer_call",
		Action: protos.ToolCallAction_TOOL_CALL_ACTION_TRANSFER_CONVERSATION,
		Args: map[string]string{
			"transfer_to":          "+15550001111" + commons.SEPARATOR + "sip:agent@example.com",
			"post_transfer_action": "end_call",
		},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"+15550001111", "sip:agent@example.com"}, gotTargets)
	assert.Equal(t, "end_call", gotPostTransferAction)
}

func TestSend_TransferConversation_MissingTransferTarget(t *testing.T) {
	s := newTestSIPStreamer(t)

	err := s.Send(&protos.ConversationToolCall{
		Id:     "ctx-transfer-missing",
		ToolId: "tool-transfer",
		Name:   "transfer_call",
		Action: protos.ToolCallAction_TOOL_CALL_ACTION_TRANSFER_CONVERSATION,
		Args:   map[string]string{"transfer_to": ""},
	})
	require.NoError(t, err)

	select {
	case msg := <-s.CriticalCh:
		result, ok := msg.(*protos.ConversationToolCallResult)
		require.True(t, ok, "expected ConversationToolCallResult, got %T", msg)
		assert.Equal(t, "ctx-transfer-missing", result.GetId())
		assert.Equal(t, "tool-transfer", result.GetToolId())
		assert.Equal(t, "failed", result.GetResult()["status"])
		assert.Contains(t, result.GetResult()["reason"], "missing transfer target")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ConversationToolCallResult")
	}
}
