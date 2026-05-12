// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_type

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

// =============================================================================
// Packet Interfaces
// =============================================================================

// Packet represents a generic request packet handled by the adapter layer.
// Concrete packet types signal specific actions or events within a given context.
//
// Naming convention:
//   - Commands (trigger an action): verb-first — ExecuteLLM, DenoiseAudio, InterruptTTS, RecordUserAudio, SaveMessage, SpeakText
//   - Events  (something happened): past-tense/noun — LLMResponseDelta, DenoisedAudio, SpeechToText, EndOfSpeech, TextToSpeechAudio
type Packet interface {
	ContextId() string
}

// MessagePacket wraps a Packet with role and text content.
type MessagePacket interface {
	Packet
	Role() string
	Content() string
}

// AudioPacket wraps a Packet with raw audio bytes.
type AudioPacket interface {
	Packet
	Content() []byte
}

// LLMPacket is a marker interface for LLM pipeline packets.
type LLMPacket interface {
	Packet
	ContextId() string
}

// LLMToolPacket is a marker interface for LLM tool-related packets.
type LLMToolPacket interface {
	ToolId() string
}

type ErrorPacket interface {
	Packet
	IsRecoverable() bool
	Err() error
	ErrMessage() string
}

type MessageEntry struct {
	Role    string
	Content string
}

// =============================================================================
// Input Pipeline — user -> denoise -> VAD -> STT -> EOS -> normalize
// =============================================================================

// UserTextReceivedPacket carries text input from the user (e.g. via WebSocket/HTTP).
type UserTextReceivedPacket struct {
	ContextID string
	Text      string

	// Language detected by STT for this turn (may be empty for text-mode input).
	Language string
}

func (f UserTextReceivedPacket) ContextId() string { return f.ContextID }
func (f UserTextReceivedPacket) Content() string   { return f.Text }
func (f UserTextReceivedPacket) Role() string      { return "user" }

// UserAudioReceivedPacket carries raw audio input from the user (e.g. via WebRTC).
type UserAudioReceivedPacket struct {
	ContextID    string
	Audio        []byte
	NoiseReduced bool
}

func (f UserAudioReceivedPacket) ContextId() string { return f.ContextID }
func (f UserAudioReceivedPacket) Content() []byte   { return f.Audio }
func (f UserAudioReceivedPacket) Role() string      { return "user" }

type SpeechToTextAudioPacket struct {
	ContextID string
	Audio     []byte
}

func (f SpeechToTextAudioPacket) ContextId() string { return f.ContextID }
func (f SpeechToTextAudioPacket) Content() []byte   { return f.Audio }
func (f SpeechToTextAudioPacket) IsAsync() bool     { return true }

// DenoiseAudioPacket carries raw user audio to be denoised before entering the pipeline.
type DenoiseAudioPacket struct {
	ContextID string
	Audio     []byte
}

func (f DenoiseAudioPacket) ContextId() string { return f.ContextID }

// DenoisedAudioPacket carries the result of the denoiser stage.
// The denoiser pushes this via onPacket instead of returning bytes to the caller.
// On error the denoiser falls back to the original audio with NoiseReduced=false.
type DenoisedAudioPacket struct {
	ContextID    string
	Audio        []byte
	Confidence   float64
	NoiseReduced bool
}

func (f DenoisedAudioPacket) ContextId() string { return f.ContextID }

// VadAudioPacket carries a processed audio chunk to submit to the VAD processor.
type VadAudioPacket struct {
	ContextID string
	Audio     []byte
}

func (f VadAudioPacket) ContextId() string { return f.ContextID }

// VadSpeechActivityPacket is a lightweight heartbeat emitted by the VAD on every
// audio chunk where the user is actively speaking. The EOS detector uses it to
// keep extending the silence timer during sustained speech.
type VadSpeechActivityPacket struct{}

func (f VadSpeechActivityPacket) ContextId() string { return "" }

// SpeechToTextPacket carries a transcript result from the STT provider.
type SpeechToTextPacket struct {
	ContextID  string
	Script     string
	Confidence float64
	Language   string
	Interim    bool
}

func (f SpeechToTextPacket) ContextId() string { return f.ContextID }

// EndOfSpeechPacket signals that the EOS detector determined the user's turn is complete.
type EndOfSpeechAudioPacket struct {
	ContextID string
	Audio     []byte
}

func (f EndOfSpeechAudioPacket) ContextId() string { return f.ContextID }
func (f EndOfSpeechAudioPacket) IsAsync() bool     { return true }

type EndOfSpeechInterruptionPacket struct {
	ContextID string
	Source    InterruptionSource
}

func (f EndOfSpeechInterruptionPacket) ContextId() string { return f.ContextID }
func (f EndOfSpeechInterruptionPacket) IsAsync() bool     { return true }

type EndOfSpeechPacket struct {
	ContextID string
	Speech    string
	Speechs   []SpeechToTextPacket // accumulated transcript chunks
}

func (f EndOfSpeechPacket) ContextId() string { return f.ContextID }

// InterimEndOfSpeechPacket carries a partial EOS result (in-progress transcript).
type InterimEndOfSpeechPacket struct {
	ContextID string
	Speech    string
}

func (p InterimEndOfSpeechPacket) ContextId() string { return p.ContextID }

// UserInputPacket carries the processed user text after input preprocessing (language detection, etc.).
type UserInputPacket struct {
	ContextID string
	Text      string
	Language  types.Language
}

func (f UserInputPacket) ContextId() string { return f.ContextID }

// =============================================================================
// Control — interrupts, directives, injected messages
// =============================================================================

type InterruptionSource string

const (
	InterruptionSourceWord InterruptionSource = "word"
	InterruptionSourceVad  InterruptionSource = "vad"
)

type InterruptionEvent string

const (
	InterruptionEventStart InterruptionEvent = "start"
	InterruptionEventEnd   InterruptionEvent = "end"
)

// InterruptionDetectedPacket signals that an interruption was detected (by VAD or word-level STT).
// Dispatch handles this event by emitting InterruptTTSPacket and InterruptLLMPacket commands.
type InterruptionDetectedPacket struct {
	ContextID string
	Source    InterruptionSource
	Event     InterruptionEvent
	StartAt   float64
	EndAt     float64
}

func (f InterruptionDetectedPacket) ContextId() string { return f.ContextID }

// TextToSpeechInterruptPacket signals the TTS transformer to stop current playback.
type TextToSpeechInterruptPacket struct {
	ContextID string
	StartAt   float64
	EndAt     float64
}

func (f TextToSpeechInterruptPacket) ContextId() string { return f.ContextID }
func (f TextToSpeechInterruptPacket) IsAsync() bool     { return true }

type STTErrorType int

const (
	STTRateLimit = 1
	STTNetworkTimeout

	// Non-Recoverable STT errors (e.g., bad API keys, invalid audio formats)
	STTAuthentication
	STTInvalidInput
	STTSystemPanic
)

// When IsRecoverable is true, the conversation should be gracefully terminated.
type SpeechToTextErrorPacket struct {
	ContextID string
	Error     error
	Type      STTErrorType
}

func (f SpeechToTextErrorPacket) ContextId() string { return f.ContextID }
func (f SpeechToTextErrorPacket) IsRecoverable() bool {
	return f.Type == STTRateLimit || f.Type == STTNetworkTimeout
}
func (f SpeechToTextErrorPacket) Err() error         { return f.Error }
func (f SpeechToTextErrorPacket) ErrMessage() string { return fmt.Sprintf("stt: %s", f.Error.Error()) }

type SpeechToTextInterruptPacket struct {
	ContextID string
	StartAt   float64
	EndAt     float64
}

func (f SpeechToTextInterruptPacket) ContextId() string { return f.ContextID }

// InterruptLLMPacket signals the LLM executor to cancel current generation.
type LLMInterruptPacket struct {
	ContextID string
}

func (f LLMInterruptPacket) ContextId() string { return f.ContextID }

// TurnChangePacket notifies components that active context changed to a new turn.
type TurnChangePacket struct {
	ContextID         string
	PreviousContextID string
	Reason            string
	Source            string
	Time              time.Time
}

func (f TurnChangePacket) ContextId() string { return f.ContextID }

// InjectMessagePacket injects a pre-written message (greeting, error, idle timeout) into the pipeline.
type InjectMessagePacket struct {
	ContextID string
	Text      string
}

func (f InjectMessagePacket) ContextId() string { return f.ContextID }
func (f InjectMessagePacket) Content() string   { return f.Text }
func (f InjectMessagePacket) Role() string      { return "rapida" }

// =============================================================================
// Initialization chain:
// InitializeAssistant → InitializeConversation → InitializeSessionRuntime
// → InitializeAuthentication → InitializeSpeechToText
// → InitializeTextToSpeech → InitializeVoiceActivityDetection
// → InitializeEndOfSpeech → InitializeBehavior → InitializationCompleted
// Each handler enqueues the next phase to bootstrapChannel, forming an ordered chain.
// =============================================================================

// InitializeAssistantPacket loads the assistant config, starts dispatchers, inits auth executor.
type InitializeAssistantPacket struct {
	ContextID string
	Config    *protos.ConversationInitialization
}

func (f InitializeAssistantPacket) ContextId() string { return f.ContextID }

// InitializeConversationPacket creates or resumes the conversation.
type InitializeConversationPacket struct {
	ContextID string
	Config    *protos.ConversationInitialization
}

func (f InitializeConversationPacket) ContextId() string { return f.ContextID }

// InitializeSessionRuntimePacket initializes collectors, recorder, normalizers, metrics.
type InitializeSessionRuntimePacket struct {
	ContextID string
	Config    *protos.ConversationInitialization
}

func (f InitializeSessionRuntimePacket) ContextId() string { return f.ContextID }

// InitializeAuthenticationPacket starts session authentication stage.
type InitializeAuthenticationPacket struct {
	ContextID string
	Config    *protos.ConversationInitialization
}

func (f InitializeAuthenticationPacket) ContextId() string { return f.ContextID }

// ExecuteSessionAuthenticationPacket triggers authentication against the configured endpoint.
type ExecuteSessionAuthenticationPacket struct {
	ContextID      string
	Arguments      map[string]interface{}
	Initialization *protos.ConversationInitialization
}

func (f ExecuteSessionAuthenticationPacket) ContextId() string { return f.ContextID }

// SessionAuthenticationSucceededPacket carries successful auth output.
// Authenticated can be false when fail_behavior=allow is applied.
type SessionAuthenticationSucceededPacket struct {
	ContextID      string
	Authenticated  bool
	Arguments      map[string]interface{}
	Metadata       map[string]interface{}
	Options        map[string]interface{}
	Initialization *protos.ConversationInitialization
}

func (f SessionAuthenticationSucceededPacket) ContextId() string { return f.ContextID }

// SessionAuthenticationFailedPacket signals auth stage failure.
type SessionAuthenticationFailedPacket struct {
	ContextID      string
	Error          error
	Initialization *protos.ConversationInitialization
}

func (f SessionAuthenticationFailedPacket) ContextId() string   { return f.ContextID }
func (f SessionAuthenticationFailedPacket) IsRecoverable() bool { return false }
func (f SessionAuthenticationFailedPacket) Err() error          { return f.Error }
func (f SessionAuthenticationFailedPacket) ErrMessage() string {
	return fmt.Sprintf("session_authentication: %s", f.Error.Error())
}

// InitializeSpeechToTextPacket initializes speech-to-text.
type InitializeSpeechToTextPacket struct {
	ContextID string
	Config    *protos.ConversationInitialization
}

func (f InitializeSpeechToTextPacket) ContextId() string { return f.ContextID }

type InitializeAssistantExecutorPacket struct {
	ContextID string
	Config    *protos.ConversationInitialization
}

func (f InitializeAssistantExecutorPacket) ContextId() string { return f.ContextID }
func (f InitializeAssistantExecutorPacket) IsAsync() bool     { return true }

// InitializeTextToSpeechPacket initializes text-to-speech.
type InitializeTextToSpeechPacket struct {
	ContextID string
	Config    *protos.ConversationInitialization
}

func (f InitializeTextToSpeechPacket) ContextId() string { return f.ContextID }

// InitializeVoiceActivityDetectionPacket initializes voice activity detection.
type InitializeVoiceActivityDetectionPacket struct {
	ContextID string
	Config    *protos.ConversationInitialization
}

func (f InitializeVoiceActivityDetectionPacket) IsAsync() bool     { return true }
func (f InitializeVoiceActivityDetectionPacket) ContextId() string { return f.ContextID }

// InitializeEndOfSpeechPacket initializes end-of-speech detection.
type InitializeEndOfSpeechPacket struct {
	ContextID string
	Config    *protos.ConversationInitialization
}

func (f InitializeEndOfSpeechPacket) IsAsync() bool     { return true }
func (f InitializeEndOfSpeechPacket) ContextId() string { return f.ContextID }

// InitializeDenoisePacket initializes the denoiser for text->audio switch.
type InitializeDenoisePacket struct {
	ContextID string
	Config    *protos.ConversationInitialization
}

func (f InitializeDenoisePacket) IsAsync() bool     { return true }
func (f InitializeDenoisePacket) ContextId() string { return f.ContextID }

// InitializeBehaviorPacket sets up greeting, idle timeout, max session.
type InitializeBehaviorPacket struct {
	ContextID string
	Config    *protos.ConversationInitialization
}

func (f InitializeBehaviorPacket) ContextId() string { return f.ContextID }

// InitializationCompletedPacket is emitted when the connect initialization chain succeeds.
type InitializationCompletedPacket struct {
	ContextID string
	Event     utils.AssistantWebhookEvent
	Config    *protos.ConversationInitialization
}

func (f InitializationCompletedPacket) ContextId() string { return f.ContextID }

// AsyncPacket marks a packet whose handler runs in its own goroutine.
type AsyncPacket interface {
	Packet
	IsAsync() bool
}

// InitializeTelemetryPacket initializes the conversation observer (collectors, exporters).
type InitializeTelemetryPacket struct {
	ContextID string
}

func (p InitializeTelemetryPacket) ContextId() string { return p.ContextID }
func (p InitializeTelemetryPacket) IsAsync() bool     { return true }

// InitializeOutboundDispatcherPacket starts control, egress, and background dispatchers.
type InitializeOutboundDispatcherPacket struct {
	ContextID string
}

func (p InitializeOutboundDispatcherPacket) ContextId() string { return p.ContextID }

// InitializeInboundDispatcherPacket starts the ingress dispatcher.
type InitializeInboundDispatcherPacket struct {
	ContextID string
}

func (p InitializeInboundDispatcherPacket) ContextId() string { return p.ContextID }

// InitializationStage identifies which initialization phase failed.
type InitializationStage string

const (
	InitializationStageAssistant           InitializationStage = "assistant"
	InitializationStageConversation        InitializationStage = "conversation"
	InitializationStageService             InitializationStage = "service"
	InitializationStageAuthentication      InitializationStage = "authentication"
	InitializationStageSpeechToText        InitializationStage = "stt"
	InitializationStageTextToSpeech        InitializationStage = "tts"
	InitializationStageVoiceActivity       InitializationStage = "vad"
	InitializationStageDenoise             InitializationStage = "denoise"
	InitializationStageEndOfSpeech         InitializationStage = "eos"
	InitializationStageBehavior            InitializationStage = "behavior"
	InitializationStageAnalysis            InitializationStage = "analysis"
	InitializationStageWebhook             InitializationStage = "webhook"
	InitializationStageInputNormalizer     InitializationStage = "input_normalizer"
	InitializationStageOutputNormalizer    InitializationStage = "output_normalizer"
	InitializationStageInitializationFinal InitializationStage = "initialization_completed"
)

// InitializationFailedPacket signals that initialization failed.
// The handler notifies the client and fires ConversationFailed webhooks.
type InitializationFailedPacket struct {
	ContextID string
	Stage     InitializationStage
	Error     error
}

func (f InitializationFailedPacket) ContextId() string   { return f.ContextID }
func (f InitializationFailedPacket) IsRecoverable() bool { return false }
func (f InitializationFailedPacket) Err() error          { return f.Error }
func (f InitializationFailedPacket) ErrMessage() string {
	if f.Stage != "" {
		return fmt.Sprintf("init[%s]: %s", string(f.Stage), f.Error.Error())
	}
	return fmt.Sprintf("init: %s", f.Error.Error())
}

// =============================================================================
// Runtime mode switch chain (text <-> audio).
// Uses dedicated mode-switch stage packets so switch flow remains independent
// from bootstrap/finalization chains.
// =============================================================================

type ModeSwitchRequestedPacket struct {
	ContextID   string
	StreamMode  protos.StreamMode
	RequestedAt time.Time
}

func (f ModeSwitchRequestedPacket) ContextId() string { return f.ContextID }

type ModeSwitchCompletedPacket struct {
	ContextID  string
	StreamMode protos.StreamMode
}

func (f ModeSwitchCompletedPacket) ContextId() string { return f.ContextID }

type ModeSwitchErrorType string

const (
	ModeSwitchErrorTypeUnknown                          ModeSwitchErrorType = "unknown"
	ModeSwitchErrorTypePreconditionNotReady             ModeSwitchErrorType = "precondition_not_ready"
	ModeSwitchErrorTypeUnsupportedMode                  ModeSwitchErrorType = "unsupported_mode"
	ModeSwitchErrorTypeInitializeSpeechToText           ModeSwitchErrorType = "initialize_stt"
	ModeSwitchErrorTypeInitializeTextToSpeech           ModeSwitchErrorType = "initialize_tts"
	ModeSwitchErrorTypeInitializeVoiceActivityDetection ModeSwitchErrorType = "initialize_vad"
	ModeSwitchErrorTypeInitializeEndOfSpeech            ModeSwitchErrorType = "initialize_eos"
	ModeSwitchErrorTypeFinalizeEndOfSpeech              ModeSwitchErrorType = "finalize_eos"
	ModeSwitchErrorTypeInitializeDenoise                ModeSwitchErrorType = "initialize_denoise"
	ModeSwitchErrorTypeFinalizeDenoise                  ModeSwitchErrorType = "finalize_denoise"
	ModeSwitchErrorTypeFinalizeVoiceActivityDetection   ModeSwitchErrorType = "finalize_vad"
	ModeSwitchErrorTypeFinalizeTextToSpeech             ModeSwitchErrorType = "finalize_tts"
	ModeSwitchErrorTypeFinalizeSpeechToText             ModeSwitchErrorType = "finalize_stt"
)

type ModeSwitchErrorPacket struct {
	ContextID  string
	StreamMode protos.StreamMode
	Type       ModeSwitchErrorType
	Error      error
}

func (f ModeSwitchErrorPacket) ContextId() string { return f.ContextID }
func (f ModeSwitchErrorPacket) IsRecoverable() bool {
	return f.Type == ModeSwitchErrorTypeUnknown ||
		f.Type == ModeSwitchErrorTypePreconditionNotReady ||
		f.Type == ModeSwitchErrorTypeUnsupportedMode
}
func (f ModeSwitchErrorPacket) Err() error { return f.Error }
func (f ModeSwitchErrorPacket) ErrMessage() string {
	msg := "unknown error"
	if f.Error != nil {
		msg = f.Error.Error()
	}
	return fmt.Sprintf("mode_switch[%s:%s]: %s", string(f.Type), f.StreamMode.String(), msg)
}

// ModeSwitchInitializeSpeechToTextPacket initializes STT for text->audio switch.
// Sync — runs serially on the bootstrap goroutine; on success emits the next
// packet in the chain, on failure emits ModeSwitchErrorPacket (non-recoverable).
type ModeSwitchInitializeSpeechToTextPacket struct {
	ContextID  string
	StreamMode protos.StreamMode
}

func (f ModeSwitchInitializeSpeechToTextPacket) ContextId() string { return f.ContextID }
func (f ModeSwitchInitializeSpeechToTextPacket) IsAsync() bool     { return true }

// ModeSwitchInitializeTextToSpeechPacket initializes TTS for text->audio switch.
type ModeSwitchInitializeTextToSpeechPacket struct {
	ContextID  string
	StreamMode protos.StreamMode
}

func (f ModeSwitchInitializeTextToSpeechPacket) ContextId() string { return f.ContextID }

// ModeSwitchInitializeVoiceActivityDetectionPacket initializes VAD for text->audio switch.
type ModeSwitchInitializeVoiceActivityDetectionPacket struct {
	ContextID  string
	StreamMode protos.StreamMode
}

func (f ModeSwitchInitializeVoiceActivityDetectionPacket) ContextId() string { return f.ContextID }
func (f ModeSwitchInitializeVoiceActivityDetectionPacket) IsAsync() bool     { return true }

// ModeSwitchInitializeEndOfSpeechPacket initializes EOS for text->audio switch.
type ModeSwitchInitializeEndOfSpeechPacket struct {
	ContextID  string
	StreamMode protos.StreamMode
}

func (f ModeSwitchInitializeEndOfSpeechPacket) ContextId() string { return f.ContextID }

// ModeSwitchInitializeDenoisePacket initializes the denoiser for text->audio switch.
type ModeSwitchInitializeDenoisePacket struct {
	ContextID  string
	StreamMode protos.StreamMode
}

func (f ModeSwitchInitializeDenoisePacket) ContextId() string { return f.ContextID }
func (f ModeSwitchInitializeDenoisePacket) IsAsync() bool     { return true }

// ModeSwitchFinalizeSpeechToTextPacket finalizes STT for audio->text switch.
// Async — runs in its own goroutine. Fire-and-forget; the client has already
// been confirmed in text mode by the time these handlers run. Errors are logged.
type ModeSwitchFinalizeSpeechToTextPacket struct {
	ContextID  string
	StreamMode protos.StreamMode
}

func (f ModeSwitchFinalizeSpeechToTextPacket) ContextId() string { return f.ContextID }
func (f ModeSwitchFinalizeSpeechToTextPacket) IsAsync() bool     { return true }

// ModeSwitchFinalizeTextToSpeechPacket finalizes TTS for audio->text switch.
type ModeSwitchFinalizeTextToSpeechPacket struct {
	ContextID  string
	StreamMode protos.StreamMode
}

func (f ModeSwitchFinalizeTextToSpeechPacket) ContextId() string { return f.ContextID }
func (f ModeSwitchFinalizeTextToSpeechPacket) IsAsync() bool     { return true }

// ModeSwitchFinalizeVoiceActivityDetectionPacket finalizes VAD for audio->text switch.
type ModeSwitchFinalizeVoiceActivityDetectionPacket struct {
	ContextID  string
	StreamMode protos.StreamMode
}

func (f ModeSwitchFinalizeVoiceActivityDetectionPacket) ContextId() string { return f.ContextID }
func (f ModeSwitchFinalizeVoiceActivityDetectionPacket) IsAsync() bool     { return true }

// ModeSwitchFinalizeEndOfSpeechPacket finalizes EOS for audio->text switch.
type ModeSwitchFinalizeEndOfSpeechPacket struct {
	ContextID  string
	StreamMode protos.StreamMode
}

func (f ModeSwitchFinalizeEndOfSpeechPacket) ContextId() string { return f.ContextID }
func (f ModeSwitchFinalizeEndOfSpeechPacket) IsAsync() bool     { return true }

// ModeSwitchFinalizeDenoisePacket finalizes the denoiser for audio->text switch.
type ModeSwitchFinalizeDenoisePacket struct {
	ContextID  string
	StreamMode protos.StreamMode
}

func (f ModeSwitchFinalizeDenoisePacket) ContextId() string { return f.ContextID }
func (f ModeSwitchFinalizeDenoisePacket) IsAsync() bool     { return true }

// =============================================================================
// Finalization chain:
// FinalizeBehavior → FinalizeEndOfSpeech → FinalizeVoiceActivityDetection
// → FinalizeTextToSpeech → FinalizeSpeechToText → FinalizeAuthentication
// → FinalizeSessionRuntime → AnalysisStart → ExecuteAnalysis* → AnalysisDone
// → WebhookStart → ExecuteWebhook* → WebhookDone
// → FinalizeConversation → FinalizeAssistant → FinalizationCompleted
// Each handler enqueues the next phase to backgroundCh, forming an ordered chain.
// =============================================================================

// FinalizeBehaviorPacket finalizes session behavior controls (timers).
type FinalizeBehaviorPacket struct {
	ContextID string
}

func (f FinalizeBehaviorPacket) ContextId() string { return f.ContextID }

// FinalizeEndOfSpeechPacket finalizes end-of-speech processing.
type FinalizeEndOfSpeechPacket struct {
	ContextID string
}

func (f FinalizeEndOfSpeechPacket) ContextId() string { return f.ContextID }

// FinalizeVoiceActivityDetectionPacket finalizes VAD processing.
type FinalizeVoiceActivityDetectionPacket struct {
	ContextID string
}

func (f FinalizeVoiceActivityDetectionPacket) ContextId() string { return f.ContextID }

// FinalizeTextToSpeechPacket finalizes text-to-speech resources.
type FinalizeTextToSpeechPacket struct {
	ContextID string
}

func (f FinalizeTextToSpeechPacket) ContextId() string { return f.ContextID }

// FinalizeSpeechToTextPacket finalizes speech-to-text resources.
type FinalizeSpeechToTextPacket struct {
	ContextID string
}

func (f FinalizeSpeechToTextPacket) ContextId() string { return f.ContextID }

// FinalizeAuthenticationPacket finalizes session authentication stage.
type FinalizeAuthenticationPacket struct {
	ContextID string
}

func (f FinalizeAuthenticationPacket) ContextId() string { return f.ContextID }

// FinalizeSessionRuntimePacket finalizes runtime resources and recording.
type FinalizeSessionRuntimePacket struct {
	ContextID string
}

func (f FinalizeSessionRuntimePacket) ContextId() string { return f.ContextID }

// FinalizeConversationPacket finalizes conversation-level collectors/events.
type FinalizeConversationPacket struct {
	ContextID string
}

func (f FinalizeConversationPacket) ContextId() string { return f.ContextID }

// FinalizeAssistantPacket finalizes assistant runtime resources.
type FinalizeAssistantPacket struct {
	ContextID string
}

func (f FinalizeAssistantPacket) ContextId() string { return f.ContextID }

// FinalizationCompletedPacket marks terminal completion of disconnect flow.
type FinalizationCompletedPacket struct {
	ContextID string
}

func (f FinalizationCompletedPacket) ContextId() string { return f.ContextID }

// ExecuteAnalysisPacket triggers a single analysis execution.
type ExecuteAnalysisPacket struct {
	ContextID      string
	Arguments      map[string]interface{}
	ConversationID uint64
	Auth           types.SimplePrinciple
}

func (f ExecuteAnalysisPacket) ContextId() string { return f.ContextID }

// ExecuteWebhookPacket triggers a single webhook execution.
type ExecuteWebhookPacket struct {
	ContextID string
	Event     utils.AssistantWebhookEvent
	Arguments map[string]interface{}
}

func (f ExecuteWebhookPacket) ContextId() string { return f.ContextID }

// StartIdleTimeoutPacket explicitly (re)starts the idle timeout timer.
// Routed on outputCh so producers can order it relative to InjectMessagePacket
// and TTS output packets that share the same channel.
type StartIdleTimeoutPacket struct {
	ContextID string
}

func (f StartIdleTimeoutPacket) ContextId() string { return f.ContextID }

// StopIdleTimeoutPacket explicitly stops the idle timeout timer.
// ResetCount = true also clears the consecutive idle backoff counter
// (used when the user actively engages, not for system-driven stops).
type StopIdleTimeoutPacket struct {
	ContextID  string
	ResetCount bool
}

func (f StopIdleTimeoutPacket) ContextId() string { return f.ContextID }

// =============================================================================
// LLM Pipeline — execute -> delta -> done -> error -> tools
// =============================================================================

// LLMResponseDeltaPacket represents a streaming text delta from the LLM.
type LLMResponseDeltaPacket struct {
	ContextID string
	Text      string
}

func (f LLMResponseDeltaPacket) ContextId() string { return f.ContextID }

// LLMResponseDonePacket signals the completion of an LLM response stream.
type LLMResponseDonePacket struct {
	ContextID string
	Text      string
}

func (f LLMResponseDonePacket) Content() string   { return f.Text }
func (f LLMResponseDonePacket) Role() string      { return "assistant" }
func (f LLMResponseDonePacket) ContextId() string { return f.ContextID }

// LLMErrorPacket signals that the LLM encountered an error during generation.

type LLMErrorType int

const (
	// UnknownError is the default zero-value fallback
	UnknownError LLMErrorType = iota

	// Recoverable errors (e.g., API rate limits, temporary network drops)
	LLMRateLimit
	LLMNetworkTimeout

	// Non-Recoverable LLMors (e.g., bad API keys, invalid prompt formats)
	LLMAuthentication
	LLMInvalidInput
	LLMSystemPanic
)

// When IsRecoverable is true, the conversation should be gracefully terminated.
type LLMErrorPacket struct {
	ContextID string
	Error     error
	Type      LLMErrorType
}

func (f LLMErrorPacket) ContextId() string { return f.ContextID }
func (f LLMErrorPacket) IsRecoverable() bool {
	return f.Type != LLMAuthentication && f.Type != LLMSystemPanic
}
func (f LLMErrorPacket) Err() error         { return f.Error }
func (f LLMErrorPacket) ErrMessage() string { return fmt.Sprintf("llm: %s", f.Error.Error()) }

// LLMToolCallPacket signals that a tool was invoked.
// Action determines whether the client/channel needs to act (e.g. end call, transfer).
type LLMToolCallPacket struct {
	ToolID    string
	Name      string
	ContextID string
	Action    protos.ToolCallAction
	Arguments map[string]string
}

func (f LLMToolCallPacket) ContextId() string { return f.ContextID }
func (f LLMToolCallPacket) ToolId() string    { return f.ToolID }

func (f LLMToolCallPacket) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"tool_id":    f.ToolID,
		"name":       f.Name,
		"context_id": f.ContextID,
		"action":     f.Action.String(),
		"arguments":  f.Arguments,
	})
}

func (f *LLMToolCallPacket) UnmarshalJSON(data []byte) error {
	var raw struct {
		ToolID    string            `json:"tool_id"`
		Name      string            `json:"name"`
		ContextID string            `json:"context_id"`
		Action    string            `json:"action"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	f.ToolID = raw.ToolID
	f.Name = raw.Name
	f.ContextID = raw.ContextID
	f.Action = protos.ToolCallAction(protos.ToolCallAction_value[raw.Action])
	f.Arguments = raw.Arguments
	return nil
}

// LLMToolResultPacket carries the result of a tool execution.
// Arrives from server-side tools (immediate) or from client/channel (directive).
type LLMToolResultPacket struct {
	ToolID    string
	Name      string
	ContextID string
	Action    protos.ToolCallAction
	Result    map[string]string
}

func (f LLMToolResultPacket) ToolId() string    { return f.ToolID }
func (f LLMToolResultPacket) ContextId() string { return f.ContextID }

func (f LLMToolResultPacket) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"tool_id":    f.ToolID,
		"name":       f.Name,
		"context_id": f.ContextID,
		"action":     f.Action.String(),
		"result":     f.Result,
	})
}

func (f *LLMToolResultPacket) UnmarshalJSON(data []byte) error {
	var raw struct {
		ToolID    string            `json:"tool_id"`
		Name      string            `json:"name"`
		ContextID string            `json:"context_id"`
		Action    string            `json:"action"`
		Result    map[string]string `json:"result"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	f.ToolID = raw.ToolID
	f.Name = raw.Name
	f.ContextID = raw.ContextID
	f.Action = protos.ToolCallAction(protos.ToolCallAction_value[raw.Action])
	f.Result = raw.Result
	return nil
}

// =============================================================================
// Output Pipeline — aggregate -> speak -> TTS audio -> TTS end
// =============================================================================

type TTSErrorType int

const (
	TTSUnknownError TTSErrorType = iota

	// Recoverable
	TTSRateLimit
	TTSNetworkTimeout

	// Non-Recoverable
	TTSAuthentication
	TTSInvalidInput
	TTSSystemPanic
)

type TextToSpeechErrorPacket struct {
	ContextID string
	Error     error
	Type      TTSErrorType
}

func (f TextToSpeechErrorPacket) ContextId() string { return f.ContextID }
func (f TextToSpeechErrorPacket) IsRecoverable() bool {
	return f.Type != TTSAuthentication
}
func (f TextToSpeechErrorPacket) Err() error         { return f.Error }
func (f TextToSpeechErrorPacket) ErrMessage() string { return fmt.Sprintf("tts: %s", f.Error.Error()) }

// TextToSpeechTextPacket carries a sentence-ready text chunk for TTS synthesis.
type TextToSpeechTextPacket struct {
	ContextID string
	Text      string
}

func (f TextToSpeechTextPacket) ContextId() string { return f.ContextID }

// TextToSpeechDonePacket signals end of this turn's output. TTS flushes remaining audio.
type TextToSpeechDonePacket struct {
	ContextID string
	Text      string
}

func (f TextToSpeechDonePacket) ContextId() string { return f.ContextID }

// TextToSpeechAudioPacket carries a TTS audio chunk produced by the TTS provider.
type TextToSpeechAudioPacket struct {
	ContextID  string
	AudioChunk []byte
}

func (f TextToSpeechAudioPacket) ContextId() string { return f.ContextID }

// TextToSpeechEndPacket signals that TTS has finished producing audio.
type TextToSpeechEndPacket struct {
	ContextID string
}

func (f TextToSpeechEndPacket) ContextId() string { return f.ContextID }

// =============================================================================
// Recording
// =============================================================================

// RecordUserAudioPacket carries a user audio chunk to be written to the recorder.
type RecordUserAudioPacket struct {
	ContextID string
	Audio     []byte
}

func (f RecordUserAudioPacket) ContextId() string { return f.ContextID }

// RecordAssistantAudioPacket carries an assistant audio chunk to the recorder.
// When Truncate is true, the recorder trims buffered assistant audio at the current
// wall-clock position, mirroring the streamer's ClearOutputBuffer on interruption.
type RecordAssistantAudioPacket struct {
	ContextID string
	Audio     []byte
	Truncate  bool
}

func (f RecordAssistantAudioPacket) ContextId() string { return f.ContextID }

// =============================================================================
// Persistence
// =============================================================================

// MessageCreatePacket persists a conversation message to the database and appends
// it to the in-memory history. It implements MessagePacket so it can be passed
// directly to onCreateMessage.
type MessageCreatePacket struct {
	ContextID   string
	MessageRole string
	Text        string
}

func (f MessageCreatePacket) ContextId() string { return f.ContextID }
func (f MessageCreatePacket) Role() string      { return f.MessageRole }
func (f MessageCreatePacket) Content() string   { return f.Text }

// ToolLogCreatePacket persists a tool call start to the database.
type ToolLogCreatePacket struct {
	ContextID string
	ToolID    string
	Name      string
	Request   []byte
}

func (f ToolLogCreatePacket) ContextId() string { return f.ContextID }

// ToolLogUpdatePacket persists a tool call result to the database.
type ToolLogUpdatePacket struct {
	ContextID string
	ToolID    string
	Response  []byte
}

func (f ToolLogUpdatePacket) ContextId() string { return f.ContextID }

// HTTPLogCreatePacket persists generic HTTP execution logs (webhook, authentication, analysis).
type HTTPLogCreatePacket struct {
	ContextID       string
	Source          string
	SourceRefID     uint64
	SourceEvent     string
	HTTPURL         string
	HTTPMethod      string
	ResponseStatus  int64
	TimeTaken       int64
	RetryCount      uint32
	Status          type_enums.RecordState
	ErrorMessage    *string
	RequestPayload  []byte
	ResponsePayload []byte
}

func (f HTTPLogCreatePacket) ContextId() string { return f.ContextID }
func (f HTTPLogCreatePacket) IsAsync() bool     { return true }

// =============================================================================
// Metrics & Metadata
// =============================================================================

// ConversationMetricPacket carries conversation-level metrics.
type ConversationMetricPacket struct {
	ContextID uint64
	Metrics   []*protos.Metric
}

func (f ConversationMetricPacket) ContextId() string      { return fmt.Sprintf("%d", f.ContextID) }
func (f ConversationMetricPacket) ConversationID() uint64 { return f.ContextID }

// ConversationMetadataPacket carries conversation-level metadata.
type ConversationMetadataPacket struct {
	ContextID uint64
	Metadata  []*protos.Metadata
}

func (f ConversationMetadataPacket) ContextId() string      { return fmt.Sprintf("%d", f.ContextID) }
func (f ConversationMetadataPacket) ConversationID() uint64 { return f.ContextID }

// UserMessageMetricPacket carries metrics for a user message turn.
type UserMessageMetricPacket struct {
	ContextID string
	Metrics   []*protos.Metric
}

func (f UserMessageMetricPacket) ContextId() string { return f.ContextID }

// AssistantMessageMetricPacket carries metrics for an assistant message turn.
type AssistantMessageMetricPacket struct {
	ContextID string
	Metrics   []*protos.Metric
}

func (f AssistantMessageMetricPacket) ContextId() string { return f.ContextID }

// AssistantMessageMetadataPacket carries metadata for an assistant message turn.
type AssistantMessageMetadataPacket struct {
	ContextID string
	Metadata  []*protos.Metadata
}

func (f AssistantMessageMetadataPacket) ContextId() string { return f.ContextID }

// UserMessageMetadataPacket carries metadata for a user message turn.
type UserMessageMetadataPacket struct {
	ContextID string
	Metadata  []*protos.Metadata
}

func (f UserMessageMetadataPacket) ContextId() string { return f.ContextID }

// =============================================================================
// Observability
// =============================================================================

// ConversationEventPacket carries a named pipeline event for the debugger.
// Each component emits these alongside its existing packets; they flow through
// lowCh so they never compete with STT/LLM/TTS audio.
type ConversationEventPacket struct {
	// ContextID identifies the interaction turn. May be empty when emitted by
	// components that don't hold the session context (e.g. STT callbacks);
	// handleConversationEvent fills it from r.GetID() in that case.
	ContextID string

	// Name is the component name: "stt", "tts", "llm", "vad", "eos",
	// "knowledge", "session", "behavior", "denoise", "audio", "tool", etc.
	Name string

	// Data carries event-specific key/value pairs. Always includes "type".
	Data map[string]string

	// Time is the wall-clock time the event was raised.
	Time time.Time
}

func (f ConversationEventPacket) ContextId() string { return f.ContextID }

// =============================================================================
// Non-packet Support Types
// =============================================================================

// KnowledgeRetrieveOption contains options for knowledge retrieval operations.
type KnowledgeRetrieveOption struct {
	EmbeddingProviderCredential *protos.VaultCredential
	RetrievalMethod             string
	TopK                        uint32
	ScoreThreshold              float32
}

// KnowledgeContextResult holds a single knowledge retrieval result.
type KnowledgeContextResult struct {
	ID         string                 `json:"id"`
	DocumentID string                 `json:"document_id"`
	Metadata   map[string]interface{} `json:"metadata"`
	Content    string                 `json:"content"`
	Score      float64                `json:"score"`
}
