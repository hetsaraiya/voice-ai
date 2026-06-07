// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observe

import (
	"context"

	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/protos"
)

// ConversationObserver provides unified observability for a conversation's lifecycle:
// DB persistence (via ConversationPersister) and telemetry export (via collectors).
type ConversationObserver struct {
	logger  commons.Logger
	meta    ConversationMeta
	auth    types.SimplePrinciple
	events  EventCollector
	metrics MetricCollector
}

type ConversationMeta struct {
	AssistantID             uint64
	AssistantConversationID uint64
	ProjectID               uint64
	OrganizationID          uint64
}

// ConversationObserverConfig holds the dependencies for creating a ConversationObserver.
type ConversationObserverConfig struct {
	Logger         commons.Logger
	Auth           types.SimplePrinciple
	AssistantID    uint64
	ConversationID uint64
	ProjectID      uint64
	OrganizationID uint64
	Events         EventCollector
	Metrics        MetricCollector
}

// NewConversationObserver creates a new observer scoped to a conversation.
// The observer is safe for concurrent use from multiple goroutines.
func NewConversationObserver(cfg *ConversationObserverConfig) *ConversationObserver {
	meta := ConversationMeta{
		AssistantID:             cfg.AssistantID,
		AssistantConversationID: cfg.ConversationID,
		ProjectID:               cfg.ProjectID,
		OrganizationID:          cfg.OrganizationID,
	}

	events := cfg.Events
	if events == nil {
		events = NewEventCollector(cfg.Logger, SessionMeta{}) // no-op when no exporters
	}
	metrics := cfg.Metrics
	if metrics == nil {
		metrics = NewMetricCollector(cfg.Logger, SessionMeta{}) // no-op when no exporters
	}

	return &ConversationObserver{
		logger: cfg.Logger,
		meta:   meta,
		auth:   cfg.Auth,
		// persist: cfg.Persist,
		events:  events,
		metrics: metrics,
	}
}

// EmitEvent sends an event to telemetry exporters only (no DB write).
// Events are temporal — they belong in event streams, not the metadata table.
func (o *ConversationObserver) EmitEvent(ctx context.Context, name string, data map[string]string) {
}

// EmitMetric persists metrics to DB and sends to telemetry exporters.
func (o *ConversationObserver) EmitMetric(ctx context.Context, metrics []*protos.Metric) {
	if len(metrics) == 0 {
		return
	}
	for _, metric := range metrics {
		if metric == nil {
			continue
		}

	}
	// if o.persist != nil {
	// 	converted := make([]*types.Metric, 0, len(metrics))
	// 	for _, pm := range metrics {
	// 		converted = append(converted, &types.Metric{
	// 			Name:        pm.Name,
	// 			Value:       pm.Value,
	// 			Description: pm.Description,
	// 		})
	// 	}
	// 	if err := o.persist.PersistMetrics(ctx, o.auth, o.meta.AssistantID, o.meta.AssistantConversationID, converted); err != nil {
	// 		o.logger.Warnw("observer: failed to persist metrics", "error", err)
	// 	}
	// }
}

// EmitMetadata persists metadata to DB only (no telemetry export).
func (o *ConversationObserver) EmitMetadata(ctx context.Context, metadata []*types.Metadata) {
	if len(metadata) == 0 {
		return
	}
	// if o.persist != nil {
	// 	if err := o.persist.PersistMetadata(ctx, o.auth, o.meta.AssistantID, o.meta.AssistantConversationID, metadata); err != nil {
	// 		o.logger.Warnw("observer: failed to persist metadata", "error", err)
	// 	}
	// }
}

// EventCollectors returns the underlying collectors for direct use by
// genericRequestor (which also calls Notify for gRPC streaming).
func (o *ConversationObserver) EventCollectors() EventCollector {
	return o.events
}

// MetricCollectors returns the underlying collectors for direct use.
func (o *ConversationObserver) MetricCollectors() MetricCollector {
	return o.metrics
}

// Shutdown waits for all in-flight exports to complete and shuts down exporters.
func (o *ConversationObserver) Shutdown(ctx context.Context) {
	o.events.Close(ctx)
	o.metrics.Close(ctx)
}

// Meta returns the session metadata for this observer.
func (o *ConversationObserver) Meta() ConversationMeta {
	return o.meta
}

// ServicePersister adapts any service with ApplyConversationMetrics/ApplyConversationMetadata
// (which return a typed result + error) to the ConversationPersister interface (error only).
type ServicePersister struct {
	ApplyMetrics  func(ctx context.Context, auth types.SimplePrinciple, assistantID, conversationID uint64, metrics []*types.Metric) (interface{}, error)
	ApplyMetadata func(ctx context.Context, auth types.SimplePrinciple, assistantID, conversationID uint64, metadata []*types.Metadata) (interface{}, error)
}

func (s *ServicePersister) PersistMetrics(ctx context.Context, auth types.SimplePrinciple, assistantID, conversationID uint64, metrics []*types.Metric) error {
	_, err := s.ApplyMetrics(ctx, auth, assistantID, conversationID, metrics)
	return err
}

func (s *ServicePersister) PersistMetadata(ctx context.Context, auth types.SimplePrinciple, assistantID, conversationID uint64, metadata []*types.Metadata) error {
	_, err := s.ApplyMetadata(ctx, auth, assistantID, conversationID, metadata)
	return err
}
