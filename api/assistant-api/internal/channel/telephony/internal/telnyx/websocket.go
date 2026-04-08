// Copyright (c) 2023-2025 RapidaAI
// Author: RapidaAI Team <team@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_telnyx_telephony

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	internal_audio "github.com/rapidaai/api/assistant-api/internal/audio"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	internal_telephony_base "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/base"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/protos"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RAPIDA_AUDIO_CONFIG is the internal Rapida audio format (linear16 16kHz).
var RAPIDA_AUDIO_CONFIG = internal_audio.NewLinear16khzMonoAudioConfig()

// PCMU_8K_AUDIO_CONFIG is the Telnyx-native audio format (PCMU 8kHz).
var PCMU_8K_AUDIO_CONFIG = internal_audio.NewMulaw8khzMonoAudioConfig()

// TelnyxWebSocketEvent represents a Telnyx WebSocket event.
type TelnyxWebSocketEvent struct {
	Event    string                 `json:"event"`
	StreamID string                 `json:"stream_id"`
	Start    *TelnyxStartEvent      `json:"start,omitempty"`
	Media    *TelnyxMediaEvent      `json:"media,omitempty"`
	Stop     *TelnyxStopEvent       `json:"stop,omitempty"`
}

// TelnyxStartEvent contains the start event details.
type TelnyxStartEvent struct {
	CallControlID string                    `json:"call_control_id"`
	MediaFormat   TelnyxMediaFormat         `json:"media_format"`
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

	streamID       string
	callControlID  string
	connection     *websocket.Conn
	encoder        *base64.Encoding
	telephony      *telnyxTelephony
}

// NewTelnyxWebsocketStreamer creates a Telnyx WebSocket streamer.
// Telnyx sends PCMU 8kHz (µ-law 8kHz) which needs resampling to linear16 16kHz.
func NewTelnyxWebsocketStreamer(logger commons.Logger, connection *websocket.Conn, cc *callcontext.CallContext, vaultCred *protos.VaultCredential) internal_type.Streamer {
	tws := &telnyxWebsocketStreamer{
		BaseTelephonyStreamer: internal_telephony_base.NewBaseTelephonyStreamer(
			logger, cc, vaultCred,
			internal_telephony_base.WithSourceAudioConfig(internal_audio.NewMulaw8khzMonoAudioConfig()),
		),
		streamID:   "",
		connection: connection,
		encoder:    base64.StdEncoding,
		telephony: &telnyxTelephony{
			logger: logger,
		},
	}
	go tws.runWebSocketReader()
	return tws
}

// runWebSocketReader reads messages from the WebSocket connection.
func (tws *telnyxWebsocketStreamer) runWebSocketReader() {
	conn := tws.connection
	if conn == nil {
		return
	}

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			tws.Logger.Errorf("WebSocket read error: %v", err)
			tws.PushDisconnection(protos.ConversationDisconnection_DISCONNECTION_TYPE_USER)
			tws.BaseStreamer.Cancel()
			return
		}

		// Telnyx sends JSON text messages (unlike Vonage which sends binary)
		if messageType != websocket.TextMessage {
			tws.Logger.Warnf("Unexpected message type: %d", messageType)
			continue
		}

		var event TelnyxWebSocketEvent
		if err := json.Unmarshal(message, &event); err != nil {
			tws.Logger.Errorf("Failed to unmarshal Telnyx event: %v", err)
			continue
		}

		switch event.Event {
		case "start":
			tws.handleStartEvent(event)
			tws.PushInput(tws.CreateConnectionRequest())
			tws.PushInputLow(&protos.ConversationEvent{
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
			msg, _ := tws.handleMediaEvent(event)
			if msg != nil {
				tws.PushInput(msg)
			}

		case "stop":
			tws.Logger.Info("Telnyx stream stopped")
			tws.Cancel()
			tws.PushDisconnection(protos.ConversationDisconnection_DISCONNECTION_TYPE_USER)
			return

		case "dtmf":
			// Handle DTMF events if needed
			tws.Logger.Debugf("DTMF event received: %+v", event)

		default:
			tws.Logger.Warnf("Unhandled Telnyx event: %s", event.Event)
		}
	}
}

// handleStartEvent processes the start event from Telnyx.
func (tws *telnyxWebsocketStreamer) handleStartEvent(event TelnyxWebSocketEvent) {
	if event.Start != nil {
		tws.streamID = event.StreamID
		tws.callControlID = event.Start.CallControlID
		tws.ChannelUUID = event.Start.CallControlID

		tws.Logger.Debugf("Telnyx stream started | stream_id: %s, call_control_id: %s, format: %s %dHz",
			tws.streamID, tws.callControlID,
			event.Start.MediaFormat.Encoding,
			event.Start.MediaFormat.SampleRate)
	}
}

// handleMediaEvent processes incoming media events from Telnyx.
func (tws *telnyxWebsocketStreamer) handleMediaEvent(event TelnyxWebSocketEvent) (*protos.ConversationUserMessage, error) {
	if event.Media == nil {
		return nil, nil
	}

	// Decode base64 payload
	payloadBytes, err := tws.encoder.DecodeString(event.Media.Payload)
	if err != nil {
		tws.Logger.Warnf("Failed to decode media payload: %v", err)
		return nil, nil
	}

	var audioRequest *protos.ConversationUserMessage
	tws.WithInputBuffer(func(buf *bytes.Buffer) {
		buf.Write(payloadBytes)
		if buf.Len() >= tws.InputBufferThreshold() {
			audioRequest = tws.CreateVoiceRequest(buf.Bytes())
			buf.Reset()
		}
	})

	return audioRequest, nil
}

// Send sends audio or control messages to Telnyx.
func (tws *telnyxWebsocketStreamer) Send(response internal_type.Stream) error {
	if tws.connection == nil {
		return nil
	}

	switch data := response.(type) {
	case *protos.ConversationAssistantMessage:
		switch content := data.Message.(type) {
		case *protos.ConversationAssistantMessage_Audio:
			// Resample from internal format (linear16 16kHz) to Telnyx format (PCMU 8kHz)
			audioData, err := tws.Resampler().Resample(content.Audio, RAPIDA_AUDIO_CONFIG, PCMU_8K_AUDIO_CONFIG)
			if err != nil {
				tws.Logger.Warnw("Failed to resample output audio to PCMU 8kHz, forwarding raw bytes",
					"error", err.Error(),
				)
				audioData = content.Audio
			}

			var sendErr error
			tws.WithOutputBuffer(func(buf *bytes.Buffer) {
				buf.Write(audioData)
				for buf.Len() >= tws.OutputFrameSize() && tws.streamID != "" {
					chunk := buf.Next(tws.OutputFrameSize())
					if err := tws.sendMedia(chunk); err != nil {
						tws.Logger.Errorf("Failed to send audio chunk: %v", err)
						sendErr = err
						return
					}
				}
				// Flush remaining audio when response is marked complete
				if data.GetCompleted() && buf.Len() > 0 {
					remainingChunk := buf.Bytes()
					if err := tws.sendMedia(remainingChunk); err != nil {
						tws.Logger.Errorf("Failed to send final audio chunk: %v", err)
						sendErr = err
						return
					}
					buf.Reset()
				}
			})
			return sendErr
		}

	case *protos.ConversationInterruption:
		if data.Type == protos.ConversationInterruption_INTERRUPTION_TYPE_WORD {
			tws.ResetOutputBuffer()
			if err := tws.sendClear(); err != nil {
				tws.Logger.Errorf("Error sending clear command: %v", err)
			}
		}

	case *protos.ConversationDirective:
		if data.GetType() == protos.ConversationDirective_END_CONVERSATION {
			if tws.callControlID != "" {
				// Use Call Control API to hang up
				if err := tws.telephony.HangupCall(tws.callControlID, tws.VaultCredential()); err != nil {
					tws.Logger.Errorf("Error ending Telnyx call: %v", err)
				}
			}
			if err := tws.Cancel(); err != nil {
				tws.Logger.Errorf("Error disconnecting: %v", err)
			}
		}
	}

	return nil
}

// sendMedia sends audio data to Telnyx via WebSocket.
func (tws *telnyxWebsocketStreamer) sendMedia(audioData []byte) error {
	if tws.connection == nil || tws.streamID == "" {
		return nil
	}

	message := map[string]interface{}{
		"event":     "media",
		"stream_id": tws.streamID,
		"media": map[string]interface{}{
			"payload": tws.encoder.EncodeToString(audioData),
		},
	}

	messageJSON, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal media message: %w", err)
	}

	return tws.connection.WriteMessage(websocket.TextMessage, messageJSON)
}

// sendClear sends a clear command to Telnyx to interrupt audio.
func (tws *telnyxWebsocketStreamer) sendClear() error {
	if tws.connection == nil || tws.streamID == "" {
		return nil
	}

	message := map[string]interface{}{
		"event":     "clear",
		"stream_id": tws.streamID,
	}

	messageJSON, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal clear message: %w", err)
	}

	return tws.connection.WriteMessage(websocket.TextMessage, messageJSON)
}

// sendDTMF sends DTMF digits to Telnyx.
func (tws *telnyxWebsocketStreamer) sendDTMF(digit string) error {
	if tws.connection == nil || tws.streamID == "" {
		return nil
	}

	message := map[string]interface{}{
		"event":     "dtmf",
		"stream_id": tws.streamID,
		"dtmf": map[string]interface{}{
			"digit": digit,
		},
	}

	messageJSON, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal dtmf message: %w", err)
	}

	return tws.connection.WriteMessage(websocket.TextMessage, messageJSON)
}

// GetConversationUuid returns the call control ID.
func (tws *telnyxWebsocketStreamer) GetConversationUuid() string {
	return tws.ChannelUUID
}

// Cancel closes the WebSocket connection.
func (tws *telnyxWebsocketStreamer) Cancel() error {
	if tws.connection != nil {
		tws.connection.Close()
		tws.connection = nil
	}
	tws.BaseStreamer.Cancel()
	return nil
}

// NotifyMode is a no-op for telephony providers.
func (tws *telnyxWebsocketStreamer) NotifyMode(mode protos.StreamMode) {
	// No-op for telephony
}

// ProcessIncomingCallWebhook handles an incoming call webhook from Telnyx.
// This is called by the inbound call handler to start streaming.
func ProcessIncomingCallWebhook(c *gin.Context, appCfg *config.AssistantConfig, logger commons.Logger) error {
	// Parse the webhook payload
	body, err := c.GetRawData()
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	var webhook map[string]interface{}
	if err := json.Unmarshal(body, &webhook); err != nil {
		return fmt.Errorf("failed to parse webhook: %w", err)
	}

	// Extract call_control_id from the webhook
	var callControlID string
	if data, ok := webhook["data"].(map[string]interface{}); ok {
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			if ccid, ok := payload["call_control_id"].(string); ok {
				callControlID = ccid
			}
		}
	}

	if callControlID == "" {
		return fmt.Errorf("call_control_id not found in webhook")
	}

	logger.Debugf("Processing incoming call webhook for call_control_id: %s", callControlID)

	// Return JSON to instruct Telnyx to start streaming
	contextID := c.Param("contextId")
	streamURL := fmt.Sprintf("wss://%s/%s",
		appCfg.PublicAssistantHost,
		internal_type.GetContextAnswerPath("telnyx", contextID))

	c.JSON(200, gin.H{
		"result": "streaming.start",
		"params": gin.H{
			"stream_url":    streamURL,
			"stream_track":  "both_tracks",
		},
	})

	return nil
}
