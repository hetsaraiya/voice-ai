// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_asterisk_websocket

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
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

type asteriskWebsocketStreamer struct {
	internal_telephony_base.BaseTelephonyStreamer

	audioProcessor *internal_asterisk.AudioProcessor
	mediaSession   *internal_telephony_media.MediaSession
	connection     *websocket.Conn
	writeMu        sync.Mutex // guards all writes to connection (gorilla WS is not concurrent-write safe)
	closed         atomic.Bool
	channelName    string

	mediaBuffering bool
	mediaBufferMu  sync.Mutex
}

// NewAsteriskWebsocketStreamer creates a new Asterisk WebSocket streamer.
func NewAsteriskWebsocketStreamer(
	logger commons.Logger,
	connection *websocket.Conn,
	cc *callcontext.CallContext,
	vaultCred *protos.VaultCredential,
) (internal_type.Streamer, error) {
	audioProcessor, err := internal_asterisk.NewAudioProcessor(logger, internal_asterisk.AudioProcessorConfig{
		AsteriskConfig:   internal_audio.NewMulaw8khzMonoAudioConfig(),
		DownstreamConfig: internal_audio.NewLinear16khzMonoAudioConfig(),
		SilenceByte:      0xFF, // mu-law silence
		FrameSize:        160,  // 20ms at 8kHz 8-bit
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create audio processor: %w", err)
	}

	aws := &asteriskWebsocketStreamer{
		BaseTelephonyStreamer: internal_telephony_base.NewBaseTelephonyStreamer(
			logger, cc, vaultCred,
		),
		audioProcessor: audioProcessor,
		connection:     connection,
	}
	audioProcessor.SetOutputChunkCallback(aws.sendAudioChunk)
	aws.mediaSession = internal_telephony_media.NewMediaSession(aws.Ctx, logger, audioProcessor, func() error {
		if aws.isMediaBuffering() {
			return aws.sendCommand("STOP_MEDIA_BUFFERING")
		}
		return nil
	})
	aws.mediaSession.SetInputSink(func(audio []byte) {
		aws.Input(&protos.ConversationUserMessage{
			Message: &protos.ConversationUserMessage_Audio{Audio: audio},
		})
	})
	aws.mediaSession.SetEventSink(func(event *protos.ConversationEvent) {
		if event != nil {
			if event.Data == nil {
				event.Data = map[string]string{}
			}
			event.Data["provider"] = "asterisk_ws"
		}
		aws.Input(event)
	})

	go aws.runWebSocketReader()
	return aws, nil
}

func (aws *asteriskWebsocketStreamer) sendAudioChunk(chunk *internal_asterisk.AudioChunk) error {
	if aws.connection == nil {
		return nil
	}
	aws.writeMu.Lock()
	defer aws.writeMu.Unlock()
	return aws.connection.WriteMessage(websocket.BinaryMessage, chunk.Data)
}

func (aws *asteriskWebsocketStreamer) runWebSocketReader() {
	conn := aws.connection
	if conn == nil {
		return
	}
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if msg := aws.Disconnect(disconnectTypeFromReadError(err)); msg != nil {
				aws.Input(msg)
			}
			aws.Cancel()
			return
		}
		switch messageType {
		case websocket.BinaryMessage:
			_ = aws.handleAudioData(message)
		case websocket.TextMessage:
			event, err := internal_asterisk.ParseAsteriskEvent(string(message))
			if err != nil {
				aws.Logger.Warn("Failed to parse Asterisk event", "error", err.Error(), "message", message)
				continue
			}
			switch event.Event {
			case "MEDIA_START":
				aws.channelName = event.Channel
				aws.ChannelUUID = event.Channel
				aws.Logger.Info("Asterisk media started", "channel", aws.channelName, "optimal_frame_size", event.OptimalFrameSize)
				if event.OptimalFrameSize > 0 {
					aws.audioProcessor.SetOptimalFrameSize(event.OptimalFrameSize)
				}
				aws.mediaSession.Start()
				aws.Input(aws.CreateConnectionRequest())
				// The inbound webhook may not have carried channel_id, so the
				// init payload's client.provider_call_id can be empty. Emit
				// the live channel as metadata so it still lands in conversation
				// metadata.
				if event.Channel != "" {
					aws.Input(&protos.ConversationMetadata{
						Metadata: []*protos.Metadata{{Key: "client.provider_call_id", Value: event.Channel}},
					})
				}
				aws.Input(&protos.ConversationEvent{
					Name: "channel",
					Data: map[string]string{"type": "media_started", "provider": "asterisk_ws", "channel_name": aws.channelName},
					Time: timestamppb.Now(),
				})
			case "MEDIA_STOP":
				aws.Logger.Info("Asterisk media stopped")
				aws.Cancel()
				return
			case "MEDIA_XON":
				aws.audioProcessor.SetXON()
				aws.Input(&protos.ConversationEvent{
					Name: "channel",
					Data: map[string]string{"type": "flow_control", "provider": "asterisk_ws", "state": "xon"},
					Time: timestamppb.Now(),
				})
			case "MEDIA_XOFF":
				aws.audioProcessor.SetXOFF()
				aws.Input(&protos.ConversationEvent{
					Name: "channel",
					Data: map[string]string{"type": "flow_control", "provider": "asterisk_ws", "state": "xoff"},
					Time: timestamppb.Now(),
				})
			case "MEDIA_BUFFERING_COMPLETED":
				aws.setMediaBuffering(false)
			default:
				if event.Command != "" {
					aws.Logger.Debug("Received Asterisk command response", "command", event.Command)
				} else if event.RawMessage != "" {
					aws.Logger.Debug("Received unhandled Asterisk message", "message", event.RawMessage)
				}
			}
		case websocket.CloseMessage:
			aws.Cancel()
			return
		default:
			aws.Logger.Warn("Received unsupported WebSocket message type", "type", messageType)
		}
	}
}

func (aws *asteriskWebsocketStreamer) handleAudioData(audio []byte) error {
	if aws.mediaSession == nil {
		return nil
	}
	if err := aws.mediaSession.HandleProviderAudio(audio); err != nil {
		aws.Logger.Debug("Failed to process input audio", "error", err.Error())
		return nil
	}
	return nil
}

func (aws *asteriskWebsocketStreamer) Send(response internal_type.Stream) error {
	switch data := response.(type) {
	case *protos.ConversationInitialization:
		if aws.mediaSession != nil {
			aws.mediaSession.HandleInitialization(data)
		}
	case *protos.ConversationAssistantMessage:
		switch content := data.Message.(type) {
		case *protos.ConversationAssistantMessage_Audio:
			if aws.mediaSession == nil {
				return nil
			}
			if err := aws.mediaSession.HandleAssistantAudio(content.Audio, data.GetCompleted()); err != nil {
				aws.Logger.Error("Failed to process output audio", "error", err.Error())
				return err
			}
		}

	case *protos.ConversationInterruption:
		if data.Type == protos.ConversationInterruption_INTERRUPTION_TYPE_WORD {
			if aws.mediaSession != nil {
				aws.mediaSession.HandleInterrupt()
			}
		}

	case *protos.ConversationDisconnection:
		// Server-initiated disconnect: the talker already knows the reason
		// (it called Notify with it). No need to round-trip back through
		// CriticalCh — just hang up the Asterisk channel and clean up.
		_ = aws.Disconnect(data.GetType())
		aws.stopAudioProcessing()
		if err := aws.hangupCall(); err != nil {
			aws.Logger.Warnw("Failed to hang up call for disconnection", "error", err)
		}
		aws.Cancel()
	case *protos.ConversationToolCall:
		switch data.GetAction() {
		case protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION:
			aws.stopAudioProcessing()
			result := map[string]string{"status": "completed"}
			if err := aws.hangupCall(); err != nil {
				aws.Logger.Error("Failed to hang up call", "error", err)
				result = map[string]string{"status": "failed", "reason": fmt.Sprintf("hangup failed: %v", err)}
			}
			aws.Input(&protos.ConversationToolCallResult{
				Id:     data.GetId(),
				ToolId: data.GetToolId(),
				Name:   data.GetName(),
				Action: data.GetAction(),
				Result: result,
			})
		case protos.ToolCallAction_TOOL_CALL_ACTION_TRANSFER_CONVERSATION:
			// Asterisk transfer is a blind transfer via ARI `channels/{id}/redirect`
			// — the channel leaves Stasis for the dialplan extension we redirect
			// to, and the AI WebSocket is closed by Cancel() below. The leg
			// cannot be resumed by the AI. Only post_transfer_action=end_call is
			// meaningful — resume_ai is NOT supported here. Supporting resume_ai
			// would require an ARI Bridge + outbound channel + StasisEnd watch
			// (B2BUA pattern, similar to sip/infra/bridge.go).
			//
			// Multi-target failover (try t1, on failure try t2 …) is NOT
			// supported either: once the redirect is dispatched, the channel
			// leaves Stasis. Only the first target from a SEPARATOR-joined
			// transfer_to is dialed; the rest are dropped with a warning.
			raw := data.GetArgs()["transfer_to"]
			targets := aws.SplitTransferTargets(raw)
			if raw == "" || len(targets) == 0 || aws.channelName == "" {
				aws.Input(&protos.ConversationToolCallResult{
					Id:     data.GetId(),
					ToolId: data.GetToolId(), Name: data.GetName(), Action: data.GetAction(),
					Result: map[string]string{"status": "failed", "reason": "missing target or channel name", "next_action": "end_call"},
				})
				return nil
			}
			to := targets[0]
			if len(targets) > 1 {
				aws.Logger.Warnw("Asterisk transfer received multiple targets; failover not supported, using first only",
					"chosen", to, "ignored", targets[1:])
			}
			aws.Logger.Infow("Transferring Asterisk call via ARI redirect", "to", to, "channel", aws.channelName)
			aws.stopAudioProcessing()
			if err := aws.redirectViaARI(to); err != nil {
				aws.Logger.Errorw("ARI redirect failed", "error", err, "to", to)
				aws.Input(&protos.ConversationToolCallResult{
					Id:     data.GetId(),
					ToolId: data.GetToolId(), Name: data.GetName(), Action: data.GetAction(),
					Result: map[string]string{"status": "failed", "reason": fmt.Sprintf("ARI redirect failed: %v", err), "next_action": "end_call"},
				})
			} else {
				aws.Input(&protos.ConversationToolCallResult{
					Id:     data.GetId(),
					ToolId: data.GetToolId(), Name: data.GetName(), Action: data.GetAction(),
					Result: map[string]string{
						"status":      "dispatched",
						"reason":      "transfer dispatched via ARI redirect; outcome not observed",
						"next_action": "end_call",
					},
				})
			}
		}
	}

	return nil
}

func disconnectTypeFromReadError(err error) protos.ConversationDisconnection_DisconnectionType {
	if err == nil {
		return protos.ConversationDisconnection_DISCONNECTION_TYPE_UNSPECIFIED
	}
	if errors.Is(err, io.EOF) {
		return protos.ConversationDisconnection_DISCONNECTION_TYPE_USER
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		return protos.ConversationDisconnection_DISCONNECTION_TYPE_USER
	}
	return protos.ConversationDisconnection_DISCONNECTION_TYPE_UNSPECIFIED
}

func (aws *asteriskWebsocketStreamer) hangupCall() error {
	if wsErr := aws.sendCommand("HANGUP"); wsErr != nil {
		aws.Logger.Warn("Failed to send HANGUP via WebSocket, trying ARI API", "error", wsErr)
		if aws.channelName == "" {
			return wsErr
		}
		if ariErr := aws.hangupViaARI(); ariErr != nil {
			return fmt.Errorf("ws hangup failed: %w; ari hangup failed: %w", wsErr, ariErr)
		}
	}
	return nil
}

func (aws *asteriskWebsocketStreamer) stopAudioProcessing() {
	if aws.mediaSession != nil {
		aws.mediaSession.Shutdown()
	}
}

func (aws *asteriskWebsocketStreamer) sendCommand(command string) error {
	if aws.connection == nil {
		return nil
	}
	aws.writeMu.Lock()
	defer aws.writeMu.Unlock()
	return aws.connection.WriteMessage(websocket.TextMessage, []byte(command))
}

func (aws *asteriskWebsocketStreamer) setMediaBuffering(buffering bool) {
	aws.mediaBufferMu.Lock()
	aws.mediaBuffering = buffering
	aws.mediaBufferMu.Unlock()
}

func (aws *asteriskWebsocketStreamer) isMediaBuffering() bool {
	aws.mediaBufferMu.Lock()
	defer aws.mediaBufferMu.Unlock()
	return aws.mediaBuffering
}

func (aws *asteriskWebsocketStreamer) hangupViaARI() error {
	vaultCredential := aws.VaultCredential()
	if vaultCredential == nil || vaultCredential.GetValue() == nil {
		return fmt.Errorf("vault credential is nil")
	}

	credMap := vaultCredential.GetValue().AsMap()

	ariURL, _ := credMap["ari_url"].(string)
	ariURL = fmt.Sprintf("%s/ari/channels/%s", ariURL, aws.channelName)
	user, _ := credMap["ari_user"].(string)
	password, _ := credMap["ari_password"].(string)

	req, err := http.NewRequest("DELETE", ariURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(user, password)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("ARI API returned status: %d", resp.StatusCode)
	}

	aws.Logger.Info("Successfully hung up call via ARI API", "channel", aws.channelName)
	return nil
}

// redirectViaARI transfers a call by redirecting the Asterisk channel to a new extension.
func (aws *asteriskWebsocketStreamer) redirectViaARI(target string) error {
	vaultCredential := aws.VaultCredential()
	if vaultCredential == nil || vaultCredential.GetValue() == nil {
		return fmt.Errorf("vault credential is nil")
	}
	credMap := vaultCredential.GetValue().AsMap()
	ariURL, _ := credMap["ari_url"].(string)
	user, _ := credMap["ari_user"].(string)
	password, _ := credMap["ari_password"].(string)

	redirectURL := fmt.Sprintf("%s/ari/channels/%s/redirect", ariURL, aws.channelName)
	req, err := http.NewRequest("POST", redirectURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create redirect request: %w", err)
	}
	req.SetBasicAuth(user, password)
	q := req.URL.Query()
	q.Set("endpoint", fmt.Sprintf("PJSIP/%s", target))
	req.URL.RawQuery = q.Encode()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ARI redirect failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("ARI redirect returned status: %d", resp.StatusCode)
	}
	aws.Logger.Infow("Asterisk call redirected via ARI", "channel", aws.channelName, "target", target)
	return nil
}

func (aws *asteriskWebsocketStreamer) Cancel() error {
	if !aws.closed.CompareAndSwap(false, true) {
		return nil
	}
	aws.stopAudioProcessing()
	aws.writeMu.Lock()
	conn := aws.connection
	aws.connection = nil
	aws.writeMu.Unlock()
	if conn != nil {
		conn.Close()
	}
	aws.BaseStreamer.Cancel()
	return nil
}
