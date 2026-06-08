// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_denoiser_rnnoise

import (
	"context"
	"fmt"
	"time"

	internal_audio "github.com/rapidaai/api/assistant-api/internal/audio"
	internal_audio_resampler "github.com/rapidaai/api/assistant-api/internal/audio/resampler"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

type rnnoiseDenoiser struct {
	rnNoise        *RNNoise
	logger         commons.Logger
	denoiserConfig *protos.AudioConfig
	// default rapida input config
	inputConfig    *protos.AudioConfig
	audioResampler internal_type.AudioResampler
	audioConverter internal_type.AudioConverter
	onPacket       func(context.Context, ...internal_type.Packet) error
}

// NewDenoiser creates a new denoiser instance
func NewRnnoiseDenoiser(
	ctx context.Context,
	logger commons.Logger, onPacket func(context.Context, ...internal_type.Packet) error, options utils.Option,
) (internal_type.VoiceDenoiserExecutor, error) {
	start := time.Now()
	rn, err := NewRNNoise()
	if err != nil {
		return nil, err
	}
	resampler, err := internal_audio_resampler.GetChunkResampler(logger)
	if err != nil {
		return nil, err
	}
	converter, err := internal_audio_resampler.GetConverter(logger)
	if err != nil {
		return nil, err
	}

	d := &rnnoiseDenoiser{
		audioResampler: resampler,
		audioConverter: converter,
		rnNoise:        rn,
		denoiserConfig: &protos.AudioConfig{
			SampleRate:  48000,
			AudioFormat: protos.AudioConfig_LINEAR16,
			Channels:    1,
		},
		inputConfig: internal_audio.RAPIDA_INTERNAL_AUDIO_CONFIG,
		logger:      logger,
		onPacket:    onPacket,
	}
	if onPacket != nil {
		_ = onPacket(ctx,
			internal_type.ObservabilityEventRecordPacket{
				Scope: internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordEvent{
					Component: observability.ComponentDenoise,
					Event:     observability.DenoiseStarted,
					Attributes: observability.Attributes{
						"provider": "rnnoise",
						"init_ms":  fmt.Sprintf("%d", time.Since(start).Milliseconds()),
					},
				},
			},
			internal_type.ObservabilityLogRecordPacket{
				Scope: internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelDebug,
					Message: "denoiser initialized",
					Attributes: observability.Attributes{
						"component": observability.ComponentDenoise.String(),
						"operation": "initialize",
						"provider":  "rnnoise",
						"init_ms":   fmt.Sprintf("%d", time.Since(start).Milliseconds()),
					},
				},
			},
		)
	}
	return d, nil
}

func (rnd *rnnoiseDenoiser) Name() string {
	return "rnnoise-denoiser"
}

func (rnd *rnnoiseDenoiser) Options() utils.Option {
	rnd.logger.Warn("RNNoise denoiser does not support any options yet")
	return nil
}

func (rnd *rnnoiseDenoiser) Arguments() (map[string]string, error) {
	return nil, nil
}

// Denoise processes the audio in pkt and pushes a DenoisedAudioPacket via
// onPacket instead of returning bytes to the caller. On error it falls back
// to the original audio and still emits the packet with NoiseReduced=false.
func (rnd *rnnoiseDenoiser) Execute(ctx context.Context, pkt internal_type.DenoiseAudioPacket) error {
	input := pkt.Audio
	if rnd.inputConfig == nil || rnd.denoiserConfig == nil {
		_ = rnd.onPacket(ctx, internal_type.DenoisedAudioPacket{
			ContextID: pkt.ContextID,
			Audio:     input,
		}, internal_type.ObservabilityEventRecordPacket{
			ContextID:   pkt.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordEvent{
				Component: observability.ComponentDenoise,
				Event:     observability.DenoiseError,
				Attributes: observability.Attributes{
					"provider":   "rnnoise",
					"context_id": pkt.ContextID,
					"error":      "audio config is not initialized",
				},
			},
		}, internal_type.ObservabilityLogRecordPacket{
			ContextID:   pkt.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "denoise failed",
				Attributes: observability.Attributes{
					"component":   observability.ComponentDenoise.String(),
					"operation":   "validate_config",
					"provider":    "rnnoise",
					"context_id":  pkt.ContextID,
					"audio_bytes": fmt.Sprintf("%d", len(input)),
					"error":       "audio config is not initialized",
				},
			},
		})
		return fmt.Errorf("audio config is not initialized")
	}

	resampledInput, err := rnd.audioResampler.Resample(input, rnd.inputConfig, rnd.denoiserConfig)
	if err != nil {
		_ = rnd.onPacket(ctx, internal_type.DenoisedAudioPacket{
			ContextID: pkt.ContextID,
			Audio:     input,
		}, internal_type.ObservabilityEventRecordPacket{
			ContextID:   pkt.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordEvent{
				Component: observability.ComponentDenoise,
				Event:     observability.DenoiseError,
				Attributes: observability.Attributes{
					"provider":   "rnnoise",
					"context_id": pkt.ContextID,
					"error":      "failed to resample audio",
				},
			},
		}, internal_type.ObservabilityLogRecordPacket{
			ContextID:   pkt.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "denoise failed",
				Attributes: observability.Attributes{
					"component":   observability.ComponentDenoise.String(),
					"operation":   "resample_input",
					"provider":    "rnnoise",
					"context_id":  pkt.ContextID,
					"audio_bytes": fmt.Sprintf("%d", len(input)),
					"error":       err.Error(),
					"error_type":  fmt.Sprintf("%T", err),
				},
			},
		})
		return fmt.Errorf("failed to resample audio: %w", err)
	}

	floatSamples, err := rnd.audioConverter.ConvertToFloat32Samples(resampledInput, rnd.denoiserConfig)
	if err != nil {
		_ = rnd.onPacket(ctx, internal_type.DenoisedAudioPacket{
			ContextID: pkt.ContextID,
			Audio:     input,
		}, internal_type.ObservabilityEventRecordPacket{
			ContextID:   pkt.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordEvent{
				Component: observability.ComponentDenoise,
				Event:     observability.DenoiseError,
				Attributes: observability.Attributes{
					"provider":   "rnnoise",
					"context_id": pkt.ContextID,
					"error":      "failed to convert audio to float32 samples",
				},
			},
		}, internal_type.ObservabilityLogRecordPacket{
			ContextID:   pkt.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "denoise failed",
				Attributes: observability.Attributes{
					"component":   observability.ComponentDenoise.String(),
					"operation":   "convert_to_float32",
					"provider":    "rnnoise",
					"context_id":  pkt.ContextID,
					"audio_bytes": fmt.Sprintf("%d", len(resampledInput)),
					"error":       err.Error(),
					"error_type":  fmt.Sprintf("%T", err),
				},
			},
		})
		return fmt.Errorf("failed to convert audio to float32 samples: %w", err)
	}

	confidence, cleanedSamples, err := rnd.rnNoise.ProcessAudio(floatSamples)
	if err != nil {
		_ = rnd.onPacket(ctx, internal_type.DenoisedAudioPacket{
			ContextID: pkt.ContextID,
			Audio:     input,
		}, internal_type.ObservabilityEventRecordPacket{
			ContextID:   pkt.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordEvent{
				Component: observability.ComponentDenoise,
				Event:     observability.DenoiseError,
				Attributes: observability.Attributes{
					"provider":   "rnnoise",
					"context_id": pkt.ContextID,
					"error":      "failed to process audio",
				},
			},
		}, internal_type.ObservabilityLogRecordPacket{
			ContextID:   pkt.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "denoise failed",
				Attributes: observability.Attributes{
					"component":    observability.ComponentDenoise.String(),
					"operation":    "process_audio",
					"provider":     "rnnoise",
					"context_id":   pkt.ContextID,
					"sample_count": fmt.Sprintf("%d", len(floatSamples)),
					"error":        err.Error(),
					"error_type":   fmt.Sprintf("%T", err),
				},
			},
		})
		return fmt.Errorf("failed to process audio: %w", err)
	}

	denoisedBytes, err := rnd.audioConverter.ConvertToByteSamples(cleanedSamples, rnd.denoiserConfig)
	if err != nil {
		_ = rnd.onPacket(ctx, internal_type.DenoisedAudioPacket{
			ContextID: pkt.ContextID,
			Audio:     input,
		}, internal_type.ObservabilityEventRecordPacket{
			ContextID:   pkt.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordEvent{
				Component: observability.ComponentDenoise,
				Event:     observability.DenoiseError,
				Attributes: observability.Attributes{
					"provider":   "rnnoise",
					"context_id": pkt.ContextID,
					"error":      "failed to convert audio to byte samples",
				},
			},
		}, internal_type.ObservabilityLogRecordPacket{
			ContextID:   pkt.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "denoise failed",
				Attributes: observability.Attributes{
					"component":    observability.ComponentDenoise.String(),
					"operation":    "convert_to_bytes",
					"provider":     "rnnoise",
					"context_id":   pkt.ContextID,
					"sample_count": fmt.Sprintf("%d", len(cleanedSamples)),
					"confidence":   fmt.Sprintf("%.4f", confidence),
					"error":        err.Error(),
					"error_type":   fmt.Sprintf("%T", err),
				},
			},
		})
		return fmt.Errorf("failed to convert audio to byte samples: %w", err)
	}

	restoredInputRate, err := rnd.audioResampler.Resample(denoisedBytes, rnd.denoiserConfig, rnd.inputConfig)
	if err != nil {
		_ = rnd.onPacket(ctx, internal_type.DenoisedAudioPacket{
			ContextID: pkt.ContextID,
			Audio:     input,
		}, internal_type.ObservabilityEventRecordPacket{
			ContextID:   pkt.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordEvent{
				Component: observability.ComponentDenoise,
				Event:     observability.DenoiseError,
				Attributes: observability.Attributes{
					"provider":   "rnnoise",
					"context_id": pkt.ContextID,
					"error":      "failed to resample denoised audio",
				},
			},
		}, internal_type.ObservabilityLogRecordPacket{
			ContextID:   pkt.ContextID,
			Scope:       internal_type.ObservabilityRecordScopeMessage,
			MessageRole: observability.MessageRoleUser,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: "denoise failed",
				Attributes: observability.Attributes{
					"component":   observability.ComponentDenoise.String(),
					"operation":   "resample_output",
					"provider":    "rnnoise",
					"context_id":  pkt.ContextID,
					"audio_bytes": fmt.Sprintf("%d", len(denoisedBytes)),
					"confidence":  fmt.Sprintf("%.4f", confidence),
					"error":       err.Error(),
					"error_type":  fmt.Sprintf("%T", err),
				},
			},
		})
		return fmt.Errorf("failed to resample denoised audio: %w", err)
	}

	if rnd.onPacket != nil {
		_ = rnd.onPacket(ctx, internal_type.DenoisedAudioPacket{
			ContextID:  pkt.ContextID,
			Audio:      restoredInputRate,
			Confidence: confidence,
		})
	}
	return nil
}

// Close releases resources
func (d *rnnoiseDenoiser) Close(ctx context.Context) error {
	if d.onPacket != nil {
		_ = d.onPacket(context.Background(),
			internal_type.ObservabilityEventRecordPacket{
				Scope: internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordEvent{
					Component: observability.ComponentDenoise,
					Event:     observability.DenoiseClosed,
					Attributes: observability.Attributes{
						"provider": "rnnoise",
					},
				},
			},
			internal_type.ObservabilityLogRecordPacket{
				Scope: internal_type.ObservabilityRecordScopeConversation,
				Record: observability.RecordLog{
					Level:   observability.LevelDebug,
					Message: "denoiser closed",
					Attributes: observability.Attributes{
						"component": observability.ComponentDenoise.String(),
						"operation": "close",
						"provider":  "rnnoise",
					},
				},
			},
		)
	}
	if d.rnNoise != nil {
		d.rnNoise.Close()
	}
	return nil
}
