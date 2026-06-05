// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package configs

import (
	"testing"

	"github.com/spf13/viper"
)

func TestTelemetryConfig_Type(t *testing.T) {
	tests := []struct {
		name string
		cfg  TelemetryConfig
		want TelemetryType
	}{
		{"otlp_http", TelemetryConfig{TelemetryType: "otlp_http"}, OTLPHTTP},
		{"otlp_grpc", TelemetryConfig{TelemetryType: "otlp_grpc"}, OTLPGRPC},
		{"opensearch", TelemetryConfig{TelemetryType: "opensearch"}, OPENSEARCH},
		{"logging", TelemetryConfig{TelemetryType: "logging"}, LOGGING},
		{"unknown", TelemetryConfig{TelemetryType: "something_else"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.Type(); got != tt.want {
				t.Fatalf("TelemetryConfig.Type() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTelemetryConfig_FromViperEnvStyle(t *testing.T) {
	v := viper.NewWithOptions(viper.KeyDelimiter("__"))
	v.Set("TELEMETRY__TYPE", "otlp_http")
	v.Set("TELEMETRY__OTLP_HTTP__ENDPOINT", "otel-collector:4318")
	v.Set("TELEMETRY__OTLP_HTTP__PROTOCOL", "http/protobuf")
	v.Set("TELEMETRY__OTLP_HTTP__HEADERS", "Authorization=Bearer test-token")
	v.Set("TELEMETRY__OTLP_HTTP__INSECURE", true)

	type wrapper struct {
		Telemetry *TelemetryConfig `mapstructure:"telemetry"`
	}
	var cfg wrapper
	if err := v.Unmarshal(&cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if cfg.Telemetry == nil {
		t.Fatalf("telemetry config is nil")
	}
	if cfg.Telemetry.Type() != OTLPHTTP {
		t.Fatalf("type = %v, want %v", cfg.Telemetry.Type(), OTLPHTTP)
	}
	if cfg.Telemetry.OTLPHTTP == nil {
		t.Fatalf("otlp_http config is nil")
	}
	if cfg.Telemetry.OTLPHTTP.Endpoint != "otel-collector:4318" {
		t.Fatalf("endpoint = %q, want %q", cfg.Telemetry.OTLPHTTP.Endpoint, "otel-collector:4318")
	}
	if cfg.Telemetry.OTLPHTTP.Headers != "Authorization=Bearer test-token" {
		t.Fatalf("headers = %q, want %q", cfg.Telemetry.OTLPHTTP.Headers, "Authorization=Bearer test-token")
	}
}

func TestTelemetryConfig_ToMap_ByType(t *testing.T) {
	cfg := &TelemetryConfig{
		TelemetryType: "datadog",
		Datadog: &TelemetryDatadogConfig{
			Endpoint: "localhost:4317",
			Protocol: "grpc",
			Headers:  "DD-API-KEY=abc",
			APIKey:   "abc",
			Insecure: true,
		},
	}

	m := cfg.ToMap()
	if m == nil {
		t.Fatalf("ToMap returned nil")
	}
	if m["endpoint"] != "localhost:4317" {
		t.Fatalf("endpoint = %v, want localhost:4317", m["endpoint"])
	}
	if m["api_key"] != "abc" {
		t.Fatalf("api_key = %v, want abc", m["api_key"])
	}
}

func TestTelemetryConfig_ToMap_OpenSearchConnectionConfig(t *testing.T) {
	cfg := &TelemetryConfig{
		TelemetryType: string(OPENSEARCH),
		OpenSearch: &OpenSearchConfig{
			Schema: "http",
			Host:   "opensearch",
		},
	}

	m := cfg.ToMap()
	if m == nil {
		t.Fatalf("ToMap returned nil")
	}
	if m["schema"] != "http" || m["host"] != "opensearch" {
		t.Fatalf("unexpected opensearch map: %+v", m)
	}
}
