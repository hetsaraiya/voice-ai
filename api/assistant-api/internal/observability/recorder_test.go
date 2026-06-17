// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/protos"
)

type recordingCollector struct {
	key        string
	contexts   []Context
	scopes     []Scope
	logs       []RecordLog
	events     []RecordEvent
	metrics    []RecordMetric
	metadata   []RecordMetadata
	usage      []RecordUsage
	webhooks   []RecordWebhook
	collectErr error
	closeErr   error
	closed     bool
}

func (c *recordingCollector) Key() string {
	return c.key
}

func (c *recordingCollector) Collect(_ context.Context, scope Scope, observationContext Context, record Record) error {
	c.contexts = append(c.contexts, observationContext)
	c.scopes = append(c.scopes, scope)
	switch typed := record.(type) {
	case RecordLog:
		c.logs = append(c.logs, typed)
	case RecordEvent:
		c.events = append(c.events, typed)
	case RecordMetric:
		c.metrics = append(c.metrics, typed)
	case RecordMetadata:
		c.metadata = append(c.metadata, typed)
	case RecordUsage:
		c.usage = append(c.usage, typed)
	case RecordWebhook:
		c.webhooks = append(c.webhooks, typed)
	}
	return c.collectErr
}

func (c *recordingCollector) Close(context.Context) error {
	c.closed = true
	return c.closeErr
}

type blockingCollector struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *blockingCollector) Key() string {
	return ""
}

func (c *blockingCollector) Collect(context.Context, Scope, Context, Record) error {
	c.once.Do(func() { close(c.started) })
	<-c.release
	return nil
}

func (c *blockingCollector) Close(context.Context) error {
	select {
	case <-c.release:
	default:
		close(c.release)
	}
	return nil
}

type blockingKeyCollector struct {
	key     string
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *blockingKeyCollector) Key() string {
	c.once.Do(func() { close(c.started) })
	<-c.release
	return c.key
}

func (c *blockingKeyCollector) Collect(context.Context, Scope, Context, Record) error {
	return nil
}

func (c *blockingKeyCollector) Close(context.Context) error {
	return nil
}

func waitForRecorderDone(t *testing.T, observabilityRecorder Recorder) error {
	t.Helper()
	concreteRecorder, ok := observabilityRecorder.(*recorder)
	if !ok {
		t.Fatalf("unexpected recorder type: %T", observabilityRecorder)
	}
	select {
	case <-concreteRecorder.done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for recorder close")
	}
	return concreteRecorder.errors()
}

func TestRecorder_RecordMetric_FansOutAndInjectsGlobalScope(t *testing.T) {
	now := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	first := &recordingCollector{key: "first"}
	second := &recordingCollector{key: "second"}
	recorder := New(
		WithGlobalScope(GlobalScope{OrganizationID: 7, ProjectID: 8}),
		WithContext(context.WithValue(context.Background(), types.REQUEST_ID_KEY, "trace-1")),
		WithClock(func() time.Time { return now }),
		WithCollectors(first, second),
	)
	scope := ConversationScope{
		AssistantScope: AssistantScope{AssistantID: 10},
		ConversationID: 20,
	}

	err := recorder.Record(context.Background(), scope, RecordMetric{
		ID:      "metric-1",
		Metrics: []*protos.Metric{{Name: MetricConversationDuration, Value: "1000"}},
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}

	if len(first.metrics) != 1 || len(second.metrics) != 1 {
		t.Fatalf("expected fanout to both collectors, got first=%d second=%d", len(first.metrics), len(second.metrics))
	}
	got := first.metrics[0]
	if got.ID != "metric-1" {
		t.Fatalf("unexpected id: %s", got.ID)
	}
	if first.contexts[0].TraceID != "trace-1" {
		t.Fatalf("unexpected trace id: %s", first.contexts[0].TraceID)
	}
	if !got.OccurredAt.Equal(now) {
		t.Fatalf("unexpected occurred_at: %s", got.OccurredAt)
	}
	if observabilityGlobal := first.scopes[0].GlobalScopeValue(); observabilityGlobal.OrganizationID != 7 || observabilityGlobal.ProjectID != 8 {
		t.Fatalf("unexpected global scope: %+v", observabilityGlobal)
	}
}

func TestRecorder_RecordInjectsAuthIntoObservationContext(t *testing.T) {
	organizationID := uint64(7)
	projectID := uint64(8)
	userID := uint64(9)
	auth := &types.ServiceScope{
		UserId:         &userID,
		OrganizationId: &organizationID,
		ProjectId:      &projectID,
	}
	collector := &recordingCollector{key: "collector"}
	recorder := New(
		WithAuth(auth),
		WithCollector(collector),
	)

	err := recorder.Record(context.Background(), ConversationScope{
		AssistantScope: AssistantScope{AssistantID: 10},
		ConversationID: 20,
	}, RecordMetric{
		Metrics: []*protos.Metric{{Name: MetricConversationStatus, Value: "FAILED"}},
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}

	if len(collector.contexts) != 1 || collector.contexts[0].Auth != auth {
		t.Fatalf("expected recorder auth in observation context, got %+v", collector.contexts)
	}
	if observabilityGlobal := collector.scopes[0].GlobalScopeValue(); observabilityGlobal.OrganizationID != organizationID || observabilityGlobal.ProjectID != projectID {
		t.Fatalf("unexpected global scope: %+v", observabilityGlobal)
	}
}

func TestRecorder_RecordUsesRequestIDFromRecordContext(t *testing.T) {
	collector := &recordingCollector{key: "collector"}
	recorder := New(
		WithGlobalScope(GlobalScope{OrganizationID: 7, ProjectID: 8}),
		WithCollector(collector),
	)

	err := recorder.Record(
		context.WithValue(context.Background(), types.REQUEST_ID_KEY, "record-trace"),
		ProjectScope{},
		RecordLog{Message: "hello"},
	)
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}

	if collector.contexts[0].TraceID != "record-trace" {
		t.Fatalf("unexpected trace id: %s", collector.contexts[0].TraceID)
	}
}

func TestRecorder_RecordUsesCanceledContextValues(t *testing.T) {
	collector := &recordingCollector{key: "collector"}
	recorder := New(
		WithGlobalScope(GlobalScope{OrganizationID: 7, ProjectID: 8}),
		WithCollector(collector),
	)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), types.REQUEST_ID_KEY, "canceled-trace"))
	cancel()

	err := recorder.Record(ctx, ProjectScope{}, RecordLog{Message: "client disconnected"})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}

	if len(collector.logs) != 1 {
		t.Fatalf("expected canceled-context record to be collected, got %d", len(collector.logs))
	}
	if collector.contexts[0].TraceID != "canceled-trace" {
		t.Fatalf("unexpected trace id: %s", collector.contexts[0].TraceID)
	}
}

func TestRecorder_RecordFallsBackToGeneratedTraceID(t *testing.T) {
	collector := &recordingCollector{key: "collector"}
	recorder := New(
		WithGlobalScope(GlobalScope{OrganizationID: 7, ProjectID: 8}),
		WithCollector(collector),
	)

	err := recorder.Record(context.Background(), ProjectScope{}, RecordLog{Message: "hello"})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}

	if collector.contexts[0].TraceID == "" {
		t.Fatal("expected generated trace id")
	}
}

func TestRecorder_RecordLog_ProjectScope(t *testing.T) {
	now := time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC)
	collector := &recordingCollector{key: "collector"}
	recorder := New(
		WithGlobalScope(GlobalScope{OrganizationID: 7, ProjectID: 8}),
		WithClock(func() time.Time { return now }),
		WithCollector(collector),
	)

	err := recorder.Record(context.Background(), ProjectScope{}, RecordLog{
		ID:      "log-1",
		Level:   LevelError,
		Message: "webrtc constructor failed",
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}

	if len(collector.logs) != 1 {
		t.Fatalf("expected project log, got %d", len(collector.logs))
	}
	if collector.scopes[0].ScopeType() != ScopeProject {
		t.Fatalf("unexpected scope type: %s", collector.scopes[0].ScopeType())
	}
	global := collector.scopes[0].GlobalScopeValue()
	if global.OrganizationID != 7 || global.ProjectID != 8 {
		t.Fatalf("unexpected global scope: %+v", global)
	}
	projectScope, ok := collector.scopes[0].(ProjectScope)
	if !ok {
		t.Fatalf("unexpected scope type: %T", collector.scopes[0])
	}
	if projectScope.ContextID() != "8" {
		t.Fatalf("unexpected context id: %s", projectScope.ContextID())
	}
	if !collector.logs[0].OccurredAt.Equal(now) {
		t.Fatalf("unexpected occurred_at: %s", collector.logs[0].OccurredAt)
	}
}

func TestRecorder_RecordEvent_UsesExplicitMessageScope(t *testing.T) {
	collector := &recordingCollector{key: "collector"}
	recorder := New(WithCollector(collector))
	scope := MessageScope{
		ConversationScope: ConversationScope{
			AssistantScope: AssistantScope{AssistantID: 10},
			ConversationID: 20,
		},
		MessageID: "msg-1",
		Role:      MessageRoleUser,
	}

	err := recorder.Record(context.Background(), scope, NewMessageEventRecord(
		"msg-1",
		MessageRoleUser,
		EOSCompleted,
		Attributes{"speech": "hello"},
	))
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}

	if len(collector.events) != 1 {
		t.Fatalf("expected one event, got %d", len(collector.events))
	}
	gotScope := collector.scopes[0]
	messageScope, ok := gotScope.(MessageScope)
	if !ok {
		t.Fatalf("unexpected scope type: %T", gotScope)
	}
	if messageScope.AssistantScopeID() != 10 || messageScope.ConversationScopeID() != 20 || messageScope.ContextID() != "msg-1" || messageScope.MessageScopeRole() != MessageRoleUser {
		t.Fatalf("unexpected message scope: assistant=%d conversation=%d context=%q role=%q",
			messageScope.AssistantScopeID(), messageScope.ConversationScopeID(), messageScope.ContextID(), messageScope.MessageScopeRole())
	}
}

func TestRecorder_RecordRequiresScope(t *testing.T) {
	recorder := New(WithCollector(&recordingCollector{key: "collector"}))
	defer recorder.Close(context.Background())

	err := recorder.Record(context.Background(), nil, NewMessageEventRecord(
		"msg-1",
		MessageRoleUser,
		EOSCompleted,
		nil,
	))
	if err == nil {
		t.Fatal("expected missing scope error")
	}
}

func TestRecorder_RecordWebhook_FansOut(t *testing.T) {
	first := &recordingCollector{key: "first"}
	second := &recordingCollector{key: "second"}
	recorder := New(WithCollectors(first, second))

	err := recorder.Record(context.Background(), AssistantScope{AssistantID: 10}, RecordWebhook{
		ID:      "wh-1",
		Event:   CallReceived,
		Payload: map[string]interface{}{"status": "ok"},
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}
	if len(first.webhooks) != 1 || len(second.webhooks) != 1 {
		t.Fatalf("expected both collectors to receive webhook record, got first=%d second=%d", len(first.webhooks), len(second.webhooks))
	}
}

func TestRecorder_RecordUsage_AllowsMessageScope(t *testing.T) {
	collector := &recordingCollector{key: "collector"}
	recorder := New(WithCollector(collector))
	defer recorder.Close(context.Background())

	err := recorder.Record(context.Background(), MessageScope{
		ConversationScope: ConversationScope{
			AssistantScope: AssistantScope{AssistantID: 10},
			ConversationID: 20,
		},
		MessageID: "user-ctx-1",
		Role:      MessageRoleUser,
	}, RecordUsage{
		Component: ComponentUsage,
		Duration:  time.Second,
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}
	if len(collector.usage) != 1 {
		t.Fatalf("expected usage record, got %d", len(collector.usage))
	}
}

func TestRecorder_RecordVariadicRecords_AllCollected(t *testing.T) {
	collector := &recordingCollector{key: "collector"}
	recorder := New(WithCollector(collector))
	scope := ConversationScope{
		AssistantScope: AssistantScope{AssistantID: 10},
		ConversationID: 20,
	}

	err := recorder.Record(context.Background(), scope,
		RecordEvent{
			Component: ComponentConversation,
			Event:     ConversationInitializing,
		},
		RecordMetric{
			Metrics: []*protos.Metric{{Name: MetricCallStatus, Value: "started"}},
		},
		RecordMetadata{
			Metadata: []*protos.Metadata{{Key: MetadataCallStatus, Value: "started"}},
		},
	)
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}

	if len(collector.events) != 1 {
		t.Fatalf("expected event to be collected, got %d", len(collector.events))
	}
	if len(collector.metrics) != 1 {
		t.Fatalf("expected metric to be collected, got %d", len(collector.metrics))
	}
	if len(collector.metadata) != 1 {
		t.Fatalf("expected metadata to be collected, got %d", len(collector.metadata))
	}
}

func TestRecorder_NewDoesNotWaitForCollectorRegistration(t *testing.T) {
	collector := &blockingKeyCollector{
		key:     "collector",
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	startedAt := time.Now()
	recorder := New(WithCollector(collector))
	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("New waited for collector registration: %s", elapsed)
	}

	select {
	case <-collector.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for collector registration")
	}
	close(collector.release)
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}
}

func TestRecorder_AddCollectorsDoesNotWaitForCollectorRegistration(t *testing.T) {
	recorder := New()
	collector := &blockingKeyCollector{
		key:     "collector",
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	startedAt := time.Now()
	if err := recorder.AddCollectors(collector); err != nil {
		t.Fatalf("AddCollectors returned error: %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("AddCollectors waited for collector registration: %s", elapsed)
	}

	select {
	case <-collector.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for collector registration")
	}
	close(collector.release)
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}
}

func TestRecorder_RecordDoesNotWaitForCollectorCollect(t *testing.T) {
	blocked := &blockingCollector{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	recorder := New(WithCollector(blocked))

	startedAt := time.Now()
	if err := recorder.Record(context.Background(), ConversationScope{
		AssistantScope: AssistantScope{AssistantID: 10},
		ConversationID: 20,
	}, RecordEvent{
		Component: ComponentConversation,
		Event:     ConversationInitializing,
	}); err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("Record waited for collector collect: %s", elapsed)
	}

	select {
	case <-blocked.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for collector collect")
	}
	close(blocked.release)
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}
}

func TestRecorder_CloseGraceAcceptsLateRecords(t *testing.T) {
	collector := &recordingCollector{key: "collector"}
	observabilityRecorder := New(WithGracePeriod(), WithCollector(collector))
	concreteRecorder, ok := observabilityRecorder.(*recorder)
	if !ok {
		t.Fatalf("unexpected recorder type: %T", observabilityRecorder)
	}
	concreteRecorder.closeGracePeriod = 200 * time.Millisecond
	scope := ConversationScope{
		AssistantScope: AssistantScope{AssistantID: 10},
		ConversationID: 20,
	}

	closeStarted := time.Now()
	closeContext, cancelCloseContext := context.WithCancel(context.Background())
	cancelCloseContext()
	if err := observabilityRecorder.Close(closeContext); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if elapsed := time.Since(closeStarted); elapsed > 100*time.Millisecond {
		t.Fatalf("Close waited for grace period: %s", elapsed)
	}
	time.Sleep(10 * time.Millisecond)

	if err := observabilityRecorder.Record(context.Background(), scope, RecordEvent{
		Component: ComponentConversation,
		Event:     ConversationCleanup,
	}); err != nil {
		t.Fatalf("late record during close grace failed: %v", err)
	}
	if err := waitForRecorderDone(t, observabilityRecorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}

	if len(collector.events) != 1 {
		t.Fatalf("expected late event to be collected, got %d", len(collector.events))
	}
	if err := observabilityRecorder.Record(context.Background(), scope, RecordEvent{
		Component: ComponentConversation,
		Event:     ConversationCompleted,
	}); !errors.Is(err, ErrRecorderClosed) {
		t.Fatalf("expected ErrRecorderClosed after close, got %v", err)
	}
}

func TestRecorder_DefaultGracePeriodIsZero(t *testing.T) {
	observabilityRecorder := New()
	concreteRecorder, ok := observabilityRecorder.(*recorder)
	if !ok {
		t.Fatalf("unexpected recorder type: %T", observabilityRecorder)
	}
	if concreteRecorder.closeGracePeriod != 0 {
		t.Fatalf("unexpected default grace period: %s", concreteRecorder.closeGracePeriod)
	}
	if err := concreteRecorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, observabilityRecorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}
}

func TestRecorder_WithGracePeriodUsesConfiguredGracePeriod(t *testing.T) {
	observabilityRecorder := New(WithGracePeriod())
	concreteRecorder, ok := observabilityRecorder.(*recorder)
	if !ok {
		t.Fatalf("unexpected recorder type: %T", observabilityRecorder)
	}
	if concreteRecorder.closeGracePeriod != recorderCloseGracePeriod {
		t.Fatalf("unexpected grace period: %s", concreteRecorder.closeGracePeriod)
	}
	concreteRecorder.closeGracePeriod = 0
	if err := concreteRecorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, observabilityRecorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}
}

func TestRecorder_Record_ReturnsBufferFullWhenOperationQueueIsFull(t *testing.T) {
	blocked := &blockingCollector{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	recorder := New(WithCollector(blocked))
	scope := ConversationScope{
		AssistantScope: AssistantScope{AssistantID: 10},
		ConversationID: 20,
	}
	record := RecordMetric{
		Metrics: []*protos.Metric{{Name: MetricConversationDuration, Value: "1"}},
	}

	if err := recorder.Record(context.Background(), scope, record); err != nil {
		t.Fatalf("first record failed: %v", err)
	}
	<-blocked.started

	for recordIndex := 0; recordIndex < recorderQueueSize; recordIndex++ {
		if err := recorder.Record(context.Background(), scope, record); err != nil {
			t.Fatalf("queued record %d failed: %v", recordIndex, err)
		}
	}
	err := recorder.Record(context.Background(), scope, record)
	if !errors.Is(err, ErrBufferFull) {
		t.Fatalf("expected ErrBufferFull, got %v", err)
	}

	close(blocked.release)
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}
}

func TestRecorder_CloseDoesNotWaitForInflightCollect(t *testing.T) {
	blocked := &blockingCollector{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	recorder := New(WithCollector(blocked))

	if err := recorder.Record(context.Background(), ConversationScope{
		AssistantScope: AssistantScope{AssistantID: 10},
		ConversationID: 20,
	}, RecordEvent{
		Component: ComponentConversation,
		Event:     ConversationInitializing,
	}); err != nil {
		t.Fatalf("record failed: %v", err)
	}
	<-blocked.started

	closeStarted := time.Now()
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if elapsed := time.Since(closeStarted); elapsed > 100*time.Millisecond {
		t.Fatalf("Close waited for inflight collect: %s", elapsed)
	}

	close(blocked.release)
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}
}

func TestRecorder_Close_JoinsCollectorErrors(t *testing.T) {
	collectErr := errors.New("collect failed")
	closeErr := errors.New("close failed")
	recorder := New(WithCollector(&recordingCollector{
		key:        "collector",
		collectErr: collectErr,
		closeErr:   closeErr,
	}))

	err := recorder.Record(context.Background(), ConversationScope{
		AssistantScope: AssistantScope{AssistantID: 10},
		ConversationID: 20,
	}, RecordEvent{
		Component: ComponentConversation,
		Event:     ConversationInitializing,
	})
	if err != nil {
		t.Fatalf("record failed: %v", err)
	}

	err = recorder.Close(context.Background())
	if err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	err = waitForRecorderDone(t, recorder)
	if !errors.Is(err, collectErr) || !errors.Is(err, closeErr) {
		t.Fatalf("expected joined collect+close errors, got %v", err)
	}
}

func TestRecorder_AddCollectors_DeduplicatesByKey(t *testing.T) {
	first := &recordingCollector{key: "telemetry"}
	duplicate := &recordingCollector{key: "telemetry"}
	second := &recordingCollector{key: "timeline"}
	recorder := New(WithCollector(first))

	if err := recorder.AddCollectors(duplicate, second); err != nil {
		t.Fatalf("AddCollectors returned error: %v", err)
	}
	err := recorder.Record(context.Background(), ConversationScope{
		AssistantScope: AssistantScope{AssistantID: 10},
		ConversationID: 20,
	}, RecordEvent{
		Component: ComponentConversation,
		Event:     ConversationInitializing,
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}
	if len(first.events) != 1 || len(second.events) != 1 {
		t.Fatalf("expected original and new collectors to receive event, got first=%d second=%d", len(first.events), len(second.events))
	}
	if len(duplicate.events) != 0 {
		t.Fatalf("expected duplicate collector to be skipped, got %d events", len(duplicate.events))
	}
}

func TestRecorder_AddCollectors_ReturnsClosedError(t *testing.T) {
	recorder := New()
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := waitForRecorderDone(t, recorder); err != nil {
		t.Fatalf("recorder close returned error: %v", err)
	}
	if err := recorder.AddCollectors(&recordingCollector{key: "collector"}); !errors.Is(err, ErrRecorderClosed) {
		t.Fatalf("expected ErrRecorderClosed, got %v", err)
	}
}

func TestValidateScope(t *testing.T) {
	if err := ValidateScope(AssistantScope{}); err == nil {
		t.Fatal("expected assistant scope error")
	}
	if err := ValidateScope(ConversationScope{AssistantScope: AssistantScope{AssistantID: 10}}); err == nil {
		t.Fatal("expected conversation scope error")
	}
	if err := ValidateScope(MessageScope{
		ConversationScope: ConversationScope{
			AssistantScope: AssistantScope{AssistantID: 10},
			ConversationID: 20,
		},
		MessageID: "msg-1",
		Role:      MessageRoleUser,
	}); err != nil {
		t.Fatalf("expected valid message scope, got %v", err)
	}
}
