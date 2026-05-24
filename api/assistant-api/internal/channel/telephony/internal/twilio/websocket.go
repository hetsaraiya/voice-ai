// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_twilio_telephony

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
	internal_twilio "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/twilio/internal"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/protos"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type twilioWebsocketStreamer struct {
	internal_telephony_base.BaseTelephonyStreamer

	mediaSession *internal_telephony_media.MediaSession

	streamID   string
	connection *websocket.Conn
	writeMu    sync.Mutex
	closed     atomic.Bool
}

func NewTwilioWebsocketStreamer(logger commons.Logger, connection *websocket.Conn, cc *callcontext.CallContext, vaultCred *protos.VaultCredential) (internal_type.Streamer, error) {
	audioProcessor, err := internal_twilio.NewAudioProcessor(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Twilio audio processor: %w", err)
	}
	tws := &twilioWebsocketStreamer{
		BaseTelephonyStreamer: internal_telephony_base.NewBaseTelephonyStreamer(
			logger, cc, vaultCred,
			internal_telephony_base.WithSourceAudioConfig(internal_audio.NewMulaw8khzMonoAudioConfig()),
		),
		streamID:   "",
		connection: connection,
	}
	audioProcessor.SetOutputChunkCallback(tws.sendAudioChunk)
	tws.mediaSession = internal_telephony_media.NewMediaSession(context.Background(), logger, audioProcessor, func() error {
		return tws.sendTwilioMessage("clear", nil)
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
			event.Data["provider"] = "twilio"
		}
		tws.Input(event)
	})
	go tws.runWebSocketReader()
	return tws, nil
}

func (tws *twilioWebsocketStreamer) runWebSocketReader() {
	conn := tws.connection
	if conn == nil {
		return
	}
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			tws.stopAudioProcessing()
			if msg := tws.Disconnect(protos.ConversationDisconnection_DISCONNECTION_TYPE_USER); msg != nil {
				tws.Input(msg)
			}
			tws.BaseStreamer.Cancel()
			return
		}
		var mediaEvent internal_twilio.TwilioMediaEvent
		if err := json.Unmarshal(message, &mediaEvent); err != nil {
			tws.Logger.Error("Failed to unmarshal Twilio media event", "error", err.Error())
			continue
		}
		switch mediaEvent.Event {
		case "connected":
			tws.Input(&protos.ConversationEvent{
				Name: "channel",
				Data: map[string]string{"type": "connected", "provider": "twilio"},
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
				Data: map[string]string{"type": "stream_started", "provider": "twilio", "stream_id": tws.streamID},
				Time: timestamppb.Now(),
			})
		case "media":
			_ = tws.handleMediaEvent(mediaEvent)
		case "stop":
			tws.Logger.Info("Twilio stream stopped")
			tws.Cancel()
			return
		default:
			tws.Logger.Warn("Unhandled Twilio event", "event", mediaEvent.Event)
		}
	}
}

func (tws *twilioWebsocketStreamer) Send(response internal_type.Stream) error {
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
		// Server-initiated disconnect: the talker already knows the reason
		// (it called Notify with it). No need to round-trip back through
		// CriticalCh — just notify the carrier via Hangup and clean up.
		_ = tws.Disconnect(data.GetType())
		if tws.GetConversationUuid() != "" {
			if client, err := twilioClient(tws.VaultCredential()); err == nil {
				params := &openapi.UpdateCallParams{}
				params.SetStatus("completed")
				client.Api.UpdateCall(tws.GetConversationUuid(), params)
			}
		}
		tws.stopAudioProcessing()
		tws.Cancel()
	case *protos.ConversationToolCall:
		switch data.GetAction() {
		case protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION:
			result := map[string]string{"status": "completed"}
			if tws.GetConversationUuid() != "" {
				client, err := twilioClient(tws.VaultCredential())
				if err != nil {
					tws.Logger.Errorf("Error creating Twilio client:", err)
					result = map[string]string{"status": "failed", "reason": fmt.Sprintf("twilio client error: %v", err)}
				} else {
					params := &openapi.UpdateCallParams{}
					params.SetStatus("completed")
					if _, err := client.Api.UpdateCall(tws.GetConversationUuid(), params); err != nil {
						tws.Logger.Errorf("Error ending Twilio call:", err)
						result = map[string]string{"status": "failed", "reason": fmt.Sprintf("end call failed: %v", err)}
					}
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
			// Twilio transfer is a blind transfer via REST `UpdateCall` with TwiML
			// `<Dial>`. Twilio takes over the leg; the AI WebSocket is closed by
			// Cancel() below and cannot be resumed. As a result, only
			// post_transfer_action=end_call is meaningful — resume_ai is NOT
			// supported on Twilio. Supporting resume_ai would require a TwiML
			// `<Dial action="...">` callback that hands the leg back to a fresh
			// assistant Stream on hangup (not implemented).
			//
			// Multi-target failover (try t1, on failure try t2 …) is NOT
			// supported either: once the redirect is dispatched, the WebSocket
			// is closed and we lose the ability to retry. Only the first
			// target from a SEPARATOR-joined transfer_to is dialed; the rest
			// are dropped with a warning.
			raw := data.GetArgs()["transfer_to"]
			targets := tws.SplitTransferTargets(raw)
			if raw == "" || len(targets) == 0 || tws.GetConversationUuid() == "" {
				tws.Input(&protos.ConversationToolCallResult{
					Id:     data.GetId(),
					ToolId: data.GetToolId(), Name: data.GetName(), Action: data.GetAction(),
					Result: map[string]string{"status": "failed", "reason": "missing target or call ID", "next_action": "end_call"},
				})
				return nil
			}
			to := targets[0]
			if len(targets) > 1 {
				tws.Logger.Warnw("Twilio transfer received multiple targets; failover not supported, using first only",
					"chosen", to, "ignored", targets[1:])
			}
			tws.Logger.Infow("Transferring Twilio call", "to", to)
			client, err := twilioClient(tws.VaultCredential())
			if err != nil {
				tws.Input(&protos.ConversationToolCallResult{
					Id:     data.GetId(),
					ToolId: data.GetToolId(), Name: data.GetName(), Action: data.GetAction(),
					Result: map[string]string{"status": "failed", "reason": fmt.Sprintf("twilio client error: %v", err), "next_action": "end_call"},
				})
				return nil
			}
			params := &openapi.UpdateCallParams{}
			params.SetTwiml(fmt.Sprintf(`<Response><Dial>%s</Dial></Response>`, to))
			if _, err := client.Api.UpdateCall(tws.GetConversationUuid(), params); err != nil {
				tws.Input(&protos.ConversationToolCallResult{
					Id:     data.GetId(),
					ToolId: data.GetToolId(), Name: data.GetName(), Action: data.GetAction(),
					Result: map[string]string{"status": "failed", "reason": fmt.Sprintf("transfer failed: %v", err), "next_action": "end_call"},
				})
			} else {
				tws.Input(&protos.ConversationToolCallResult{
					Id:     data.GetId(),
					ToolId: data.GetToolId(), Name: data.GetName(), Action: data.GetAction(),
					Result: map[string]string{
						"status":      "dispatched",
						"reason":      "transfer dispatched to Twilio; outcome not observed",
						"next_action": "end_call",
					},
				})
			}
		}
	default:
		tws.Logger.Warnw("Twilio Send: unknown message type, skipping", "type", fmt.Sprintf("%T", response))
	}
	return nil
}

func (tws *twilioWebsocketStreamer) handleStartEvent(mediaEvent internal_twilio.TwilioMediaEvent) {
	tws.streamID = mediaEvent.StreamSid
}

func (tws *twilioWebsocketStreamer) GetConversationUuid() string {
	return tws.ChannelUUID
}

func (tws *twilioWebsocketStreamer) Cancel() error {
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

func (tws *twilioWebsocketStreamer) sendAudioChunk(chunk *internal_twilio.AudioChunk) error {
	if chunk == nil || len(chunk.Data) == 0 {
		return nil
	}
	return tws.sendTwilioMessage("media", map[string]interface{}{
		"payload": tws.Encoder().EncodeToString(chunk.Data),
	})
}

func (tws *twilioWebsocketStreamer) stopAudioProcessing() {
	if tws.mediaSession != nil {
		tws.mediaSession.Shutdown()
	}
}

func (tws *twilioWebsocketStreamer) handleMediaEvent(mediaEvent internal_twilio.TwilioMediaEvent) error {
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

func (tws *twilioWebsocketStreamer) sendTwilioMessage(
	eventType string,
	mediaData map[string]interface{}) error {
	if tws.connection == nil || tws.streamID == "" {
		return nil
	}
	message := map[string]interface{}{
		"event":     eventType,
		"streamSid": tws.streamID,
	}
	if mediaData != nil {
		message["media"] = mediaData
	}

	twilioMessageJSON, err := json.Marshal(message)
	if err != nil {
		return tws.handleError("Failed to marshal Twilio message", err)
	}

	tws.writeMu.Lock()
	defer tws.writeMu.Unlock()
	if tws.connection == nil {
		return nil
	}
	if err := tws.connection.WriteMessage(websocket.TextMessage, twilioMessageJSON); err != nil {
		return tws.handleError("Failed to send message to Twilio", err)
	}

	return nil
}

func (tws *twilioWebsocketStreamer) handleError(message string, err error) error {
	tws.Logger.Error(message, "error", err.Error())
	return err
}
