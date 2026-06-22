// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package collectors

import (
	"context"
	"testing"

	assistant_config "github.com/rapidaai/api/assistant-api/config"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_telemetry_entity "github.com/rapidaai/api/assistant-api/internal/entity/telemetry"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/configs"
	gorm_model "github.com/rapidaai/pkg/models/gorm"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/protos"
)

func TestNew_AppendsTelemetryWhenDefaultTelemetryEnabled(t *testing.T) {
	collectors := NewWithEnv(context.Background(), testLogger(t), &assistant_config.AssistantConfig{
		TelemetryConfig: &configs.TelemetryConfig{
			TelemetryType: string(configs.LOGGING),
			Logging:       &configs.TelemetryLoggingConfig{},
		},
	})
	if len(collectors) != 1 {
		t.Fatalf("expected telemetry collector only, got %d", len(collectors))
	}
}

func TestTimelineConfig_IgnoresTelemetryOpenSearch(t *testing.T) {
	collectors := NewWithEnv(context.Background(), testLogger(t), &assistant_config.AssistantConfig{
		TelemetryConfig: &configs.TelemetryConfig{
			TelemetryType: string(configs.LOGGING),
			OpenSearch:    &configs.OpenSearchConfig{Schema: "http", Host: "localhost"},
			Logging:       &configs.TelemetryLoggingConfig{},
		},
	})
	if len(collectors) != 1 {
		t.Fatalf("expected telemetry collector only, got %d", len(collectors))
	}
}

func TestNew_SkipsInactiveConfig(t *testing.T) {
	collectors := NewWithEnv(context.Background(), testLogger(t), &assistant_config.AssistantConfig{
		TelemetryConfig: &configs.TelemetryConfig{},
	})
	if len(collectors) != 0 {
		t.Fatalf("expected no collectors, got %d", len(collectors))
	}
}

func TestNew_SkipsUnknownDefaultTelemetryType(t *testing.T) {
	collectors := NewWithEnv(context.Background(), testLogger(t), &assistant_config.AssistantConfig{
		TelemetryConfig: &configs.TelemetryConfig{TelemetryType: "unknown"},
	})
	if len(collectors) != 0 {
		t.Fatalf("expected no collectors, got %d", len(collectors))
	}
}

func TestNew_LogsAndSkipsTelemetryWhenCollectorFails(t *testing.T) {
	collectors := NewWithAssistantTelemetry(context.Background(), nil, []*internal_telemetry_entity.AssistantTelemetryProvider{
		{ProviderType: "unknown", Enabled: true},
	})
	if len(collectors) != 0 {
		t.Fatalf("expected failed telemetry collector to be skipped, got %d", len(collectors))
	}
}

func TestNew_AppendsNoopForEmptyAssistantProvider(t *testing.T) {
	collectors := NewWithAssistantTelemetry(context.Background(), testLogger(t), []*internal_telemetry_entity.AssistantTelemetryProvider{
		{ProviderType: "", Enabled: true},
	})
	if len(collectors) != 1 {
		t.Fatalf("expected empty assistant provider to append noop collector, got %d", len(collectors))
	}
}

func TestNew_LogsAndSkipsUnknownAssistantProvider(t *testing.T) {
	collectors := NewWithAssistantTelemetry(context.Background(), testLogger(t), []*internal_telemetry_entity.AssistantTelemetryProvider{
		{ProviderType: "unknown", Enabled: true},
	})
	if len(collectors) != 0 {
		t.Fatalf("expected unknown assistant provider to be skipped, got %d", len(collectors))
	}
}

func TestNew_AssistantTelemetryCollectorsUseProviderIDKeys(t *testing.T) {
	collectors := NewWithAssistantTelemetry(context.Background(), testLogger(t), []*internal_telemetry_entity.AssistantTelemetryProvider{
		{
			Audited:      gorm_model.Audited{Id: 101},
			ProviderType: "logging",
			Enabled:      true,
		},
		{
			Audited:      gorm_model.Audited{Id: 202},
			ProviderType: "logging",
			Enabled:      true,
		},
	})
	if len(collectors) != 2 {
		t.Fatalf("expected two telemetry collectors, got %d", len(collectors))
	}
	if collectors[0].Key() == collectors[1].Key() {
		t.Fatalf("expected distinct telemetry collector keys, got %q", collectors[0].Key())
	}
}

func TestNew_AppendsWebhookCollector(t *testing.T) {
	organizationID := uint64(10)
	projectID := uint64(20)
	auth := &types.ServiceScope{
		OrganizationId: &organizationID,
		ProjectId:      &projectID,
	}
	webhookService := &recordingAssistantWebhookService{
		webhooks: []*internal_assistant_entity.AssistantWebhook{
			{Provider: internal_assistant_entity.AssistantWebhookProviderHTTP},
		},
	}

	collectors := NewWithAssistantWebhook(context.Background(), testLogger(t), auth, 30, webhookService, nil)
	if len(collectors) != 1 {
		t.Fatalf("expected webhook collector only, got %d", len(collectors))
	}
	if webhookService.getAllCalls != 0 {
		t.Fatalf("NewWithAssistantWebhook should not load webhooks, got %d calls", webhookService.getAllCalls)
	}
}

type recordingAssistantWebhookService struct {
	webhooks    []*internal_assistant_entity.AssistantWebhook
	getAllCalls int
}

func (s *recordingAssistantWebhookService) Get(context.Context, types.SimplePrinciple, uint64, uint64) (*internal_assistant_entity.AssistantWebhook, error) {
	return nil, nil
}

func (s *recordingAssistantWebhookService) Delete(context.Context, types.SimplePrinciple, uint64, uint64) (*internal_assistant_entity.AssistantWebhook, error) {
	return nil, nil
}

func (s *recordingAssistantWebhookService) Create(context.Context, types.SimplePrinciple, uint64, string, []string, []*protos.Metadata, uint32, *string) (*internal_assistant_entity.AssistantWebhook, error) {
	return nil, nil
}

func (s *recordingAssistantWebhookService) Update(context.Context, types.SimplePrinciple, uint64, uint64, string, []string, []*protos.Metadata, uint32, *string) (*internal_assistant_entity.AssistantWebhook, error) {
	return nil, nil
}

func (s *recordingAssistantWebhookService) GetAll(context.Context, types.SimplePrinciple, uint64, []*protos.Criteria, *protos.Paginate) (int64, []*internal_assistant_entity.AssistantWebhook, error) {
	s.getAllCalls++
	return int64(len(s.webhooks)), s.webhooks, nil
}

func testLogger(t *testing.T) commons.Logger {
	t.Helper()

	logger, err := commons.NewApplicationLogger(
		commons.Name("observability-collectors-test"),
		commons.Level("error"),
		commons.EnableFile(false),
	)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return logger
}
