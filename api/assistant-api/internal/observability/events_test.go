// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability

import "testing"

func TestEvents_NoDuplicatesAndKnownComponents(t *testing.T) {
	seen := map[EventName]bool{}
	for _, event := range AllEvents() {
		if seen[event] {
			t.Fatalf("duplicate event %q", event)
		}
		seen[event] = true
		if event.Component() == ComponentUnknown {
			t.Fatalf("event %q has unknown component", event)
		}
		if !event.IsKnown() {
			t.Fatalf("event %q was not recognized as known", event)
		}
	}
}

func TestEvents_ComponentLists(t *testing.T) {
	for _, event := range CallEvents() {
		if !event.HasComponent(ComponentCall) {
			t.Fatalf("call event %q has component %q", event, event.Component())
		}
	}
	for _, event := range ConversationEvents() {
		if !event.HasComponent(ComponentConversation) {
			t.Fatalf("conversation event %q has component %q", event, event.Component())
		}
	}
}
