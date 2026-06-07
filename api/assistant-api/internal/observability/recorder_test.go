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

	"github.com/rapidaai/protos"
)

type recordingCollector struct {
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

func (c *recordingCollector) Collect(_ context.Context, scope Scope, record Record) error {
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

func (c *blockingCollector) Collect(context.Context, Scope, Record) error {
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

func TestRecorder_RecordMetric_FansOutAndInjectsGlobalScope(t *testing.T) {
	now := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	first := &recordingCollector{}
	second := &recordingCollector{}
	recorder := New(
		WithGlobalScope(GlobalScope{OrganizationID: 7, ProjectID: 8}),
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

	if len(first.metrics) != 1 || len(second.metrics) != 1 {
		t.Fatalf("expected fanout to both collectors, got first=%d second=%d", len(first.metrics), len(second.metrics))
	}
	got := first.metrics[0]
	if got.ID != "metric-1" {
		t.Fatalf("unexpected id: %s", got.ID)
	}
	if !got.OccurredAt.Equal(now) {
		t.Fatalf("unexpected occurred_at: %s", got.OccurredAt)
	}
	if observabilityGlobal := first.scopes[0].GlobalScopeValue(); observabilityGlobal.OrganizationID != 7 || observabilityGlobal.ProjectID != 8 {
		t.Fatalf("unexpected global scope: %+v", observabilityGlobal)
	}
}

func TestRecorder_RecordEvent_UsesExplicitMessageScope(t *testing.T) {
	collector := &recordingCollector{}
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

	if len(collector.events) != 1 {
		t.Fatalf("expected one event, got %d", len(collector.events))
	}
	gotScope := collector.scopes[0]
	if gotScope.AssistantScopeID() != 10 || gotScope.ConversationScopeID() != 20 || gotScope.ContextID() != "msg-1" || gotScope.MessageScopeRole() != MessageRoleUser {
		t.Fatalf("unexpected message scope: assistant=%d conversation=%d context=%q role=%q",
			gotScope.AssistantScopeID(), gotScope.ConversationScopeID(), gotScope.ContextID(), gotScope.MessageScopeRole())
	}
}

func TestRecorder_RecordRequiresScope(t *testing.T) {
	recorder := New(WithCollector(&recordingCollector{}))
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
	first := &recordingCollector{}
	second := &recordingCollector{}
	recorder := New(WithCollectors(first, second))

	err := recorder.Record(context.Background(), AssistantScope{AssistantID: 10}, RecordWebhook{
		ID:      "wh-1",
		Event:   WebhookDispatched,
		Payload: map[string]interface{}{"status": "ok"},
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if len(first.webhooks) != 1 || len(second.webhooks) != 1 {
		t.Fatalf("expected both collectors to receive webhook record, got first=%d second=%d", len(first.webhooks), len(second.webhooks))
	}
}

func TestRecorder_RecordUsage_AllowsMessageScope(t *testing.T) {
	collector := &recordingCollector{}
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
	if len(collector.usage) != 1 {
		t.Fatalf("expected usage record, got %d", len(collector.usage))
	}
}

func TestRecorder_Record_ReturnsBufferFull(t *testing.T) {
	blocked := &blockingCollector{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	recorder := New(WithBuffer(1), WithCollector(blocked))
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

	if err := recorder.Record(context.Background(), scope, record); err != nil {
		t.Fatalf("second record failed: %v", err)
	}
	err := recorder.Record(context.Background(), scope, record)
	if !errors.Is(err, ErrBufferFull) {
		t.Fatalf("expected ErrBufferFull, got %v", err)
	}

	close(blocked.release)
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

func TestRecorder_Close_JoinsCollectorErrors(t *testing.T) {
	collectErr := errors.New("collect failed")
	closeErr := errors.New("close failed")
	recorder := New(WithCollector(&recordingCollector{
		collectErr: collectErr,
		closeErr:   closeErr,
	}))

	err := recorder.Record(context.Background(), ConversationScope{
		AssistantScope: AssistantScope{AssistantID: 10},
		ConversationID: 20,
	}, RecordEvent{
		Component: ComponentConversation,
		Event:     ConversationStarted,
	})
	if err != nil {
		t.Fatalf("record failed: %v", err)
	}

	err = recorder.Close(context.Background())
	if !errors.Is(err, collectErr) || !errors.Is(err, closeErr) {
		t.Fatalf("expected joined collect+close errors, got %v", err)
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
