// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observe

import (
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/protos"
)

// =============================================================================
// Event Components — the "who" that emitted the event
// =============================================================================

const (
	// ComponentSession is the conversation session lifecycle.
	ComponentSession = "session"

	// ComponentTelephony is the telephony provider layer (Twilio, Asterisk, etc.)
	ComponentTelephony = "telephony"

	// ComponentWebRTC is the WebRTC peer connection layer.
	ComponentWebRTC = "webrtc"

	// ComponentSTT is the speech-to-text transformer.
	ComponentSTT = "stt"

	// ComponentTTS is the text-to-speech transformer.
	ComponentTTS = "tts"

	// ComponentLLM is the language model executor.
	ComponentLLM = "llm"

	// ComponentVAD is the voice activity detector.
	ComponentVAD = "vad"

	// ComponentEOS is the end-of-speech detector.
	ComponentEOS = "eos"

	// ComponentDenoise is the audio denoiser.
	ComponentDenoise = "denoise"

	// ComponentTool is the tool/function calling layer.
	ComponentTool = "tool"

	// ComponentKnowledge is the knowledge base retrieval layer.
	ComponentKnowledge = "knowledge"

	// ComponentRecording is the audio recording layer.
	ComponentRecording = "recording"
)

// =============================================================================
// Event Types — the "what" that happened
// =============================================================================

const (
	EventConnected            = "connected"
	EventCallback             = "callback"
	EventInitializing         = "initializing"
	EventInitialized          = "initialized"
	EventConnectFailed        = "connect_failed"
	EventDisconnected         = "disconnected"
	EventDisconnectRequested  = "disconnect_requested"
	EventCompleted            = "completed"
	EventCleanup              = "cleanup"
	EventModeSwitch           = "mode_switch"
	EventResumed              = "resumed"
	EventSessionResolved      = "session_resolved"
	EventSessionResolveFailed = "session_resolve_failed"
	EventStreamerCreated      = "streamer_created"
	EventStreamerFailed       = "streamer_failed"
	EventTalkerCreated        = "talker_created"
	EventTalkerFailed         = "talker_failed"
	EventTalkStarted          = "talk_started"
	EventHooksBegin           = "hooks_begin"
	EventHooksEnd             = "hooks_end"

	EventCallReceived           = "call_received"
	EventCallAnswered           = "call_answered"
	EventCallStarted            = "call_started"
	EventCallEnded              = "call_ended"
	EventCallFailed             = "call_failed"
	EventCallCompleted          = "call_completed"
	EventOutboundRequested      = "outbound_requested"
	EventOutboundDialed         = "outbound_dialed"
	EventOutboundDispatched     = "outbound_dispatched"
	EventOutboundDispatchFailed = "outbound_dispatch_failed"
	EventProviderAnswered       = "provider_answered"
	EventSessionConnected       = "session_connected"
	EventAssistantLoaded        = "assistant_loaded"
	EventConversationCreated    = "conversation_created"
	EventContextSaved           = "context_saved"

	// --- SIP-specific ---
	EventInviteReceived    = "invite_received"
	EventRouteResolved     = "route_resolved"
	EventAuthenticated     = "authenticated"
	EventByeReceived       = "bye_received"
	EventCancelReceived    = "cancel_received"
	EventHold              = "hold"
	EventResume            = "resume"
	EventReInvite          = "reinvite"
	EventTransferRequested = "transfer_requested"
	EventTransferring      = "transferring"
	EventTransferConnected = "transfer_connected"
	EventTransferCompleted = "transfer_completed"
	EventTransferFailed    = "transfer_failed"
	EventRegisterActive    = "register_active"
	EventRegisterFailed    = "register_failed"
	EventDTMF              = "dtmf"

	// --- WebRTC-specific ---
	EventICEConnected       = "ice_connected"
	EventICEFailed          = "ice_failed"
	EventPeerConnected      = "peer_connected"
	EventPeerFailed         = "peer_failed"
	EventPeerDisconnected   = "peer_disconnected"
	EventAudioTrackReceived = "audio_track_received"

	// --- Tool ---
	EventToolCallStarted   = "tool_call_started"
	EventToolCallCompleted = "tool_call_completed"

	// --- Recording ---
	EventRecordingStarted = "recording_started"
	EventRecordingStopped = "recording_stopped"

	// --- Errors ---
	EventError = "error"
)

// =============================================================================
// Metric Names — standardized across all channels
// =============================================================================

const (
	// --- Call duration ---
	MetricCallDurationMs  = "call.duration_ms"
	MetricSetupDurationMs = "call.setup_duration_ms"
	MetricRingDurationMs  = "call.ring_duration_ms"

	// --- Call status ---
	MetricCallStatus    = "call.status"
	MetricCallEndReason = "call.end_reason"
	MetricCallFailed    = "call.failed"

	// --- SIP ---
	MetricSIPRegisterFailure = "sip.register_failure"

	// --- Transfer ---
	MetricTransferDurationMs = "transfer.bridge_duration_ms"

	// --- RTP ---
	MetricRTPPacketsSent     = "rtp.packets_sent"
	MetricRTPPacketsReceived = "rtp.packets_received"
	MetricRTPBytesSent       = "rtp.bytes_sent"
	MetricRTPBytesReceived   = "rtp.bytes_received"

	// --- WebRTC ---
	MetricICELatencyMs           = "webrtc.ice_latency_ms"
	MetricWebRTCOutputQueueDrops = "webrtc.output_queue_dropped_frames"

	// --- Telephony ---
	MetricTelephonyStatus   = "telephony.status"
	MetricTelephonyDuration = "telephony_duration"
	MetricTelephonyPrice    = "telephony.price"
)

// =============================================================================
// Data Keys — standardized keys for event Data maps
// =============================================================================

const (
	DataType           = "type"
	DataProvider       = "provider"
	DataDirection      = "direction"
	DataReason         = "reason"
	DataError          = "error"
	DataStage          = "stage"
	DataDID            = "did"
	DataCaller         = "caller"
	DataCallee         = "callee"
	DataContextID      = "context_id"
	DataCodec          = "codec"
	DataMode           = "mode"
	DataFrom           = "from"
	DataTo             = "to"
	DataDuration       = "duration_ms"
	DataMessages       = "messages"
	DataDigit          = "digit"
	DataTarget         = "target"
	DataOutboundCallID = "outbound_call_id"
	DataStatus         = "status"
)

// =============================================================================
// Client Metadata Keys — standardized keys for conversation metadata
// =============================================================================

const (
	ClientPhone          = "client.phone"            // Client's phone number (caller on inbound, callee on outbound)
	ClientAssistantPhone = "client.assistant_phone"  // Our phone number / DID
	ClientDirection      = "client.direction"        // "inbound" or "outbound"
	ClientChannel        = "client.channel"          // sip, twilio, vonage, exotel, asterisk, webrtc
	ClientProviderCallID = "client.provider_call_id" // Provider-specific call ID (CallSid, UUID, SIP Call-ID, etc.)
	ClientContextID      = "client.context_id"       // Internal context ID
	ClientCodec          = "client.codec"            // Audio codec (PCMU, opus, linear16, etc.)
	ClientSampleRate     = "client.sample_rate"      // Audio sample rate (8000, 16000, 48000)
)

// ClientMetadata returns standardized client metadata for a conversation.
// Each field is emitted only when non-empty so callers can pass "" for fields
// already covered by ConversationInitialization.Metadata.
func ClientMetadata(phone, assistantPhone, direction, provider, providerCallID, contextID, codec, sampleRate string) []*types.Metadata {
	md := make([]*types.Metadata, 0, 8)
	if direction != "" {
		md = append(md, types.NewMetadata(ClientDirection, direction))
	}
	if provider != "" {
		md = append(md, types.NewMetadata(ClientChannel, provider))
	}
	if phone != "" {
		md = append(md, types.NewMetadata(ClientPhone, phone))
	}
	if assistantPhone != "" {
		md = append(md, types.NewMetadata(ClientAssistantPhone, assistantPhone))
	}
	if providerCallID != "" {
		md = append(md, types.NewMetadata(ClientProviderCallID, providerCallID))
	}
	if contextID != "" {
		md = append(md, types.NewMetadata(ClientContextID, contextID))
	}
	if codec != "" {
		md = append(md, types.NewMetadata(ClientCodec, codec))
	}
	if sampleRate != "" {
		md = append(md, types.NewMetadata(ClientSampleRate, sampleRate))
	}
	return md
}

// CallStatusMetric returns a CONVERSATION_STATUS metric for call lifecycle tracking.
func CallStatusMetric(status, reason string) []*protos.Metric {
	return []*protos.Metric{
		{Name: "status", Value: status, Description: reason},
	}
}
