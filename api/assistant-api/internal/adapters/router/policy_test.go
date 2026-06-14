// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package router

import (
	"context"
	"testing"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
)

func TestRoutePolicy_DefaultPassthroughRoutesPacket(t *testing.T) {
	policy := NewRoutePolicy()
	packet := internal_type.UserAudioReceivedPacket{ContextID: "audio"}

	var routed internal_type.Packet
	policy.Route(context.Background(), packet, func(ctx context.Context, p internal_type.Packet) {
		routed = p
	})

	if routed == nil {
		t.Fatalf("expected default policy to route packet")
	}
	if routed.ContextId() != packet.ContextId() {
		t.Fatalf("routed packet mismatch: got=%s want=%s", routed.ContextId(), packet.ContextId())
	}
}

func TestRoutePolicy_IgnoreStopsMatchingPacket(t *testing.T) {
	policy := NewRoutePolicy()
	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionIgnore,
	})

	called := false
	policy.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "audio"}, func(ctx context.Context, p internal_type.Packet) {
		called = true
	})

	if called {
		t.Fatalf("expected ignored packet to stop before next")
	}
}

func TestRoutePolicy_IgnoreDoesNotStopUnrelatedPacket(t *testing.T) {
	policy := NewRoutePolicy()
	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionIgnore,
	})

	var routed internal_type.Packet
	policy.Route(context.Background(), internal_type.LLMToolResultPacket{ContextID: "tool"}, func(ctx context.Context, p internal_type.Packet) {
		routed = p
	})

	if routed == nil {
		t.Fatalf("expected unrelated packet to route")
	}
	if routed.ContextId() != "tool" {
		t.Fatalf("routed packet mismatch: got=%s want=tool", routed.ContextId())
	}
}

func TestRoutePolicy_PassthroughRemovesIgnorePolicy(t *testing.T) {
	policy := NewRoutePolicy()
	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionIgnore,
	})
	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionPassthrough,
	})

	called := false
	policy.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "audio"}, func(ctx context.Context, p internal_type.Packet) {
		called = true
	})

	if !called {
		t.Fatalf("expected passthrough policy to restore routing")
	}
}

func TestRoutePolicy_BlockUnblockAndBlockAgain(t *testing.T) {
	policy := NewRoutePolicy()

	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionIgnore,
	})
	blocked := true
	policy.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "blocked"}, func(ctx context.Context, p internal_type.Packet) {
		blocked = false
	})
	if !blocked {
		t.Fatalf("expected packet to be blocked after ignore policy")
	}

	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionPassthrough,
	})
	unblocked := false
	policy.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "unblocked"}, func(ctx context.Context, p internal_type.Packet) {
		unblocked = true
	})
	if !unblocked {
		t.Fatalf("expected packet to route after passthrough policy")
	}

	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionIgnore,
	})
	blockedAgain := true
	policy.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "blocked-again"}, func(ctx context.Context, p internal_type.Packet) {
		blockedAgain = false
	})
	if !blockedAgain {
		t.Fatalf("expected packet to be blocked after ignore policy is applied again")
	}
}

func TestRoutePolicy_UnblockOneTargetDoesNotUnblockAnother(t *testing.T) {
	policy := NewRoutePolicy()
	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionIgnore,
	})
	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameInterruptionDetected,
		Action: internal_type.DispatchActionIgnore,
	})
	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionPassthrough,
	})

	audioCalled := false
	policy.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "audio"}, func(ctx context.Context, p internal_type.Packet) {
		audioCalled = true
	})
	interruptCalled := false
	policy.Route(context.Background(), internal_type.InterruptionDetectedPacket{ContextID: "interrupt"}, func(ctx context.Context, p internal_type.Packet) {
		interruptCalled = true
	})

	if !audioCalled {
		t.Fatalf("expected unblocked audio packet to route")
	}
	if interruptCalled {
		t.Fatalf("expected interruption packet to remain blocked")
	}
}

func TestRoutePolicy_MultiplePoliciesAreIndependent(t *testing.T) {
	policy := NewRoutePolicy()
	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionIgnore,
	})
	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameInterruptionDetected,
		Action: internal_type.DispatchActionIgnore,
	})
	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionPassthrough,
	})

	audioCalled := false
	policy.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "audio"}, func(ctx context.Context, p internal_type.Packet) {
		audioCalled = true
	})
	interruptCalled := false
	policy.Route(context.Background(), internal_type.InterruptionDetectedPacket{ContextID: "interrupt"}, func(ctx context.Context, p internal_type.Packet) {
		interruptCalled = true
	})

	if !audioCalled {
		t.Fatalf("expected audio passthrough to route")
	}
	if interruptCalled {
		t.Fatalf("expected interruption ignore policy to remain active")
	}
}

func TestRoutePolicy_TargetNameMatchesPacketName(t *testing.T) {
	policy := NewRoutePolicy()
	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionIgnore,
	})

	called := false
	policy.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "audio"}, func(ctx context.Context, p internal_type.Packet) {
		called = true
	})

	if called {
		t.Fatalf("expected target name policy to match packet name")
	}
}

func TestRoutePolicy_UnsupportedActionFailsOpen(t *testing.T) {
	policy := NewRoutePolicy()
	policy.Apply(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchAction("unsupported"),
	})

	called := false
	policy.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "audio"}, func(ctx context.Context, p internal_type.Packet) {
		called = true
	})

	if !called {
		t.Fatalf("expected unsupported action to leave packet passthrough")
	}
}
