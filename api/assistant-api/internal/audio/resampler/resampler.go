// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_audio_resampler

import (
	internal_resampler_default "github.com/rapidaai/api/assistant-api/internal/audio/resampler/internal/default"
	internal_resampler_soxr "github.com/rapidaai/api/assistant-api/internal/audio/resampler/internal/soxr"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
)

// GetResampler returns the high-quality streaming resampler used for transport
// pipelines where consecutive chunks belong to one continuous audio stream.
func GetResampler(logger commons.Logger) (internal_type.AudioResampler, error) {
	return internal_resampler_soxr.NewLibsoxrAudioResampler(logger), nil
}

// GetChunkResampler returns a stateless exact-length resampler for bounded
// preprocessing stages that must preserve duration per call.
func GetChunkResampler(logger commons.Logger) (internal_type.AudioResampler, error) {
	return internal_resampler_soxr.NewLibsoxrChunkAudioResampler(logger), nil
}

func GetConverter(logger commons.Logger) (internal_type.AudioConverter, error) {
	return internal_resampler_default.NewDefaultAudioConverter(logger), nil
}
