package internal_llm_agentkit

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_conversation_entity "github.com/rapidaai/api/assistant-api/internal/entity/conversations"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/protos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type initializeTestCommunication struct {
	internal_type.Communication
	assistant    *internal_assistant_entity.Assistant
	conversation *internal_conversation_entity.AssistantConversation
	collector    *packetCollector
}

func (c *initializeTestCommunication) Assistant() *internal_assistant_entity.Assistant {
	return c.assistant
}

func (c *initializeTestCommunication) Conversation() *internal_conversation_entity.AssistantConversation {
	return c.conversation
}

func (c *initializeTestCommunication) OnPacket(ctx context.Context, pkts ...internal_type.Packet) error {
	if c.collector == nil {
		return nil
	}
	return c.collector.collect(ctx, pkts...)
}

type testAgentKitServer struct {
	protos.UnimplementedAgentKitServer
	talkFn func(stream grpc.BidiStreamingServer[protos.TalkInput, protos.TalkOutput]) error
}

func (s *testAgentKitServer) Talk(stream grpc.BidiStreamingServer[protos.TalkInput, protos.TalkOutput]) error {
	if s.talkFn == nil {
		return nil
	}
	return s.talkFn(stream)
}

func newInitializeCommunication(provider *internal_assistant_entity.AssistantProviderAgentkit, conversationID uint64) (*initializeTestCommunication, *packetCollector) {
	collector := &packetCollector{}
	assistant := &internal_assistant_entity.Assistant{
		AssistantProviderAgentkit: provider,
	}
	assistant.Id = 77

	conversation := &internal_conversation_entity.AssistantConversation{}
	conversation.Id = conversationID

	return &initializeTestCommunication{
		assistant:    assistant,
		conversation: conversation,
		collector:    collector,
	}, collector
}

func startAgentKitTestServer(
	t *testing.T,
	talkFn func(stream grpc.BidiStreamingServer[protos.TalkInput, protos.TalkOutput]) error,
) (string, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	protos.RegisterAgentKitServer(server, &testAgentKitServer{talkFn: talkFn})
	go func() {
		_ = server.Serve(listener)
	}()

	return listener.Addr().String(), func() {
		server.Stop()
		_ = listener.Close()
	}
}

func acquireClosedTCPAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	_ = listener.Close()
	return addr
}

func findEventPacketByType(pkts []internal_type.Packet, eventType observability.EventName) (internal_type.ObservabilityEventRecordPacket, bool) {
	for _, pkt := range pkts {
		event, ok := pkt.(internal_type.ObservabilityEventRecordPacket)
		if !ok {
			continue
		}
		if event.Record.Event == eventType {
			return event, true
		}
	}
	return internal_type.ObservabilityEventRecordPacket{}, false
}

func TestInitialize_ReturnsErrorWhenConnectionFails(t *testing.T) {
	e := newTestExecutor()
	e.stateMu.Lock()
	e.closing = true
	e.stateMu.Unlock()

	comm, _ := newInitializeCommunication(&internal_assistant_entity.AssistantProviderAgentkit{
		Url: acquireClosedTCPAddress(t),
	}, 2002)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := e.Initialize(ctx, comm, &protos.ConversationInitialization{})
	require.Error(t, err)
	assert.True(
		t,
		strings.Contains(err.Error(), "connect failed") || strings.Contains(err.Error(), "stream start failed"),
		"unexpected initialize error: %v",
		err,
	)

	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	assert.False(t, e.closing, "Initialize should reset closing state before connecting")
	assert.Nil(t, e.transport.stream)
	assert.Nil(t, e.transport.conn)
	assert.Nil(t, e.transport.listenerDone)
}

func TestInitialize_SendsInitializationAndEmitsInitializedEvent(t *testing.T) {
	receivedInitialization := make(chan *protos.TalkInput, 1)
	addr, shutdown := startAgentKitTestServer(t, func(stream grpc.BidiStreamingServer[protos.TalkInput, protos.TalkOutput]) error {
		req, err := stream.Recv()
		if err != nil {
			return err
		}
		receivedInitialization <- req

		if err := stream.Send(&protos.TalkOutput{
			Data: &protos.TalkOutput_Initialization{
				Initialization: &protos.ConversationInitialization{
					AssistantConversationId: req.GetInitialization().GetAssistantConversationId(),
				},
			},
		}); err != nil {
			return err
		}

		for {
			if _, err := stream.Recv(); err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return err
			}
		}
	})
	defer shutdown()

	e := newTestExecutor()
	comm, collector := newInitializeCommunication(&internal_assistant_entity.AssistantProviderAgentkit{
		AssistantProvider: internal_assistant_entity.AssistantProvider{
			AssistantId: 5005,
		},
		Url: addr,
	}, 3003)

	err := e.Initialize(context.Background(), comm, &protos.ConversationInitialization{})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = e.Close(context.Background())
	})

	select {
	case req := <-receivedInitialization:
		initReq := req.GetInitialization()
		require.NotNil(t, initReq)
		assert.Equal(t, uint64(3003), initReq.GetAssistantConversationId())
		require.NotNil(t, initReq.GetAssistant())
		assert.Equal(t, uint64(5005), initReq.GetAssistant().GetAssistantId())
	case <-time.After(time.Second):
		t.Fatal("did not receive initialization request on server")
	}

	pkts := collector.all()
	event, found := findEventPacketByType(pkts, observability.LLMStarted)
	require.True(t, found, "expected agentkit_initialized event")
	assert.Equal(t, "agentkit", event.Record.Attributes["provider"])
	assert.Equal(t, addr, event.Record.Attributes["url"])

	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	assert.NotNil(t, e.transport.stream)
	assert.NotNil(t, e.transport.conn)
	assert.NotNil(t, e.transport.listenerDone)
}
