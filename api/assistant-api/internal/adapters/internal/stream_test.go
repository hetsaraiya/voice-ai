package adapter_internal

import (
	"context"
	"io"
	"sync"
	"testing"

	adapter_channel "github.com/rapidaai/api/assistant-api/internal/adapters/channel"
	adapter_lifecycle "github.com/rapidaai/api/assistant-api/internal/adapters/lifecycle"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_conversation_entity "github.com/rapidaai/api/assistant-api/internal/entity/conversations"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	gorm_model "github.com/rapidaai/pkg/models/gorm"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type streamTestStreamer struct {
	ctx      context.Context
	recv     []internal_type.Stream
	recvErr  error
	recvIdx  int
	recvCall int

	mu    sync.Mutex
	sent  []internal_type.Stream
	modes []protos.StreamMode
}

func (s *streamTestStreamer) Context() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *streamTestStreamer) Recv() (internal_type.Stream, error) {
	s.recvCall++
	if s.recvIdx < len(s.recv) {
		msg := s.recv[s.recvIdx]
		s.recvIdx++
		return msg, nil
	}
	if s.recvErr != nil {
		return nil, s.recvErr
	}
	return nil, io.EOF
}

func (s *streamTestStreamer) Send(in internal_type.Stream) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, in)
	return nil
}

func (s *streamTestStreamer) NotifyMode(mode protos.StreamMode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modes = append(s.modes, mode)
}

func TestTalk_RecvErrorBeforeInitialization_ReturnsNil(t *testing.T) {
	streamer := &streamTestStreamer{recvErr: io.EOF}
	sessionCtx, cancelSession := context.WithCancel(context.Background())
	t.Cleanup(cancelSession)
	r := &genericRequestor{
		streamer:         streamer,
		messageLifecycle: adapter_lifecycle.NewMessageLifecycle(),
		sessionLifecycle: adapter_lifecycle.NewSessionLifecycle(),
		sessionCtx:       sessionCtx,
		cancelSession:    cancelSession,
		channels:         adapter_channel.NewRequestorChannels(),
	}

	err := r.Talk(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 1, streamer.recvCall)
}

func TestTalk_BuffersPacketsBeforeInitialization(t *testing.T) {
	streamer := &streamTestStreamer{
		recv: []internal_type.Stream{
			&protos.ConversationUserMessage{
				Message: &protos.ConversationUserMessage_Text{Text: "hello"},
			},
			&protos.ConversationMetadata{
				AssistantConversationId: 42,
				Metadata: []*protos.Metadata{{
					Key:   "k",
					Value: "v",
				}},
			},
			&protos.ConversationMetric{
				AssistantConversationId: 42,
				Metrics: []*protos.Metric{{
					Name:  "status",
					Value: "in_progress",
				}},
			},
			&protos.ConversationEvent{
				Name: "session",
				Data: map[string]string{"kind": "noop"},
				Time: timestamppb.Now(),
			},
			&protos.ConversationDisconnection{
				Type: protos.ConversationDisconnection_DISCONNECTION_TYPE_USER,
			},
		},
		recvErr: io.EOF,
	}

	sessionCtx, cancelSession := context.WithCancel(context.Background())
	t.Cleanup(cancelSession)
	r := &genericRequestor{
		streamer:         streamer,
		messageLifecycle: adapter_lifecycle.NewMessageLifecycle(),
		sessionLifecycle: adapter_lifecycle.NewSessionLifecycle(),
		sessionCtx:       sessionCtx,
		cancelSession:    cancelSession,
		// Before initialization completes, packets should be buffered in channels.
		channels: func() *adapter_channel.RequestorChannels {
			ch := adapter_channel.NewRequestorChannels()
			return ch
		}(),
	}

	err := r.Talk(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, len(r.channels.ControlChannel()))
	assert.Equal(t, 1, len(r.channels.IngressChannel()))
	assert.Equal(t, 0, len(r.channels.EgressChannel()))
	assert.Equal(t, 1, len(r.channels.DataChannel()))
	assert.Equal(t, 3, len(r.channels.BackgroundChannel()))
	assert.Equal(t, 0, len(streamer.modes))
}

func TestNotify_ForwardsAllActionData(t *testing.T) {
	streamer := &streamTestStreamer{}
	sessionCtx, cancelSession := context.WithCancel(context.Background())
	t.Cleanup(cancelSession)
	r := &genericRequestor{
		streamer:         streamer,
		messageLifecycle: adapter_lifecycle.NewMessageLifecycle(),
		sessionLifecycle: adapter_lifecycle.NewSessionLifecycle(),
		sessionCtx:       sessionCtx,
		cancelSession:    cancelSession,
		channels:         adapter_channel.NewRequestorChannels(),
	}

	a := &protos.ConversationEvent{Name: "alpha"}
	b := &protos.ConversationMetric{
		AssistantConversationId: 77,
		Metrics:                 []*protos.Metric{{Name: "m1", Value: "v1"}},
	}

	err := r.Notify(context.Background(), a, b)
	require.NoError(t, err)
	require.Len(t, streamer.sent, 2)
	assert.Same(t, a, streamer.sent[0])
	assert.Same(t, b, streamer.sent[1])
}

func TestOnNotifyAssistantConfiguration_ForwardsAmbientOptions(t *testing.T) {
	streamer := &streamTestStreamer{}
	sessionCtx, cancelSession := context.WithCancel(context.Background())
	t.Cleanup(cancelSession)
	r := &genericRequestor{
		source:   utils.SDK,
		streamer: streamer,
		assistant: &internal_assistant_entity.Assistant{
			AssistantApiDeployment: &internal_assistant_entity.AssistantApiDeployment{
				OutputAudio: &internal_assistant_entity.AssistantDeploymentAudio{
					AudioOptions: []*internal_assistant_entity.AssistantDeploymentAudioOption{
						{Metadata: gorm_model.Metadata{Key: "speaker.ambient", Value: "cafe"}},
						{Metadata: gorm_model.Metadata{Key: "speaker.ambient_volume", Value: "34"}},
					},
				},
			},
		},
		options:          map[string]interface{}{"existing": "kept"},
		args:             map[string]interface{}{},
		metadata:         map[string]interface{}{},
		messageLifecycle: adapter_lifecycle.NewMessageLifecycle(),
		sessionLifecycle: adapter_lifecycle.NewSessionLifecycle(),
		sessionCtx:       sessionCtx,
		cancelSession:    cancelSession,
		channels:         adapter_channel.NewRequestorChannels(),
	}
	r.assistant.Id = 11
	r.assistant.AssistantProviderId = 22
	conversation := &internal_conversation_entity.AssistantConversation{}
	conversation.Id = 33

	r.OnNotifyAssistantConfiguration(context.Background(), &protos.ConversationInitialization{
		StreamMode: protos.StreamMode_STREAM_MODE_AUDIO,
	}, conversation)

	require.Len(t, streamer.sent, 1)
	init, ok := streamer.sent[0].(*protos.ConversationInitialization)
	require.True(t, ok)
	options, err := utils.AnyMapToInterfaceMap(init.GetOptions())
	require.NoError(t, err)
	assert.Equal(t, "cafe", options["speaker.ambient"])
	assert.Equal(t, "34", options["speaker.ambient_volume"])
	assert.Equal(t, "kept", options["existing"])
}
