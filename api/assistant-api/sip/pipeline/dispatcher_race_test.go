// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package sip_pipeline

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	"github.com/rapidaai/api/assistant-api/internal/observe"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
	"github.com/stretchr/testify/require"
)

func newPipelineTestLogger(t *testing.T) commons.Logger {
	t.Helper()
	l, err := commons.NewApplicationLogger(
		commons.Level("error"),
		commons.Name("sip-pipeline-test"),
		commons.EnableFile(false),
	)
	require.NoError(t, err)
	return l
}

func newPipelineTestSession(t *testing.T) *sip_infra.Session {
	t.Helper()
	s, err := sip_infra.NewSession(context.Background(), &sip_infra.SessionConfig{
		Config: &sip_infra.Config{
			Server:            "127.0.0.1",
			Port:              5060,
			RTPPortRangeStart: 10000,
			RTPPortRangeEnd:   10020,
		},
		Direction: sip_infra.CallDirectionInbound,
	})
	require.NoError(t, err)
	return s
}

func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()
	select {
	case <-done:
		return false
	case <-time.After(timeout):
		return true
	}
}

type fakePreparedCallRuntime struct {
	started chan struct{}
	closed  chan struct{}
}

func (f *fakePreparedCallRuntime) Start(_ context.Context) error {
	select {
	case f.started <- struct{}{}:
	default:
	}
	return nil
}

func (f *fakePreparedCallRuntime) Close(_ context.Context) {
	select {
	case f.closed <- struct{}{}:
	default:
	}
}

func TestHandleSessionEstablished_SetupErrorEndsSession(t *testing.T) {
	t.Parallel()

	d := NewDispatcher(&DispatcherConfig{
		Logger:         newPipelineTestLogger(t),
		TransferServer: &fakeTransferServer{},
		OnCallSetup: func(ctx context.Context, session *sip_infra.Session, auth types.SimplePrinciple, assistantID uint64, conversationID uint64, cc *callcontext.CallContext) (*CallSetupResult, error) {
			return nil, fmt.Errorf("setup failed")
		},
		OnCallStart: func(ctx context.Context, session *sip_infra.Session, setup *CallSetupResult, vaultCred interface{}, sipConfig *sip_infra.Config, direction string) error {
			return nil
		},
	})

	s := newPipelineTestSession(t)
	d.handleSessionEstablished(context.Background(), sip_infra.SessionEstablishedPipeline{
		ID:             "call-setup-fail",
		Session:        s,
		Direction:      sip_infra.CallDirectionInbound,
		AssistantID:    1,
		ConversationID: 42,
	})

	require.Eventually(t, s.IsEnded, 2*time.Second, 10*time.Millisecond)
}

func TestHandleSessionEstablished_PanicStillCallsOnCallEnd(t *testing.T) {
	t.Parallel()

	onEnd := make(chan struct{}, 1)
	var onEndCount atomic.Int32

	d := NewDispatcher(&DispatcherConfig{
		Logger: newPipelineTestLogger(t),
		OnCallSetup: func(ctx context.Context, session *sip_infra.Session, auth types.SimplePrinciple, assistantID uint64, conversationID uint64, cc *callcontext.CallContext) (*CallSetupResult, error) {
			return &CallSetupResult{AssistantID: assistantID, ConversationID: conversationID}, nil
		},
		OnCallStart: func(ctx context.Context, session *sip_infra.Session, setup *CallSetupResult, vaultCred interface{}, sipConfig *sip_infra.Config, direction string) error {
			panic("boom")
		},
		OnCallEnd: func(callID string) {
			onEndCount.Add(1)
			select {
			case onEnd <- struct{}{}:
			default:
			}
		},
	})

	s := newPipelineTestSession(t)
	d.handleSessionEstablished(context.Background(), sip_infra.SessionEstablishedPipeline{
		ID:             "call-panic",
		Session:        s,
		Direction:      sip_infra.CallDirectionInbound,
		AssistantID:    1,
		ConversationID: 42,
	})

	select {
	case <-onEnd:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnCallEnd after panic")
	}

	require.Equal(t, int32(1), onEndCount.Load())
}

func TestPrepareSessionDefersCallStartUntilExplicitStart(t *testing.T) {
	t.Parallel()

	var setupCount atomic.Int32
	var startCount atomic.Int32
	started := make(chan struct{}, 1)

	d := NewDispatcher(&DispatcherConfig{
		Logger: newPipelineTestLogger(t),
		OnCallSetup: func(ctx context.Context, session *sip_infra.Session, auth types.SimplePrinciple, assistantID uint64, conversationID uint64, cc *callcontext.CallContext) (*CallSetupResult, error) {
			setupCount.Add(1)
			return &CallSetupResult{AssistantID: assistantID, ConversationID: conversationID}, nil
		},
		OnCallStart: func(ctx context.Context, session *sip_infra.Session, setup *CallSetupResult, vaultCred interface{}, sipConfig *sip_infra.Config, direction string) error {
			startCount.Add(1)
			started <- struct{}{}
			return nil
		},
	})

	stage := sip_infra.SessionEstablishedPipeline{
		ID:             "call-prepared",
		Session:        newPipelineTestSession(t),
		Direction:      sip_infra.CallDirectionInbound,
		AssistantID:    1,
		ConversationID: 42,
	}

	require.NoError(t, d.PrepareSession(context.Background(), stage))
	require.Equal(t, int32(1), setupCount.Load())
	require.Equal(t, int32(0), startCount.Load())

	require.NoError(t, d.StartPreparedSession(context.Background(), stage))

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for prepared call start")
	}
	require.Equal(t, int32(1), startCount.Load())
}

func TestPrepareSession_PreparesInboundRuntimeBeforeExplicitStart(t *testing.T) {
	t.Parallel()

	var prepareRuntimeCount atomic.Int32
	var legacyStartCount atomic.Int32
	runtime := &fakePreparedCallRuntime{started: make(chan struct{}, 1), closed: make(chan struct{}, 1)}

	d := NewDispatcher(&DispatcherConfig{
		Logger: newPipelineTestLogger(t),
		OnCallSetup: func(ctx context.Context, session *sip_infra.Session, auth types.SimplePrinciple, assistantID uint64, conversationID uint64, cc *callcontext.CallContext) (*CallSetupResult, error) {
			return &CallSetupResult{AssistantID: assistantID, ConversationID: conversationID}, nil
		},
		OnPrepareCallRuntime: func(ctx context.Context, stage sip_infra.SessionEstablishedPipeline, setup *CallSetupResult, observer *observe.ConversationObserver) (PreparedCallRuntime, error) {
			prepareRuntimeCount.Add(1)
			return runtime, nil
		},
		OnCallStart: func(ctx context.Context, session *sip_infra.Session, setup *CallSetupResult, vaultCred interface{}, sipConfig *sip_infra.Config, direction string) error {
			legacyStartCount.Add(1)
			return nil
		},
	})

	stage := sip_infra.SessionEstablishedPipeline{
		ID:             "call-runtime-prepared",
		Session:        newPipelineTestSession(t),
		Direction:      sip_infra.CallDirectionInbound,
		AssistantID:    1,
		ConversationID: 42,
	}

	require.NoError(t, d.PrepareSession(context.Background(), stage))
	require.Equal(t, int32(1), prepareRuntimeCount.Load())
	require.Equal(t, int32(0), legacyStartCount.Load())

	require.NoError(t, d.StartPreparedSession(context.Background(), stage))
	select {
	case <-runtime.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for prepared runtime start")
	}
	require.Equal(t, int32(0), legacyStartCount.Load())
}

func TestDiscardPreparedSessionClosesPreparedRuntime(t *testing.T) {
	t.Parallel()

	runtime := &fakePreparedCallRuntime{started: make(chan struct{}, 1), closed: make(chan struct{}, 1)}
	d := NewDispatcher(&DispatcherConfig{
		Logger: newPipelineTestLogger(t),
		OnCallSetup: func(ctx context.Context, session *sip_infra.Session, auth types.SimplePrinciple, assistantID uint64, conversationID uint64, cc *callcontext.CallContext) (*CallSetupResult, error) {
			return &CallSetupResult{AssistantID: assistantID, ConversationID: conversationID}, nil
		},
		OnPrepareCallRuntime: func(ctx context.Context, stage sip_infra.SessionEstablishedPipeline, setup *CallSetupResult, observer *observe.ConversationObserver) (PreparedCallRuntime, error) {
			return runtime, nil
		},
		OnCallStart: func(ctx context.Context, session *sip_infra.Session, setup *CallSetupResult, vaultCred interface{}, sipConfig *sip_infra.Config, direction string) error {
			return nil
		},
	})
	stage := sip_infra.SessionEstablishedPipeline{
		ID:             "call-runtime-discarded",
		Session:        newPipelineTestSession(t),
		Direction:      sip_infra.CallDirectionInbound,
		AssistantID:    1,
		ConversationID: 42,
	}

	require.NoError(t, d.PrepareSession(context.Background(), stage))
	d.DiscardPreparedSession(context.Background(), stage.ID)

	select {
	case <-runtime.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for prepared runtime close")
	}
	require.Error(t, d.StartPreparedSession(context.Background(), stage))
}

func TestDiscardPreparedSessionPreventsLateStart(t *testing.T) {
	t.Parallel()

	var startCount atomic.Int32
	d := NewDispatcher(&DispatcherConfig{
		Logger: newPipelineTestLogger(t),
		OnCallSetup: func(ctx context.Context, session *sip_infra.Session, auth types.SimplePrinciple, assistantID uint64, conversationID uint64, cc *callcontext.CallContext) (*CallSetupResult, error) {
			return &CallSetupResult{AssistantID: assistantID, ConversationID: conversationID}, nil
		},
		OnCallStart: func(ctx context.Context, session *sip_infra.Session, setup *CallSetupResult, vaultCred interface{}, sipConfig *sip_infra.Config, direction string) error {
			startCount.Add(1)
			return nil
		},
	})
	stage := sip_infra.SessionEstablishedPipeline{
		ID:             "call-discarded",
		Session:        newPipelineTestSession(t),
		Direction:      sip_infra.CallDirectionInbound,
		AssistantID:    1,
		ConversationID: 42,
	}

	require.NoError(t, d.PrepareSession(context.Background(), stage))
	d.DiscardPreparedSession(context.Background(), stage.ID)

	require.Error(t, d.StartPreparedSession(context.Background(), stage))
	require.Equal(t, int32(0), startCount.Load())
}

func TestDispatcherBackpressureAndTeardownStress(t *testing.T) {
	logger := newPipelineTestLogger(t)

	const calls = 400
	var endedWG sync.WaitGroup
	endedWG.Add(calls)

	var startCount atomic.Int32
	var endCount atomic.Int32

	d := NewDispatcher(&DispatcherConfig{
		Logger: logger,
		OnCallSetup: func(ctx context.Context, session *sip_infra.Session, auth types.SimplePrinciple, assistantID uint64, conversationID uint64, cc *callcontext.CallContext) (*CallSetupResult, error) {
			return &CallSetupResult{AssistantID: assistantID, ConversationID: conversationID}, nil
		},
		OnCallStart: func(ctx context.Context, session *sip_infra.Session, setup *CallSetupResult, vaultCred interface{}, sipConfig *sip_infra.Config, direction string) error {
			startCount.Add(1)
			session.End()
			return nil
		},
		OnCallEnd: func(callID string) {
			endCount.Add(1)
			endedWG.Done()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	enqueueDone := make(chan struct{})
	go func() {
		for i := 0; i < 2000; i++ {
			d.OnPipeline(ctx, sip_infra.EventEmittedPipeline{
				ID:    fmt.Sprintf("evt-%d", i),
				Event: "tick",
				Data:  map[string]string{"i": fmt.Sprintf("%d", i)},
			})
		}
		close(enqueueDone)
	}()

	select {
	case <-enqueueDone:
	case <-time.After(10 * time.Second):
		t.Fatal("event enqueue blocked under control-channel pressure")
	}

	for i := 0; i < calls; i++ {
		s := newPipelineTestSession(t)
		d.OnPipeline(ctx, sip_infra.SessionEstablishedPipeline{
			ID:             fmt.Sprintf("call-%d", i),
			Session:        s,
			Direction:      sip_infra.CallDirectionInbound,
			AssistantID:    1,
			ConversationID: uint64(i + 1),
		})
	}

	if waitTimeout(&endedWG, 10*time.Second) {
		t.Fatalf("timed out waiting for call teardown completion (started=%d ended=%d)", startCount.Load(), endCount.Load())
	}

	require.Equal(t, int32(calls), startCount.Load())
	require.Equal(t, int32(calls), endCount.Load())
}
