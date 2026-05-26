// Copyright (c) 2023-2025 RapidaAI
// Author: RapidaAI Team <team@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_telnyx_telephony

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	internal_audio "github.com/rapidaai/api/assistant-api/internal/audio"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	internal_telephony_base "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/base"
	internal_telephony_media "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/media"
	internal_telnyx "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/telnyx/internal"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/protos"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TelnyxWebSocketEvent represents a Telnyx WebSocket event.
type TelnyxWebSocketEvent struct {
	Event    string            `json:"event"`
	StreamID string            `json:"stream_id"`
	Start    *TelnyxStartEvent `json:"start,omitempty"`
	Media    *TelnyxMediaEvent `json:"media,omitempty"`
	Stop     *TelnyxStopEvent  `json:"stop,omitempty"`
}

// TelnyxStartEvent contains the start event details.
type TelnyxStartEvent struct {
	CallControlID string            `json:"call_control_id"`
	MediaFormat   TelnyxMediaFormat `json:"media_format"`
}

// TelnyxMediaFormat describes the audio format.
type TelnyxMediaFormat struct {
	Encoding   string `json:"encoding"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
}

// TelnyxMediaEvent contains the media event details.
type TelnyxMediaEvent struct {
	Track   string `json:"track"`
	Payload string `json:"payload"`
}

// TelnyxStopEvent contains the stop event details.
type TelnyxStopEvent struct {
	CallControlID string `json:"call_control_id"`
}

type telnyxWebsocketStreamer struct {
	internal_telephony_base.BaseTelephonyStreamer

	mediaSession *internal_telephony_media.MediaSession

	streamID      string
	callControlID string
	connection    *websocket.Conn
	writeMu       sync.Mutex
	closed        atomic.Bool
	telephony     *telnyxTelephony
}

// NewTelnyxWebsocketStreamer creates a Telnyx WebSocket streamer.
// Telnyx sends PCMU 8kHz, matching Twilio's provider audio format.
func NewTelnyxWebsocketStreamer(logger commons.Logger, connection *websocket.Conn, cc *callcontext.CallContext, vaultCred *protos.VaultCredential) internal_type.Streamer {
	audioProcessor, err := internal_telnyx.NewAudioProcessor(logger)
	if err != nil {
		logger.Errorf("failed to initialize Telnyx audio processor: %v", err)
	}

	tws := &telnyxWebsocketStreamer{
		BaseTelephonyStreamer: internal_telephony_base.NewBaseTelephonyStreamer(
			logger, cc, vaultCred,
			internal_telephony_base.WithSourceAudioConfig(internal_audio.NewMulaw8khzMonoAudioConfig()),
		),
		streamID:   "",
		connection: connection,
		telephony: &telnyxTelephony{
			logger: logger,
		},
	}

	if audioProcessor != nil {
		audioProcessor.SetOutputChunkCallback(tws.sendAudioChunk)
		tws.mediaSession = internal_telephony_media.NewMediaSession(context.Background(), logger, audioProcessor, func() error {
			return tws.sendTelnyxMessage("clear", nil)
		})
		tws.mediaSession.SetInputSink(func(audio []byte) {
			tws.Input(&protos.ConversationUserMessage{
				Message: &protos.ConversationUserMessage_Audio{Audio: audio},
			})
		})
		tws.mediaSession.SetEventSink(func(event *protos.ConversationEvent) {
			if event != nil {
				if event.Data == nil {
					event.Data = map[string]string{}
				}
				event.Data["provider"] = "telnyx"
			}
			tws.Input(event)
		})
	}

	go tws.runWebSocketReader()
	return tws
}

func (tws *telnyxWebsocketStreamer) runWebSocketReader() {
	conn := tws.connection
	if conn == nil {
		return
	}

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			tws.stopAudioProcessing()
			if msg := tws.Disconnect(protos.ConversationDisconnection_DISCONNECTION_TYPE_USER); msg != nil {
				tws.Input(msg)
			}
			tws.BaseStreamer.Cancel()
			return
		}

		if messageType != websocket.TextMessage {
			tws.Logger.Warn("Unhandled message type", "type", messageType)
			continue
		}

		var mediaEvent TelnyxWebSocketEvent
		if err := json.Unmarshal(message, &mediaEvent); err != nil {
			tws.Logger.Error("Failed to unmarshal Telnyx media event", "error", err.Error())
			continue
		}

		switch mediaEvent.Event {
		case "connected":
			tws.Input(&protos.ConversationEvent{
				Name: "channel",
				Data: map[string]string{"type": "connected", "provider": "telnyx"},
				Time: timestamppb.Now(),
			})
		case "start":
			tws.handleStartEvent(mediaEvent)
			if tws.mediaSession != nil {
				tws.mediaSession.Start()
			}
			tws.Input(tws.CreateConnectionRequest())
			tws.Input(&protos.ConversationEvent{
				Name: "channel",
				Data: map[string]string{
					"type":            "stream_started",
					"provider":        "telnyx",
					"stream_id":       tws.streamID,
					"call_control_id": tws.callControlID,
				},
				Time: timestamppb.Now(),
			})
		case "media":
			_ = tws.handleMediaEvent(mediaEvent)
		case "dtmf":
			tws.Input(&protos.ConversationEvent{
				Name: "channel",
				Data: map[string]string{"type": "dtmf", "provider": "telnyx"},
				Time: timestamppb.Now(),
			})
		case "stop":
			tws.Logger.Info("Telnyx stream stopped")
			if msg := tws.Disconnect(protos.ConversationDisconnection_DISCONNECTION_TYPE_USER); msg != nil {
				tws.Input(msg)
			}
			tws.Cancel()
			return
		default:
			tws.Logger.Warn("Unhandled Telnyx event", "event", mediaEvent.Event)
		}
	}
}

func (tws *telnyxWebsocketStreamer) Send(response internal_type.Stream) error {
	if tws.connection == nil {
		return nil
	}
	switch data := response.(type) {
	case *protos.ConversationInitialization:
		if tws.mediaSession != nil {
			tws.mediaSession.HandleInitialization(data)
		}
	case *protos.ConversationAssistantMessage:
		switch content := data.Message.(type) {
		case *protos.ConversationAssistantMessage_Audio:
			if tws.mediaSession == nil {
				return nil
			}
			if err := tws.mediaSession.HandleAssistantAudio(content.Audio, data.GetCompleted()); err != nil {
				return err
			}
			return nil
		}
	case *protos.ConversationInterruption:
		if data.Type == protos.ConversationInterruption_INTERRUPTION_TYPE_WORD {
			if tws.mediaSession != nil {
				tws.mediaSession.HandleInterrupt()
			}
		}
	case *protos.ConversationDisconnection:
		_ = tws.Disconnect(data.GetType())
		if tws.GetConversationUuid() != "" {
			if err := tws.telephony.HangupCall(tws.GetConversationUuid(), tws.VaultCredential()); err != nil {
				tws.Logger.Errorf("Error ending Telnyx call: %v", err)
			}
		}
		tws.stopAudioProcessing()
		tws.Cancel()
	case *protos.ConversationToolCall:
		switch data.GetAction() {
		case protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION:
			result := map[string]string{"status": "completed"}
			if tws.GetConversationUuid() != "" {
				if err := tws.telephony.HangupCall(tws.GetConversationUuid(), tws.VaultCredential()); err != nil {
					tws.Logger.Errorf("Error ending Telnyx call: %v", err)
					result = map[string]string{"status": "failed", "reason": fmt.Sprintf("hangup failed: %v", err)}
				}
			}
			tws.Input(&protos.ConversationToolCallResult{
				Id:     data.GetId(),
				ToolId: data.GetToolId(),
				Name:   data.GetName(),
				Action: data.GetAction(),
				Result: result,
			})
		case protos.ToolCallAction_TOOL_CALL_ACTION_TRANSFER_CONVERSATION:
			tws.Logger.Warnw("Telnyx call transfer not yet implemented", "transfer_to", data.GetArgs()["transfer_to"])
			tws.Input(&protos.ConversationToolCallResult{
				Id:     data.GetId(),
				ToolId: data.GetToolId(), Name: data.GetName(), Action: data.GetAction(),
				Result: map[string]string{"status": "failed", "reason": "transfer not supported for Telnyx", "next_action": "end_call"},
			})
		}
	default:
		tws.Logger.Warnw("Telnyx Send: unknown message type, skipping", "type", fmt.Sprintf("%T", response))
	}
	return nil
}

func (tws *telnyxWebsocketStreamer) handleStartEvent(mediaEvent TelnyxWebSocketEvent) {
	tws.streamID = mediaEvent.StreamID
	if mediaEvent.Start == nil {
		return
	}
	tws.callControlID = mediaEvent.Start.CallControlID
	tws.ChannelUUID = mediaEvent.Start.CallControlID
}

func (tws *telnyxWebsocketStreamer) handleMediaEvent(mediaEvent TelnyxWebSocketEvent) error {
	if mediaEvent.Media == nil {
		return nil
	}
	payloadBytes, err := tws.Encoder().DecodeString(mediaEvent.Media.Payload)
	if err != nil {
		tws.Logger.Warn("Failed to decode media payload", "error", err.Error())
		return nil
	}

	if tws.mediaSession == nil {
		return nil
	}
	if err := tws.mediaSession.HandleProviderAudio(payloadBytes); err != nil {
		return err
	}
	return nil
}

func (tws *telnyxWebsocketStreamer) sendAudioChunk(chunk *internal_telnyx.AudioChunk) error {
	if chunk == nil || len(chunk.Data) == 0 {
		return nil
	}
	return tws.sendTelnyxMessage("media", map[string]interface{}{
		"payload": tws.Encoder().EncodeToString(chunk.Data),
	})
}

func (tws *telnyxWebsocketStreamer) sendTelnyxMessage(eventType string, mediaData map[string]interface{}) error {
	if tws.connection == nil || tws.streamID == "" {
		return nil
	}
	message := map[string]interface{}{
		"event":     eventType,
		"stream_id": tws.streamID,
	}
	if mediaData != nil {
		message["media"] = mediaData
	}

	messageJSON, err := json.Marshal(message)
	if err != nil {
		return tws.handleError("Failed to marshal Telnyx message", err)
	}

	tws.writeMu.Lock()
	defer tws.writeMu.Unlock()
	if tws.connection == nil {
		return nil
	}
	if err := tws.connection.WriteMessage(websocket.TextMessage, messageJSON); err != nil {
		return tws.handleError("Failed to send message to Telnyx", err)
	}
	return nil
}

func (tws *telnyxWebsocketStreamer) handleError(message string, err error) error {
	tws.Logger.Error(message, "error", err.Error())
	return err
}

func (tws *telnyxWebsocketStreamer) stopAudioProcessing() {
	if tws.mediaSession != nil {
		tws.mediaSession.Shutdown()
	}
}

func (tws *telnyxWebsocketStreamer) GetConversationUuid() string {
	return tws.ChannelUUID
}

func (tws *telnyxWebsocketStreamer) Cancel() error {
	if !tws.closed.CompareAndSwap(false, true) {
		return nil
	}
	tws.stopAudioProcessing()
	tws.writeMu.Lock()
	conn := tws.connection
	tws.connection = nil
	tws.writeMu.Unlock()
	if conn != nil {
		conn.Close()
	}
	tws.BaseStreamer.Cancel()
	return nil
}
