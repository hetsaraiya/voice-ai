// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rapidaai/api/assistant-api/internal/observability"
)

type usagePublisherStub struct {
	records []Usage
	err     error
}

func (s *usagePublisherStub) PublishUsage(_ context.Context, usage Usage) error {
	s.records = append(s.records, usage)
	return s.err
}

func TestCollector_ForwardsUsageRecord(t *testing.T) {
	publisher := &usagePublisherStub{}
	collector := New(publisher)
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)

	err := collector.Collect(context.Background(), observability.RecordUsage{
		CommonRecord: observability.CommonRecord{
			ID: "usage-1",
			Scope: observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: 10},
				ConversationID: 20,
			},
			OccurredAt: now,
		},
		Component:  observability.ComponentUsage,
		Provider:   "deepgram",
		Duration:   2 * time.Second,
		Attributes: observability.Attributes{"source": "stt"},
	})
	if err != nil {
		t.Fatalf("CollectUsage returned error: %v", err)
	}
	if len(publisher.records) != 1 {
		t.Fatalf("expected one usage record, got %d", len(publisher.records))
	}
	got := publisher.records[0]
	if got.ID != "usage-1" || got.Component != observability.ComponentUsage || got.Provider != "deepgram" {
		t.Fatalf("unexpected usage record: %+v", got)
	}
	if got.Scope.ConversationScopeID() != 20 {
		t.Fatalf("unexpected scope: %+v", got.Scope)
	}
}

func TestCollector_ReturnsPublisherError(t *testing.T) {
	publishErr := errors.New("publish failed")
	collector := New(&usagePublisherStub{err: publishErr})

	err := collector.Collect(context.Background(), observability.RecordUsage{})
	if !errors.Is(err, publishErr) {
		t.Fatalf("expected publish error, got %v", err)
	}
}
