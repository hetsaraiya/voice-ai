// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package telemetry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	telemetry "github.com/rapidaai/pkg/telemetry"
	"github.com/rapidaai/protos"
)

type exporterStub struct {
	events  []telemetry.EventRecord
	metrics []telemetry.MetricRecord
	closed  bool
	err     error
}

func (e *exporterStub) ExportEvent(_ context.Context, _ telemetry.SessionMeta, rec telemetry.EventRecord) error {
	e.events = append(e.events, rec)
	return e.err
}

func (e *exporterStub) ExportMetric(_ context.Context, _ telemetry.SessionMeta, rec telemetry.MetricRecord) error {
	e.metrics = append(e.metrics, rec)
	return e.err
}

func (e *exporterStub) Close(context.Context) error {
	e.closed = true
	return e.err
}

func TestNew_ReturnsNoopWithoutExporter(t *testing.T) {
	collector, err := New(context.Background(), Config{})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, ok := collector.(observability.NoopCollector); !ok {
		t.Fatalf("expected noop collector, got %T", collector)
	}
}

func TestCollector_ExportsEventsAndMetrics(t *testing.T) {
	exporter := &exporterStub{}
	collector, err := New(context.Background(), Config{Exporters: exporter})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	err = collector.Collect(context.Background(), observability.RecordEvent{
		CommonRecord: observability.CommonRecord{
			Scope: observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: 10},
				ConversationID: 20,
			},
			OccurredAt: now,
		},
		Component:  observability.ComponentCall,
		Event:      observability.CallRinging,
		Attributes: observability.Attributes{"status": "ringing"},
	})
	if err != nil {
		t.Fatalf("CollectEvent returned error: %v", err)
	}

	err = collector.Collect(context.Background(), observability.RecordMetric{
		CommonRecord: observability.CommonRecord{
			Scope: observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: 10},
				ConversationID: 20,
			},
			OccurredAt: now,
		},
		Metrics: []*protos.Metric{{
			Name:        observability.MetricConversationDuration,
			Value:       "1000",
			Description: "duration",
		}},
	})
	if err != nil {
		t.Fatalf("CollectMetric returned error: %v", err)
	}

	if len(exporter.events) != 1 {
		t.Fatalf("expected one event, got %d", len(exporter.events))
	}
	event := exporter.events[0]
	if event.Name != observability.CallRinging.String() || !event.Time.Equal(now) {
		t.Fatalf("unexpected event record: %+v", event)
	}
	if event.Data["status"] != "ringing" {
		t.Fatalf("unexpected event data: %+v", event.Data)
	}

	if len(exporter.metrics) != 1 {
		t.Fatalf("expected one metric, got %d", len(exporter.metrics))
	}
	metric, ok := exporter.metrics[0].(telemetry.ConversationMetricRecord)
	if !ok {
		t.Fatalf("expected conversation metric record, got %T", exporter.metrics[0])
	}
	if metric.ConversationID != "20" || len(metric.Metrics) != 1 {
		t.Fatalf("unexpected metric record: %+v", metric)
	}
}

func TestCollector_ExportsMessageMetrics(t *testing.T) {
	exporter := &exporterStub{}
	collector, err := New(context.Background(), Config{Exporters: exporter})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	err = collector.Collect(context.Background(), observability.RecordMetric{
		CommonRecord: observability.CommonRecord{
			Scope: observability.MessageScope{
				ConversationScope: observability.ConversationScope{
					AssistantScope: observability.AssistantScope{AssistantID: 10},
					ConversationID: 20,
				},
				MessageID: "user-ctx-1",
				Role:      observability.MessageRoleUser,
			},
		},
		Metrics: []*protos.Metric{{
			Name:  observability.MetricUserTurn,
			Value: "complete",
		}},
	})
	if err != nil {
		t.Fatalf("CollectMetric returned error: %v", err)
	}
	metric, ok := exporter.metrics[0].(telemetry.MessageMetricRecord)
	if !ok {
		t.Fatalf("expected message metric record, got %T", exporter.metrics[0])
	}
	if metric.MessageID != "user-ctx-1" || metric.ConversationID != "20" {
		t.Fatalf("unexpected message metric: %+v", metric)
	}
}

func TestCollector_ReturnsExporterErrors(t *testing.T) {
	exportErr := errors.New("export failed")
	collector, err := New(context.Background(), Config{Exporters: &exporterStub{err: exportErr}})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	err = collector.Collect(context.Background(), observability.RecordEvent{
		Event: observability.CallRinging,
	})
	if !errors.Is(err, exportErr) {
		t.Fatalf("expected export error, got %v", err)
	}
}

func TestCollector_CloseExporter(t *testing.T) {
	exporter := &exporterStub{}
	collector, err := New(context.Background(), Config{Exporters: exporter})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := collector.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !exporter.closed {
		t.Fatal("expected exporter to close")
	}
}

func TestCloneOptions(t *testing.T) {
	options := map[string]interface{}{"endpoint": "localhost:4318"}
	cloned := cloneOptions(options)
	cloned["endpoint"] = "changed"
	if options["endpoint"] != "localhost:4318" {
		t.Fatalf("source options mutated: %+v", options)
	}
}

var _ telemetry.Exporter = (*exporterStub)(nil)
