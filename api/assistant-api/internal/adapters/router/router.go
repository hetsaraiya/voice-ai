// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package router

import (
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
)

// Route identifies which dispatcher channel should receive a packet.
type Route uint8

const (
	RouteControl Route = iota + 1
	RouteBootstrap
	RouteIngress
	RouteEgress
	RouteData
	RouteBackground
)

// Classify maps a packet to its dispatcher route.
// Unknown packets default to RouteBackground.
func Classify(p internal_type.Packet) Route {
	switch p.(type) {
	// Critical — interrupts, tool lifecycle
	case internal_type.InterruptionDetectedPacket,
		internal_type.TTSInterruptPacket,
		internal_type.LLMInterruptPacket,
		internal_type.STTInterruptPacket,
		internal_type.TurnChangePacket:
		return RouteControl

	// Bootstrap — connect/session initialization pipeline
	case internal_type.InitializeAssistantPacket,
		internal_type.InitializeConversationPacket,
		internal_type.InitializeSessionRuntimePacket,
		internal_type.InitializeAuthenticationPacket,
		internal_type.ExecuteSessionAuthenticationPacket,
		internal_type.SessionAuthenticationSucceededPacket,
		internal_type.SessionAuthenticationFailedPacket,
		internal_type.InitializeSpeechToTextPacket,
		internal_type.InitializeTextToSpeechPacket,
		internal_type.InitializeVoiceActivityDetectionPacket,
		internal_type.InitializeEndOfSpeechPacket,
		internal_type.InitializeDenoisePacket,
		internal_type.InitializeBehaviorPacket,
		internal_type.InitializationCompletedPacket,
		internal_type.InitializationFailedPacket,
		internal_type.InitializeTelemetryPacket,
		internal_type.InitializeOutboundDispatcherPacket,
		internal_type.InitializeInboundDispatcherPacket,
		internal_type.ModeSwitchRequestedPacket,
		internal_type.ModeSwitchCompletedPacket,
		internal_type.ModeSwitchInitializeSpeechToTextPacket,
		internal_type.ModeSwitchInitializeTextToSpeechPacket,
		internal_type.ModeSwitchInitializeVoiceActivityDetectionPacket,
		internal_type.ModeSwitchInitializeEndOfSpeechPacket,
		internal_type.ModeSwitchFinalizeEndOfSpeechPacket,
		internal_type.ModeSwitchInitializeDenoisePacket,
		internal_type.ModeSwitchFinalizeDenoisePacket,
		internal_type.ModeSwitchFinalizeVoiceActivityDetectionPacket,
		internal_type.ModeSwitchFinalizeTextToSpeechPacket,
		internal_type.ModeSwitchFinalizeSpeechToTextPacket:
		return RouteBootstrap

	// Input — inbound audio pipeline, VAD, STT, EOS
	case internal_type.UserAudioReceivedPacket,
		internal_type.UserTextReceivedPacket,
		internal_type.DenoiseAudioPacket,
		internal_type.DenoisedAudioPacket,
		internal_type.VadAudioPacket,
		internal_type.VadSpeechActivityPacket,
		internal_type.SpeechToTextPacket,
		internal_type.EndOfSpeechPacket,
		internal_type.InterimEndOfSpeechPacket,
		internal_type.UserInputPacket,
		internal_type.LLMToolResultPacket:
		return RouteIngress

	// Output — LLM generation, TTS, outbound pipeline
	case internal_type.LLMResponseDeltaPacket,
		internal_type.LLMResponseDonePacket,
		internal_type.ErrorPacket,
		internal_type.InjectMessagePacket,
		internal_type.StartIdleTimeoutPacket,
		internal_type.StopIdleTimeoutPacket,
		internal_type.TTSTextPacket,
		internal_type.TTSDonePacket,
		internal_type.TextToSpeechAudioPacket,
		internal_type.TextToSpeechEndPacket,
		internal_type.LLMToolCallPacket:
		return RouteEgress

	// Data — DB writes, recording, lifecycle orchestration. No observer dependency,
	// dispatcher starts at NewGenericRequestor.
	case internal_type.RecordUserAudioPacket,
		internal_type.RecordAssistantAudioPacket,
		internal_type.MessageCreatePacket,
		internal_type.ConversationMetadataPacket,
		internal_type.UserMessageMetadataPacket,
		internal_type.AssistantMessageMetadataPacket,
		internal_type.ToolLogCreatePacket,
		internal_type.ToolLogUpdatePacket,
		internal_type.HTTPLogCreatePacket,
		internal_type.FinalizeBehaviorPacket,
		internal_type.FinalizeEndOfSpeechPacket,
		internal_type.FinalizeVoiceActivityDetectionPacket,
		internal_type.FinalizeTextToSpeechPacket,
		internal_type.FinalizeSpeechToTextPacket,
		internal_type.FinalizeAuthenticationPacket,
		internal_type.FinalizeSessionRuntimePacket,
		internal_type.FinalizeConversationPacket,
		internal_type.FinalizeAssistantPacket,
		internal_type.FinalizationCompletedPacket,
		internal_type.ExecuteAnalysisPacket,
		internal_type.ExecuteWebhookPacket:
		return RouteData

	// Background — observer-touching telemetry. Dispatcher starts after telemetry init.
	case internal_type.ConversationEventPacket,
		internal_type.ConversationMetricPacket,
		internal_type.UserMessageMetricPacket,
		internal_type.AssistantMessageMetricPacket:
		return RouteBackground
	default:
		return RouteBackground
	}
}
