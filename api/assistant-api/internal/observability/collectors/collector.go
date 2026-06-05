// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package collectors

import (
	"context"

	assistant_config "github.com/rapidaai/api/assistant-api/config"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_telemetry_entity "github.com/rapidaai/api/assistant-api/internal/entity/telemetry"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/api/assistant-api/internal/observability/collectors/telemetry"
	"github.com/rapidaai/api/assistant-api/internal/observability/collectors/timeline"
	"github.com/rapidaai/api/assistant-api/internal/observability/collectors/webhook"
	"github.com/rapidaai/pkg/commons"
)

func NewWithEnv(ctx context.Context, logger commons.Logger, config *assistant_config.AssistantConfig) []observability.Collector {
	collectors := make([]observability.Collector, 0)
	if config == nil {
		return collectors
	}

	if config.TelemetryConfig != nil && config.TelemetryConfig.Type() != "" {
		if collector, err := telemetry.New(ctx, telemetry.Config{Logger: logger,
			Providers: telemetry.Provider{
				Name:    string(config.TelemetryConfig.Type()),
				Options: config.TelemetryConfig.ToMap(),
			}}); err == nil {
			collectors = append(collectors, collector)
		}
	}

	if config.ObservabilityConfig != nil && config.ObservabilityConfig.OpenSearch != nil {
		collector, err := timeline.New(ctx, timeline.Config{
			Logger:           logger,
			OpenSearchConfig: config.ObservabilityConfig.OpenSearch,
		})
		if err != nil {
			if logger != nil {
				logger.Warnf("observability: timeline collector initialization failed: %v", err)
			}
		} else if collector != nil {
			collectors = append(collectors, collector)
		}
	}
	return collectors
}

func NewWithAssistantTelemetry(ctx context.Context, logger commons.Logger, providers []*internal_telemetry_entity.AssistantTelemetryProvider) []observability.Collector {
	collectors := make([]observability.Collector, 0)
	for _, assistantTelemetryProvider := range providers {
		if assistantTelemetryProvider == nil || !assistantTelemetryProvider.Enabled {
			continue
		}
		if collector, err := telemetry.New(ctx, telemetry.Config{Logger: logger,
			Providers: telemetry.Provider{
				Name:    assistantTelemetryProvider.ProviderType,
				Options: assistantTelemetryProvider.GetOptions(),
			}}); err == nil {
			collectors = append(collectors, collector)
		}
	}
	return collectors
}

func NewWithAssistantWebhook(logger commons.Logger, webhooks []*internal_assistant_entity.AssistantWebhook) []observability.Collector {
	if len(webhooks) == 0 {
		return nil
	}
	return []observability.Collector{webhook.New(webhook.Config{
		Logger:   logger,
		Webhooks: webhooks,
	})}
}
