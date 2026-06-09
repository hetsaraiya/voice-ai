// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability

import (
	"testing"

	"github.com/rapidaai/protos"
)

func TestConversationMetricNames_MirrorCurrentImplementation(t *testing.T) {
	tests := []struct {
		actual   string
		expected string
	}{
		{MetricConversationStatus, "status"},
		{MetricConversationDuration, "duration"},
		{MetricConversationSTTDuration, "stt_duration"},
		{MetricConversationTTSDuration, "tts_duration"},
		{MetricConversationComplete, "complete"},
		{MetricConversationInProgress, "in_progress"},
		{MetricCallDurationMs, "call.duration_ms"},
		{MetricSetupDurationMs, "call.setup_duration_ms"},
		{MetricRingDurationMs, "call.ring_duration_ms"},
		{MetricCallStatus, "call.status"},
		{MetricSIPRegisterFailure, "sip.register_failure"},
		{MetricTransferDurationMs, "transfer.bridge_duration_ms"},
		{MetricRTPPacketsSent, "rtp.packets_sent"},
		{MetricRTPPacketsReceived, "rtp.packets_received"},
		{MetricRTPBytesSent, "rtp.bytes_sent"},
		{MetricRTPBytesReceived, "rtp.bytes_received"},
		{MetricICELatencyMs, "webrtc.ice_latency_ms"},
		{MetricWebRTCOutputQueueDrops, "webrtc.output_queue_dropped_frames"},
		{MetricTelephonyStatus, "telephony.status"},
		{MetricTelephonyDuration, "telephony_duration"},
		{MetricTelephonyPrice, "telephony.price"},
		{MetricCallStatusComplete, "COMPLETE"},
		{MetricCallStatusFailed, "FAILED"},
		{MetricCallStatusInProgress, "INPROGRESS"},
		{MetricUserTurn, "user_turn"},
		{MetricAssistantTurn, "assistant_turn"},
		{MetricSTTLatencyMs, "stt_latency_ms"},
		{MetricTTSLatencyMs, "tts_latency_ms"},
		{MetricEOSLatencyMs, "eos_latency_ms"},
		{MetricEOSTextToTriggerMs, "eos_text_to_trigger_ms"},
		{MetricEOSWordCount, "eos_word_count"},
		{MetricEOSCharCount, "eos_char_count"},
		{MetricEOSConfidence, "eos_confidence"},
		{MetricKnowledgeLatencyMs, "knowledge_latency_ms"},
		{MetricLLMError, "llm_error"},
		{MetricSTTError, "stt_error"},
		{MetricTTSError, "tts_error"},
		{MetricDiscardedTTSChunk, "discarded_tts_chunk"},
		{MetricDiscardedTTS, "discarded_tts"},
		{MetricTimeTaken, "time_taken"},
		{MetricStatus, "status"},
		{MetricInputToken, "input_token"},
		{MetricOutputToken, "output_token"},
		{MetricTotalToken, "total_token"},
		{MetricCachedContentToken, "cached_content_token"},
		{MetricCost, "cost"},
		{MetricInputCost, "input_cost"},
		{MetricOutputCost, "output_cost"},
		{MetricLLMRequestID, "llm_request_id"},
		{MetricTokenPerSecond, "token_pre_second"},
		{MetricTimeToFirstToken, "time_to_first_token"},
		{MetricProviderTotalTime, "provider_total_time"},
		{MetricProviderGenerateTime, "provider_generate_time"},
	}

	for _, test := range tests {
		if test.actual != test.expected {
			t.Fatalf("expected metric name %q, got %q", test.expected, test.actual)
		}
	}
}

func TestClientMetadata_MirrorCurrentImplementation(t *testing.T) {
	metadata := ClientMetadata(
		"+14155550100",
		"+14155550200",
		"inbound",
		"sip",
		"provider-call-1",
		"context-1",
		"PCMU",
		"8000",
	)

	expected := []*protos.Metadata{
		{Key: ClientDirection, Value: "inbound"},
		{Key: ClientChannel, Value: "sip"},
		{Key: ClientPhone, Value: "+14155550100"},
		{Key: ClientAssistantPhone, Value: "+14155550200"},
		{Key: ClientProviderCallID, Value: "provider-call-1"},
		{Key: ClientContextID, Value: "context-1"},
		{Key: ClientCodec, Value: "PCMU"},
		{Key: ClientSampleRate, Value: "8000"},
	}

	if len(metadata) != len(expected) {
		t.Fatalf("expected %d metadata values, got %d", len(expected), len(metadata))
	}
	for i, item := range expected {
		if metadata[i].Key != item.Key || metadata[i].Value != item.Value {
			t.Fatalf("metadata[%d] expected %+v, got %+v", i, item, metadata[i])
		}
	}
}

func TestConversationMetadataNames_MirrorCurrentImplementation(t *testing.T) {
	tests := []struct {
		actual   string
		expected string
	}{
		{MetadataClientPhone, "client.phone"},
		{MetadataClientAssistantPhone, "client.assistant_phone"},
		{MetadataClientDirection, "client.direction"},
		{MetadataClientChannel, "client.channel"},
		{MetadataClientProviderCallID, "client.provider_call_id"},
		{MetadataClientContextID, "client.context_id"},
		{MetadataClientCodec, "client.codec"},
		{MetadataClientSampleRate, "client.sample_rate"},
		{MetadataClientTimezone, "client.timezone"},
		{MetadataClientPlatform, "client.platform"},
		{MetadataClientLanguage, "client.language"},
		{MetadataClientUserAgent, "client.user_agent"},
		{MetadataClientReferrer, "client.referrer"},
		{MetadataClientConnectionType, "client.connection_type"},
		{MetadataClientLatitude, "client.latitude"},
		{MetadataClientLongitude, "client.longitude"},
		{MetadataLanguage, "language"},
		{MetadataLanguageCode, "language_code"},
		{MetadataDisconnectReason, "disconnect_reason"},
		{MetadataDisconnectText, "disconnect_text"},
		{MetadataDisconnectRawReason, "disconnect_raw_reason"},
		{MetadataCallStatus, "call_status"},
		{MetadataCallError, "call_error"},
		{MetadataFailureClass, "failure_class"},
		{MetadataFailureReason, "failure_reason"},
		{MetadataRetryable, "retryable"},
		{MetadataProviderStatusCode, "provider_status_code"},
		{MetadataTelephonyError, "telephony.error"},
		{MetadataBridgeTransferTarget, "bridge_transfer_target"},
		{MetadataBridgeTransferStatus, "bridge_transfer_status"},
		{MetadataBridgeTransferDuration, "bridge_transfer_duration"},
		{MetadataBridgeTransferOutboundCallID, "bridge_transfer_outbound_call_id"},
	}

	for _, test := range tests {
		if test.actual != test.expected {
			t.Fatalf("expected metadata name %q, got %q", test.expected, test.actual)
		}
	}
}

func TestClientMetadata_SkipsBlankValues(t *testing.T) {
	metadata := ClientMetadata(" ", "", "\t", "\n", "", "", "", "")
	if len(metadata) != 0 {
		t.Fatalf("expected blank metadata to be skipped, got %+v", metadata)
	}
}

func TestDisconnectMetadata_MirrorCurrentImplementation(t *testing.T) {
	metadata := DisconnectMetadata("remote_hangup", "Normal Clearing", "Q.850;cause=16")
	expected := []*protos.Metadata{
		{Key: MetadataDisconnectReason, Value: "remote_hangup"},
		{Key: MetadataDisconnectText, Value: "Normal Clearing"},
		{Key: MetadataDisconnectRawReason, Value: "Q.850;cause=16"},
	}

	if len(metadata) != len(expected) {
		t.Fatalf("expected %d metadata values, got %d", len(expected), len(metadata))
	}
	for i, item := range expected {
		if metadata[i].Key != item.Key || metadata[i].Value != item.Value {
			t.Fatalf("metadata[%d] expected %+v, got %+v", i, item, metadata[i])
		}
	}
}

func TestCallStatusMetric_UsesCurrentConversationStatusShape(t *testing.T) {
	metrics := CallStatusMetric("failed", "no_answer_timeout")
	if len(metrics) != 1 {
		t.Fatalf("expected one status metric, got %d", len(metrics))
	}

	metric := metrics[0]
	if metric.Name != MetricConversationStatus {
		t.Fatalf("expected status metric name %q, got %q", MetricConversationStatus, metric.Name)
	}
	if metric.Value != "failed" || metric.Description != "no_answer_timeout" {
		t.Fatalf("unexpected status metric: %+v", metric)
	}
}
