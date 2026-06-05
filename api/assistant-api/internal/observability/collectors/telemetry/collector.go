// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package telemetry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/pkg/commons"
	telemetry "github.com/rapidaai/pkg/telemetry"
	"github.com/rapidaai/pkg/telemetry/providers"
	"github.com/rapidaai/pkg/validator"
)

type Provider struct {
	Name    string
	Options map[string]interface{}
}

type Config struct {
	Logger    commons.Logger
	Providers Provider
	Exporters telemetry.Exporter
}

type Collector struct {
	exporter telemetry.Exporter
}

func New(ctx context.Context, cfg Config) (observability.Collector, error) {
	if validator.NonNil(cfg.Exporters) {
		return &Collector{exporter: cfg.Exporters}, nil
	}

	exporter, err := newExporter(ctx, cfg.Logger, cfg.Providers)
	if err != nil {
		return nil, err
	}
	if !validator.NonNil(exporter) {
		return observability.NoopCollector{}, nil
	}
	return &Collector{exporter: exporter}, nil
}

func (c *Collector) Collect(ctx context.Context, record observability.Record) error {
	if !validator.NonNil(c.exporter) {
		return nil
	}
	switch typed := record.(type) {
	case observability.RecordEvent:
		meta := sessionMeta(typed.Scope.GlobalScopeValue(), typed.Scope)
		return c.exportEvent(ctx, meta, typed)
	case observability.RecordMetric:
		meta := sessionMeta(typed.Scope.GlobalScopeValue(), typed.Scope)
		return c.exportMetric(ctx, meta, typed)
	default:
		return nil
	}
}

func (c *Collector) Close(ctx context.Context) error {
	var errs []error
	if validator.NonNil(c.exporter) {
		if err := c.exporter.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func newExporter(ctx context.Context, logger commons.Logger, provider Provider) (telemetry.Exporter, error) {
	providerName := strings.TrimSpace(provider.Name)
	if !validator.NotBlank(providerName) {
		return nil, nil
	}
	return providers.NewExporterFromOptions(logger, ctx, providerName, cloneOptions(provider.Options))
}

func (c *Collector) exportEvent(ctx context.Context, meta telemetry.SessionMeta, record observability.RecordEvent) error {
	rec := telemetry.EventRecord{
		ConversationID: record.Scope.ConversationScopeID(),
		MessageID:      messageID(record.Scope),
		Name:           record.Event.String(),
		Data:           eventData(record.Attributes),
		Time:           occurredAt(record.OccurredAt),
	}

	var errs []error
	if validator.NonNil(c.exporter) {
		if err := c.exporter.ExportEvent(ctx, meta, rec); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *Collector) exportMetric(ctx context.Context, meta telemetry.SessionMeta, record observability.RecordMetric) error {
	if !validator.NotEmpty(record.Metrics) {
		return nil
	}

	rec := newMetricRecord(record)
	var errs []error
	if validator.NonNil(c.exporter) {
		if err := c.exporter.ExportMetric(ctx, meta, rec); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func sessionMeta(global observability.GlobalScope, scope observability.Scope) telemetry.SessionMeta {
	return telemetry.SessionMeta{
		AssistantID:             scope.AssistantScopeID(),
		AssistantConversationID: scope.ConversationScopeID(),
		ProjectID:               global.ProjectID,
		OrganizationID:          global.OrganizationID,
	}
}

func newMetricRecord(record observability.RecordMetric) telemetry.MetricRecord {
	conversationID := fmt.Sprintf("%d", record.Scope.ConversationScopeID())
	if record.Scope.ScopeType() == observability.ScopeMessage {
		return telemetry.MessageMetricRecord{
			MessageID:      record.Scope.MessageScopeID(),
			ConversationID: conversationID,
			Metrics:        record.Metrics,
			Time:           occurredAt(record.OccurredAt),
		}
	}
	return telemetry.ConversationMetricRecord{
		ConversationID: conversationID,
		Metrics:        record.Metrics,
		Time:           occurredAt(record.OccurredAt),
	}
}

func eventData(attributes observability.Attributes) map[string]string {
	data := make(map[string]string, len(attributes))
	for key, value := range attributes {
		data[key] = value
	}
	return data
}

func messageID(scope observability.Scope) string {
	return scope.ContextID()
}

func occurredAt(at time.Time) time.Time {
	if !at.IsZero() {
		return at
	}
	return time.Now()
}

func cloneOptions(options map[string]interface{}) map[string]interface{} {
	if len(options) == 0 {
		return nil
	}
	cloned := make(map[string]interface{}, len(options))
	for key, value := range options {
		cloned[key] = value
	}
	return cloned
}
