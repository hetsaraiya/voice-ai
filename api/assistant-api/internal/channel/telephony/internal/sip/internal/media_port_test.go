// Copyright (c) 2023-2026 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_sip

import (
	"context"
	"testing"
	"time"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	"github.com/rapidaai/protos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMediaPortTestSession(t *testing.T) *sip_infra.Session {
	t.Helper()
	session, err := sip_infra.NewSession(context.Background(), &sip_infra.SessionConfig{
		Config: &sip_infra.Config{
			Server:            "127.0.0.1",
			Port:              5060,
			RTPPortRangeStart: 10000,
			RTPPortRangeEnd:   10010,
		},
		Direction: sip_infra.CallDirectionInbound,
		CallID:    "media-port-test",
		Codec:     &sip_infra.CodecPCMU,
	})
	require.NoError(t, err)
	return session
}

func newMediaPortTestRTP(t *testing.T) (*sip_infra.RTPHandler, chan []byte, chan []byte) {
	t.Helper()
	rtpHandler := &sip_infra.RTPHandler{}
	audioIn := make(chan []byte, 100)
	audioOut := make(chan []byte, 100)
	setUnexportedField(t, rtpHandler, "codec", &sip_infra.CodecPCMU)
	setUnexportedField(t, rtpHandler, "audioInChan", audioIn)
	setUnexportedField(t, rtpHandler, "audioOutChan", audioOut)
	setUnexportedField(t, rtpHandler, "flushAudioCh", make(chan struct{}, 1))
	return rtpHandler, audioIn, audioOut
}

func newMediaPortForTest(t *testing.T, streamSink func(internal_type.Stream)) (*MediaPort, chan []byte, chan []byte) {
	t.Helper()
	rtpHandler, audioIn, audioOut := newMediaPortTestRTP(t)
	mediaPort, err := NewMediaPort(MediaPortConfig{
		Context:    context.Background(),
		Session:    newMediaPortTestSession(t),
		RTPHandler: rtpHandler,
		Resampler:  &mockResampler{out: make([]byte, BridgeOutputFrameSize)},
		StreamSink: streamSink,
	})
	require.NoError(t, err)
	return mediaPort, audioIn, audioOut
}

func TestMediaPort_StartForwardsProviderAudio(t *testing.T) {
	streams := make(chan internal_type.Stream, 4)
	mediaPort, audioIn, _ := newMediaPortForTest(t, func(stream internal_type.Stream) {
		streams <- stream
	})

	mediaPort.Start()
	defer func() { require.NoError(t, mediaPort.Close()) }()

	for i := 0; i < 3; i++ {
		audioIn <- make([]byte, MulawFrameSize)
	}

	require.Eventually(t, func() bool {
		for {
			select {
			case stream := <-streams:
				if userMessage, ok := stream.(*protos.ConversationUserMessage); ok {
					return len(userMessage.GetAudio()) == InputBufferThreshold
				}
			default:
				return false
			}
		}
	}, time.Second, 10*time.Millisecond)
}

func TestMediaPort_AssistantAudioReachesRTPOutput(t *testing.T) {
	mediaPort, _, audioOut := newMediaPortForTest(t, nil)

	mediaPort.Start()
	defer func() { require.NoError(t, mediaPort.Close()) }()
	assert.True(t, mediaPort.session.GetInboundSetupTimings().FirstAssistantAudioSentAt.IsZero())
	require.NoError(t, mediaPort.HandleAssistantAudio(make([]byte, BridgeOutputFrameSize), false))
	assert.False(t, mediaPort.session.GetInboundSetupTimings().FirstAssistantAudioSentAt.IsZero())

	select {
	case frame := <-audioOut:
		assert.Len(t, frame, MulawFrameSize)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for RTP output")
	}
}

func TestMediaPort_TransferModeSuppressesAssistantAudio(t *testing.T) {
	mediaPort, _, audioOut := newMediaPortForTest(t, nil)

	require.True(t, mediaPort.EnterTransferMode(DefaultRingtone))
	require.NoError(t, mediaPort.HandleAssistantAudio(make([]byte, BridgeOutputFrameSize), false))

	select {
	case frame := <-audioOut:
		t.Fatalf("assistant audio was queued during transfer mode: %v", frame)
	default:
	}
	require.True(t, mediaPort.ResumeAssistant())
	require.NoError(t, mediaPort.Close())
}

func TestMediaPort_InterruptPreservesBufferedInput(t *testing.T) {
	streams := make(chan internal_type.Stream, 8)
	mediaPort, audioIn, _ := newMediaPortForTest(t, func(stream internal_type.Stream) {
		streams <- stream
	})

	mediaPort.Start()
	defer func() { require.NoError(t, mediaPort.Close()) }()
	audioIn <- make([]byte, MulawFrameSize)
	audioIn <- make([]byte, MulawFrameSize)
	mediaPort.HandleInterrupt()
	audioIn <- make([]byte, MulawFrameSize)

	require.Eventually(t, func() bool {
		for {
			select {
			case stream := <-streams:
				if userMessage, ok := stream.(*protos.ConversationUserMessage); ok {
					return len(userMessage.GetAudio()) == InputBufferThreshold
				}
			default:
				return false
			}
		}
	}, time.Second, 10*time.Millisecond)
}

func TestMediaPort_ConnectTransferMediaForwardsCallerAudio(t *testing.T) {
	streams := make(chan internal_type.Stream, 1)
	mediaPort, audioIn, _ := newMediaPortForTest(t, func(stream internal_type.Stream) {
		streams <- stream
	})
	bridgeRTP, _, bridgeAudioOut := newMediaPortTestRTP(t)

	mediaPort.Start()
	defer func() { require.NoError(t, mediaPort.Close()) }()
	mediaPort.ConnectTransferMedia(bridgeRTP, sip_infra.CodecPCMU.Name)
	audioIn <- []byte{0x01, 0x02, 0x03}

	select {
	case frame := <-bridgeAudioOut:
		assert.Equal(t, []byte{0x01, 0x02, 0x03}, frame)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bridged caller audio")
	}

	require.Eventually(t, func() bool {
		for {
			select {
			case stream := <-streams:
				_, ok := stream.(*protos.ConversationBridgeUserAudio)
				if ok {
					return true
				}
			default:
				return false
			}
		}
	}, time.Second, 10*time.Millisecond)
}

func TestMediaPort_CloseIsIdempotent(t *testing.T) {
	mediaPort, _, _ := newMediaPortForTest(t, nil)

	mediaPort.Start()

	require.NoError(t, mediaPort.Close())
	require.NoError(t, mediaPort.Close())
}
