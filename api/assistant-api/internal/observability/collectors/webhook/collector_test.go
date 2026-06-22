// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	gorm_model "github.com/rapidaai/pkg/models/gorm"
	gorm_types "github.com/rapidaai/pkg/models/gorm/types"
	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/protos"
)

func TestCollector_SendsWebhookEventPayload(t *testing.T) {
	var got map[string]interface{}
	requestLogRecorder := &recordingRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("X-Test") != "yes" {
			t.Fatalf("X-Test header = %q, want yes", r.Header.Get("X-Test"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	organizationID := uint64(30)
	projectID := uint64(40)
	auth := &types.ServiceScope{
		OrganizationId: &organizationID,
		ProjectId:      &projectID,
	}
	webhookService := &recordingAssistantWebhookService{
		webhooks: []*internal_assistant_entity.AssistantWebhook{
			testWebhook(1, []string{observability.CallRinging.String()}, map[string]interface{}{
				WebhookOptionHTTPURLKey:     server.URL,
				WebhookOptionHTTPHeadersKey: map[string]interface{}{"X-Test": "yes"},
			}),
		},
	}
	collector := New(context.Background(), Config{
		Auth:                    auth,
		AssistantID:             10,
		AssistantWebhookService: webhookService,
		Recorder:                requestLogRecorder,
	})

	scope := observability.ConversationScope{
		AssistantScope: observability.AssistantScope{AssistantID: 10},
		ConversationID: 20,
	}
	err := collector.Collect(context.Background(), scope, observability.Context{Auth: auth}, observability.RecordWebhook{
		ID:        "evt-1",
		Event:     observability.CallRinging,
		ContextID: "call-context-1",
		Payload:   map[string]interface{}{"status": "ringing", "callId": "call-1"},
	})
	if err != nil {
		t.Fatalf("CollectWebhook returned error: %v", err)
	}
	if got["status"] != "ringing" || got["callId"] != "call-1" {
		t.Fatalf("unexpected payload: %+v", got)
	}
	if _, ok := got["scope"]; ok {
		t.Fatalf("webhook payload should not include observability scope: %+v", got)
	}
	if _, ok := got["context_id"]; ok {
		t.Fatalf("webhook payload should not include observability context_id: %+v", got)
	}
	if len(requestLogRecorder.records) != 1 {
		t.Fatalf("expected one request log record, got %d", len(requestLogRecorder.records))
	}
	requestLogRecord, ok := requestLogRecorder.records[0].(observability.RecordRequestLog)
	if !ok {
		t.Fatalf("expected RecordRequestLog, got %T", requestLogRecorder.records[0])
	}
	if requestLogRecord.Source != "webhook" || requestLogRecord.SourceRefID != 1 || requestLogRecord.SourceEvent != observability.CallRinging.String() {
		t.Fatalf("unexpected request log source: %+v", requestLogRecord)
	}
	if requestLogRecord.ContextID != "call-context-1" {
		t.Fatalf("unexpected request log context_id: %q", requestLogRecord.ContextID)
	}
	requestLogScope, ok := requestLogRecorder.scopes[0].(observability.ConversationScope)
	if !ok {
		t.Fatalf("expected conversation scope, got %T", requestLogRecorder.scopes[0])
	}
	if requestLogScope.AssistantScopeID() != 10 || requestLogScope.ConversationScopeID() != 20 {
		t.Fatalf("unexpected request log scope: %+v", requestLogScope)
	}
	if requestLogRecord.Status != type_enums.RECORD_COMPLETE || requestLogRecord.ResponseStatus != http.StatusNoContent {
		t.Fatalf("unexpected request log status: %+v", requestLogRecord)
	}
}

func TestNew_DoesNotLoadWebhooks(t *testing.T) {
	organizationID := uint64(30)
	projectID := uint64(40)
	auth := &types.ServiceScope{
		OrganizationId: &organizationID,
		ProjectId:      &projectID,
	}
	webhookService := &recordingAssistantWebhookService{
		webhooks: []*internal_assistant_entity.AssistantWebhook{
			testWebhook(1, []string{observability.CallRinging.String()}, map[string]interface{}{WebhookOptionHTTPURLKey: "https://example.com/webhook"}),
		},
	}

	collector := New(context.Background(), Config{
		Auth:                    auth,
		AssistantID:             10,
		AssistantWebhookService: webhookService,
	})
	if _, ok := collector.(observability.NoopCollector); ok {
		t.Fatal("expected webhook collector")
	}
	if webhookService.getAllCalls != 0 {
		t.Fatalf("New should not load webhooks, got %d calls", webhookService.getAllCalls)
	}
}

func TestCollector_LoadsWebhooksOnce(t *testing.T) {
	organizationID := uint64(30)
	projectID := uint64(40)
	auth := &types.ServiceScope{
		OrganizationId: &organizationID,
		ProjectId:      &projectID,
	}
	webhookService := &recordingAssistantWebhookService{
		webhooks: []*internal_assistant_entity.AssistantWebhook{
			testWebhook(1, []string{observability.CallFailed.String()}, map[string]interface{}{WebhookOptionHTTPURLKey: "https://example.com/webhook"}),
		},
	}
	collector := New(context.Background(), Config{
		Auth:                    auth,
		AssistantID:             10,
		AssistantWebhookService: webhookService,
	})

	for i := 0; i < 2; i++ {
		err := collector.Collect(context.Background(), observability.AssistantScope{AssistantID: 10}, observability.Context{}, observability.RecordWebhook{
			Event: observability.CallRinging,
		})
		if err != nil {
			t.Fatalf("CollectWebhook returned error: %v", err)
		}
	}
	if webhookService.getAllCalls != 1 {
		t.Fatalf("expected one webhook load, got %d", webhookService.getAllCalls)
	}
}

func TestCollector_IgnoresUnallowedWebhookEvent(t *testing.T) {
	organizationID := uint64(30)
	projectID := uint64(40)
	auth := &types.ServiceScope{
		OrganizationId: &organizationID,
		ProjectId:      &projectID,
	}
	webhookService := &recordingAssistantWebhookService{
		webhooks: []*internal_assistant_entity.AssistantWebhook{
			testWebhook(1, []string{observability.CallFailed.String()}, map[string]interface{}{WebhookOptionHTTPURLKey: "https://example.com/webhook"}),
		},
	}
	collector := New(context.Background(), Config{
		Auth:                    auth,
		AssistantID:             10,
		AssistantWebhookService: webhookService,
	})

	err := collector.Collect(context.Background(), observability.AssistantScope{AssistantID: 10}, observability.Context{}, observability.RecordWebhook{
		Event: observability.CallRinging,
	})
	if err != nil {
		t.Fatalf("CollectWebhook returned error: %v", err)
	}
}

func TestCollector_ReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	organizationID := uint64(30)
	projectID := uint64(40)
	auth := &types.ServiceScope{
		OrganizationId: &organizationID,
		ProjectId:      &projectID,
	}
	webhookService := &recordingAssistantWebhookService{
		webhooks: []*internal_assistant_entity.AssistantWebhook{
			testWebhook(1, []string{observability.CallFailed.String()}, map[string]interface{}{WebhookOptionHTTPURLKey: server.URL}),
		},
	}
	collector := New(context.Background(), Config{
		Auth:                    auth,
		AssistantID:             10,
		AssistantWebhookService: webhookService,
	})

	err := collector.Collect(context.Background(), observability.AssistantScope{AssistantID: 10}, observability.Context{}, observability.RecordWebhook{
		Event:   observability.CallFailed,
		Payload: map[string]interface{}{"status": "failed"},
	})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
}

func testWebhook(id uint64, events []string, options map[string]interface{}) *internal_assistant_entity.AssistantWebhook {
	webhook := &internal_assistant_entity.AssistantWebhook{
		Audited:         gorm_model.Audited{Id: id},
		Provider:        internal_assistant_entity.AssistantWebhookProviderHTTP,
		AssistantEvents: gorm_types.StringArray(events),
	}
	for key, value := range options {
		webhook.AssistantWebhookOption = append(webhook.AssistantWebhookOption, &internal_assistant_entity.AssistantWebhookOption{
			Metadata: *gorm_model.NewMetadata(key, value),
		})
	}
	return webhook
}

type recordingAssistantWebhookService struct {
	webhooks    []*internal_assistant_entity.AssistantWebhook
	getAllCalls int
}

type recordingRecorder struct {
	scopes  []observability.Scope
	records []observability.Record
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

func (r *recordingRecorder) Record(_ context.Context, scope observability.Scope, records ...observability.Record) error {
	for _, record := range records {
		r.scopes = append(r.scopes, scope)
		r.records = append(r.records, record)
	}
	return nil
}

func (r *recordingRecorder) AddCollectors(...observability.Collector) error {
	return nil
}

func (r *recordingRecorder) Close(context.Context) error {
	return nil
}
