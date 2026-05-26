// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_denoiser

import (
	"context"

	internal_denoiser_krisp "github.com/rapidaai/api/assistant-api/internal/denoiser/internal/krisp"
	internal_denoiser_rnnoise "github.com/rapidaai/api/assistant-api/internal/denoiser/internal/rn_noise"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/utils"
)

type DenoiserIdentifier string

const (
	RN_NOISE                   DenoiserIdentifier = "rn_noise"
	KRISP                      DenoiserIdentifier = "krisp"
	DenoiserOptionsKeyProvider                    = "microphone.denoising.provider"
)

// logger, audioConfig, onPacket, opts
func GetDenoiser(ctx context.Context, logger commons.Logger, onPacket func(context.Context, ...internal_type.Packet) error, options utils.Option) (internal_type.VoiceDenoiserExecutor, error) {
	provider, _ := options.GetString(DenoiserOptionsKeyProvider)
	switch DenoiserIdentifier(provider) {
	case KRISP:
		return internal_denoiser_krisp.NewKrispDenoiser(ctx, logger, onPacket, options)
	default:
		return internal_denoiser_rnnoise.NewRnnoiseDenoiser(ctx, logger, onPacket, options)
	}
}
