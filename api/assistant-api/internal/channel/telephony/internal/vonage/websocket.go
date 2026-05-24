// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_vonage_telephony

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	internal_telephony_base "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/base"
	internal_telephony_media "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/media"
	internal_vonage "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/vonage/internal"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	protos "github.com/rapidaai/protos"
	"github.com/vonage/vonage-go-sdk"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type vonageWebsocketStreamer struct {
	internal_telephony_base.BaseTelephonyStreamer

	mediaSession *internal_telephony_media.MediaSession

	connection *websocket.Conn
	writeMu    sync.Mutex
	closed     atomic.Bool
}

// NewVonageWebsocketStreamer creates a Vonage WebSocket streamer.
// Vonage sends linear16 16kHz — same as the internal Rapida format, so no
// resampling is needed (nil source audio config defaults to linear16 16kHz).
func NewVonageWebsocketStreamer(logger commons.Logger, connection *websocket.Conn, cc *callcontext.CallContext, vaultCred *protos.VaultCredential) (internal_type.Streamer, error) {
	audioProcessor, err := internal_vonage.NewAudioProcessor(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Vonage audio processor: %w", err)
	}
	vng := &vonageWebsocketStreamer{
		BaseTelephonyStreamer: internal_telephony_base.NewBaseTelephonyStreamer(
			logger, cc, vaultCred,
		),
		connection: connection,
	}
	audioProcessor.SetOutputChunkCallback(vng.sendAudioChunk)
	vng.mediaSession = internal_telephony_media.NewMediaSession(context.Background(), logger, audioProcessor, func() error {
		vng.writeMu.Lock()
		defer vng.writeMu.Unlock()
		if vng.connection == nil {
			return nil
		}
		return vng.connection.WriteMessage(websocket.TextMessage, []byte(`{"action":"clear"}`))
	})
	vng.mediaSession.SetInputSink(func(audio []byte) {
		vng.Input(&protos.ConversationUserMessage{
			Message: &protos.ConversationUserMessage_Audio{Audio: audio},
		})
	})
	vng.mediaSession.SetEventSink(func(event *protos.ConversationEvent) {
		if event != nil {
			if event.Data == nil {
				event.Data = map[string]string{}
			}
			event.Data["provider"] = "vonage"
		}
		vng.Input(event)
	})
	go vng.runWebSocketReader()
	return vng, nil
}

func (vng *vonageWebsocketStreamer) runWebSocketReader() {
	conn := vng.connection
	if conn == nil {
		return
	}
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			vng.stopAudioProcessing()
			if msg := vng.Disconnect(protos.ConversationDisconnection_DISCONNECTION_TYPE_USER); msg != nil {
				vng.Input(msg)
			}
			vng.BaseStreamer.Cancel()
			return
		}
		switch messageType {
		case websocket.TextMessage:
			var textEvent map[string]interface{}
			if err := json.Unmarshal(message, &textEvent); err != nil {
				vng.Logger.Error("Failed to unmarshal text event", "error", err.Error())
				continue
			}
			switch textEvent["event"] {
			case "websocket:connected":
				if vng.mediaSession != nil {
					vng.mediaSession.Start()
				}
				vng.Input(vng.CreateConnectionRequest())
				vng.Input(&protos.ConversationEvent{
					Name: "channel",
					Data: map[string]string{"type": "connected", "provider": "vonage"},
					Time: timestamppb.Now(),
				})
			case "stop":
				vng.Cancel()
				return
			default:
				vng.Logger.Debugf("Unhandled event type: %s", textEvent["event"])
			}
		case websocket.BinaryMessage:
			_ = vng.handleMediaEvent(message)
		default:
			vng.Logger.Warn("Unhandled message type", "type", messageType)
		}
	}
}

func (vng *vonageWebsocketStreamer) Send(response internal_type.Stream) error {
	if vng.connection == nil {
		return nil
	}
	switch data := response.(type) {
	case *protos.ConversationInitialization:
		if vng.mediaSession != nil {
			vng.mediaSession.HandleInitialization(data)
		}
	case *protos.ConversationAssistantMessage:
		switch content := data.Message.(type) {
		case *protos.ConversationAssistantMessage_Audio:
			if vng.mediaSession == nil {
				return nil
			}
			if err := vng.mediaSession.HandleAssistantAudio(content.Audio, data.GetCompleted()); err != nil {
				return err
			}
			return nil
		}
	case *protos.ConversationInterruption:
		if data.Type == protos.ConversationInterruption_INTERRUPTION_TYPE_WORD {
			if vng.mediaSession != nil {
				vng.mediaSession.HandleInterrupt()
			}
		}
	case *protos.ConversationDisconnection:
		_ = vng.Disconnect(data.GetType())
		if vng.GetConversationUuid() != "" {
			if cAuth, err := vonageAuth(vng.VaultCredential()); err == nil {
				vonage.NewVoiceClient(cAuth).Hangup(vng.GetConversationUuid())
			}
		}
		vng.stopAudioProcessing()
		vng.Cancel()
	case *protos.ConversationToolCall:
		switch data.GetAction() {
		case protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION:
			result := map[string]string{"status": "completed"}
			if vng.GetConversationUuid() != "" {
				cAuth, err := vonageAuth(vng.VaultCredential())
				if err != nil {
					vng.Logger.Errorf("Error creating Vonage client:", err)
					result = map[string]string{"status": "failed", "reason": fmt.Sprintf("vonage client error: %v", err)}
				} else if _, _, err := vonage.NewVoiceClient(cAuth).Hangup(vng.GetConversationUuid()); err != nil {
					vng.Logger.Errorf("Error ending Vonage call:", err)
					result = map[string]string{"status": "failed", "reason": fmt.Sprintf("hangup failed: %v", err)}
				}
			}
			vng.Input(&protos.ConversationToolCallResult{
				Id:     data.GetId(),
				ToolId: data.GetToolId(),
				Name:   data.GetName(),
				Action: data.GetAction(),
				Result: result,
			})
		case protos.ToolCallAction_TOOL_CALL_ACTION_TRANSFER_CONVERSATION:
			// Vonage transfer is NOT implemented. A blind transfer would be
			// possible via the Voice API "Transfer Call" PUT
			// (https://api.nexmo.com/v1/calls/{uuid}) with an NCCO containing a
			// `connect` action — equivalent to Twilio `<Dial>`. That path would
			// support post_transfer_action=end_call only. resume_ai would
			// require a B2BUA bridge (separate outbound call + WebSocket
			// reconnect on hangup) which Vonage does not natively support.
			vng.Logger.Warnw("Vonage call transfer not yet implemented", "transfer_to", data.GetArgs()["transfer_to"])
			vng.Input(&protos.ConversationToolCallResult{
				Id:     data.GetId(),
				ToolId: data.GetToolId(), Name: data.GetName(), Action: data.GetAction(),
				Result: map[string]string{"status": "failed", "reason": "transfer not supported for Vonage", "next_action": "end_call"},
			})
		}
	default:
		// vng.Logger.Warnw("Vonage Send: unknown message type, skipping", "type", fmt.Sprintf("%T", response))
	}
	return nil
}

func (vng *vonageWebsocketStreamer) handleMediaEvent(message []byte) error {
	if vng.mediaSession == nil {
		return nil
	}
	if err := vng.mediaSession.HandleProviderAudio(message); err != nil {
		return err
	}
	return nil
}

func (vng *vonageWebsocketStreamer) GetConversationUuid() string {
	return vng.ChannelUUID
}

func (vng *vonageWebsocketStreamer) Cancel() error {
	if !vng.closed.CompareAndSwap(false, true) {
		return nil
	}
	vng.stopAudioProcessing()
	vng.writeMu.Lock()
	conn := vng.connection
	vng.connection = nil
	vng.writeMu.Unlock()
	if conn != nil {
		conn.Close()
	}
	vng.BaseStreamer.Cancel()
	return nil
}

func (vng *vonageWebsocketStreamer) sendAudioChunk(chunk *internal_vonage.AudioChunk) error {
	if chunk == nil || len(chunk.Data) == 0 {
		return nil
	}
	vng.writeMu.Lock()
	defer vng.writeMu.Unlock()
	if vng.connection == nil {
		return nil
	}
	return vng.connection.WriteMessage(websocket.BinaryMessage, chunk.Data)
}

func (vng *vonageWebsocketStreamer) stopAudioProcessing() {
	if vng.mediaSession != nil {
		vng.mediaSession.Shutdown()
	}
}
