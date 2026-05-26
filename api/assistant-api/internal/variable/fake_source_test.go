// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package variable_test

import (
	"time"

	"github.com/rapidaai/api/assistant-api/internal/variable"
)

// fixedTime returns a stable UTC instant used across the test suite.
func fixedTime() time.Time {
	return time.Date(2026, 4, 26, 12, 30, 45, 0, time.UTC)
}

// newTestSource builds a *VariableSource with the given options set.
func newTestSource(opts ...variable.SourceOption) *variable.VariableSource {
	all := append([]variable.SourceOption{variable.WithClockFunc(func() time.Time { return fixedTime() })}, opts...)
	return variable.NewVariableSource(all...)
}

// newFixtureSource builds a VariableSource preloaded with representative data
// so individual tests stay short.
func newFixtureSource() *variable.VariableSource {
	return newTestSource(
		variable.WithAssistant(&variable.AssistantInfo{
			ID:          42,
			VersionID:   7,
			Name:        "Sage",
			Language:    "english",
			Description: "test assistant",
		}),
		variable.WithConversation(&variable.ConversationInfo{
			ID:          100,
			Identifier:  "conv-abc",
			Source:      "phone",
			Direction:   "inbound",
			CreatedDate: fixedTime().Add(-2 * time.Minute),
		}),
		variable.WithHistories([]variable.ConversationMessageInfo{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
		}),
		variable.WithArguments(map[string]any{"foo": "bar", "count": 3}),
		variable.WithMetadata(map[string]any{
			"client.direction": "outbound",
			"client.channel":   "sip",
			"client.phone":     "6001",
			"analysis.summary": "ok",
			"loose":            "value",
		}),
		variable.WithOptions(map[string]any{"max_tokens": "1024"}),
		variable.WithMode("audio"),
	)
}
