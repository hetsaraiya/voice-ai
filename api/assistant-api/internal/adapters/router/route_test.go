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

type routeQueueStub struct {
	control    []internal_type.Packet
	bootstrap  []internal_type.Packet
	ingress    []internal_type.Packet
	egress     []internal_type.Packet
	data       []internal_type.Packet
	background []internal_type.Packet
	flushes    []Route
}

func (q *routeQueueStub) FlushControlMatching(match func(internal_type.Packet) bool) int {
	q.flushes = append(q.flushes, RouteControl)
	return flushPacketsMatching(&q.control, match)
}

func (q *routeQueueStub) FlushBootstrapMatching(match func(internal_type.Packet) bool) int {
	q.flushes = append(q.flushes, RouteBootstrap)
	return flushPacketsMatching(&q.bootstrap, match)
}

func (q *routeQueueStub) FlushIngressMatching(match func(internal_type.Packet) bool) int {
	q.flushes = append(q.flushes, RouteIngress)
	return flushPacketsMatching(&q.ingress, match)
}

func (q *routeQueueStub) FlushEgressMatching(match func(internal_type.Packet) bool) int {
	q.flushes = append(q.flushes, RouteEgress)
	return flushPacketsMatching(&q.egress, match)
}

func (q *routeQueueStub) FlushDataMatching(match func(internal_type.Packet) bool) int {
	q.flushes = append(q.flushes, RouteData)
	return flushPacketsMatching(&q.data, match)
}

func (q *routeQueueStub) FlushBackgroundMatching(match func(internal_type.Packet) bool) int {
	q.flushes = append(q.flushes, RouteBackground)
	return flushPacketsMatching(&q.background, match)
}

func flushPacketsMatching(packets *[]internal_type.Packet, match func(internal_type.Packet) bool) int {
	dropped := 0
	keep := make([]internal_type.Packet, 0, len(*packets))
	for _, packet := range *packets {
		if match != nil && match(packet) {
			dropped++
			continue
		}
		keep = append(keep, packet)
	}
	*packets = keep
	return dropped
}

func TestDispatchRoute_RoutesThroughPolicy(t *testing.T) {
	route := NewDispatchRoute(NewRoutePolicy(), &routeQueueStub{})
	packet := internal_type.UserTextReceivedPacket{ContextID: "text", Text: "hello"}

	var routed internal_type.Packet
	route.Route(context.Background(), packet, func(ctx context.Context, p internal_type.Packet) {
		routed = p
	})

	if routed == nil {
		t.Fatalf("expected route to call next")
	}
	if routed.ContextId() != packet.ContextId() {
		t.Fatalf("routed packet mismatch: got=%s want=%s", routed.ContextId(), packet.ContextId())
	}
}

func TestDispatchRoute_ApplyIgnorePolicyDrainsTargetRouteAndBlocksFuturePackets(t *testing.T) {
	queue := &routeQueueStub{
		ingress: []internal_type.Packet{
			internal_type.UserAudioReceivedPacket{ContextID: "drop"},
			internal_type.UserTextReceivedPacket{ContextID: "keep"},
		},
		egress: []internal_type.Packet{
			internal_type.UserAudioReceivedPacket{ContextID: "wrong-route"},
		},
	}
	route := NewDispatchRoute(NewRoutePolicy(), queue)

	route.ApplyPolicy(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionIgnore,
	})

	if len(queue.flushes) != 1 || queue.flushes[0] != RouteIngress {
		t.Fatalf("expected only ingress flush, got=%v", queue.flushes)
	}
	if len(queue.ingress) != 1 {
		t.Fatalf("expected one ingress packet to remain, got=%d", len(queue.ingress))
	}
	if queue.ingress[0].ContextId() != "keep" {
		t.Fatalf("expected unrelated ingress packet to remain, got=%s", queue.ingress[0].ContextId())
	}
	if len(queue.egress) != 1 {
		t.Fatalf("expected matching packet on non-target route to remain, got=%d", len(queue.egress))
	}

	called := false
	route.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "future"}, func(ctx context.Context, p internal_type.Packet) {
		called = true
	})
	if called {
		t.Fatalf("expected future matching packet to be blocked")
	}
}

func TestDispatchRoute_ApplyPassthroughRestoresFuturePacketsWithoutDraining(t *testing.T) {
	queue := &routeQueueStub{
		ingress: []internal_type.Packet{
			internal_type.UserAudioReceivedPacket{ContextID: "queued"},
		},
	}
	route := NewDispatchRoute(NewRoutePolicy(), queue)
	route.ApplyPolicy(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionIgnore,
	})
	route.ApplyPolicy(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionPassthrough,
	})

	if len(queue.flushes) != 1 || queue.flushes[0] != RouteIngress {
		t.Fatalf("expected only ignore policy to flush ingress, got=%v", queue.flushes)
	}

	called := false
	route.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "future"}, func(ctx context.Context, p internal_type.Packet) {
		called = true
	})
	if !called {
		t.Fatalf("expected passthrough policy to restore future packet routing")
	}
}

func TestDispatchRoute_BlockUnblockAndBlockAgainLifecycle(t *testing.T) {
	queue := &routeQueueStub{
		ingress: []internal_type.Packet{
			internal_type.UserAudioReceivedPacket{ContextID: "first-block-drain"},
			internal_type.UserTextReceivedPacket{ContextID: "first-block-keep"},
		},
	}
	route := NewDispatchRoute(NewRoutePolicy(), queue)

	route.ApplyPolicy(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionIgnore,
	})
	if len(queue.flushes) != 1 || queue.flushes[0] != RouteIngress {
		t.Fatalf("expected first block to drain ingress, got=%v", queue.flushes)
	}
	if len(queue.ingress) != 1 || queue.ingress[0].ContextId() != "first-block-keep" {
		t.Fatalf("expected first block to keep unrelated packet, got=%v", queue.ingress)
	}
	firstBlocked := true
	route.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "blocked"}, func(ctx context.Context, p internal_type.Packet) {
		firstBlocked = false
	})
	if !firstBlocked {
		t.Fatalf("expected future packet to be blocked after first block")
	}

	route.ApplyPolicy(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionPassthrough,
	})
	if len(queue.flushes) != 1 {
		t.Fatalf("expected unblock not to drain queued packets again, got=%v", queue.flushes)
	}
	unblocked := false
	route.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "unblocked"}, func(ctx context.Context, p internal_type.Packet) {
		unblocked = true
	})
	if !unblocked {
		t.Fatalf("expected future packet to route after unblock")
	}

	queue.ingress = append(queue.ingress, internal_type.UserAudioReceivedPacket{ContextID: "second-block-drain"})
	route.ApplyPolicy(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionIgnore,
	})
	if len(queue.flushes) != 2 || queue.flushes[1] != RouteIngress {
		t.Fatalf("expected second block to drain ingress again, got=%v", queue.flushes)
	}
	if len(queue.ingress) != 1 || queue.ingress[0].ContextId() != "first-block-keep" {
		t.Fatalf("expected second block to keep only unrelated packet, got=%v", queue.ingress)
	}
	blockedAgain := true
	route.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "blocked-again"}, func(ctx context.Context, p internal_type.Packet) {
		blockedAgain = false
	})
	if !blockedAgain {
		t.Fatalf("expected future packet to be blocked after second block")
	}
}

func TestDispatchRoute_UnblockOnePolicyKeepsOtherPolicyBlocked(t *testing.T) {
	queue := &routeQueueStub{
		ingress: []internal_type.Packet{
			internal_type.UserAudioReceivedPacket{ContextID: "audio-drop"},
		},
		control: []internal_type.Packet{
			internal_type.InterruptionDetectedPacket{ContextID: "interrupt-drop"},
		},
	}
	route := NewDispatchRoute(NewRoutePolicy(), queue)

	route.ApplyPolicy(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionIgnore,
	})
	route.ApplyPolicy(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameInterruptionDetected,
		Action: internal_type.DispatchActionIgnore,
	})
	route.ApplyPolicy(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchActionPassthrough,
	})

	if len(queue.flushes) != 2 {
		t.Fatalf("expected only block policies to drain, got=%v", queue.flushes)
	}

	audioCalled := false
	route.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "audio"}, func(ctx context.Context, p internal_type.Packet) {
		audioCalled = true
	})
	interruptCalled := false
	route.Route(context.Background(), internal_type.InterruptionDetectedPacket{ContextID: "interrupt"}, func(ctx context.Context, p internal_type.Packet) {
		interruptCalled = true
	})

	if !audioCalled {
		t.Fatalf("expected unblocked audio packet to route")
	}
	if interruptCalled {
		t.Fatalf("expected interruption packet to remain blocked")
	}
}

func TestDispatchRoute_UnsupportedActionDoesNotDrainAndFailsOpen(t *testing.T) {
	queue := &routeQueueStub{
		ingress: []internal_type.Packet{
			internal_type.UserAudioReceivedPacket{ContextID: "queued"},
		},
	}
	route := NewDispatchRoute(NewRoutePolicy(), queue)
	route.ApplyPolicy(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameUserAudioReceived,
		Action: internal_type.DispatchAction("unsupported"),
	})

	if len(queue.flushes) != 0 {
		t.Fatalf("expected unsupported action not to flush, got=%v", queue.flushes)
	}
	if len(queue.ingress) != 1 {
		t.Fatalf("expected queued packet to remain, got=%d", len(queue.ingress))
	}

	called := false
	route.Route(context.Background(), internal_type.UserAudioReceivedPacket{ContextID: "future"}, func(ctx context.Context, p internal_type.Packet) {
		called = true
	})
	if !called {
		t.Fatalf("expected unsupported action to leave future packet passthrough")
	}
}

func TestDispatchRoute_DispatchPolicyPacketCannotBeTargeted(t *testing.T) {
	queue := &routeQueueStub{
		control: []internal_type.Packet{
			internal_type.DispatchPolicyPacket{ContextID: "queued"},
		},
	}
	route := NewDispatchRoute(NewRoutePolicy(), queue)

	route.ApplyPolicy(internal_type.DispatchPolicy{
		Target: internal_type.PacketNameDispatchPolicy,
		Action: internal_type.DispatchActionIgnore,
	})

	if len(queue.flushes) != 0 {
		t.Fatalf("expected dispatch policy target not to flush, got=%v", queue.flushes)
	}
	if len(queue.control) != 1 {
		t.Fatalf("expected queued dispatch policy packet to remain, got=%d", len(queue.control))
	}

	called := false
	route.Route(context.Background(), internal_type.DispatchPolicyPacket{
		ContextID: "future",
		Policy: internal_type.DispatchPolicy{
			Target: internal_type.PacketNameUserAudioReceived,
			Action: internal_type.DispatchActionIgnore,
		},
	}, func(ctx context.Context, p internal_type.Packet) {
		called = true
	})
	if !called {
		t.Fatalf("expected dispatch policy packet to remain routable")
	}
}

func TestDispatchRoute_ApplyIgnorePolicyDrainsByClassifiedRoute(t *testing.T) {
	cases := []struct {
		name       string
		target     internal_type.PacketName
		seedQueue  func(*routeQueueStub)
		remaining  func(*routeQueueStub) int
		wantFlush  Route
		wantRemain int
	}{
		{
			name:   "control",
			target: internal_type.PacketNameInterruptionDetected,
			seedQueue: func(q *routeQueueStub) {
				q.control = []internal_type.Packet{
					internal_type.InterruptionDetectedPacket{ContextID: "drop"},
					internal_type.TurnChangePacket{ContextID: "keep"},
				}
			},
			remaining:  func(q *routeQueueStub) int { return len(q.control) },
			wantFlush:  RouteControl,
			wantRemain: 1,
		},
		{
			name:   "bootstrap",
			target: internal_type.PacketNameInitializeAssistant,
			seedQueue: func(q *routeQueueStub) {
				q.bootstrap = []internal_type.Packet{
					internal_type.InitializeAssistantPacket{ContextID: "drop"},
					internal_type.InitializeConversationPacket{ContextID: "keep"},
				}
			},
			remaining:  func(q *routeQueueStub) int { return len(q.bootstrap) },
			wantFlush:  RouteBootstrap,
			wantRemain: 1,
		},
		{
			name:   "ingress",
			target: internal_type.PacketNameUserAudioReceived,
			seedQueue: func(q *routeQueueStub) {
				q.ingress = []internal_type.Packet{
					internal_type.UserAudioReceivedPacket{ContextID: "drop"},
					internal_type.UserTextReceivedPacket{ContextID: "keep"},
				}
			},
			remaining:  func(q *routeQueueStub) int { return len(q.ingress) },
			wantFlush:  RouteIngress,
			wantRemain: 1,
		},
		{
			name:   "egress",
			target: internal_type.PacketNameTextToSpeechText,
			seedQueue: func(q *routeQueueStub) {
				q.egress = []internal_type.Packet{
					internal_type.TextToSpeechTextPacket{ContextID: "drop"},
					internal_type.TextToSpeechDonePacket{ContextID: "keep"},
				}
			},
			remaining:  func(q *routeQueueStub) int { return len(q.egress) },
			wantFlush:  RouteEgress,
			wantRemain: 1,
		},
		{
			name:   "data",
			target: internal_type.PacketNameMessageCreate,
			seedQueue: func(q *routeQueueStub) {
				q.data = []internal_type.Packet{
					internal_type.MessageCreatePacket{ContextID: "drop"},
					internal_type.ToolLogCreatePacket{ContextID: "keep"},
				}
			},
			remaining:  func(q *routeQueueStub) int { return len(q.data) },
			wantFlush:  RouteData,
			wantRemain: 1,
		},
		{
			name:   "background",
			target: internal_type.PacketNameObservabilityEventRecord,
			seedQueue: func(q *routeQueueStub) {
				q.background = []internal_type.Packet{
					internal_type.ObservabilityEventRecordPacket{ContextID: "drop"},
					internal_type.ObservabilityMetricRecordPacket{ContextID: "keep"},
				}
			},
			remaining:  func(q *routeQueueStub) int { return len(q.background) },
			wantFlush:  RouteBackground,
			wantRemain: 1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			queue := &routeQueueStub{}
			tc.seedQueue(queue)
			route := NewDispatchRoute(NewRoutePolicy(), queue)

			route.ApplyPolicy(internal_type.DispatchPolicy{
				Target: tc.target,
				Action: internal_type.DispatchActionIgnore,
			})

			if len(queue.flushes) != 1 || queue.flushes[0] != tc.wantFlush {
				t.Fatalf("flush route mismatch: got=%v want=%v", queue.flushes, tc.wantFlush)
			}
			if got := tc.remaining(queue); got != tc.wantRemain {
				t.Fatalf("remaining packet count mismatch: got=%d want=%d", got, tc.wantRemain)
			}
		})
	}
}
