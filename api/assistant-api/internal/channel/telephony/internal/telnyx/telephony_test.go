// Copyright (c) 2023-2025 RapidaAI
// Author: RapidaAI Team <team@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_telnyx_telephony

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rapidaai/api/assistant-api/config"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/protos"
	"google.golang.org/protobuf/types/known/structpb"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestNewTelnyxTelephony(t *testing.T) {
	cfg := &config.AssistantConfig{
		PublicAssistantHost: "test.example.com",
	}
	logger := commons.NewLogger("test")

	telephony, err := NewTelnyxTelephony(cfg, logger)

	if err != nil {
		t.Fatalf("NewTelnyxTelephony returned error: %v", err)
	}

	if telephony == nil {
		t.Fatal("NewTelnyxTelephony returned nil")
	}
}

func TestCatchAllStatusCallback(t *testing.T) {
	cfg := &config.AssistantConfig{}
	logger := commons.NewLogger("test")
	telephony, _ := NewTelnyxTelephony(cfg, logger)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/telnyx/event", nil)

	statusInfo, err := telephony.CatchAllStatusCallback(c)

	if err != nil {
		t.Errorf("CatchAllStatusCallback returned error: %v", err)
	}

	if statusInfo != nil {
		t.Errorf("CatchAllStatusCallback should return nil, got: %v", statusInfo)
	}
}

func TestStatusCallback(t *testing.T) {
	cfg := &config.AssistantConfig{}
	logger := commons.NewLogger("test")
	telephony, _ := NewTelnyxTelephony(cfg, logger)

	tests := []struct {
		name       string
		payload    map[string]interface{}
		expectErr  bool
		expectEvent string
	}{
		{
			name: "valid call.answered event",
			payload: map[string]interface{}{
				"data": map[string]interface{}{
					"event_type": "call.answered",
					"id":         "call-123",
					"payload": map[string]interface{}{
						"call_control_id": "call-control-123",
					},
				},
			},
			expectErr:  false,
			expectEvent: "call.answered",
		},
		{
			name: "valid call.hangup event",
			payload: map[string]interface{}{
				"data": map[string]interface{}{
					"event_type": "call.hangup",
					"id":         "call-456",
				},
			},
			expectErr:  false,
			expectEvent: "call.hangup",
		},
		{
			name: "missing data field",
			payload: map[string]interface{}{
				"event": "test",
			},
			expectErr: true,
		},
		{
			name: "missing event_type",
			payload: map[string]interface{}{
				"data": map[string]interface{}{
					"id": "call-123",
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.payload)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/telnyx/event", strings.NewReader(string(body)))

			statusInfo, err := telephony.StatusCallback(c, nil, 1, 1)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if statusInfo.Event != tt.expectEvent {
				t.Errorf("expected event %s, got %s", tt.expectEvent, statusInfo.Event)
			}
		})
	}
}

func TestReceiveCall(t *testing.T) {
	cfg := &config.AssistantConfig{}
	logger := commons.NewLogger("test")
	telephony, _ := NewTelnyxTelephony(cfg, logger)

	tests := []struct {
		name         string
		queryParams  map[string]string
		expectErr    bool
		expectNumber string
	}{
		{
			name: "valid from parameter",
			queryParams: map[string]string{
				"from": "+15551234567",
				"to":   "+15559876543",
			},
			expectErr:    false,
			expectNumber: "+15551234567",
		},
		{
			name: "valid caller_id parameter",
			queryParams: map[string]string{
				"caller_id": "+15551112222",
			},
			expectErr:    false,
			expectNumber: "+15551112222",
		},
		{
			name:         "missing caller number",
			queryParams:  map[string]string{},
			expectErr:    true,
			expectNumber: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/telnyx/incoming"
			if len(tt.queryParams) > 0 {
				params := make([]string, 0)
				for k, v := range tt.queryParams {
					params = append(params, k+"="+v)
				}
				url = url + "?" + strings.Join(params, "&")
			}

			c.Request = httptest.NewRequest("POST", url, nil)

			callInfo, err := telephony.ReceiveCall(c)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if callInfo.CallerNumber != tt.expectNumber {
				t.Errorf("expected caller number %s, got %s", tt.expectNumber, callInfo.CallerNumber)
			}

			if callInfo.Provider != telnyxProvider {
				t.Errorf("expected provider %s, got %s", telnyxProvider, callInfo.Provider)
			}
		})
	}
}

func TestInboundCall(t *testing.T) {
	cfg := &config.AssistantConfig{
		PublicAssistantHost: "test.example.com",
	}
	logger := commons.NewLogger("test")
	telephony, _ := NewTelnyxTelephony(cfg, logger)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("contextId", "test-context-123")
	c.Request = httptest.NewRequest("POST", "/telnyx/incoming?call_control_id=call-123", nil)

	err := telephony.InboundCall(c, nil, 1, "+15551234567", 1)

	if err != nil {
		t.Errorf("InboundCall returned error: %v", err)
	}

	// Check response contains stream_url
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Errorf("failed to parse response: %v", err)
	}

	if result, ok := response["result"].(string); !ok || result != "streaming.start" {
		t.Errorf("expected result streaming.start, got %v", response["result"])
	}
}

func TestTelnyxWebSocketEventParsing(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		expected TelnyxWebSocketEvent
	}{
		{
			name: "start event",
			jsonStr: `{"event":"start","stream_id":"stream-123","start":{"call_control_id":"call-456","media_format":{"encoding":"PCMU","sample_rate":8000,"channels":1}}}`,
			expected: TelnyxWebSocketEvent{
				Event:    "start",
				StreamID: "stream-123",
				Start: &TelnyxStartEvent{
					CallControlID: "call-456",
					MediaFormat: TelnyxMediaFormat{
						Encoding:   "PCMU",
						SampleRate: 8000,
						Channels:   1,
					},
				},
			},
		},
		{
			name: "media event",
			jsonStr: `{"event":"media","stream_id":"stream-123","media":{"track":"inbound","payload":"dGVzdA=="}}`,
			expected: TelnyxWebSocketEvent{
				Event:    "media",
				StreamID: "stream-123",
				Media: &TelnyxMediaEvent{
					Track:   "inbound",
					Payload: "dGVzdA==",
				},
			},
		},
		{
			name: "stop event",
			jsonStr: `{"event":"stop","stream_id":"stream-123","stop":{"call_control_id":"call-456"}}`,
			expected: TelnyxWebSocketEvent{
				Event:    "stop",
				StreamID: "stream-123",
				Stop: &TelnyxStopEvent{
					CallControlID: "call-456",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var event TelnyxWebSocketEvent
			if err := json.Unmarshal([]byte(tt.jsonStr), &event); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			if event.Event != tt.expected.Event {
				t.Errorf("expected event %s, got %s", tt.expected.Event, event.Event)
			}

			if event.StreamID != tt.expected.StreamID {
				t.Errorf("expected stream_id %s, got %s", tt.expected.StreamID, event.StreamID)
			}
		})
	}
}

func TestGetCredentials(t *testing.T) {
	cfg := &config.AssistantConfig{}
	logger := commons.NewLogger("test")
	telephony, _ := NewTelnyxTelephony(cfg, logger)

	tests := []struct {
		name       string
		credMap    map[string]interface{}
		expectErr  bool
		expectAPI  string
		expectConn string
	}{
		{
			name: "valid credentials",
			credMap: map[string]interface{}{
				"api_key":       "test-api-key",
				"connection_id": "test-connection-id",
			},
			expectErr:  false,
			expectAPI:  "test-api-key",
			expectConn: "test-connection-id",
		},
		{
			name: "missing api_key",
			credMap: map[string]interface{}{
				"connection_id": "test-connection-id",
			},
			expectErr: true,
		},
		{
			name: "missing connection_id",
			credMap: map[string]interface{}{
				"api_key": "test-api-key",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structValue, _ := structpb.NewStruct(tt.credMap)
			vaultCred := &protos.VaultCredential{
				Value: structValue,
			}

			apiKey, connID, err := telephony.get_credentials(vaultCred)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if apiKey != tt.expectAPI {
				t.Errorf("expected api_key %s, got %s", tt.expectAPI, apiKey)
			}

			if connID != tt.expectConn {
				t.Errorf("expected connection_id %s, got %s", tt.expectConn, connID)
			}
		})
	}
}

func TestTelephonyInterfaceCompliance(t *testing.T) {
	// Compile-time check that telnyxTelephony implements internal_type.Telephony
	var _ internal_type.Telephony = (*telnyxTelephony)(nil)
}
