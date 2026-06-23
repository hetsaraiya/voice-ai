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
	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/configs"
	gorm_model "github.com/rapidaai/pkg/models/gorm"
	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/protos"
)

func TestNew_AppendsTelemetryWhenDefaultTelemetryEnabled(t *testing.T) {
	collector := NewWithEnv(context.Background(), testLogger(t), &assistant_config.AssistantConfig{
		TelemetryConfig: &configs.TelemetryConfig{
			TelemetryType: string(configs.LOGGING),
			Logging:       &configs.TelemetryLoggingConfig{},
		},
	})
	if collector == nil {
		t.Fatal("expected telemetry collector")
	}
}

func TestTimelineConfig_IgnoresTelemetryOpenSearch(t *testing.T) {
	collector := NewWithEnv(context.Background(), testLogger(t), &assistant_config.AssistantConfig{
		TelemetryConfig: &configs.TelemetryConfig{
			TelemetryType: string(configs.LOGGING),
			OpenSearch:    &configs.OpenSearchConfig{Schema: "http", Host: "localhost"},
			Logging:       &configs.TelemetryLoggingConfig{},
		},
	})
	if collector == nil {
		t.Fatal("expected telemetry collector")
	}
}

func TestNew_SkipsInactiveConfig(t *testing.T) {
	collector := NewWithEnv(context.Background(), testLogger(t), &assistant_config.AssistantConfig{
		TelemetryConfig: &configs.TelemetryConfig{},
	})
	if collector != nil {
		t.Fatalf("expected no collector, got %T", collector)
	}
}

func TestNew_SkipsUnknownDefaultTelemetryType(t *testing.T) {
	collector := NewWithEnv(context.Background(), testLogger(t), &assistant_config.AssistantConfig{
		TelemetryConfig: &configs.TelemetryConfig{TelemetryType: "unknown"},
	})
	if collector != nil {
		t.Fatalf("expected no collector, got %T", collector)
	}
}

func TestNew_LogsAndSkipsTelemetryWhenCollectorFails(t *testing.T) {
	organizationID := uint64(10)
	projectID := uint64(20)
	auth := &types.ServiceScope{
		OrganizationId: &organizationID,
		ProjectId:      &projectID,
	}
	telemetryService := &recordingAssistantTelemetryProviderService{
		providers: []*internal_telemetry_entity.AssistantTelemetryProvider{
			{ProviderType: "unknown", Enabled: true},
		},
	}
	collector := NewWithAssistantTelemetry(context.Background(), nil, auth, 30, telemetryService)
	if collector == nil {
		t.Fatal("expected assistant telemetry collector")
	}
	if telemetryService.getAllCalls != 0 {
		t.Fatalf("NewWithAssistantTelemetry should not load providers, got %d calls", telemetryService.getAllCalls)
	}
	if err := collector.Collect(context.Background(), observability.AssistantScope{AssistantID: 30}, observability.Context{}, observability.RecordLog{
		Level:   observability.LevelInfo,
		Message: "test",
	}); err != nil {
		t.Fatalf("expected unknown telemetry provider to be skipped without error, got %v", err)
	}
	if telemetryService.getAllCalls != 1 {
		t.Fatalf("expected one telemetry provider load, got %d", telemetryService.getAllCalls)
	}
}

func TestNew_SkipsAssistantTelemetryWithoutRequiredConfig(t *testing.T) {
	organizationID := uint64(10)
	projectID := uint64(20)
	auth := &types.ServiceScope{
		OrganizationId: &organizationID,
		ProjectId:      &projectID,
	}
	if collector := NewWithAssistantTelemetry(context.Background(), testLogger(t), nil, 30, &recordingAssistantTelemetryProviderService{}); collector != nil {
		t.Fatalf("expected no collector without auth, got %T", collector)
	}
	if collector := NewWithAssistantTelemetry(context.Background(), testLogger(t), auth, 0, &recordingAssistantTelemetryProviderService{}); collector != nil {
		t.Fatalf("expected no collector without assistant ID, got %T", collector)
	}
	if collector := NewWithAssistantTelemetry(context.Background(), testLogger(t), auth, 30, nil); collector != nil {
		t.Fatalf("expected no collector without service, got %T", collector)
	}
}

func TestNew_AssistantTelemetryLoadsByAssistantID(t *testing.T) {
	organizationID := uint64(10)
	projectID := uint64(20)
	auth := &types.ServiceScope{
		OrganizationId: &organizationID,
		ProjectId:      &projectID,
	}
	telemetryService := &recordingAssistantTelemetryProviderService{
		providers: []*internal_telemetry_entity.AssistantTelemetryProvider{
			{
				Audited:      gorm_model.Audited{Id: 101},
				ProviderType: "logging",
				Enabled:      true,
			},
			{
				Audited:      gorm_model.Audited{Id: 202},
				ProviderType: "logging",
				Enabled:      false,
			},
		},
	}
	collector := NewWithAssistantTelemetry(context.Background(), testLogger(t), auth, 30, telemetryService)
	if collector == nil {
		t.Fatal("expected assistant telemetry collector")
	}
	if collector.Key() != "telemetry:assistant:30" {
		t.Fatalf("expected assistant telemetry key, got %q", collector.Key())
	}
	if telemetryService.getAllCalls != 0 {
		t.Fatalf("NewWithAssistantTelemetry should not load providers, got %d calls", telemetryService.getAllCalls)
	}
	if err := collector.Collect(context.Background(), observability.AssistantScope{AssistantID: 30}, observability.Context{}, observability.RecordLog{
		Level:   observability.LevelInfo,
		Message: "test",
	}); err != nil {
		t.Fatalf("expected assistant telemetry collect to succeed, got %v", err)
	}
	if telemetryService.getAllCalls != 1 {
		t.Fatalf("expected telemetry provider load, got %d", telemetryService.getAllCalls)
	}
	if telemetryService.assistantID != 30 {
		t.Fatalf("expected assistant ID 30, got %d", telemetryService.assistantID)
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

	collector := NewWithAssistantWebhook(context.Background(), testLogger(t), auth, 30, webhookService, &recordingHTTPLogService{})
	if collector == nil {
		t.Fatal("expected webhook collector")
	}
	if webhookService.getAllCalls != 0 {
		t.Fatalf("NewWithAssistantWebhook should not load webhooks, got %d calls", webhookService.getAllCalls)
	}
}

type recordingAssistantTelemetryProviderService struct {
	providers   []*internal_telemetry_entity.AssistantTelemetryProvider
	getAllCalls int
	assistantID uint64
}

func (s *recordingAssistantTelemetryProviderService) Get(context.Context, types.SimplePrinciple, uint64, uint64) (*internal_telemetry_entity.AssistantTelemetryProvider, error) {
	return nil, nil
}

func (s *recordingAssistantTelemetryProviderService) GetAll(_ context.Context, _ types.SimplePrinciple, assistantID uint64, _ []*protos.Criteria, _ *protos.Paginate) (int64, []*internal_telemetry_entity.AssistantTelemetryProvider, error) {
	s.getAllCalls++
	s.assistantID = assistantID
	return int64(len(s.providers)), s.providers, nil
}

func (s *recordingAssistantTelemetryProviderService) Create(context.Context, types.SimplePrinciple, uint64, string, bool, []*protos.Metadata) (*internal_telemetry_entity.AssistantTelemetryProvider, error) {
	return nil, nil
}

func (s *recordingAssistantTelemetryProviderService) Update(context.Context, types.SimplePrinciple, uint64, uint64, string, bool, []*protos.Metadata) (*internal_telemetry_entity.AssistantTelemetryProvider, error) {
	return nil, nil
}

func (s *recordingAssistantTelemetryProviderService) Delete(context.Context, types.SimplePrinciple, uint64, uint64) (*internal_telemetry_entity.AssistantTelemetryProvider, error) {
	return nil, nil
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

type recordingHTTPLogService struct{}

func (s *recordingHTTPLogService) CreateLog(context.Context, types.SimplePrinciple, string, uint64, string, string, uint64, *uint64, string, string, int64, int64, uint32, type_enums.RecordState, *string, []byte, []byte) (*internal_assistant_entity.AssistantHTTPLog, error) {
	return nil, nil
}

func (s *recordingHTTPLogService) GetLog(context.Context, types.SimplePrinciple, uint64, uint64) (*internal_assistant_entity.AssistantHTTPLog, error) {
	return nil, nil
}

func (s *recordingHTTPLogService) GetAllLog(context.Context, types.SimplePrinciple, uint64, []*protos.Criteria, *protos.Paginate, *protos.Ordering) (int64, []*internal_assistant_entity.AssistantHTTPLog, error) {
	return 0, nil, nil
}

func (s *recordingHTTPLogService) GetLogObject(context.Context, uint64, uint64, uint64) ([]byte, []byte, error) {
	return nil, nil, nil
}

func (s *recordingHTTPLogService) RetryLog(context.Context, types.SimplePrinciple, uint64, uint64) (*internal_assistant_entity.AssistantHTTPLog, error) {
	return nil, nil
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
