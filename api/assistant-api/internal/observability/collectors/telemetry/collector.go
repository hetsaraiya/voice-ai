// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package telemetry

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
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
	logger      commons.Logger
	provider    Provider
	exporter    telemetry.Exporter
	initialized bool
	mu          sync.Mutex
	key         string
}

func New(ctx context.Context, cfg Config) (observability.Collector, error) {
	key := "telemetry"
	if providerName := strings.TrimSpace(cfg.Providers.Name); validator.NotBlank(providerName) {
		key = "telemetry:" + providerName
	}
	if validator.NonNil(cfg.Exporters) {
		return &Collector{exporter: cfg.Exporters, initialized: true, key: key}, nil
	}
	providerName := strings.TrimSpace(cfg.Providers.Name)
	if !validator.NotBlank(providerName) {
		return observability.NoopCollector{}, nil
	}
	switch telemetry.ExporterType(providerName) {
	case telemetry.OTLP_HTTP, telemetry.OTLP_GRPC, telemetry.XRAY, telemetry.GOOGLE_TRACE,
		telemetry.AZURE_MONITOR, telemetry.DATADOG, telemetry.OPENSEARCH, telemetry.LOGGING:
	default:
		return nil, errors.New("telemetry: unknown exporter type " + strconv.Quote(providerName))
	}
	options := make(map[string]interface{}, len(cfg.Providers.Options))
	for key, value := range cfg.Providers.Options {
		options[key] = value
	}
	return &Collector{
		logger: cfg.Logger,
		provider: Provider{
			Name:    providerName,
			Options: options,
		},
		key: key,
	}, nil
}

func (c *Collector) Key() string {
	return c.key
}

func (c *Collector) Collect(ctx context.Context, scope observability.Scope, observationContext observability.Context, record observability.Record) error {
	c.mu.Lock()
	if !c.initialized {
		exporter, err := newExporter(ctx, c.logger, c.provider)
		if err != nil {
			c.mu.Unlock()
			return err
		}
		c.exporter = exporter
		c.initialized = true
	}
	exporter := c.exporter
	c.mu.Unlock()
	if !validator.NonNil(exporter) {
		return nil
	}

	switch typed := record.(type) {
	case observability.RecordLog:
		occurredAt := typed.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = time.Now()
		}
		attributes := make(map[string]string, len(typed.Attributes))
		for key, value := range typed.Attributes {
			attributes[key] = value
		}
		return exporter.Export(ctx, c.toTelemetryScope(scope), telemetry.LogRecord{
			ID:         typed.ID,
			Context:    map[string]string{"traceId": observationContext.TraceID},
			Level:      string(typed.Level),
			Message:    typed.Message,
			Attributes: attributes,
			OccurredAt: occurredAt,
		})
	case observability.RecordEvent:
		occurredAt := typed.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = time.Now()
		}
		attributes := make(map[string]string, len(typed.Attributes))
		for key, value := range typed.Attributes {
			attributes[key] = value
		}
		return exporter.Export(ctx, c.toTelemetryScope(scope), telemetry.EventRecord{
			ID:         typed.ID,
			Context:    map[string]string{"traceId": observationContext.TraceID},
			Event:      typed.Event.String(),
			Component:  typed.Component.String(),
			Attributes: attributes,
			OccurredAt: occurredAt,
		})
	case observability.RecordMetric:
		if !validator.NotEmpty(typed.Metrics) {
			return nil
		}
		occurredAt := typed.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = time.Now()
		}
		attributes := make(map[string]string, len(typed.Attributes))
		for key, value := range typed.Attributes {
			attributes[key] = value
		}
		var errs []error
		for _, metric := range typed.Metrics {
			if metric == nil {
				continue
			}
			if err := exporter.Export(ctx, c.toTelemetryScope(scope), telemetry.MetricRecord{
				ID:          typed.ID,
				Context:     map[string]string{"traceId": observationContext.TraceID},
				Name:        metric.GetName(),
				Value:       metric.GetValue(),
				Description: metric.GetDescription(),
				Attributes:  attributes,
				OccurredAt:  occurredAt,
			}); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	default:
		return nil
	}
}

func (c *Collector) toTelemetryScope(scope observability.Scope) telemetry.Scope {
	global := scope.GlobalScopeValue()
	scopeAttributes := map[string]string{}
	switch typed := scope.(type) {
	case observability.AssistantScope:
		scopeAttributes["assistantId"] = strconv.FormatUint(typed.AssistantScopeID(), 10)
	case observability.ConversationScope:
		scopeAttributes["assistantId"] = strconv.FormatUint(typed.AssistantScopeID(), 10)
		scopeAttributes["assistantConversationId"] = strconv.FormatUint(typed.ConversationScopeID(), 10)
	case observability.MessageScope:
		scopeAttributes["assistantId"] = strconv.FormatUint(typed.AssistantScopeID(), 10)
		scopeAttributes["assistantConversationId"] = strconv.FormatUint(typed.ConversationScopeID(), 10)
		scopeAttributes["messageId"] = typed.ContextID()
		scopeAttributes["messageRole"] = string(typed.MessageScopeRole())
	}
	return telemetry.Scope{
		ProjectID:       global.ProjectID,
		OrganizationID:  global.OrganizationID,
		Name:            string(scope.ScopeType()),
		ScopeAttributes: scopeAttributes,
	}
}

func (c *Collector) Close(ctx context.Context) error {
	var errs []error
	c.mu.Lock()
	if !c.initialized {
		c.initialized = true
		c.mu.Unlock()
		return nil
	}
	exporter := c.exporter
	c.mu.Unlock()
	if validator.NonNil(exporter) {
		if err := exporter.Close(ctx); err != nil {
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
	if len(provider.Options) == 0 {
		return providers.NewExporterFromOptions(logger, ctx, providerName, nil)
	}
	options := make(map[string]interface{}, len(provider.Options))
	for key, value := range provider.Options {
		options[key] = value
	}
	return providers.NewExporterFromOptions(logger, ctx, providerName, options)
}
