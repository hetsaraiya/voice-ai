// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability

import "strings"

// ComponentName is the first segment of a stable event name, such as "call" in
// "call.ringing".
type ComponentName string

const (
	ComponentUnknown      ComponentName = ""
	ComponentCall         ComponentName = "call"
	ComponentConversation ComponentName = "conversation"
	ComponentTurn         ComponentName = "turn"
	ComponentSession      ComponentName = "session"
	ComponentAudio        ComponentName = "audio"
	ComponentSTT          ComponentName = "stt"
	ComponentTTS          ComponentName = "tts"
	ComponentLLM          ComponentName = "llm"
	ComponentVAD          ComponentName = "vad"
	ComponentEOS          ComponentName = "eos"
	ComponentDenoise      ComponentName = "denoise"
	ComponentTool         ComponentName = "tool"
	ComponentWebhook      ComponentName = "webhook"
	ComponentRecording    ComponentName = "recording"
	ComponentSIP          ComponentName = "sip"
	ComponentWebRTC       ComponentName = "webrtc"
	ComponentUsage        ComponentName = "usage"
	ComponentLog          ComponentName = "log"
	ComponentMetric       ComponentName = "metric"
	ComponentMetadata     ComponentName = "metadata"
	ComponentError        ComponentName = "error"
)

func (c ComponentName) String() string {
	return string(c)
}

// EventName is the stable domain event name emitted by the assistant runtime.
type EventName string

const (
	CallStatus EventName = "call.status"

	CallReceived               EventName = "call.received"
	CallInitiated              EventName = "call.initiated"
	CallQueued                 EventName = "call.queued"
	CallRinging                EventName = "call.ringing"
	CallAnswered               EventName = "call.answered"
	CallStarted                EventName = "call.started"
	CallInProgress             EventName = "call.in_progress"
	CallMediaStarted           EventName = "call.media_started"
	CallHangup                 EventName = "call.hangup"
	CallEnded                  EventName = "call.ended"
	CallCompleted              EventName = "call.completed"
	CallFailed                 EventName = "call.failed"
	CallBusy                   EventName = "call.busy"
	CallNoAnswer               EventName = "call.no_answer"
	CallRejected               EventName = "call.rejected"
	CallCancelled              EventName = "call.cancelled"
	CallOutboundRequested      EventName = "call.outbound_requested"
	CallOutboundDialed         EventName = "call.outbound_dialed"
	CallOutboundDispatched     EventName = "call.outbound_dispatched"
	CallOutboundDispatchFailed EventName = "call.outbound_dispatch_failed"
	CallProviderAnswered       EventName = "call.provider_answered"
	CallSessionConnected       EventName = "call.session_connected"
	CallAssistantLoaded        EventName = "call.assistant_loaded"
	CallConversationCreated    EventName = "call.conversation_created"
	CallContextSaved           EventName = "call.context_saved"
)

const (
	ConversationBegin                EventName = "conversation.begin"
	ConversationResume               EventName = "conversation.resume"
	ConversationStarted              EventName = "conversation.started"
	ConversationInitializing         EventName = "conversation.initializing"
	ConversationInitialized          EventName = "conversation.initialized"
	ConversationEnding               EventName = "conversation.ending"
	ConversationCompleted            EventName = "conversation.completed"
	ConversationCleanup              EventName = "conversation.cleanup"
	ConversationFinalized            EventName = "conversation.finalized"
	ConversationFailed               EventName = "conversation.failed"
	ConversationError                EventName = "conversation.error"
	ConversationAgentStateChanged    EventName = "conversation.agent_state_changed"
	ConversationUserStateChanged     EventName = "conversation.user_state_changed"
	ConversationUserInputTranscribed EventName = "conversation.user_input_transcribed"
	ConversationItemAdded            EventName = "conversation.item_added"
	ConversationSpeechCreated        EventName = "conversation.speech_created"
	ConversationFalseInterruption    EventName = "conversation.false_interruption"
	ConversationUsageUpdated         EventName = "conversation.usage_updated"
	ConversationClosed               EventName = "conversation.closed"
)

const (
	TurnStarted                  EventName = "turn.started"
	TurnUserSpeechStarted        EventName = "turn.user_speech_started"
	TurnUserSpeechFinal          EventName = "turn.user_speech_final"
	TurnAssistantResponseStarted EventName = "turn.assistant_response_started"
	TurnAssistantResponseFinal   EventName = "turn.assistant_response_final"
	TurnChange                   EventName = "turn.change"
	TurnInterrupted              EventName = "turn.interrupted"
	TurnOverlappingSpeech        EventName = "turn.overlapping_speech"
	TurnUserTurnExceeded         EventName = "turn.user_turn_exceeded"
	TurnCompleted                EventName = "turn.completed"
	TurnFailed                   EventName = "turn.failed"
)

const (
	SessionConnected             EventName = "session.connected"
	SessionInitializing          EventName = "session.initializing"
	SessionInitialized           EventName = "session.initialized"
	SessionConnectFailed         EventName = "session.connect_failed"
	SessionDisconnected          EventName = "session.disconnected"
	SessionDisconnectRequested   EventName = "session.disconnect_requested"
	SessionCleanup               EventName = "session.cleanup"
	SessionModeSwitch            EventName = "session.mode_switch"
	SessionModeSwitchFailed      EventName = "session.mode_switch_failed"
	SessionAuthenticationStarted EventName = "session.authentication_started"
	SessionSessionResolved       EventName = "session.session_resolved"
	SessionSessionResolveFailed  EventName = "session.session_resolve_failed"
	SessionStreamerCreated       EventName = "session.streamer_created"
	SessionStreamerFailed        EventName = "session.streamer_failed"
	SessionTalkerCreated         EventName = "session.talker_created"
	SessionTalkerFailed          EventName = "session.talker_failed"
	SessionTalkStarted           EventName = "session.talk_started"
	SessionHooksBegin            EventName = "session.hooks_begin"
	SessionHooksEnd              EventName = "session.hooks_end"
)

const (
	AudioInputStarted  EventName = "audio.input_started"
	AudioInputStopped  EventName = "audio.input_stopped"
	AudioOutputStarted EventName = "audio.output_started"
	AudioOutputStopped EventName = "audio.output_stopped"
	AudioFrameReceived EventName = "audio.frame_received"
	AudioFrameSent     EventName = "audio.frame_sent"
	AudioResampled     EventName = "audio.resampled"
	AudioCodecChanged  EventName = "audio.codec_changed"
	AudioError         EventName = "audio.error"
)

const (
	STTConnected           EventName = "stt.connected"
	STTStarted             EventName = "stt.started"
	STTStartOfSpeech       EventName = "stt.start_of_speech"
	STTInterimTranscript   EventName = "stt.interim_transcript"
	STTPreflightTranscript EventName = "stt.preflight_transcript"
	STTPartial             EventName = "stt.partial"
	STTFinalTranscript     EventName = "stt.final_transcript"
	STTFinal               EventName = "stt.final"
	STTRecognitionUsage    EventName = "stt.recognition_usage"
	STTEndOfSpeech         EventName = "stt.end_of_speech"
	STTDisconnected        EventName = "stt.disconnected"
	STTError               EventName = "stt.error"
)

const (
	TTSStarted    EventName = "tts.started"
	TTSFirstAudio EventName = "tts.first_audio"
	TTSAudio      EventName = "tts.audio"
	TTSCompleted  EventName = "tts.completed"
	TTSDiscarded  EventName = "tts.discarded"
	TTSError      EventName = "tts.error"
)

const (
	LLMStarted    EventName = "llm.started"
	LLMFirstToken EventName = "llm.first_token"
	LLMToken      EventName = "llm.token"
	LLMCompleted  EventName = "llm.completed"
	LLMDiscarded  EventName = "llm.discarded"
	LLMError      EventName = "llm.error"
)

const (
	VADStarted       EventName = "vad.started"
	VADSpeechStarted EventName = "vad.speech_started"
	VADInferenceDone EventName = "vad.inference_done"
	VADSpeechEnded   EventName = "vad.speech_ended"
	VADClosed        EventName = "vad.closed"
	VADError         EventName = "vad.error"
)

const (
	EOSStarted   EventName = "eos.started"
	EOSCompleted EventName = "eos.completed"
	EOSClosed    EventName = "eos.closed"
	EOSError     EventName = "eos.error"
)

const (
	DenoiseStarted   EventName = "denoise.started"
	DenoiseCompleted EventName = "denoise.completed"
	DenoiseClosed    EventName = "denoise.closed"
	DenoiseError     EventName = "denoise.error"
)

const (
	ToolCallStarted   EventName = "tool.call_started"
	ToolCallCompleted EventName = "tool.call_completed"
	ToolCallFailed    EventName = "tool.call_failed"
)

const (
	WebhookDispatched EventName = "webhook.dispatched"
	WebhookCompleted  EventName = "webhook.completed"
	WebhookFailed     EventName = "webhook.failed"
	WebhookRetrying   EventName = "webhook.retrying"
)

const (
	RecordingStarted EventName = "recording.started"
	RecordingStopped EventName = "recording.stopped"
	RecordingFailed  EventName = "recording.failed"
)

const (
	SIPInviteReceived    EventName = "sip.invite_received"
	SIPRouteResolved     EventName = "sip.route_resolved"
	SIPAuthenticated     EventName = "sip.authenticated"
	SIPByeReceived       EventName = "sip.bye_received"
	SIPCancelReceived    EventName = "sip.cancel_received"
	SIPHold              EventName = "sip.hold"
	SIPResume            EventName = "sip.resume"
	SIPReInvite          EventName = "sip.reinvite"
	SIPTransferRequested EventName = "sip.transfer_requested"
	SIPTransferring      EventName = "sip.transferring"
	SIPTransferConnected EventName = "sip.transfer_connected"
	SIPTransferCompleted EventName = "sip.transfer_completed"
	SIPTransferFailed    EventName = "sip.transfer_failed"
	SIPRegisterActive    EventName = "sip.register_active"
	SIPRegisterFailed    EventName = "sip.register_failed"
	SIPDTMF              EventName = "sip.dtmf"
)

const (
	WebRTCConnecting   EventName = "webrtc.connecting"
	WebRTCConnected    EventName = "webrtc.connected"
	WebRTCReconnecting EventName = "webrtc.reconnecting"
	WebRTCReconnected  EventName = "webrtc.reconnected"
	WebRTCDisconnected EventName = "webrtc.disconnected"
	WebRTCFailed       EventName = "webrtc.failed"
	WebRTCICEConnected EventName = "webrtc.ice_connected"
	WebRTCICEFailed    EventName = "webrtc.ice_failed"
)

const (
	UsageRecorded EventName = "usage.recorded"
)

const (
	ErrorRaised    EventName = "error.raised"
	ErrorRecovered EventName = "error.recovered"
)

var eventsByComponent = map[ComponentName][]EventName{
	ComponentCall: {
		CallStatus,
		CallReceived,
		CallInitiated,
		CallQueued,
		CallRinging,
		CallAnswered,
		CallStarted,
		CallInProgress,
		CallMediaStarted,
		CallHangup,
		CallEnded,
		CallCompleted,
		CallFailed,
		CallBusy,
		CallNoAnswer,
		CallRejected,
		CallCancelled,
		CallOutboundRequested,
		CallOutboundDialed,
		CallOutboundDispatched,
		CallOutboundDispatchFailed,
		CallProviderAnswered,
		CallSessionConnected,
		CallAssistantLoaded,
		CallConversationCreated,
		CallContextSaved,
	},
	ComponentConversation: {
		ConversationBegin,
		ConversationResume,
		ConversationStarted,
		ConversationInitializing,
		ConversationInitialized,
		ConversationEnding,
		ConversationCompleted,
		ConversationCleanup,
		ConversationFinalized,
		ConversationFailed,
		ConversationError,
		ConversationAgentStateChanged,
		ConversationUserStateChanged,
		ConversationUserInputTranscribed,
		ConversationItemAdded,
		ConversationSpeechCreated,
		ConversationFalseInterruption,
		ConversationUsageUpdated,
		ConversationClosed,
	},
	ComponentTurn: {
		TurnStarted,
		TurnUserSpeechStarted,
		TurnUserSpeechFinal,
		TurnAssistantResponseStarted,
		TurnAssistantResponseFinal,
		TurnChange,
		TurnInterrupted,
		TurnOverlappingSpeech,
		TurnUserTurnExceeded,
		TurnCompleted,
		TurnFailed,
	},
	ComponentSession: {
		SessionConnected,
		SessionInitializing,
		SessionInitialized,
		SessionConnectFailed,
		SessionDisconnected,
		SessionDisconnectRequested,
		SessionCleanup,
		SessionModeSwitch,
		SessionModeSwitchFailed,
		SessionAuthenticationStarted,
		SessionSessionResolved,
		SessionSessionResolveFailed,
		SessionStreamerCreated,
		SessionStreamerFailed,
		SessionTalkerCreated,
		SessionTalkerFailed,
		SessionTalkStarted,
		SessionHooksBegin,
		SessionHooksEnd,
	},
	ComponentAudio: {
		AudioInputStarted,
		AudioInputStopped,
		AudioOutputStarted,
		AudioOutputStopped,
		AudioFrameReceived,
		AudioFrameSent,
		AudioResampled,
		AudioCodecChanged,
		AudioError,
	},
	ComponentSTT: {
		STTConnected,
		STTStarted,
		STTStartOfSpeech,
		STTInterimTranscript,
		STTPreflightTranscript,
		STTPartial,
		STTFinalTranscript,
		STTFinal,
		STTRecognitionUsage,
		STTEndOfSpeech,
		STTDisconnected,
		STTError,
	},
	ComponentTTS: {
		TTSStarted,
		TTSFirstAudio,
		TTSAudio,
		TTSCompleted,
		TTSDiscarded,
		TTSError,
	},
	ComponentLLM: {
		LLMStarted,
		LLMFirstToken,
		LLMToken,
		LLMCompleted,
		LLMDiscarded,
		LLMError,
	},
	ComponentVAD: {
		VADStarted,
		VADSpeechStarted,
		VADInferenceDone,
		VADSpeechEnded,
		VADClosed,
		VADError,
	},
	ComponentEOS: {
		EOSStarted,
		EOSCompleted,
		EOSClosed,
		EOSError,
	},
	ComponentDenoise: {
		DenoiseStarted,
		DenoiseCompleted,
		DenoiseClosed,
		DenoiseError,
	},
	ComponentTool: {
		ToolCallStarted,
		ToolCallCompleted,
		ToolCallFailed,
	},
	ComponentWebhook: {
		WebhookDispatched,
		WebhookCompleted,
		WebhookFailed,
		WebhookRetrying,
	},
	ComponentRecording: {
		RecordingStarted,
		RecordingStopped,
		RecordingFailed,
	},
	ComponentSIP: {
		SIPInviteReceived,
		SIPRouteResolved,
		SIPAuthenticated,
		SIPByeReceived,
		SIPCancelReceived,
		SIPHold,
		SIPResume,
		SIPReInvite,
		SIPTransferRequested,
		SIPTransferring,
		SIPTransferConnected,
		SIPTransferCompleted,
		SIPTransferFailed,
		SIPRegisterActive,
		SIPRegisterFailed,
		SIPDTMF,
	},
	ComponentWebRTC: {
		WebRTCConnecting,
		WebRTCConnected,
		WebRTCReconnecting,
		WebRTCReconnected,
		WebRTCDisconnected,
		WebRTCFailed,
		WebRTCICEConnected,
		WebRTCICEFailed,
	},
	ComponentUsage: {
		UsageRecorded,
	},
	ComponentError: {
		ErrorRaised,
		ErrorRecovered,
	},
}

func (e EventName) String() string {
	return string(e)
}

func (e EventName) Component() ComponentName {
	value := e.String()
	idx := strings.IndexByte(value, '.')
	if idx <= 0 {
		return ComponentUnknown
	}
	return ComponentName(value[:idx])
}

func (e EventName) HasComponent(component ComponentName) bool {
	return e.Component() == component
}

func (e EventName) HasCategory(component ComponentName) bool {
	return e.HasComponent(component)
}

func (e EventName) IsKnown() bool {
	for _, known := range Events(e.Component()) {
		if known == e {
			return true
		}
	}
	return false
}

func Events(component ComponentName) []EventName {
	return append([]EventName(nil), eventsByComponent[component]...)
}

func CallEvents() []EventName {
	return Events(ComponentCall)
}

func ConversationEvents() []EventName {
	return Events(ComponentConversation)
}

func AllEvents() []EventName {
	var total int
	for _, events := range eventsByComponent {
		total += len(events)
	}
	all := make([]EventName, 0, total)
	for _, events := range eventsByComponent {
		all = append(all, events...)
	}
	return all
}
