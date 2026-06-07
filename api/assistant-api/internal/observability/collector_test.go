// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability

import (
	"context"
	"errors"
	"testing"
)

func TestCollectors_CollectMetric_FansOutAndJoinsErrors(t *testing.T) {
	collectorErr := errors.New("collector failed")
	first := &recordingCollector{key: "first"}
	second := &recordingCollector{key: "second", collectErr: collectorErr}
	fanout := NewCollectors(first, nil, second)

	err := fanout.Collect(context.Background(), ConversationScope{
		AssistantScope: AssistantScope{AssistantID: 1},
		ConversationID: 2,
	}, RecordMetric{})
	if !errors.Is(err, collectorErr) {
		t.Fatalf("expected collector error, got %v", err)
	}
	if len(first.metrics) != 1 || len(second.metrics) != 1 {
		t.Fatalf("expected both collectors to receive record, got first=%d second=%d", len(first.metrics), len(second.metrics))
	}
}

func TestCollectors_CloseFansOut(t *testing.T) {
	first := &recordingCollector{key: "first"}
	second := &recordingCollector{key: "second"}
	fanout := NewCollectors(first, second)

	if err := fanout.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !first.closed || !second.closed {
		t.Fatalf("expected both collectors to close, got first=%t second=%t", first.closed, second.closed)
	}
}

func TestNoopCollector(t *testing.T) {
	collector := NewCollectors()
	if err := collector.Collect(context.Background(), ConversationScope{
		AssistantScope: AssistantScope{AssistantID: 1},
		ConversationID: 2,
	}, RecordEvent{}); err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if err := collector.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}
