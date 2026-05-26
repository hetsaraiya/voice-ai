// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_denoiser_krisp

import (
	"context"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/utils"
)

type krispDenoiser struct {
	logger   commons.Logger
	onPacket func(context.Context, ...internal_type.Packet) error
	options  utils.Option
}

func NewKrispDenoiser(ctx context.Context, logger commons.Logger, onPacket func(context.Context, ...internal_type.Packet) error, options utils.Option) (internal_type.VoiceDenoiserExecutor, error) {
	return &krispDenoiser{logger: logger, onPacket: onPacket, options: options}, nil
}

func (krisp *krispDenoiser) Name() string {
	return "krisp-denoiser"
}
func (krisp *krispDenoiser) Options() utils.Option {
	krisp.logger.Warn("Krisp denoiser does not support any options yet")
	return krisp.options
}
func (krisp *krispDenoiser) Arguments() (map[string]string, error) {
	return map[string]string{}, nil
}

func (krisp *krispDenoiser) Execute(ctx context.Context, pkt internal_type.DenoiseAudioPacket) error {
	panic("not yet implimented")
}

func (krisp *krispDenoiser) Close(ctx context.Context) error {
	panic("not yet implimented")
}
