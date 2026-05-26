// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package variable_test

import (
	"testing"
	"time"

	"github.com/rapidaai/api/assistant-api/internal/variable"
	"github.com/rapidaai/api/assistant-api/internal/variable/namespace"
)

func TestSystemNamespace_Get(t *testing.T) {
	src := newFixtureSource()
	ns := &namespace.SystemNamespace{}

	cases := map[string]string{
		"current_date":     "2026-04-26",
		"current_time":     "12:30:45",
		"current_datetime": "2026-04-26T12:30:45Z",
		"day_of_week":      "Sunday",
		"date_unix":        "1777206645",
		"date_unix_ms":     "1777206645000",
	}
	for suffix, want := range cases {
		got, ok := ns.Get(suffix, src, variable.ResolveContext{})
		if !ok || got != want {
			t.Errorf("system.%s = %v, %v; want %v, true", suffix, got, ok, want)
		}
	}

	if _, ok := ns.Get("missing", src, variable.ResolveContext{}); ok {
		t.Errorf("system.missing should return ok=false")
	}
}

func TestSystemNamespace_Enumerate_StableOnSameNow(t *testing.T) {
	src := newFixtureSource()
	ns := &namespace.SystemNamespace{}
	a := ns.Enumerate(src, variable.ResolveContext{})
	b := ns.Enumerate(src, variable.ResolveContext{})
	if len(a) != 7 || len(b) != 7 {
		t.Errorf("system.Enumerate should expose 7 keys, got %d / %d", len(a), len(b))
	}
	for k, v := range a {
		if b[k] != v {
			t.Errorf("Enumerate not deterministic for key %s: %v vs %v", k, v, b[k])
		}
	}
}

func TestAssistantNamespace_Get(t *testing.T) {
	src := newFixtureSource()
	ns := &namespace.AssistantNamespace{}
	cases := map[string]string{
		"id":          "42",
		"version":     "vrsn_7",
		"name":        "Sage",
		"language":    "english",
		"description": "test assistant",
	}
	for suffix, want := range cases {
		got, ok := ns.Get(suffix, src, variable.ResolveContext{})
		if !ok || got != want {
			t.Errorf("assistant.%s = %v, %v; want %v, true", suffix, got, ok, want)
		}
	}
}

func TestAssistantNamespace_NilAssistant(t *testing.T) {
	src := newTestSource()
	ns := &namespace.AssistantNamespace{}
	if got := ns.Enumerate(src, variable.ResolveContext{}); len(got) != 0 {
		t.Errorf("nil assistant should enumerate empty, got %v", got)
	}
}

func TestConversationNamespace_Get(t *testing.T) {
	src := newFixtureSource()
	ns := &namespace.ConversationNamespace{}

	if got, _ := ns.Get("id", src, variable.ResolveContext{}); got != "100" {
		t.Errorf("conversation.id = %v, want 100", got)
	}
	if got, _ := ns.Get("identifier", src, variable.ResolveContext{}); got != "conv-abc" {
		t.Errorf("conversation.identifier = %v", got)
	}
	if got, _ := ns.Get("source", src, variable.ResolveContext{}); got != "phone" {
		t.Errorf("conversation.source = %v", got)
	}
	if got, _ := ns.Get("direction", src, variable.ResolveContext{}); got != "inbound" {
		t.Errorf("conversation.direction = %v", got)
	}

	msgs, ok := ns.Get("messages", src, variable.ResolveContext{})
	if !ok {
		t.Fatalf("conversation.messages missing")
	}
	hist, ok := msgs.([]map[string]string)
	if !ok || len(hist) != 2 {
		t.Fatalf("messages shape wrong: %T %v", msgs, msgs)
	}
	if hist[0]["role"] != "user" || hist[0]["message"] != "hello" {
		t.Errorf("first message wrong: %v", hist[0])
	}
}

func TestConversationNamespace_NilConversation_StillEnumeratesMessages(t *testing.T) {
	src := newTestSource(variable.WithHistories([]variable.ConversationMessageInfo{{Role: "user", Content: "hi"}}))
	ns := &namespace.ConversationNamespace{}
	got := ns.Enumerate(src, variable.ResolveContext{})
	if _, ok := got["messages"]; !ok {
		t.Errorf("nil conversation should still expose messages, got %v", got)
	}
	if _, ok := got["id"]; ok {
		t.Errorf("nil conversation should not expose id")
	}
}

func TestSessionNamespace(t *testing.T) {
	src := newFixtureSource()
	ns := &namespace.SessionNamespace{}
	if got, _ := ns.Get("mode", src, variable.ResolveContext{}); got != "audio" {
		t.Errorf("session.mode = %v", got)
	}
	if got, _ := ns.Get("source", src, variable.ResolveContext{}); got != "phone" {
		t.Errorf("session.source = %v", got)
	}

	empty := newTestSource()
	if got := ns.Enumerate(empty, variable.ResolveContext{}); len(got) != 0 {
		t.Errorf("empty session should enumerate empty, got %v", got)
	}
}

func TestArgumentNamespace(t *testing.T) {
	src := newFixtureSource()
	ns := &namespace.ArgumentNamespace{}
	if got, _ := ns.Get("foo", src, variable.ResolveContext{}); got != "bar" {
		t.Errorf("argument.foo = %v", got)
	}
	if _, ok := ns.Get("missing", src, variable.ResolveContext{}); ok {
		t.Errorf("argument.missing should return ok=false")
	}
	enum := ns.Enumerate(src, variable.ResolveContext{})
	if enum["count"] != 3 {
		t.Errorf("Enumerate count = %v", enum["count"])
	}
}

func TestMetadataNamespace(t *testing.T) {
	src := newFixtureSource()
	ns := &namespace.MetadataNamespace{}
	if got, _ := ns.Get("loose", src, variable.ResolveContext{}); got != "value" {
		t.Errorf("metadata.loose = %v", got)
	}
	if got, _ := ns.Get("client.direction", src, variable.ResolveContext{}); got != "outbound" {
		t.Errorf("metadata.client.direction = %v (literal key)", got)
	}
}

func TestOptionNamespace(t *testing.T) {
	src := newFixtureSource()
	ns := &namespace.OptionNamespace{}
	if got, _ := ns.Get("max_tokens", src, variable.ResolveContext{}); got != "1024" {
		t.Errorf("option.max_tokens = %v", got)
	}
}

func TestMetadataPrefixNamespace_Client(t *testing.T) {
	src := newFixtureSource()
	ns := &namespace.MetadataPrefixNamespace{Prefix: "client."}

	if got, _ := ns.Get("direction", src, variable.ResolveContext{}); got != "outbound" {
		t.Errorf("client.direction = %v", got)
	}
	if got, _ := ns.Get("phone", src, variable.ResolveContext{}); got != "6001" {
		t.Errorf("client.phone = %v", got)
	}
	if _, ok := ns.Get("missing", src, variable.ResolveContext{}); ok {
		t.Errorf("client.missing should return ok=false")
	}

	enum := ns.Enumerate(src, variable.ResolveContext{})
	want := map[string]string{
		"direction": "outbound",
		"channel":   "sip",
		"phone":     "6001",
	}
	if len(enum) != len(want) {
		t.Errorf("client enumerate size = %d, want %d (%v)", len(enum), len(want), enum)
	}
	for k, v := range want {
		if enum[k] != v {
			t.Errorf("client.%s = %v, want %v", k, enum[k], v)
		}
	}
	if _, leaked := enum["analysis.summary"]; leaked {
		t.Errorf("client namespace should not expose analysis.* keys")
	}
}

func TestMetadataPrefixNamespace_Analysis(t *testing.T) {
	src := newFixtureSource()
	ns := &namespace.MetadataPrefixNamespace{Prefix: "analysis."}
	if got, _ := ns.Get("summary", src, variable.ResolveContext{}); got != "ok" {
		t.Errorf("analysis.summary = %v", got)
	}
}

func TestToolNamespace(t *testing.T) {
	ns := &namespace.ToolNamespace{}
	ctx := variable.ResolveContext{ToolName: "search", ToolArgs: map[string]any{"q": "hi"}}
	if got, _ := ns.Get("name", nil, ctx); got != "search" {
		t.Errorf("tool.name = %v", got)
	}
	gotArg, _ := ns.Get("argument", nil, ctx)
	if m, ok := gotArg.(map[string]any); !ok || m["q"] != "hi" {
		t.Errorf("tool.argument = %v", gotArg)
	}
	if _, ok := ns.Get("other", nil, ctx); ok {
		t.Errorf("tool.other should return ok=false")
	}
}

func TestEventNamespace(t *testing.T) {
	src := newFixtureSource()
	ns := &namespace.EventNamespace{}
	ctx := variable.ResolveContext{Event: "conversation.ended"}

	if got, _ := ns.Get("type", src, ctx); got != "conversation.ended" {
		t.Errorf("event.type = %v", got)
	}
	data, ok := ns.Get("data", src, ctx)
	if !ok {
		t.Fatalf("event.data missing")
	}
	dataMap, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("event.data shape: %T", data)
	}
	asst, _ := dataMap["assistant"].(map[string]any)
	if asst["id"] != "42" {
		t.Errorf("event.data.assistant.id = %v", asst["id"])
	}
	an, _ := dataMap["analysis"].(map[string]any)
	if an["summary"] != "ok" {
		t.Errorf("event.data.analysis.summary = %v", an["summary"])
	}
}

func TestConversationNamespace_DurationFormat(t *testing.T) {
	src := newFixtureSource()
	ns := &namespace.ConversationNamespace{}
	got, ok := ns.Get("duration", src, variable.ResolveContext{})
	if !ok {
		t.Fatalf("conversation.duration missing")
	}
	// Duration is computed via time.Since against wall clock; just verify the
	// type so we don't go flaky on CI.
	if _, isString := got.(string); !isString {
		t.Errorf("duration should be string, got %T", got)
	}
	_ = time.Now // pull time package usage
}
