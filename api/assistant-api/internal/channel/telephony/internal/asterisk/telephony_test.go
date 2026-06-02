// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_asterisk_telephony

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rapidaai/api/assistant-api/config"
	"github.com/rapidaai/pkg/commons"
)

func newAsteriskTelephonyForTest(t *testing.T) *asteriskTelephony {
	t.Helper()
	logger, err := commons.NewApplicationLogger()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return &asteriskTelephony{
		appCfg: &config.AssistantConfig{},
		logger: logger,
	}
}

func TestReceiveCall_PopulatesDialedNumberFromFallbackParams(t *testing.T) {
	tel := newAsteriskTelephonyForTest(t)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/?callerid=15551234567&dnid=18005550100&channel_id=ast-chan-1", nil)
	c.Request = req

	info, err := tel.ReceiveCall(c)
	if err != nil {
		t.Fatalf("ReceiveCall() error = %v", err)
	}

	if info.CallerNumber != "15551234567" {
		t.Fatalf("expected CallerNumber 15551234567, got %q", info.CallerNumber)
	}
	if info.FromNumber != "18005550100" {
		t.Fatalf("expected FromNumber from dnid fallback, got %q", info.FromNumber)
	}
	if info.ChannelUUID != "ast-chan-1" {
		t.Fatalf("expected ChannelUUID ast-chan-1, got %q", info.ChannelUUID)
	}
	payload, ok := info.StatusInfo.Payload.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string payload, got %T", info.StatusInfo.Payload)
	}
	if got := payload["to"]; got != "18005550100" {
		t.Fatalf("expected status payload to=18005550100, got %q", got)
	}
}

func TestReceiveCall_PopulatesCallerNumberFromCallerParam(t *testing.T) {
	tel := newAsteriskTelephonyForTest(t)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/?caller=15557654321&to=18005550101", nil)
	c.Request = req

	info, err := tel.ReceiveCall(c)
	if err != nil {
		t.Fatalf("ReceiveCall() error = %v", err)
	}

	if info.CallerNumber != "15557654321" {
		t.Fatalf("expected CallerNumber 15557654321, got %q", info.CallerNumber)
	}
	if info.FromNumber != "18005550101" {
		t.Fatalf("expected FromNumber 18005550101, got %q", info.FromNumber)
	}
	payload, ok := info.StatusInfo.Payload.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string payload, got %T", info.StatusInfo.Payload)
	}
	if got := payload["caller"]; got != "15557654321" {
		t.Fatalf("expected status payload caller=15557654321, got %q", got)
	}
}
