// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package configs

type TelemetryType string

const (
	OTLPHTTP     TelemetryType = "otlp_http"
	OTLPGRPC     TelemetryType = "otlp_grpc"
	XRAY         TelemetryType = "xray"
	GOOGLETRACE  TelemetryType = "google_trace"
	AZUREMONITOR TelemetryType = "azure_monitor"
	DATADOG      TelemetryType = "datadog"
	OPENSEARCH   TelemetryType = "opensearch"
	LOGGING      TelemetryType = "logging"
)

type TelemetryConfig struct {
	TelemetryType string                        `mapstructure:"type"`
	OTLPHTTP      *TelemetryOTLPProviderConfig  `mapstructure:"otlp_http"`
	OTLPGRPC      *TelemetryOTLPProviderConfig  `mapstructure:"otlp_grpc"`
	XRay          *TelemetryXRayProviderConfig  `mapstructure:"xray"`
	GoogleTrace   *TelemetryCloudProviderConfig `mapstructure:"google_trace"`
	AzureMonitor  *TelemetryCloudProviderConfig `mapstructure:"azure_monitor"`
	Datadog       *TelemetryDatadogConfig       `mapstructure:"datadog"`
	OpenSearch    *OpenSearchConfig             `mapstructure:"opensearch"`
	Logging       *TelemetryLoggingConfig       `mapstructure:"logging"`
}

type TelemetryOTLPProviderConfig struct {
	Endpoint string `mapstructure:"endpoint"`
	Protocol string `mapstructure:"protocol"`
	Headers  string `mapstructure:"headers"`
	Insecure bool   `mapstructure:"insecure"`
}

type TelemetryDatadogConfig struct {
	Endpoint string `mapstructure:"endpoint"`
	Protocol string `mapstructure:"protocol"`
	Headers  string `mapstructure:"headers"`
	APIKey   string `mapstructure:"api_key"`
	Insecure bool   `mapstructure:"insecure"`
}

type TelemetryCloudProviderConfig struct {
	Endpoint    string `mapstructure:"endpoint"`
	Headers     string `mapstructure:"headers"`
	APIKey      string `mapstructure:"api_key"`
	AccessToken string `mapstructure:"access_token"`
	Insecure    bool   `mapstructure:"insecure"`
}

type TelemetryXRayProviderConfig struct {
	Endpoint string `mapstructure:"endpoint"`
	Protocol string `mapstructure:"protocol"`
	Insecure bool   `mapstructure:"insecure"`
	Region   string `mapstructure:"region"`
}

type TelemetryLoggingConfig struct{}

func (cfg *TelemetryConfig) Type() TelemetryType {
	switch cfg.TelemetryType {
	case string(OTLPHTTP):
		return OTLPHTTP
	case string(OTLPGRPC):
		return OTLPGRPC
	case string(XRAY):
		return XRAY
	case string(GOOGLETRACE):
		return GOOGLETRACE
	case string(AZUREMONITOR):
		return AZUREMONITOR
	case string(DATADOG):
		return DATADOG
	case string(OPENSEARCH):
		return OPENSEARCH
	case string(LOGGING):
		return LOGGING
	default:
		return ""
	}
}

func (cfg *TelemetryConfig) ToMap() map[string]interface{} {
	if cfg == nil {
		return nil
	}
	switch cfg.Type() {
	case OTLPHTTP:
		if cfg.OTLPHTTP != nil {
			return cfg.OTLPHTTP.ToMap()
		}
	case OTLPGRPC:
		if cfg.OTLPGRPC != nil {
			return cfg.OTLPGRPC.ToMap()
		}
	case XRAY:
		if cfg.XRay != nil {
			return cfg.XRay.ToMap()
		}
	case GOOGLETRACE:
		if cfg.GoogleTrace != nil {
			return cfg.GoogleTrace.ToMap()
		}
	case AZUREMONITOR:
		if cfg.AzureMonitor != nil {
			return cfg.AzureMonitor.ToMap()
		}
	case DATADOG:
		if cfg.Datadog != nil {
			return cfg.Datadog.ToMap()
		}
	case OPENSEARCH:
		if cfg.OpenSearch != nil {
			return cfg.OpenSearch.ToMap()
		}
		return map[string]interface{}{}
	case LOGGING:
		if cfg.Logging != nil {
			return cfg.Logging.ToMap()
		}
		return map[string]interface{}{}
	}
	return nil
}

func (cfg *TelemetryOTLPProviderConfig) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"endpoint": cfg.Endpoint,
		"protocol": cfg.Protocol,
		"headers":  cfg.Headers,
		"insecure": cfg.Insecure,
	}
}

func (cfg *TelemetryDatadogConfig) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"endpoint": cfg.Endpoint,
		"protocol": cfg.Protocol,
		"headers":  cfg.Headers,
		"api_key":  cfg.APIKey,
		"insecure": cfg.Insecure,
	}
}

func (cfg *TelemetryCloudProviderConfig) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"endpoint":     cfg.Endpoint,
		"headers":      cfg.Headers,
		"api_key":      cfg.APIKey,
		"access_token": cfg.AccessToken,
		"insecure":     cfg.Insecure,
	}
}

func (cfg *TelemetryXRayProviderConfig) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"endpoint": cfg.Endpoint,
		"protocol": cfg.Protocol,
		"insecure": cfg.Insecure,
		"region":   cfg.Region,
	}
}

func (cfg *TelemetryLoggingConfig) ToMap() map[string]interface{} {
	return map[string]interface{}{}
}
