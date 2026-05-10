package adapter_internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	adapter_channel "github.com/rapidaai/api/assistant-api/internal/adapters/channel"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func behaviorStrPtr(v string) *string {
	return &v
}

func behaviorU64Ptr(v uint64) *uint64 {
	return &v
}

func newBehaviorTestRequestor(t *testing.T) *genericRequestor {
	t.Helper()
	r := newTestRequestor(t, context.Background())
	r.source = utils.Debugger
	return r
}

func setBehaviorForSource(r *genericRequestor, source utils.RapidaSource, b internal_assistant_entity.AssistantDeploymentBehavior) {
	r.source = source
	if r.assistant == nil {
		r.assistant = &internal_assistant_entity.Assistant{}
	}

	switch source {
	case utils.PhoneCall:
		r.assistant.AssistantPhoneDeployment = &internal_assistant_entity.AssistantPhoneDeployment{AssistantDeploymentBehavior: b}
	case utils.Whatsapp:
		r.assistant.AssistantWhatsappDeployment = &internal_assistant_entity.AssistantWhatsappDeployment{AssistantDeploymentBehavior: b}
	case utils.SDK:
		r.assistant.AssistantApiDeployment = &internal_assistant_entity.AssistantApiDeployment{AssistantDeploymentBehavior: b}
	case utils.WebPlugin:
		r.assistant.AssistantWebPluginDeployment = &internal_assistant_entity.AssistantWebPluginDeployment{AssistantDeploymentBehavior: b}
	case utils.Debugger:
		r.assistant.AssistantDebuggerDeployment = &internal_assistant_entity.AssistantDebuggerDeployment{AssistantDeploymentBehavior: b}
	}
}

func collectChannelPackets(ch chan adapter_channel.Envelope) []internal_type.Packet {
	out := make([]internal_type.Packet, 0)
	for {
		select {
		case env := <-ch:
			out = append(out, env.Pkt)
		default:
			return out
		}
	}
}

func countPacketType[T internal_type.Packet](pkts []internal_type.Packet) int {
	count := 0
	for _, p := range pkts {
		if _, ok := p.(T); ok {
			count++
		}
	}
	return count
}

func TestBehavior_GetBehavior_AssistantNil(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	r.assistant = nil

	b, err := r.GetBehavior()
	require.ErrorIs(t, err, errDeploymentNotEnabled)
	assert.Nil(t, b)
}

func TestBehavior_GetBehavior_BySource(t *testing.T) {
	cases := []struct {
		name   string
		source utils.RapidaSource
	}{
		{name: "Phone", source: utils.PhoneCall},
		{name: "Whatsapp", source: utils.Whatsapp},
		{name: "SDK", source: utils.SDK},
		{name: "WebPlugin", source: utils.WebPlugin},
		{name: "Debugger", source: utils.Debugger},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := newBehaviorTestRequestor(t)
			greeting := tc.name + " greeting"
			behavior := internal_assistant_entity.AssistantDeploymentBehavior{
				Greeting: behaviorStrPtr(greeting),
			}
			setBehaviorForSource(r, tc.source, behavior)

			got, err := r.GetBehavior()
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, greeting, *got.Greeting)
		})
	}
}

func TestBehavior_GetBehavior_DeploymentMissingForSource(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	r.assistant = &internal_assistant_entity.Assistant{}

	missingCases := []utils.RapidaSource{
		utils.PhoneCall,
		utils.Whatsapp,
		utils.SDK,
		utils.WebPlugin,
		utils.Debugger,
	}

	for _, source := range missingCases {
		r.source = source
		got, err := r.GetBehavior()
		require.ErrorIs(t, err, errDeploymentNotEnabled)
		assert.Nil(t, got)
	}
}

func TestBehavior_GetBehavior_UnknownSource(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	r.assistant = &internal_assistant_entity.Assistant{}
	r.source = utils.RapidaSource("unknown-source")

	got, err := r.GetBehavior()
	require.ErrorIs(t, err, errDeploymentNotEnabled)
	assert.Nil(t, got)
}

func TestBehavior_InitializeBehavior_NoBehaviorConfigured_NoOp(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	r.assistant = nil

	err := r.initializeBehavior(context.Background())
	require.NoError(t, err)
	assert.Zero(t, len(r.channels.EgressChannel()))
	assert.Zero(t, len(r.channels.BackgroundChannel()))
	assert.Nil(t, r.maxSessionTimer)
}

func TestBehavior_InitializeBehavior_Configured_EnqueuesGreetingAndIdle(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	behavior := internal_assistant_entity.AssistantDeploymentBehavior{
		Greeting:    behaviorStrPtr("Hello from behavior"),
		IdleTimeout: behaviorU64Ptr(9),
	}
	setBehaviorForSource(r, utils.Debugger, behavior)

	err := r.initializeBehavior(context.Background())
	require.NoError(t, err)

	egressPkts := collectChannelPackets(r.channels.EgressChannel())
	require.Len(t, egressPkts, 3)
	assert.Equal(t, 1, countPacketType[internal_type.InjectMessagePacket](egressPkts))
	assert.Equal(t, 2, countPacketType[internal_type.StartIdleTimeoutPacket](egressPkts))

	backgroundPkts := collectChannelPackets(r.channels.BackgroundChannel())
	require.Len(t, backgroundPkts, 1)
	event, ok := backgroundPkts[0].(internal_type.ConversationEventPacket)
	require.True(t, ok)
	assert.Equal(t, "behavior", event.Name)
	assert.Equal(t, "greeting", event.Data["type"])
	assert.Equal(t, "19", event.Data["text_chars"])
}

func TestBehavior_InitializeGreeting_EmptyOrWhitespace_NoPackets(t *testing.T) {
	r := newBehaviorTestRequestor(t)

	cases := []struct {
		name     string
		greeting *string
	}{
		{name: "NilGreeting", greeting: nil},
		{name: "WhitespaceGreeting", greeting: behaviorStrPtr("   \t\n  ")},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r.channels.FlushAll()
			r.initializeGreeting(context.Background(), &internal_assistant_entity.AssistantDeploymentBehavior{
				Greeting: tc.greeting,
			})
			assert.Zero(t, len(r.channels.EgressChannel()))
			assert.Zero(t, len(r.channels.BackgroundChannel()))
		})
	}
}

func TestBehavior_InitializeGreeting_Valid_EnqueuesPackets(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	r.setContextIDForTest("ctx-greeting")

	r.initializeGreeting(context.Background(), &internal_assistant_entity.AssistantDeploymentBehavior{
		Greeting: behaviorStrPtr("Welcome!"),
	})

	inject, ok := drainPacket[internal_type.InjectMessagePacket](r.channels.EgressChannel(), time.Second)
	require.True(t, ok)
	assert.Equal(t, "ctx-greeting", inject.ContextID)
	assert.Equal(t, "Welcome!", inject.Text)

	start, ok := drainPacket[internal_type.StartIdleTimeoutPacket](r.channels.EgressChannel(), time.Second)
	require.True(t, ok)
	assert.Equal(t, "ctx-greeting", start.ContextID)

	event, ok := drainPacket[internal_type.ConversationEventPacket](r.channels.BackgroundChannel(), time.Second)
	require.True(t, ok)
	assert.Equal(t, "behavior", event.Name)
	assert.Equal(t, "greeting", event.Data["type"])
	assert.Equal(t, "8", event.Data["text_chars"])
}

func TestBehavior_InitializeIdleTimeout_NotConfigured_NoPacket(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	cases := []struct {
		name     string
		timeout  *uint64
		expected int
	}{
		{name: "NilTimeout", timeout: nil, expected: 0},
		{name: "ZeroTimeout", timeout: behaviorU64Ptr(0), expected: 0},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r.channels.FlushAll()
			r.initializeIdleTimeout(context.Background(), &internal_assistant_entity.AssistantDeploymentBehavior{
				IdleTimeout: tc.timeout,
			})
			assert.Equal(t, tc.expected, len(r.channels.EgressChannel()))
		})
	}
}

func TestBehavior_InitializeIdleTimeout_Configured_EnqueuesStartPacket(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	r.setContextIDForTest("ctx-idle")
	r.initializeIdleTimeout(context.Background(), &internal_assistant_entity.AssistantDeploymentBehavior{
		IdleTimeout: behaviorU64Ptr(5),
	})

	pkt, ok := drainPacket[internal_type.StartIdleTimeoutPacket](r.channels.EgressChannel(), time.Second)
	require.True(t, ok)
	assert.Equal(t, "ctx-idle", pkt.ContextID)
}

func TestBehavior_InitializeMaxSessionDuration_NotConfigured_NoTimer(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	cases := []struct {
		name string
		max  *uint64
	}{
		{name: "NilDuration", max: nil},
		{name: "ZeroDuration", max: behaviorU64Ptr(0)},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r.maxSessionTimer = nil
			r.initializeMaxSessionDuration(context.Background(), &internal_assistant_entity.AssistantDeploymentBehavior{
				MaxSessionDuration: tc.max,
			})
			assert.Nil(t, r.maxSessionTimer)
		})
	}
}

func TestBehavior_InitializeMaxSessionDuration_Configured_EmitsDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := newBehaviorTestRequestor(t)
	streamer := &capturingStreamer{ctx: ctx}
	r.streamer = streamer

	r.initializeMaxSessionDuration(ctx, &internal_assistant_entity.AssistantDeploymentBehavior{
		MaxSessionDuration: behaviorU64Ptr(1),
	})
	require.NotNil(t, r.maxSessionTimer)
	defer r.maxSessionTimer.Stop()

	require.Eventually(t, func() bool {
		for _, msg := range streamer.getSent() {
			disc, ok := msg.(*protos.ConversationDisconnection)
			if !ok {
				continue
			}
			if disc.GetType() == protos.ConversationDisconnection_DISCONNECTION_TYPE_MAX_DURATION {
				return true
			}
		}
		return false
	}, 2*time.Second, 20*time.Millisecond)
}

func TestBehavior_OnError_NoBehaviorConfigured_NoPackets(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	r.assistant = nil

	err := r.OnError(context.Background())
	require.NoError(t, err)
	assert.Zero(t, len(r.channels.ControlChannel()))
	assert.Zero(t, len(r.channels.EgressChannel()))
	assert.Zero(t, len(r.channels.BackgroundChannel()))
}

func TestBehavior_OnError_DefaultMistakeMessage(t *testing.T) {
	const defaultMistakeMessage = "Oops! It looks like something went wrong. Let me look into that for you right away. I really appreciate your patience—hang tight while I get this sorted!"

	r := newBehaviorTestRequestor(t)
	r.setInteractionStateForTest(LLMGenerating)
	r.setContextIDForTest("ctx-before-error")
	setBehaviorForSource(r, utils.Debugger, internal_assistant_entity.AssistantDeploymentBehavior{})

	err := r.OnError(context.Background())
	require.NoError(t, err)

	newID := r.GetID()
	assert.NotEqual(t, "ctx-before-error", newID)

	interrupt, ok := drainPacket[internal_type.TTSInterruptPacket](r.channels.ControlChannel(), time.Second)
	require.True(t, ok)
	assert.Equal(t, newID, interrupt.ContextID)

	inject, ok := drainPacket[internal_type.InjectMessagePacket](r.channels.EgressChannel(), time.Second)
	require.True(t, ok)
	assert.Equal(t, newID, inject.ContextID)
	assert.Equal(t, defaultMistakeMessage, inject.Text)

	start, ok := drainPacket[internal_type.StartIdleTimeoutPacket](r.channels.EgressChannel(), time.Second)
	require.True(t, ok)
	assert.Equal(t, newID, start.ContextID)

	event, ok := drainPacket[internal_type.ConversationEventPacket](r.channels.BackgroundChannel(), time.Second)
	require.True(t, ok)
	assert.Equal(t, "behavior", event.Name)
	assert.Equal(t, "error", event.Data["type"])
	assert.Equal(t, fmt.Sprintf("%d", len(defaultMistakeMessage)), event.Data["text_chars"])
}

func TestBehavior_OnError_UsesConfiguredMistakeMessage(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	r.setInteractionStateForTest(LLMGenerating)
	custom := "Custom error message"
	setBehaviorForSource(r, utils.Debugger, internal_assistant_entity.AssistantDeploymentBehavior{
		Mistake: behaviorStrPtr(custom),
	})

	err := r.OnError(context.Background())
	require.NoError(t, err)

	inject, ok := drainPacket[internal_type.InjectMessagePacket](r.channels.EgressChannel(), time.Second)
	require.True(t, ok)
	assert.Equal(t, custom, inject.Text)
}

func TestBehavior_OnIdleTimeout_NoBehaviorConfigured_NoOp(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	r.assistant = nil

	err := r.onIdleTimeout(context.Background())
	require.NoError(t, err)
	assert.Equal(t, uint64(0), r.idleTimeoutCount)
	assert.Zero(t, len(r.channels.ControlChannel()))
	assert.Zero(t, len(r.channels.EgressChannel()))
}

func TestBehavior_OnIdleTimeout_IdleTimeoutNotConfigured_NoOp(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	setBehaviorForSource(r, utils.Debugger, internal_assistant_entity.AssistantDeploymentBehavior{})

	err := r.onIdleTimeout(context.Background())
	require.NoError(t, err)
	assert.Equal(t, uint64(0), r.idleTimeoutCount)
	assert.Zero(t, len(r.channels.ControlChannel()))
	assert.Zero(t, len(r.channels.EgressChannel()))
	assert.Zero(t, len(r.channels.BackgroundChannel()))
}

func TestBehavior_OnIdleTimeout_BackoffReached_EmitsDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := newBehaviorTestRequestor(t)
	streamer := &capturingStreamer{ctx: ctx}
	r.streamer = streamer

	backoff := uint64(3)
	setBehaviorForSource(r, utils.Debugger, internal_assistant_entity.AssistantDeploymentBehavior{
		IdleTimeout:        behaviorU64Ptr(1),
		IdleTimeoutBackoff: &backoff,
		IdleTimeoutMessage: behaviorStrPtr("Hello?"),
	})
	r.idleTimeoutCount = backoff

	err := r.onIdleTimeout(ctx)
	require.NoError(t, err)
	assert.Equal(t, backoff, r.idleTimeoutCount)

	require.Eventually(t, func() bool {
		for _, msg := range streamer.getSent() {
			disc, ok := msg.(*protos.ConversationDisconnection)
			if !ok {
				continue
			}
			if disc.GetType() == protos.ConversationDisconnection_DISCONNECTION_TYPE_IDLE_TIMEOUT {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)

	assert.Zero(t, len(r.channels.ControlChannel()))
	assert.Zero(t, len(r.channels.EgressChannel()))
	assert.Zero(t, len(r.channels.BackgroundChannel()))
}

func TestBehavior_OnIdleTimeout_Configured_EmitsInterruptInjectEventAndRestart(t *testing.T) {
	r := newBehaviorTestRequestor(t)
	r.setInteractionStateForTest(LLMGenerated)
	r.setContextIDForTest("ctx-before-idle")

	backoff := uint64(4)
	setBehaviorForSource(r, utils.Debugger, internal_assistant_entity.AssistantDeploymentBehavior{
		IdleTimeout:        behaviorU64Ptr(1),
		IdleTimeoutBackoff: &backoff,
		IdleTimeoutMessage: behaviorStrPtr("Still there?"),
	})
	r.idleTimeoutCount = 1

	err := r.onIdleTimeout(context.Background())
	require.NoError(t, err)
	assert.Equal(t, uint64(2), r.idleTimeoutCount)

	newID := r.GetID()
	assert.NotEqual(t, "ctx-before-idle", newID)

	interrupt, ok := drainPacket[internal_type.TTSInterruptPacket](r.channels.ControlChannel(), time.Second)
	require.True(t, ok)
	assert.Equal(t, newID, interrupt.ContextID)

	inject, ok := drainPacket[internal_type.InjectMessagePacket](r.channels.EgressChannel(), time.Second)
	require.True(t, ok)
	assert.Equal(t, newID, inject.ContextID)
	assert.Equal(t, "Still there?", inject.Text)

	start, ok := drainPacket[internal_type.StartIdleTimeoutPacket](r.channels.EgressChannel(), time.Second)
	require.True(t, ok)
	assert.Equal(t, newID, start.ContextID)

	event, ok := drainPacket[internal_type.ConversationEventPacket](r.channels.BackgroundChannel(), time.Second)
	require.True(t, ok)
	assert.Equal(t, "behavior", event.Name)
	assert.Equal(t, "idle_timeout", event.Data["type"])
	assert.Equal(t, "2", event.Data["count"])
	assert.Equal(t, "4", event.Data["max_count"])
}

func TestBehavior_GetIdleTimeoutMessage(t *testing.T) {
	r := newBehaviorTestRequestor(t)

	cases := []struct {
		name     string
		message  *string
		expected string
	}{
		{name: "NilMessage", message: nil, expected: "Are you still there?"},
		{name: "BlankMessage", message: behaviorStrPtr("   \n \t "), expected: "Are you still there?"},
		{name: "ConfiguredMessage", message: behaviorStrPtr("Ping!"), expected: "Ping!"},
		{name: "ConfiguredWithSpaces", message: behaviorStrPtr("  ping  "), expected: "  ping  "},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := r.getIdleTimeoutMessage(&internal_assistant_entity.AssistantDeploymentBehavior{
				IdleTimeoutMessage: tc.message,
			})
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestBehavior_ExtendIdleTimeoutTimer_NoOpCases(t *testing.T) {
	r := newBehaviorTestRequestor(t)

	// no-op: nil timer
	originalDeadline := time.Now().Add(2 * time.Second)
	r.idleTimeoutDeadline = originalDeadline
	r.idleTimeoutTimer = nil
	r.extendIdleTimeoutTimer(time.Second)
	assert.WithinDuration(t, originalDeadline, r.idleTimeoutDeadline, 5*time.Millisecond)

	// no-op: non-positive duration
	fired := make(chan struct{}, 1)
	originalDeadline = time.Now().Add(200 * time.Millisecond)
	r.idleTimeoutDeadline = originalDeadline
	r.idleTimeoutTimer = time.AfterFunc(200*time.Millisecond, func() { fired <- struct{}{} })
	defer r.idleTimeoutTimer.Stop()

	r.extendIdleTimeoutTimer(0)
	assert.WithinDuration(t, originalDeadline, r.idleTimeoutDeadline, 5*time.Millisecond)
	r.extendIdleTimeoutTimer(-50 * time.Millisecond)
	assert.WithinDuration(t, originalDeadline, r.idleTimeoutDeadline, 5*time.Millisecond)
}

func TestBehavior_ExtendIdleTimeoutTimer_ExtendsDeadlineAndDelaysFire(t *testing.T) {
	r := newBehaviorTestRequestor(t)

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
		// expected: still waiting
	}

	select {
	case <-fired:
		// expected: fires after extension
	case <-time.After(260 * time.Millisecond):
		t.Fatal("idle timeout did not fire after extension")
	}
}
