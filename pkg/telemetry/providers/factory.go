// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package providers

import (
	"context"
	"fmt"

	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/telemetry"
)

// NewExporterFromOptions constructs a telemetry exporter using provider-specific
// typed config parsed from a flat option map.
func NewExporterFromOptions(
	Logger commons.Logger,
	ctx context.Context,
	provider string,
	opts map[string]interface{},
) (telemetry.Exporter, error) {
	switch telemetry.ExporterType(provider) {
	case telemetry.OTLP_HTTP, telemetry.OTLP_GRPC:
		otlpCfg := OTLPConfigFromOptions(opts, provider)
		if otlpCfg.Endpoint == "" {
			return nil, nil
		}
		return NewOTLPExporter(ctx, otlpCfg)
	case telemetry.XRAY:
		cfg, err := XRayConfigFromOptions(opts)
		if err != nil {
			return nil, err
		}
		return NewXRayExporter(ctx, cfg)
	case telemetry.GOOGLE_TRACE:
		cfg, err := GoogleTraceConfigFromOptions(opts)
		if err != nil {
			return nil, err
		}
		return NewGoogleTraceExporter(ctx, cfg)
	case telemetry.AZURE_MONITOR:
		cfg, err := AzureMonitorConfigFromOptions(opts)
		if err != nil {
			return nil, err
		}
		return NewAzureMonitorExporter(ctx, cfg)
	case telemetry.DATADOG:
		cfg, err := DatadogConfigFromOptions(opts)
		if err != nil {
			return nil, err
		}
		return NewDatadogExporter(ctx, cfg)
	case telemetry.LOGGING:
		cfg, err := LoggingConfigFromOptions(opts)
		if err != nil {
			return nil, err
		}
		return NewLoggingExporter(Logger, cfg), nil
	case telemetry.OPENSEARCH:
		return NewOpenSearchExporterFromOptions(ctx, Logger, opts)
	default:
		return nil, fmt.Errorf("telemetry: unknown exporter type %q", provider)
	}
}
