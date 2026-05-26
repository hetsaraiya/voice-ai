// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_telephony_base

import (
	"encoding/base64"
	"strings"

	internal_audio "github.com/rapidaai/api/assistant-api/internal/audio"
	internal_audio_resampler "github.com/rapidaai/api/assistant-api/internal/audio/resampler"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	channel_base "github.com/rapidaai/api/assistant-api/internal/channel/base"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

// TelephonyOption configures a BaseTelephonyStreamer.
type TelephonyOption func(*telephonyConfig)

type telephonyConfig struct {
	// sourceAudioConfig is the native audio format received from the telephony
	// provider. Defaults to rapida16kConfig (linear16 16kHz) if nil.
	sourceAudioConfig *protos.AudioConfig

	// baseOpts are forwarded to channel_base.NewBaseStreamer.
	baseOpts []channel_base.Option
}

// WithSourceAudioConfig sets the native audio format of the telephony provider.
// This drives automatic derivation of input/output buffer thresholds in BaseStreamer.
// Pass nil to default to rapida16kConfig (linear16 16kHz — no resampling).
func WithSourceAudioConfig(cfg *protos.AudioConfig) TelephonyOption {
	return func(c *telephonyConfig) { c.sourceAudioConfig = cfg }
}

// WithBaseOption appends one or more channel_base.Option to the underlying
// BaseStreamer configuration. Use this for advanced overrides (channel sizes,
// explicit thresholds, etc.).
func WithBaseOption(opts ...channel_base.Option) TelephonyOption {
	return func(c *telephonyConfig) { c.baseOpts = append(c.baseOpts, opts...) }
}

// internal rapida audio config
var rapida16kConfig = internal_audio.NewLinear16khzMonoAudioConfig()

// BaseTelephonyStreamer embeds channel_base.BaseStreamer for common buffer,
// channel, and lifecycle management. It adds telephony-specific concerns:
// call context, audio resampler, base64 encoder, and vault credentials.
//
// Concrete telephony streamers (Twilio, Exotel, Vonage, SIP, Asterisk) embed
// this struct and only implement transport-specific I/O logic.
type BaseTelephonyStreamer struct {
	channel_base.BaseStreamer

	// callCtx holds IDs and metadata from the call setup phase (Redis).
	// Replaces separate assistant/conversation entity references — the
	// streamer only needs IDs, not full DB entities.
	callCtx *callcontext.CallContext

	resampler       internal_type.AudioResampler
	encoder         *base64.Encoding
	vaultCredential *protos.VaultCredential

	// ChannelUUID is the provider-specific call identifier, propagated from
	// CallContext so concrete streamers can use it for call control.
	ChannelUUID string

	// sourceAudioConfig is the native audio format received from the telephony
	// provider (e.g. µ-law 8kHz for Twilio, linear16 16kHz for Vonage).
	// CreateVoiceRequest uses this to resample input audio to the internal
	// Rapida format (linear16 16kHz) before sending downstream.
	sourceAudioConfig *protos.AudioConfig
}

// NewBaseTelephonyStreamer creates a new BaseTelephonyStreamer from a
// CallContext and vault credential. Use TelephonyOption values to configure
// the source audio format and any BaseStreamer overrides.
//
// By default the source audio config is rapida16kConfig (linear16 16kHz),
// and input/output thresholds are automatically derived from that config
// (60 ms input, 20 ms output frames). Concrete streamers only need to provide
// WithSourceAudioConfig to declare their native format — everything else is
// handled by the BaseStreamer defaults.
//
// Example:
//
//	base := NewBaseTelephonyStreamer(logger, cc, vaultCred,
//	    WithSourceAudioConfig(audio.NewMulaw8khzMonoAudioConfig()),
//	)
func NewBaseTelephonyStreamer(
	logger commons.Logger,
	cc *callcontext.CallContext,
	vaultCred *protos.VaultCredential,
	opts ...TelephonyOption,
) BaseTelephonyStreamer {
	tc := telephonyConfig{}
	for _, opt := range opts {
		opt(&tc)
	}

	sourceAudioCfg := tc.sourceAudioConfig
	if sourceAudioCfg == nil {
		sourceAudioCfg = rapida16kConfig
	}

	// Build base options: derive thresholds from the source audio config,
	// then allow caller to override via WithBaseOption.
	baseOpts := []channel_base.Option{
		channel_base.WithInputAudioConfig(sourceAudioCfg),
		channel_base.WithOutputAudioConfig(sourceAudioCfg),
	}
	baseOpts = append(baseOpts, tc.baseOpts...)

	resampler, _ := internal_audio_resampler.GetResampler(logger)
	return BaseTelephonyStreamer{
		BaseStreamer:      channel_base.NewBaseStreamer(logger, baseOpts...),
		callCtx:           cc,
		resampler:         resampler,
		encoder:           base64.StdEncoding,
		vaultCredential:   vaultCred,
		ChannelUUID:       cc.ChannelUUID,
		sourceAudioConfig: sourceAudioCfg,
	}
}

// CreateVoiceRequest resamples raw audio from the provider's native format
// to the internal Rapida format (linear16 16kHz) and wraps it in a
// ConversationUserMessage for downstream processing.
func (base *BaseTelephonyStreamer) CreateVoiceRequest(audioData []byte) *protos.ConversationUserMessage {
	// base.Logger.Debugw("CreateVoiceRequest: Resampling audio",
	// 	"input_size", len(audioData),
	// 	"source_format", base.sourceAudioConfig.GetAudioFormat(),
	// 	"source_rate", base.sourceAudioConfig.GetSampleRate(),
	// 	"target_format", rapida16kConfig.GetAudioFormat(),
	// 	"target_rate", rapida16kConfig.GetSampleRate())

	resampled, err := base.resampler.Resample(audioData, base.sourceAudioConfig, rapida16kConfig)
	if err != nil {
		base.Logger.Warnw("Failed to resample input audio, forwarding raw bytes",
			"error", err.Error(),
			"source_format", base.sourceAudioConfig.GetAudioFormat(),
			"source_rate", base.sourceAudioConfig.GetSampleRate(),
		)
		resampled = audioData
	}

	// base.Logger.Debugw("CreateVoiceRequest: Resampling complete",
	// 	"input_size", len(audioData),
	// 	"output_size", len(resampled))

	return &protos.ConversationUserMessage{
		Message: &protos.ConversationUserMessage_Audio{
			Audio: resampled,
		},
	}
}

// GetAssistantDefinition returns the protobuf assistant definition.
func (base *BaseTelephonyStreamer) GetAssistantDefinition() *protos.AssistantDefinition {
	return &protos.AssistantDefinition{
		AssistantId: base.callCtx.AssistantID,
		Version:     utils.GetVersionString(base.callCtx.AssistantProviderId),
	}
}

// GetConversationId returns the conversation ID.
func (base *BaseTelephonyStreamer) GetConversationId() uint64 {
	return base.callCtx.ConversationID
}

// CallContext returns the underlying call context.
func (base *BaseTelephonyStreamer) CallContext() *callcontext.CallContext {
	return base.callCtx
}

// Encoder returns the base64 encoder used by the streamer.
func (base *BaseTelephonyStreamer) Encoder() *base64.Encoding {
	return base.encoder
}

// VaultCredential returns the vault credential associated with the streamer.
func (base *BaseTelephonyStreamer) VaultCredential() *protos.VaultCredential {
	return base.vaultCredential
}

// Resampler returns the audio resampler.
func (base *BaseTelephonyStreamer) Resampler() internal_type.AudioResampler {
	return base.resampler
}

// SourceAudioConfig returns the native audio format of the telephony provider.
func (base *BaseTelephonyStreamer) SourceAudioConfig() *protos.AudioConfig {
	return base.sourceAudioConfig
}

// CreateConnectionRequest builds the initial ConversationInitialization message.
// Carries non-empty client.* metadata in the init payload so the requestor's
// in-memory state is populated at connect time — avoids races with downstream
// metadata writes that only persist to DB. Empty fields are omitted so they
// can't overwrite previously-stored values during resume.
func (base *BaseTelephonyStreamer) CreateConnectionRequest() *protos.ConversationInitialization {
	clientMetadata := map[string]interface{}{
		"client.direction": base.callCtx.Direction,
		"client.channel":   base.callCtx.Provider,
	}
	if v := base.callCtx.CallerNumber; v != "" {
		clientMetadata["client.phone"] = v
	}
	if v := base.callCtx.FromNumber; v != "" {
		clientMetadata["client.assistant_phone"] = v
	}
	if v := base.callCtx.ChannelUUID; v != "" {
		clientMetadata["client.provider_call_id"] = v
	}
	if v := base.callCtx.ContextID; v != "" {
		clientMetadata["client.context_id"] = v
	}
	metadata, _ := utils.InterfaceMapToAnyMap(clientMetadata)
	return &protos.ConversationInitialization{
		AssistantConversationId: base.GetConversationId(),
		Assistant:               base.GetAssistantDefinition(),
		StreamMode:              protos.StreamMode_STREAM_MODE_AUDIO,
		Metadata:                metadata,
	}
}

// SplitTransferTargets parses a multi-target transfer_to argument into an
// ordered list of targets joined by commons.SEPARATOR. Empty/whitespace
// entries are dropped. If parsing yields nothing, the original raw string is
// returned as a single-element slice so callers always have at least one
// candidate to act on.
func (base *BaseTelephonyStreamer) SplitTransferTargets(raw string) []string {
	parts := strings.Split(raw, commons.SEPARATOR)
	targets := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			targets = append(targets, t)
		}
	}
	if len(targets) == 0 {
		return []string{raw}
	}
	return targets
}
