// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_end_of_speech

import (
	"context"
	"fmt"

	internal_livekit "github.com/rapidaai/api/assistant-api/internal/end_of_speech/internal/livekit"
	internal_pipecat "github.com/rapidaai/api/assistant-api/internal/end_of_speech/internal/pipecat"
	internal_silence_based "github.com/rapidaai/api/assistant-api/internal/end_of_speech/internal/silence_based"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/utils"
)

type EndOfSpeechIdentifier string

const (
	SilenceBasedEndOfSpeech       EndOfSpeechIdentifier = "silence_based_eos"
	LiveKitEndOfSpeech            EndOfSpeechIdentifier = "livekit_eos"
	PipecatSmartTurnEndOfSpeech   EndOfSpeechIdentifier = "pipecat_smart_turn_eos"
	EndOfSpeechOptionsKeyProvider                       = "microphone.eos.provider"
)

func GetEndOfSpeech(
	_ context.Context,
	logger commons.Logger,
	onCallback func(context.Context, ...internal_type.Packet) error,
	opts utils.Option,
) (internal_type.EndOfSpeechExecutor, error) {
	provider, _ := opts.GetString(EndOfSpeechOptionsKeyProvider)
	switch EndOfSpeechIdentifier(provider) {
	case SilenceBasedEndOfSpeech:
		return internal_silence_based.NewSilenceBasedEndOfSpeech(logger, onCallback, opts)
	case LiveKitEndOfSpeech:
		return internal_livekit.NewLivekitEndOfSpeech(logger, onCallback, opts)
	case PipecatSmartTurnEndOfSpeech:
		return internal_pipecat.NewPipecatEndOfSpeech(logger, onCallback, opts)
	default:
		return nil, fmt.Errorf("end_of_speech: unsupported provider %q", provider)
	}
}
