package router

import (
	"testing"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
)

type unroutedPacket struct {
	contextID string
}

func (p unroutedPacket) ContextId() string { return p.contextID }

func TestClassify(t *testing.T) {
	type testCase struct {
		name      string
		pkt       internal_type.Packet
		wantRoute Route
		wantOK    bool
	}

	cases := []testCase{
		{
			name:      "control",
			pkt:       internal_type.TurnChangePacket{ContextID: "c"},
			wantRoute: RouteControl,
			wantOK:    true,
		},
		{
			name:      "bootstrap",
			pkt:       internal_type.InitializeAssistantPacket{ContextID: "c"},
			wantRoute: RouteBootstrap,
			wantOK:    true,
		},
		{
			name:      "ingress",
			pkt:       internal_type.UserTextReceivedPacket{ContextID: "c", Text: "hi"},
			wantRoute: RouteIngress,
			wantOK:    true,
		},
		{
			name:      "egress",
			pkt:       internal_type.TextToSpeechTextPacket{ContextID: "c", Text: "hi"},
			wantRoute: RouteEgress,
			wantOK:    true,
		},
		{
			name:      "egress-mode-switch-error",
			pkt:       internal_type.ModeSwitchErrorPacket{ContextID: "c"},
			wantRoute: RouteEgress,
			wantOK:    true,
		},
		{
			name:      "background",
			pkt:       internal_type.ObservabilityEventRecordPacket{ContextID: "c"},
			wantRoute: RouteBackground,
			wantOK:    true,
		},
		{
			name:      "background-finalize",
			pkt:       internal_type.FinalizeBehaviorPacket{ContextID: "c"},
			wantRoute: RouteData,
			wantOK:    true,
		},
		{
			name:      "fallback-background-for-unrouted-packet",
			pkt:       unroutedPacket{contextID: "c"},
			wantRoute: RouteBackground,
			wantOK:    false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotRoute := Classify(tc.pkt)
			if gotRoute != tc.wantRoute {
				t.Fatalf("route mismatch: got=%v want=%v", gotRoute, tc.wantRoute)
			}
		})
	}
}
