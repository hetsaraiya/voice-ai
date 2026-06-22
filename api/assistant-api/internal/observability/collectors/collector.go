// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package collectors

import (
	"context"
	"strconv"

	assistant_config "github.com/rapidaai/api/assistant-api/config"
	internal_telemetry_entity "github.com/rapidaai/api/assistant-api/internal/entity/telemetry"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/api/assistant-api/internal/observability/collectors/telemetry"
	"github.com/rapidaai/api/assistant-api/internal/observability/collectors/webhook"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
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
	return collectors
}

func NewWithAssistantTelemetry(ctx context.Context, logger commons.Logger, providers []*internal_telemetry_entity.AssistantTelemetryProvider) []observability.Collector {
	collectors := make([]observability.Collector, 0)
	for _, assistantTelemetryProvider := range providers {
		if assistantTelemetryProvider == nil || !assistantTelemetryProvider.Enabled {
			continue
		}
		collectorKey := ""
		if assistantTelemetryProvider.Id != 0 {
			collectorKey = "telemetry:assistant:" + strconv.FormatUint(assistantTelemetryProvider.Id, 10)
		}
		if collector, err := telemetry.New(ctx, telemetry.Config{Logger: logger,
			Providers: telemetry.Provider{
				Name:    assistantTelemetryProvider.ProviderType,
				Options: assistantTelemetryProvider.GetOptions(),
			},
			Key: collectorKey,
		}); err == nil {
			collectors = append(collectors, collector)
		}
	}
	return collectors
}

func NewWithAssistantWebhook(ctx context.Context, logger commons.Logger, auth types.SimplePrinciple, assistantID uint64, assistantWebhookService internal_services.AssistantWebhookService, recorder observability.Recorder) []observability.Collector {
	collector := webhook.New(ctx, webhook.Config{
		Logger:                  logger,
		Auth:                    auth,
		AssistantID:             assistantID,
		AssistantWebhookService: assistantWebhookService,
		Recorder:                recorder,
	})
	if _, ok := collector.(observability.NoopCollector); ok {
		return nil
	}
	return []observability.Collector{collector}
}
