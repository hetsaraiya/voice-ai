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
)

func TestCollector_SendsWebhookEventPayload(t *testing.T) {
	var got map[string]interface{}
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

	collector := New(Config{Webhooks: []*internal_assistant_entity.AssistantWebhook{
		testWebhook(1, []string{observability.CallRinging.String()}, map[string]interface{}{
			WebhookOptionHTTPURLKey:     server.URL,
			WebhookOptionHTTPHeadersKey: map[string]interface{}{"X-Test": "yes"},
		}),
	}})

	err := collector.Collect(context.Background(), observability.RecordWebhook{
		CommonRecord: observability.CommonRecord{
			ID: "evt-1",
			Scope: observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: 10},
				ConversationID: 20,
			},
		},
		Event:   observability.CallRinging,
		Payload: map[string]interface{}{"status": "ringing", "callId": "call-1"},
	})
	if err != nil {
		t.Fatalf("CollectWebhook returned error: %v", err)
	}
	if got["status"] != "ringing" || got["callId"] != "call-1" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestCollector_IgnoresUnallowedWebhookEvent(t *testing.T) {
	collector := New(Config{Webhooks: []*internal_assistant_entity.AssistantWebhook{
		testWebhook(1, []string{observability.CallFailed.String()}, map[string]interface{}{WebhookOptionHTTPURLKey: "https://example.com/webhook"}),
	}})

	err := collector.Collect(context.Background(), observability.RecordWebhook{
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

	collector := New(Config{Webhooks: []*internal_assistant_entity.AssistantWebhook{
		testWebhook(1, []string{observability.CallFailed.String()}, map[string]interface{}{WebhookOptionHTTPURLKey: server.URL}),
	}})

	err := collector.Collect(context.Background(), observability.RecordWebhook{
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
