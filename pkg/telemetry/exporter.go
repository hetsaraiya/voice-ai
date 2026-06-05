// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package telemetry

import "context"

// Exporter writes telemetry records to an external backend.
type Exporter interface {
	ExportEvent(ctx context.Context, meta SessionMeta, rec EventRecord) error
	ExportMetric(ctx context.Context, meta SessionMeta, rec MetricRecord) error
	Close(ctx context.Context) error
}

// ExporterType enumerates supported telemetry exporter backends.
type ExporterType string

const (
	OTLP_HTTP     ExporterType = "otlp_http"
	OTLP_GRPC     ExporterType = "otlp_grpc"
	XRAY          ExporterType = "xray"
	GOOGLE_TRACE  ExporterType = "google_trace"
	AZURE_MONITOR ExporterType = "azure_monitor"
	DATADOG       ExporterType = "datadog"
	OPENSEARCH    ExporterType = "opensearch"
	LOGGING       ExporterType = "logging"
)

func (et ExporterType) String() string {
	return string(et)
}
