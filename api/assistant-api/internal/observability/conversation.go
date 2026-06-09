// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability

import (
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
)

// Conversation metric names mirror the current observe/type_enums names.
const (
	MetricConversationStatus      = "status"
	MetricConversationDuration    = "duration"
	MetricConversationSTTDuration = "stt_duration"
	MetricConversationTTSDuration = "tts_duration"

	MetricConversationComplete   = "complete"
	MetricConversationInProgress = "in_progress"
)

// Current call, telephony, SIP, transfer, RTP, and WebRTC metric names.
const (
	MetricCallDurationMs         = "call.duration_ms"
	MetricSetupDurationMs        = "call.setup_duration_ms"
	MetricRingDurationMs         = "call.ring_duration_ms"
	MetricCallStatus             = "call.status"
	MetricSIPRegisterFailure     = "sip.register_failure"
	MetricSIPRegistrationStatus  = "sip.registration.status"
	MetricTransferDurationMs     = "transfer.bridge_duration_ms"
	MetricRTPPacketsSent         = "rtp.packets_sent"
	MetricRTPPacketsReceived     = "rtp.packets_received"
	MetricRTPBytesSent           = "rtp.bytes_sent"
	MetricRTPBytesReceived       = "rtp.bytes_received"
	MetricICELatencyMs           = "webrtc.ice_latency_ms"
	MetricWebRTCOutputQueueDrops = "webrtc.output_queue_dropped_frames"
	MetricTelephonyStatus        = "telephony.status"
	MetricTelephonyDuration      = "telephony_duration"
	MetricTelephonyPrice         = "telephony.price"
)

// Current call.status metric values.
const (
	MetricCallStatusComplete   = "COMPLETE"
	MetricCallStatusFailed     = "FAILED"
	MetricCallStatusInProgress = "INPROGRESS"
)

// Current turn and provider metric names.
const (
	MetricUserTurn             = "user_turn"
	MetricAssistantTurn        = "assistant_turn"
	MetricSTTLatencyMs         = "stt_latency_ms"
	MetricTTSLatencyMs         = "tts_latency_ms"
	MetricEOSLatencyMs         = "eos_latency_ms"
	MetricEOSTextToTriggerMs   = "eos_text_to_trigger_ms"
	MetricEOSWordCount         = "eos_word_count"
	MetricEOSCharCount         = "eos_char_count"
	MetricEOSConfidence        = "eos_confidence"
	MetricKnowledgeLatencyMs   = "knowledge_latency_ms"
	MetricLLMError             = "llm_error"
	MetricSTTError             = "stt_error"
	MetricTTSError             = "tts_error"
	MetricDiscardedTTSChunk    = "discarded_tts_chunk"
	MetricDiscardedTTS         = "discarded_tts"
	MetricTimeTaken            = "time_taken"
	MetricStatus               = "status"
	MetricInputToken           = "input_token"
	MetricOutputToken          = "output_token"
	MetricTotalToken           = "total_token"
	MetricCachedContentToken   = "cached_content_token"
	MetricCost                 = "cost"
	MetricInputCost            = "input_cost"
	MetricOutputCost           = "output_cost"
	MetricLLMRequestID         = "llm_request_id"
	MetricTokenPerSecond       = "token_pre_second"
	MetricTimeToFirstToken     = "time_to_first_token"
	MetricProviderTotalTime    = "provider_total_time"
	MetricProviderGenerateTime = "provider_generate_time"
)

// Conversation metadata names mirror the current metadata keys.
const (
	MetadataClientPhone          = "client.phone"
	MetadataClientAssistantPhone = "client.assistant_phone"
	MetadataClientDirection      = "client.direction"
	MetadataClientChannel        = "client.channel"
	MetadataClientProviderCallID = "client.provider_call_id"
	MetadataClientContextID      = "client.context_id"
	MetadataClientCodec          = "client.codec"
	MetadataClientSampleRate     = "client.sample_rate"

	MetadataClientTimezone       = "client.timezone"
	MetadataClientPlatform       = "client.platform"
	MetadataClientLanguage       = "client.language"
	MetadataClientUserAgent      = "client.user_agent"
	MetadataClientReferrer       = "client.referrer"
	MetadataClientConnectionType = "client.connection_type"
	MetadataClientLatitude       = "client.latitude"
	MetadataClientLongitude      = "client.longitude"
)

// Current metadata constant names kept as aliases for the existing observe names.
const (
	ClientPhone          = MetadataClientPhone
	ClientAssistantPhone = MetadataClientAssistantPhone
	ClientDirection      = MetadataClientDirection
	ClientChannel        = MetadataClientChannel
	ClientProviderCallID = MetadataClientProviderCallID
	ClientContextID      = MetadataClientContextID
	ClientCodec          = MetadataClientCodec
	ClientSampleRate     = MetadataClientSampleRate
)

// Current conversation, call-context, and SIP lifecycle metadata names.
const (
	MetadataLanguage     = "language"
	MetadataLanguageCode = "language_code"

	MetadataDisconnectReason    = "disconnect_reason"
	MetadataDisconnectText      = "disconnect_text"
	MetadataDisconnectRawReason = "disconnect_raw_reason"

	MetadataCallStatus         = "call_status"
	MetadataCallError          = "call_error"
	MetadataFailureClass       = "failure_class"
	MetadataFailureReason      = "failure_reason"
	MetadataRetryable          = "retryable"
	MetadataProviderStatusCode = "provider_status_code"
	MetadataTelephonyError     = "telephony.error"
)

// Current SIP transfer metadata names.
const (
	MetadataBridgeTransferTarget         = "bridge_transfer_target"
	MetadataBridgeTransferStatus         = "bridge_transfer_status"
	MetadataBridgeTransferDuration       = "bridge_transfer_duration"
	MetadataBridgeTransferOutboundCallID = "bridge_transfer_outbound_call_id"
)

// ClientMetadata returns standardized client metadata for a conversation.
func ClientMetadata(phone, assistantPhone, direction, provider, providerCallID, contextID, codec, sampleRate string) []*protos.Metadata {
	metadata := make([]*protos.Metadata, 0, 8)
	metadata = appendMetadata(metadata, MetadataClientDirection, direction)
	metadata = appendMetadata(metadata, MetadataClientChannel, provider)
	metadata = appendMetadata(metadata, MetadataClientPhone, phone)
	metadata = appendMetadata(metadata, MetadataClientAssistantPhone, assistantPhone)
	metadata = appendMetadata(metadata, MetadataClientProviderCallID, providerCallID)
	metadata = appendMetadata(metadata, MetadataClientContextID, contextID)
	metadata = appendMetadata(metadata, MetadataClientCodec, codec)
	metadata = appendMetadata(metadata, MetadataClientSampleRate, sampleRate)
	return metadata
}

// DisconnectMetadata returns standardized terminal disconnect metadata.
func DisconnectMetadata(reason, text, rawReason string) []*protos.Metadata {
	metadata := make([]*protos.Metadata, 0, 3)
	metadata = appendMetadata(metadata, MetadataDisconnectReason, reason)
	metadata = appendMetadata(metadata, MetadataDisconnectText, text)
	metadata = appendMetadata(metadata, MetadataDisconnectRawReason, rawReason)
	return metadata
}

// CallStatusMetric returns the current CONVERSATION_STATUS metric shape.
func CallStatusMetric(status, reason string) []*protos.Metric {
	return []*protos.Metric{{
		Name:        MetricConversationStatus,
		Value:       status,
		Description: reason,
	}}
}

func appendMetadata(metadata []*protos.Metadata, key, value string) []*protos.Metadata {
	if validator.NotBlank(value) {
		return append(metadata, &protos.Metadata{Key: key, Value: value})
	}
	return metadata
}
