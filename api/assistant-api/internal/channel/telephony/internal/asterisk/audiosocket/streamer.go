// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_asterisk_audiosocket

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	internal_audio "github.com/rapidaai/api/assistant-api/internal/audio"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	internal_asterisk "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/asterisk/internal"
	internal_telephony_base "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/base"
	internal_telephony_media "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/media"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/protos"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Streamer implements AudioSocket media streaming over TCP.
type Streamer struct {
	internal_telephony_base.BaseTelephonyStreamer

	conn           net.Conn
	reader         *bufio.Reader
	writer         *bufio.Writer
	writeMu        sync.Mutex
	closed         atomic.Bool
	audioProcessor *internal_asterisk.AudioProcessor
	mediaSession   *internal_telephony_media.MediaSession

	ctx    context.Context
	cancel context.CancelFunc

	initialUUID string
}

// NewStreamer creates a new AudioSocket streamer.
// initialUUID is the contextId already read from the first UUID frame by the AudioSocket
// engine -- when set, the streamer emits ConversationInitialization on the first Recv()
// without waiting for another UUID frame from the wire.
func NewStreamer(
	logger commons.Logger,
	conn net.Conn,
	reader *bufio.Reader,
	writer *bufio.Writer,
	cc *callcontext.CallContext,
	vaultCred *protos.VaultCredential,
) (internal_type.Streamer, error) {
	audioProcessor, err := internal_asterisk.NewAudioProcessor(logger, internal_asterisk.AudioProcessorConfig{
		AsteriskConfig:   internal_audio.NewLinear8khzMonoAudioConfig(),
		DownstreamConfig: internal_audio.NewLinear16khzMonoAudioConfig(),
		SilenceByte:      0x00, // SLIN silence
		FrameSize:        320,  // 20ms at 8kHz 16-bit
	})
	if err != nil {
		return nil, err
	}

	if reader == nil {
		reader = bufio.NewReader(conn)
	}
	if writer == nil {
		writer = bufio.NewWriter(conn)
	}

	as := &Streamer{
		BaseTelephonyStreamer: internal_telephony_base.NewBaseTelephonyStreamer(
			logger, cc, vaultCred,
		),
		conn:           conn,
		reader:         reader,
		writer:         writer,
		audioProcessor: audioProcessor,
		initialUUID:    cc.ContextID,
	}
	as.ctx, as.cancel = context.WithCancel(as.Ctx)

	audioProcessor.SetOutputChunkCallback(as.sendAudioChunk)
	as.mediaSession = internal_telephony_media.NewMediaSession(as.ctx, logger, audioProcessor, nil)
	as.mediaSession.SetInputSink(func(audio []byte) {
		as.Input(&protos.ConversationUserMessage{
			Message: &protos.ConversationUserMessage_Audio{Audio: audio},
		})
	})
	as.mediaSession.SetEventSink(func(event *protos.ConversationEvent) {
		if event != nil {
			if event.Data == nil {
				event.Data = map[string]string{}
			}
			event.Data["provider"] = "asterisk_as"
		}
		as.Input(event)
	})
	as.mediaSession.Start()
	go as.runFrameReader()
	return as, nil
}

func (as *Streamer) sendAudioChunk(chunk *internal_asterisk.AudioChunk) error {
	if as.conn == nil {
		return nil
	}
	if err := as.writeFrame(FrameTypeAudio, chunk.Data); err != nil {
		// Connection dead — stop media session output sender.
		if as.mediaSession != nil {
			as.mediaSession.Shutdown()
		}
		return err
	}
	return nil
}

func (as *Streamer) writeFrame(frameType byte, payload []byte) error {
	as.writeMu.Lock()
	defer as.writeMu.Unlock()

	if err := WriteFrame(as.writer, frameType, payload); err != nil {
		return err
	}
	return as.writer.Flush()
}

func (as *Streamer) Context() context.Context {
	return as.ctx
}

func (as *Streamer) runFrameReader() {
	as.Input(&protos.ConversationEvent{
		Name: "channel",
		Data: map[string]string{
			"type":     "connected",
			"provider": "asterisk_as",
		},
		Time: timestamppb.Now(),
	})
	if as.initialUUID != "" {
		as.Input(as.CreateConnectionRequest())
	}
	for {
		select {
		case <-as.ctx.Done():
			return
		default:
		}
		frame, err := ReadFrame(as.reader)
		if err != nil {
			disconnectType := protos.ConversationDisconnection_DISCONNECTION_TYPE_UNSPECIFIED
			if err == io.EOF {
				disconnectType = protos.ConversationDisconnection_DISCONNECTION_TYPE_USER
			}
			if msg := as.Disconnect(disconnectType); msg != nil {
				as.Input(msg)
			}
			as.Cancel()
			return
		}
		switch frame.Type {
		case FrameTypeUUID:
			if as.initialUUID == "" {
				as.initialUUID = strings.TrimSpace(string(frame.Payload))
				as.Input(as.CreateConnectionRequest())
			}
		case FrameTypeAudio:
			if as.mediaSession == nil {
				continue
			}
			if err := as.mediaSession.HandleProviderAudio(frame.Payload); err != nil {
				as.Logger.Debug("Failed to process input audio", "error", err.Error())
				continue
			}
		case FrameTypeSilence:
			// no-op
		case FrameTypeHangup:
			if msg := as.Disconnect(protos.ConversationDisconnection_DISCONNECTION_TYPE_USER); msg != nil {
				as.Input(msg)
			}
			as.Cancel()
			return
		case FrameTypeError:
			if msg := as.Disconnect(protos.ConversationDisconnection_DISCONNECTION_TYPE_UNSPECIFIED); msg != nil {
				as.Input(msg)
			}
			as.Cancel()
			return
		}
	}
}

func (as *Streamer) Send(response internal_type.Stream) error {
	switch data := response.(type) {
	case *protos.ConversationInitialization:
		if as.mediaSession != nil {
			as.mediaSession.HandleInitialization(data)
		}
	case *protos.ConversationAssistantMessage:
		switch content := data.GetMessage().(type) {
		case *protos.ConversationAssistantMessage_Audio:
			if as.mediaSession == nil {
				return nil
			}
			if err := as.mediaSession.HandleAssistantAudio(content.Audio, data.GetCompleted()); err != nil {
				return err
			}
		}
	case *protos.ConversationInterruption:
		if data.GetType() == protos.ConversationInterruption_INTERRUPTION_TYPE_WORD {
			if as.mediaSession != nil {
				as.mediaSession.HandleInterrupt()
			}
		}
	case *protos.ConversationDisconnection:
		// Server-initiated disconnect: the talker already knows the reason
		// (it called Notify with it). No need to round-trip back through
		// CriticalCh — just signal hangup over AudioSocket and clean up.
		_ = as.Disconnect(data.GetType())
		_ = as.writeFrame(FrameTypeHangup, nil)
		as.Cancel()
	case *protos.ConversationToolCall:
		switch data.GetAction() {
		case protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION:
			_ = as.writeFrame(FrameTypeHangup, nil)
			as.Input(&protos.ConversationToolCallResult{
				Id:     data.GetId(),
				ToolId: data.GetToolId(),
				Name:   data.GetName(),
				Action: data.GetAction(),
				Result: map[string]string{"status": "completed"},
			})
		case protos.ToolCallAction_TOOL_CALL_ACTION_TRANSFER_CONVERSATION:
			// AudioSocket transfer is NOT supported. AudioSocket is a raw
			// audio-only TCP protocol with no signalling channel back to
			// Asterisk — there is no way to instruct Asterisk to redirect /
			// bridge from inside the AudioSocket session. To implement
			// transfer for an AudioSocket-attached channel, route the call
			// through the Asterisk WS/ARI streamer instead, which can issue
			// `channels/{id}/redirect` (blind transfer; end_call only).
			as.Logger.Warnw("Call transfer not supported for AudioSocket")
			as.Input(&protos.ConversationToolCallResult{
				Id:     data.GetId(),
				ToolId: data.GetToolId(), Name: data.GetName(), Action: data.GetAction(),
				Result: map[string]string{"status": "failed", "reason": "transfer not supported for AudioSocket", "next_action": "end_call"},
			})
		}
	default:
		as.Logger.Warnw("AudioSocket Send: unknown message type, skipping", "type", fmt.Sprintf("%T", response))
	}

	return nil
}

func (as *Streamer) Cancel() error {
	if !as.closed.CompareAndSwap(false, true) {
		return nil
	}
	if as.mediaSession != nil {
		as.mediaSession.Shutdown()
	}
	as.cancel()
	as.writeMu.Lock()
	conn := as.conn
	as.conn = nil
	as.writeMu.Unlock()
	if conn != nil {
		conn.Close()
	}
	as.BaseStreamer.Cancel()
	return nil
}
