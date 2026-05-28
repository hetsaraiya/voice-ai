package internal_llm_agentkit

import (
	"context"
	"testing"
	"time"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClose_ClosesStreamWhenPresent(t *testing.T) {
	e := newTestExecutor()
	talker := newMockTalker()
	e.transport.stream = talker
	e.transport.listenerDone = make(chan struct{})
	close(e.transport.listenerDone)

	err := e.Close(context.Background())
	require.NoError(t, err)
	assert.True(t, talker.closeSent.Load(), "CloseSend should have been called")
}

func TestClose_SucceedsWhenTransportIsEmpty(t *testing.T) {
	e := newTestExecutor()
	e.transport.stream = nil
	e.transport.conn = nil
	e.transport.listenerDone = nil

	err := e.Close(context.Background())
	require.NoError(t, err)
}

func TestClose_ClearsTransportState(t *testing.T) {
	e := newTestExecutor()
	talker := newMockTalker()
	e.transport.stream = talker
	e.transport.listenerDone = make(chan struct{})
	close(e.transport.listenerDone)

	_ = e.Close(context.Background())

	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	assert.Nil(t, e.transport.stream)
	assert.Nil(t, e.transport.conn)
	assert.Nil(t, e.transport.listenerDone)
}

func TestClose_WaitsForListenerExitWithTimeout(t *testing.T) {
	e := newTestExecutor()
	talker := newMockTalker()
	e.transport.stream = talker
	e.transport.listenerDone = make(chan struct{})

	start := time.Now()
	done := make(chan error, 1)
	go func() {
		done <- e.Close(context.Background())
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
		elapsed := time.Since(start)
		assert.Greater(t, elapsed, 4*time.Second)
	case <-time.After(7 * time.Second):
		t.Fatal("Close did not return within expected timeout")
	}
}

func TestClose_ResetsActiveContextID(t *testing.T) {
	e := newTestExecutor()
	talker := newMockTalker()
	e.transport.stream = talker
	e.transport.listenerDone = make(chan struct{})
	close(e.transport.listenerDone)
	e.activeContextID = "active"

	_ = e.Close(context.Background())

	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	assert.Equal(t, "", e.activeContextID)
	assert.Nil(t, e.transport.stream)
}

func TestClose_DisconnectsExecutorForFutureExecute(t *testing.T) {
	e := newTestExecutor()
	talker := newMockTalker()
	e.transport.stream = talker
	e.transport.listenerDone = make(chan struct{})
	close(e.transport.listenerDone)
	_ = e.Close(context.Background())

	comm, _ := newTestComm()
	err := e.Execute(context.Background(), comm, internal_type.UserInputPacket{
		ContextID: "ctx-1",
		Text:      "after close",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}
