// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package notification

import (
	"context"
	"errors"
	"testing"

	"github.com/rapidaai/api/assistant-api/internal/observability"
)

type notifierStub struct {
	notifications []Notification
	err           error
}

func (n *notifierStub) Notify(_ context.Context, notification Notification) error {
	n.notifications = append(n.notifications, notification)
	return n.err
}

func TestCollector_NotifiesSelectedEvent(t *testing.T) {
	notifier := &notifierStub{}
	collector := New(Config{Notifier: notifier})

	err := collector.Collect(context.Background(), observability.RecordEvent{
		CommonRecord: observability.CommonRecord{
			ID: "evt-1",
			Scope: observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: 10},
				ConversationID: 20,
			},
		},
		Component: observability.ComponentCall,
		Event:     observability.CallFailed,
	})
	if err != nil {
		t.Fatalf("CollectEvent returned error: %v", err)
	}
	if len(notifier.notifications) != 1 {
		t.Fatalf("expected one notification, got %d", len(notifier.notifications))
	}
	got := notifier.notifications[0]
	if got.ID != "evt-1" || got.Event != observability.CallFailed {
		t.Fatalf("unexpected notification: %+v", got)
	}
}

func TestCollector_DefaultSelectorSkipsOtherEvents(t *testing.T) {
	notifier := &notifierStub{}
	collector := New(Config{Notifier: notifier})

	err := collector.Collect(context.Background(), observability.RecordEvent{
		Event: observability.CallRinging,
	})
	if err != nil {
		t.Fatalf("CollectEvent returned error: %v", err)
	}
	if len(notifier.notifications) != 0 {
		t.Fatalf("expected no notifications, got %+v", notifier.notifications)
	}
}

func TestCollector_ReturnsNotifierError(t *testing.T) {
	notifyErr := errors.New("notify failed")
	collector := New(Config{Notifier: &notifierStub{err: notifyErr}})

	err := collector.Collect(context.Background(), observability.RecordEvent{
		Event: observability.ErrorRaised,
	})
	if !errors.Is(err, notifyErr) {
		t.Fatalf("expected notifier error, got %v", err)
	}
}
