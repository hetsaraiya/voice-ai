// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability

import (
	"strconv"
	"time"

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
	MetricUserTurn      = "user_turn"
	MetricAssistantTurn = "assistant_turn"

	MetricSTTInitLatencyMs = "stt_init_ms"
	MetricSTTLatencyMs     = "stt_latency_ms"

	MetricTTSInitLatencyMs = "tts_init_ms"
	MetricTTSLatencyMs     = "tts_latency_ms"

	MetricVADInitLatencyMs = "vad_init_ms"

	MetricEOSInitLatencyMs     = "eos_init_ms"
	MetricEOSLatencyMs         = "eos_latency_ms"
	MetricEOSTextToTriggerMs   = "eos_text_to_trigger_ms"
	MetricEOSWordCount         = "eos_word_count"
	MetricEOSCharCount         = "eos_char_count"
	MetricEOSConfidence        = "eos_confidence"
	MetricDenoiseInitLatencyMs = "denoise_init_ms"
	MetricLLMInitLatencyMs     = "llm_init_ms"
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

func NewMetricSTTInitLatencyMs(duration time.Duration, attr Attributes) RecordMetric {
	record := NewConversationMetricRecord([]*protos.Metric{{
		Name:        MetricSTTInitLatencyMs,
		Value:       strconv.FormatInt(duration.Milliseconds(), 10),
		Description: "STT initialization latency in milliseconds",
	}})
	record.Attributes = attr
	return record
}

func NewMetricSTTLatencyMs(duration time.Duration, attr Attributes) RecordMetric {
	record := NewConversationMetricRecord([]*protos.Metric{{
		Name:        MetricSTTLatencyMs,
		Value:       strconv.FormatInt(duration.Milliseconds(), 10),
		Description: "STT latency from speech start to final transcript in milliseconds",
	}})
	record.Attributes = attr
	return record
}

func NewMetricTTSInitLatencyMs(duration time.Duration, attr Attributes) RecordMetric {
	record := NewConversationMetricRecord([]*protos.Metric{{
		Name:        MetricTTSInitLatencyMs,
		Value:       strconv.FormatInt(duration.Milliseconds(), 10),
		Description: "TTS initialization latency in milliseconds",
	}})
	record.Attributes = attr
	return record
}

func NewMetricTTSLatencyMs(duration time.Duration, attr Attributes) RecordMetric {
	record := NewConversationMetricRecord([]*protos.Metric{{
		Name:        MetricTTSLatencyMs,
		Value:       strconv.FormatInt(duration.Milliseconds(), 10),
		Description: "TTS latency from text input to first audio in milliseconds",
	}})
	record.Attributes = attr
	return record
}

func NewMetricVADInitLatencyMs(duration time.Duration, attr Attributes) RecordMetric {
	record := NewConversationMetricRecord([]*protos.Metric{{
		Name:        MetricVADInitLatencyMs,
		Value:       strconv.FormatInt(duration.Milliseconds(), 10),
		Description: "VAD initialization latency in milliseconds",
	}})
	record.Attributes = attr
	return record
}

func NewMetricEOSInitLatencyMs(duration time.Duration, attr Attributes) RecordMetric {
	record := NewConversationMetricRecord([]*protos.Metric{{
		Name:        MetricEOSInitLatencyMs,
		Value:       strconv.FormatInt(duration.Milliseconds(), 10),
		Description: "EOS initialization latency in milliseconds",
	}})
	record.Attributes = attr
	return record
}

func NewMetricDenoiseInitLatencyMs(duration time.Duration, attr Attributes) RecordMetric {
	record := NewConversationMetricRecord([]*protos.Metric{{
		Name:        MetricDenoiseInitLatencyMs,
		Value:       strconv.FormatInt(duration.Milliseconds(), 10),
		Description: "Denoise initialization latency in milliseconds",
	}})
	record.Attributes = attr
	return record
}

func NewMetricLLMInitLatencyMs(duration time.Duration, attr Attributes) RecordMetric {
	record := NewConversationMetricRecord([]*protos.Metric{{
		Name:        MetricLLMInitLatencyMs,
		Value:       strconv.FormatInt(duration.Milliseconds(), 10),
		Description: "LLM initialization latency in milliseconds",
	}})
	record.Attributes = attr
	return record
}

func NewMetricSTTDuration(duration time.Duration, attr Attributes) RecordMetric {
	record := NewConversationMetricRecord([]*protos.Metric{{
		Name:        MetricConversationSTTDuration,
		Value:       strconv.FormatInt(duration.Nanoseconds(), 10),
		Description: "Total STT connection duration in nanoseconds",
	}})
	record.Attributes = attr
	return record
}

func NewMetricTTSDuration(duration time.Duration, attr Attributes) RecordMetric {
	record := NewConversationMetricRecord([]*protos.Metric{{
		Name:        MetricConversationTTSDuration,
		Value:       strconv.FormatInt(duration.Nanoseconds(), 10),
		Description: "Total TTS connection duration in nanoseconds",
	}})
	record.Attributes = attr
	return record
}
