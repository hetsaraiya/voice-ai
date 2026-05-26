// Copyright (c) 2023-2026 RapidaAI
// Author: RapidaAI Team <team@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_telnyx

import (
	internal_ambient "github.com/rapidaai/api/assistant-api/internal/audio/ambient"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/protos"
	"github.com/zaf/g711"
)

type audioAdapter struct {
	resampler        internal_type.AudioResampler
	downstreamConfig *protos.AudioConfig
	providerConfig   *protos.AudioConfig
	frameSize        int
	silenceByte      byte
}

func newAudioAdapter(
	resampler internal_type.AudioResampler,
	downstreamConfig *protos.AudioConfig,
	providerConfig *protos.AudioConfig,
	frameSize int,
	silenceByte byte,
) *audioAdapter {
	return &audioAdapter{
		resampler:        resampler,
		downstreamConfig: downstreamConfig,
		providerConfig:   providerConfig,
		frameSize:        frameSize,
		silenceByte:      silenceByte,
	}
}

func (a *audioAdapter) FrameSize() int    { return a.frameSize }
func (a *audioAdapter) SilenceByte() byte { return a.silenceByte }

func (a *audioAdapter) ConvertOutput(audio []byte) ([]byte, error) {
	return a.resampler.Resample(audio, a.downstreamConfig, a.providerConfig)
}

func (a *audioAdapter) MixAmbient(frame []byte, mixer internal_ambient.Mixer) []byte {
	if mixer == nil {
		return frame
	}
	if len(frame) > 0 {
		primaryPCM := g711.DecodeUlaw(frame)
		mixedPCM, err := mixer.Mix(primaryPCM)
		if err != nil || len(mixedPCM) == 0 {
			return frame
		}
		return g711.EncodeUlaw(mixedPCM)
	}
	mixedPCM, err := mixer.Mix(nil)
	if err != nil || len(mixedPCM) == 0 {
		return nil
	}
	return g711.EncodeUlaw(mixedPCM)
}
