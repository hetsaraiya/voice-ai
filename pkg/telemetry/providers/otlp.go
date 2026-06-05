// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package providers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/rapidaai/pkg/telemetry"
)

// OTLPExporter converts EventRecord and MetricRecord to OTEL spans and ships
// them to any OTLP-compatible backend via the configured endpoint.
type OTLPExporter struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	once     sync.Once
}

// NewOTLPExporter creates an OTLPExporter connected to the given OTLP endpoint.
func NewOTLPExporter(ctx context.Context, cfg OTLPConfig) (*OTLPExporter, error) {
	spanExporter, err := newOTLPSpanExporter(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("telemetry/otlp: create span exporter: %w", err)
	}

	res, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("rapida-voice-assistant"),
			semconv.ServiceVersion("1.0"),
			semconv.TelemetrySDKLanguageGo,
		),
	)

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
	)

	return &OTLPExporter{
		provider: provider,
		tracer:   provider.Tracer("rapida.voice"),
	}, nil
}

func (e *OTLPExporter) ExportEvent(ctx context.Context, meta telemetry.SessionMeta, rec telemetry.EventRecord) error {
	t := rec.Time
	if t.IsZero() {
		t = time.Now()
	}
	attrs := []attribute.KeyValue{
		attribute.Int64("rapida.assistant.id", int64(meta.AssistantID)),
		attribute.Int64("rapida.conversation.id", int64(meta.AssistantConversationID)),
		attribute.Int64("rapida.project.id", int64(meta.ProjectID)),
		attribute.Int64("rapida.organization.id", int64(meta.OrganizationID)),
		attribute.String("rapida.event.message_id", rec.MessageID),
		attribute.String("rapida.event.name", rec.Name),
	}
	for k, v := range rec.Data {
		attrs = append(attrs, attribute.String("rapida.event.data."+k, v))
	}
	_, span := e.tracer.Start(ctx, "rapida.voice.event."+rec.Name,
		trace.WithTimestamp(t),
		trace.WithAttributes(attrs...),
	)
	span.End(trace.WithTimestamp(t))
	return nil
}

func (e *OTLPExporter) ExportMetric(ctx context.Context, meta telemetry.SessionMeta, rec telemetry.MetricRecord) error {
	base := []attribute.KeyValue{
		attribute.Int64("rapida.assistant.id", int64(meta.AssistantID)),
		attribute.Int64("rapida.conversation.id", int64(meta.AssistantConversationID)),
		attribute.Int64("rapida.project.id", int64(meta.ProjectID)),
		attribute.Int64("rapida.organization.id", int64(meta.OrganizationID)),
	}
	switch m := rec.(type) {
	case telemetry.ConversationMetricRecord:
		t := m.Time
		if t.IsZero() {
			t = time.Now()
		}
		attrs := append(base, attribute.String("rapida.metric.conversation_id", m.ConversationID))
		for _, metric := range m.Metrics {
			attrs = append(attrs, attribute.String("rapida.metric."+metric.GetName(), metric.GetValue()))
		}
		_, span := e.tracer.Start(ctx, "rapida.voice.metric.conversation",
			trace.WithTimestamp(t),
			trace.WithAttributes(attrs...),
		)
		span.End(trace.WithTimestamp(t))
	case telemetry.MessageMetricRecord:
		t := m.Time
		if t.IsZero() {
			t = time.Now()
		}
		attrs := append(base,
			attribute.String("rapida.metric.message_id", m.MessageID),
			attribute.String("rapida.metric.conversation_id", m.ConversationID),
		)
		for _, metric := range m.Metrics {
			attrs = append(attrs, attribute.String("rapida.metric."+metric.GetName(), metric.GetValue()))
		}
		_, span := e.tracer.Start(ctx, "rapida.voice.metric.message",
			trace.WithTimestamp(t),
			trace.WithAttributes(attrs...),
		)
		span.End(trace.WithTimestamp(t))
	}
	return nil
}

// Close flushes the batch processor and releases OTLP resources.
func (e *OTLPExporter) Close(ctx context.Context) error {
	var err error
	e.once.Do(func() {
		if ferr := e.provider.ForceFlush(ctx); ferr != nil {
			err = ferr
		}
		if serr := e.provider.Close(ctx); serr != nil && err == nil {
			err = serr
		}
	})
	return err
}

func newOTLPSpanExporter(ctx context.Context, cfg OTLPConfig) (sdktrace.SpanExporter, error) {
	headers := parseOTLPHeaders(cfg.Headers)
	switch strings.ToLower(cfg.Protocol) {
	case "grpc":
		opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.Endpoint)}
		if len(headers) > 0 {
			opts = append(opts, otlptracegrpc.WithHeaders(headers))
		}
		if cfg.Insecure {
			opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
			opts = append(opts, otlptracegrpc.WithInsecure()) //nolint:staticcheck
		}
		return otlptracegrpc.New(ctx, opts...)
	default:
		opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.Endpoint)}
		if len(headers) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(headers))
		}
		if cfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		return otlptracehttp.New(ctx, opts...)
	}
}

func parseOTLPHeaders(pairs []string) map[string]string {
	headers := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		if k, v, ok := strings.Cut(pair, "="); ok {
			headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return headers
}
