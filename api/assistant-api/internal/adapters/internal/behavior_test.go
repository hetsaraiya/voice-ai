package adapter_internal

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	adapter_channel "github.com/rapidaai/api/assistant-api/internal/adapters/channel"
	adapter_lifecycle "github.com/rapidaai/api/assistant-api/internal/adapters/lifecycle"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_conversation_entity "github.com/rapidaai/api/assistant-api/internal/entity/conversations"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	gorm_model "github.com/rapidaai/pkg/models/gorm"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type behaviorCapturingStreamer struct {
	ctx    context.Context
	onSend func(internal_type.Stream)
	mu     sync.Mutex
	sent   []internal_type.Stream
}

func (s *behaviorCapturingStreamer) Context() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *behaviorCapturingStreamer) Recv() (internal_type.Stream, error) {
	return nil, io.EOF
}

func (s *behaviorCapturingStreamer) Send(msg internal_type.Stream) error {
	s.mu.Lock()
	s.sent = append(s.sent, msg)
	s.mu.Unlock()
	if s.onSend != nil {
		s.onSend(msg)
	}
	return nil
}

func (s *behaviorCapturingStreamer) hasDisconnect(reason protos.ConversationDisconnection_DisconnectionType) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, msg := range s.sent {
		disconnect, ok := msg.(*protos.ConversationDisconnection)
		if ok && disconnect.GetType() == reason {
			return true
		}
	}
	return false
}

func behaviorTestLogger(t *testing.T) commons.Logger {
	t.Helper()
	logger, err := commons.NewApplicationLogger(
		commons.Name("behavior-test"),
		commons.Level("error"),
		commons.EnableFile(false),
	)
	require.NoError(t, err)
	return logger
}

func behaviorU64(v uint64) *uint64 {
	return &v
}

func behaviorStr(v string) *string {
	return &v
}

// Test requestor with only the fields behavior methods need.
func newBehaviorDisconnectTestRequestor(t *testing.T, streamer internal_type.Streamer) *genericRequestor {
	t.Helper()
	sessionCtx, cancelSession := context.WithCancel(context.Background())
	t.Cleanup(cancelSession)

	messageLifecycle := adapter_lifecycle.NewMessageLifecycle()
	messageLifecycle.SetContextID("ctx-behavior")

	return &genericRequestor{
		logger:           behaviorTestLogger(t),
		source:           utils.Debugger,
		streamer:         streamer,
		channels:         adapter_channel.NewRequestorChannels(),
		messageLifecycle: messageLifecycle,
		sessionLifecycle: adapter_lifecycle.NewSessionLifecycle(),
		sessionCtx:       sessionCtx,
		cancelSession:    cancelSession,
		assistant: &internal_assistant_entity.Assistant{
			Audited: gorm_model.Audited{Id: 101},
			AssistantDebuggerDeployment: &internal_assistant_entity.AssistantDebuggerDeployment{
				AssistantDeploymentBehavior: internal_assistant_entity.AssistantDeploymentBehavior{},
			},
		},
		assistantConversation: &internal_conversation_entity.AssistantConversation{
			Audited: gorm_model.Audited{Id: 202},
		},
	}
}

func setBehaviorForSource(
	r *genericRequestor,
	source utils.RapidaSource,
	behavior internal_assistant_entity.AssistantDeploymentBehavior,
) {
	r.source = source
	if r.assistant == nil {
		r.assistant = &internal_assistant_entity.Assistant{}
	}

	switch source {
	case utils.PhoneCall:
		r.assistant.AssistantPhoneDeployment = &internal_assistant_entity.AssistantPhoneDeployment{
			AssistantDeploymentBehavior: behavior,
		}
	case utils.Whatsapp:
		r.assistant.AssistantWhatsappDeployment = &internal_assistant_entity.AssistantWhatsappDeployment{
			AssistantDeploymentBehavior: behavior,
		}
	case utils.SDK:
		r.assistant.AssistantApiDeployment = &internal_assistant_entity.AssistantApiDeployment{
			AssistantDeploymentBehavior: behavior,
		}
	case utils.WebPlugin:
		r.assistant.AssistantWebPluginDeployment = &internal_assistant_entity.AssistantWebPluginDeployment{
			AssistantDeploymentBehavior: behavior,
		}
	case utils.Debugger:
		r.assistant.AssistantDebuggerDeployment = &internal_assistant_entity.AssistantDebuggerDeployment{
			AssistantDeploymentBehavior: behavior,
		}
	}
}

func collectBehaviorPackets(ch chan adapter_channel.Envelope) []internal_type.Packet {
	packets := make([]internal_type.Packet, 0)
	for {
		select {
		case env := <-ch:
			packets = append(packets, env.Pkt)
		default:
			return packets
		}
	}
}

func countBehaviorPacketType[T internal_type.Packet](packets []internal_type.Packet) int {
	count := 0
	for _, packet := range packets {
		if _, ok := packet.(T); ok {
			count++
		}
	}
	return count
}

func requireBehaviorPacketEventually[T internal_type.Packet](
	t *testing.T,
	ch <-chan adapter_channel.Envelope,
	timeout time.Duration,
) T {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case env := <-ch:
			packet, ok := env.Pkt.(T)
			if ok {
				return packet
			}
		case <-deadline:
			t.Fatalf("expected packet %T", *new(T))
			var zero T
			return zero
		}
	}
}

type behaviorDisconnectPacketCapture struct {
	event    internal_type.ObservabilityEventRecordPacket
	metadata internal_type.ObservabilityMetadataRecordPacket
	err      string
}

// Captures disconnect packets at the exact moment Notify writes to the stream.
func captureBehaviorDisconnectPacketsBeforeNotify(r *genericRequestor) behaviorDisconnectPacketCapture {
	var capture behaviorDisconnectPacketCapture
	select {
	case env := <-r.channels.BackgroundChannel():
		event, ok := env.Pkt.(internal_type.ObservabilityEventRecordPacket)
		if !ok {
			capture.err = "background packet was not ObservabilityEventRecordPacket"
			return capture
		}
		capture.event = event
	default:
		capture.err = "missing ObservabilityEventRecordPacket before notify"
		return capture
	}

	select {
	case env := <-r.channels.BackgroundChannel():
		metadata, ok := env.Pkt.(internal_type.ObservabilityMetadataRecordPacket)
		if !ok {
			capture.err = "background packet was not ObservabilityMetadataRecordPacket"
			return capture
		}
		capture.metadata = metadata
	default:
		capture.err = "missing ObservabilityMetadataRecordPacket before notify"
	}
	return capture
}

func requireBehaviorDisconnectCapture(t *testing.T, ch <-chan behaviorDisconnectPacketCapture) behaviorDisconnectPacketCapture {
	t.Helper()
	select {
	case capture := <-ch:
		require.Empty(t, capture.err)
		return capture
	case <-time.After(2 * time.Second):
		t.Fatal("expected disconnect notify")
		var zero behaviorDisconnectPacketCapture
		return zero
	}
}

func assertDisconnectReasonPackets(
	t *testing.T,
	capture behaviorDisconnectPacketCapture,
	reason protos.ConversationDisconnection_DisconnectionType,
) {
	t.Helper()

	event := capture.event
	assert.Equal(t, "ctx-behavior", event.ContextID)
	assert.Equal(t, observability.ConversationCompleted, event.Record.Event)
	assert.Equal(t, reason.String(), event.Record.Attributes["reason"])

	metadata := capture.metadata
	require.Len(t, metadata.Record.Metadata, 1)
	assert.Equal(t, "ctx-behavior", metadata.ContextID)
	assert.Equal(t, "disconnect_reason", metadata.Record.Metadata[0].Key)
	assert.Equal(t, reason.String(), metadata.Record.Metadata[0].Value)
}

// Max duration should persist the disconnect reason before notifying the client.
func TestBehavior_InitializeMaxSessionDuration_QueuesDisconnectReasonBeforeNotify(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	streamer := &behaviorCapturingStreamer{ctx: ctx}
	r := newBehaviorDisconnectTestRequestor(t, streamer)
	reason := protos.ConversationDisconnection_DISCONNECTION_TYPE_MAX_DURATION
	captureCh := make(chan behaviorDisconnectPacketCapture, 1)
	streamer.onSend = func(msg internal_type.Stream) {
		disconnect, ok := msg.(*protos.ConversationDisconnection)
		if ok && disconnect.GetType() == reason {
			captureCh <- captureBehaviorDisconnectPacketsBeforeNotify(r)
		}
	}

	r.initializeMaxSessionDuration(ctx, &internal_assistant_entity.AssistantDeploymentBehavior{
		MaxSessionDuration: behaviorU64(1),
	})
	require.NotNil(t, r.maxSessionTimer)
	defer r.maxSessionTimer.Stop()

	assertDisconnectReasonPackets(t, requireBehaviorDisconnectCapture(t, captureCh), reason)
}

// Idle timeout at backoff limit should persist IDLE_TIMEOUT before notifying.
func TestBehavior_OnIdleTimeout_BackoffReached_QueuesIdleDisconnectReasonBeforeNotify(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	streamer := &behaviorCapturingStreamer{ctx: ctx}
	r := newBehaviorDisconnectTestRequestor(t, streamer)
	backoff := uint64(2)
	r.assistant.AssistantDebuggerDeployment.AssistantDeploymentBehavior = internal_assistant_entity.AssistantDeploymentBehavior{
		IdleTimeout:        behaviorU64(1),
		IdleTimeoutBackoff: &backoff,
	}
	r.idleTimeoutCount = backoff

	reason := protos.ConversationDisconnection_DISCONNECTION_TYPE_IDLE_TIMEOUT
	captureCh := make(chan behaviorDisconnectPacketCapture, 1)
	streamer.onSend = func(msg internal_type.Stream) {
		disconnect, ok := msg.(*protos.ConversationDisconnection)
		if ok && disconnect.GetType() == reason {
			captureCh <- captureBehaviorDisconnectPacketsBeforeNotify(r)
		}
	}

	require.NoError(t, r.onIdleTimeout(ctx))

	assertDisconnectReasonPackets(t, requireBehaviorDisconnectCapture(t, captureCh), reason)
}

// Behavior config should be selected from the deployment matching the source.
func TestBehavior_GetBehavior(t *testing.T) {
	t.Run("assistant nil", func(t *testing.T) {
		r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
		r.assistant = nil

		behavior, err := r.GetBehavior()

		require.ErrorIs(t, err, errDeploymentNotEnabled)
		assert.Nil(t, behavior)
	})

	t.Run("by source", func(t *testing.T) {
		cases := []struct {
			name   string
			source utils.RapidaSource
		}{
			{name: "phone", source: utils.PhoneCall},
			{name: "whatsapp", source: utils.Whatsapp},
			{name: "sdk", source: utils.SDK},
			{name: "web plugin", source: utils.WebPlugin},
			{name: "debugger", source: utils.Debugger},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
				greeting := tc.name + " greeting"
				setBehaviorForSource(r, tc.source, internal_assistant_entity.AssistantDeploymentBehavior{
					Greeting: behaviorStr(greeting),
				})

				behavior, err := r.GetBehavior()

				require.NoError(t, err)
				require.NotNil(t, behavior)
				require.NotNil(t, behavior.Greeting)
				assert.Equal(t, greeting, *behavior.Greeting)
			})
		}
	})

	t.Run("deployment missing for source", func(t *testing.T) {
		for _, source := range []utils.RapidaSource{
			utils.PhoneCall,
			utils.Whatsapp,
			utils.SDK,
			utils.WebPlugin,
			utils.Debugger,
		} {
			r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
			r.assistant = &internal_assistant_entity.Assistant{}
			r.source = source

			behavior, err := r.GetBehavior()

			require.ErrorIs(t, err, errDeploymentNotEnabled)
			assert.Nil(t, behavior)
		}
	})

	t.Run("unknown source", func(t *testing.T) {
		r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
		r.assistant = &internal_assistant_entity.Assistant{}
		r.source = utils.RapidaSource("unknown-source")

		behavior, err := r.GetBehavior()

		require.ErrorIs(t, err, errDeploymentNotEnabled)
		assert.Nil(t, behavior)
	})
}

// Greeting should be ignored when empty and queued when configured.
func TestBehavior_InitializeGreeting(t *testing.T) {
	cases := []struct {
		name     string
		greeting *string
	}{
		{name: "nil greeting", greeting: nil},
		{name: "whitespace greeting", greeting: behaviorStr("   \t\n  ")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})

			r.initializeGreeting(context.Background(), &internal_assistant_entity.AssistantDeploymentBehavior{
				Greeting: tc.greeting,
			})

			assert.Zero(t, len(r.channels.EgressChannel()))
			assert.Zero(t, len(r.channels.BackgroundChannel()))
		})
	}

	t.Run("valid greeting enqueues inject, event, and idle restart", func(t *testing.T) {
		r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
		r.messageLifecycle.SetContextID("ctx-greeting")

		r.initializeGreeting(context.Background(), &internal_assistant_entity.AssistantDeploymentBehavior{
			Greeting: behaviorStr("Welcome!"),
		})

		inject := requireBehaviorPacketEventually[internal_type.InjectMessagePacket](
			t, r.channels.EgressChannel(), time.Second,
		)
		assert.Equal(t, "ctx-greeting", inject.ContextID)
		assert.Equal(t, "Welcome!", inject.Text)

		start := requireBehaviorPacketEventually[internal_type.StartIdleTimeoutPacket](
			t, r.channels.EgressChannel(), time.Second,
		)
		assert.Equal(t, "ctx-greeting", start.ContextID)

		event := requireBehaviorPacketEventually[internal_type.ObservabilityEventRecordPacket](
			t, r.channels.BackgroundChannel(), time.Second,
		)
		assert.Equal(t, observability.ConversationAgentStateChanged, event.Record.Event)
		assert.Equal(t, "greeting", event.Record.Attributes["type"])
		assert.Equal(t, "8", event.Record.Attributes["text_chars"])
	})
}

// Idle timeout initialization should only enqueue a timer start when configured.
func TestBehavior_InitializeIdleTimeout(t *testing.T) {
	cases := []struct {
		name    string
		timeout *uint64
	}{
		{name: "nil timeout", timeout: nil},
		{name: "zero timeout", timeout: behaviorU64(0)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})

			r.initializeIdleTimeout(context.Background(), &internal_assistant_entity.AssistantDeploymentBehavior{
				IdleTimeout: tc.timeout,
			})

			assert.Zero(t, len(r.channels.EgressChannel()))
		})
	}

	t.Run("configured timeout enqueues start packet", func(t *testing.T) {
		r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
		r.messageLifecycle.SetContextID("ctx-idle")

		r.initializeIdleTimeout(context.Background(), &internal_assistant_entity.AssistantDeploymentBehavior{
			IdleTimeout: behaviorU64(5),
		})

		start := requireBehaviorPacketEventually[internal_type.StartIdleTimeoutPacket](
			t, r.channels.EgressChannel(), time.Second,
		)
		assert.Equal(t, "ctx-idle", start.ContextID)
	})
}

// Missing max duration should not create a timer.
func TestBehavior_InitializeMaxSessionDuration_NotConfigured_NoTimer(t *testing.T) {
	cases := []struct {
		name string
		max  *uint64
	}{
		{name: "nil duration", max: nil},
		{name: "zero duration", max: behaviorU64(0)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})

			r.initializeMaxSessionDuration(context.Background(), &internal_assistant_entity.AssistantDeploymentBehavior{
				MaxSessionDuration: tc.max,
			})

			assert.Nil(t, r.maxSessionTimer)
		})
	}
}

// Error behavior should rotate context and enqueue recovery prompt packets.
func TestBehavior_OnError(t *testing.T) {
	t.Run("no behavior configured is no-op", func(t *testing.T) {
		r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
		r.assistant = nil

		require.NoError(t, r.OnError(context.Background()))
		assert.Zero(t, len(r.channels.ControlChannel()))
		assert.Zero(t, len(r.channels.EgressChannel()))
		assert.Zero(t, len(r.channels.BackgroundChannel()))
	})

	t.Run("default mistake message", func(t *testing.T) {
		r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
		r.messageLifecycle.SetContextID("ctx-before-error")
		require.NoError(t, r.Transition(LLMGenerating))
		setBehaviorForSource(r, utils.Debugger, internal_assistant_entity.AssistantDeploymentBehavior{})

		require.NoError(t, r.OnError(context.Background()))

		newContextID := r.GetID()
		assert.NotEqual(t, "ctx-before-error", newContextID)

		interrupt := requireBehaviorPacketEventually[internal_type.TextToSpeechInterruptPacket](
			t, r.channels.ControlChannel(), time.Second,
		)
		assert.Equal(t, newContextID, interrupt.ContextID)

		inject := requireBehaviorPacketEventually[internal_type.InjectMessagePacket](
			t, r.channels.EgressChannel(), time.Second,
		)
		assert.Equal(t, newContextID, inject.ContextID)
		assert.NotEmpty(t, inject.Text)

		start := requireBehaviorPacketEventually[internal_type.StartIdleTimeoutPacket](
			t, r.channels.EgressChannel(), time.Second,
		)
		assert.Equal(t, newContextID, start.ContextID)

		event := requireBehaviorPacketEventually[internal_type.ObservabilityEventRecordPacket](
			t, r.channels.BackgroundChannel(), time.Second,
		)
		assert.Equal(t, observability.ConversationAgentStateChanged, event.Record.Event)
		assert.Equal(t, "error", event.Record.Attributes["type"])
		assert.Equal(t, fmt.Sprintf("%d", len(inject.Text)), event.Record.Attributes["text_chars"])
	})

	t.Run("configured mistake message", func(t *testing.T) {
		r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
		require.NoError(t, r.Transition(LLMGenerating))
		custom := "Custom error message"
		setBehaviorForSource(r, utils.Debugger, internal_assistant_entity.AssistantDeploymentBehavior{
			Mistake: behaviorStr(custom),
		})

		require.NoError(t, r.OnError(context.Background()))

		inject := requireBehaviorPacketEventually[internal_type.InjectMessagePacket](
			t, r.channels.EgressChannel(), time.Second,
		)
		assert.Equal(t, custom, inject.Text)
	})
}

// Idle timeout should no-op when disabled and prompt until backoff is reached.
func TestBehavior_OnIdleTimeout(t *testing.T) {
	t.Run("no behavior configured is no-op", func(t *testing.T) {
		r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
		r.assistant = nil

		require.NoError(t, r.onIdleTimeout(context.Background()))
		assert.Equal(t, uint64(0), r.idleTimeoutCount)
		assert.Zero(t, len(r.channels.ControlChannel()))
		assert.Zero(t, len(r.channels.EgressChannel()))
	})

	t.Run("idle timeout not configured is no-op", func(t *testing.T) {
		r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
		setBehaviorForSource(r, utils.Debugger, internal_assistant_entity.AssistantDeploymentBehavior{})

		require.NoError(t, r.onIdleTimeout(context.Background()))
		assert.Equal(t, uint64(0), r.idleTimeoutCount)
		assert.Zero(t, len(r.channels.ControlChannel()))
		assert.Zero(t, len(r.channels.EgressChannel()))
		assert.Zero(t, len(r.channels.BackgroundChannel()))
	})

	t.Run("configured retry enqueues interrupt, prompt, event, and restart", func(t *testing.T) {
		r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
		r.messageLifecycle.SetContextID("ctx-before-idle")
		require.NoError(t, r.Transition(LLMGenerated))
		backoff := uint64(4)
		setBehaviorForSource(r, utils.Debugger, internal_assistant_entity.AssistantDeploymentBehavior{
			IdleTimeout:        behaviorU64(1),
			IdleTimeoutBackoff: &backoff,
			IdleTimeoutMessage: behaviorStr("Still there?"),
		})
		r.idleTimeoutCount = 1

		require.NoError(t, r.onIdleTimeout(context.Background()))

		assert.Equal(t, uint64(2), r.idleTimeoutCount)
		newContextID := r.GetID()
		assert.NotEqual(t, "ctx-before-idle", newContextID)

		interrupt := requireBehaviorPacketEventually[internal_type.TextToSpeechInterruptPacket](
			t, r.channels.ControlChannel(), time.Second,
		)
		assert.Equal(t, newContextID, interrupt.ContextID)

		inject := requireBehaviorPacketEventually[internal_type.InjectMessagePacket](
			t, r.channels.EgressChannel(), time.Second,
		)
		assert.Equal(t, newContextID, inject.ContextID)
		assert.Equal(t, "Still there?", inject.Text)

		start := requireBehaviorPacketEventually[internal_type.StartIdleTimeoutPacket](
			t, r.channels.EgressChannel(), time.Second,
		)
		assert.Equal(t, newContextID, start.ContextID)

		event := requireBehaviorPacketEventually[internal_type.ObservabilityEventRecordPacket](
			t, r.channels.BackgroundChannel(), time.Second,
		)
		assert.Equal(t, observability.ConversationAgentStateChanged, event.Record.Event)
		assert.Equal(t, "idle_timeout", event.Record.Attributes["type"])
		assert.Equal(t, "2", event.Record.Attributes["count"])
		assert.Equal(t, "4", event.Record.Attributes["max_count"])
	})
}

// Idle timeout messages should use the configured value or the default fallback.
func TestBehavior_GetIdleTimeoutMessage(t *testing.T) {
	r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
	cases := []struct {
		name     string
		message  *string
		expected string
	}{
		{name: "nil message", message: nil, expected: "Are you still there?"},
		{name: "blank message", message: behaviorStr("   \n \t "), expected: "Are you still there?"},
		{name: "configured message", message: behaviorStr("Ping!"), expected: "Ping!"},
		{name: "configured with spaces", message: behaviorStr("  ping  "), expected: "  ping  "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := r.getIdleTimeoutMessage(&internal_assistant_entity.AssistantDeploymentBehavior{
				IdleTimeoutMessage: tc.message,
			})
			assert.Equal(t, tc.expected, got)
		})
	}
}

// Timer extension should move the deadline only when a positive extension exists.
func TestBehavior_ExtendIdleTimeoutTimer(t *testing.T) {
	t.Run("no-op cases", func(t *testing.T) {
		r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
		originalDeadline := time.Now().Add(2 * time.Second)
		r.idleTimeoutDeadline = originalDeadline

		r.extendIdleTimeoutTimer(time.Second)

		assert.WithinDuration(t, originalDeadline, r.idleTimeoutDeadline, 5*time.Millisecond)

		originalDeadline = time.Now().Add(200 * time.Millisecond)
		r.idleTimeoutDeadline = originalDeadline
		r.idleTimeoutTimer = time.AfterFunc(200*time.Millisecond, func() {})
		defer r.idleTimeoutTimer.Stop()

		r.extendIdleTimeoutTimer(0)
		assert.WithinDuration(t, originalDeadline, r.idleTimeoutDeadline, 5*time.Millisecond)
		r.extendIdleTimeoutTimer(-50 * time.Millisecond)
		assert.WithinDuration(t, originalDeadline, r.idleTimeoutDeadline, 5*time.Millisecond)
	})

	t.Run("extends deadline and delays timer", func(t *testing.T) {
		r := newBehaviorDisconnectTestRequestor(t, &behaviorCapturingStreamer{})
		fired := make(chan time.Time, 1)
		initial := 120 * time.Millisecond
		extension := 180 * time.Millisecond

		r.idleTimeoutDeadline = time.Now().Add(initial)
		before := r.idleTimeoutDeadline
		r.idleTimeoutTimer = time.AfterFunc(initial, func() {
			fired <- time.Now()
		})
		defer r.idleTimeoutTimer.Stop()

		r.extendIdleTimeoutTimer(extension)

		assert.WithinDuration(t, before.Add(extension), r.idleTimeoutDeadline, 20*time.Millisecond)
		select {
		case <-fired:
			t.Fatal("idle timeout fired before extended deadline")
		case <-time.After(180 * time.Millisecond):
		}

		select {
		case <-fired:
		case <-time.After(260 * time.Millisecond):
			t.Fatal("idle timeout did not fire after extension")
		}
	})
}
