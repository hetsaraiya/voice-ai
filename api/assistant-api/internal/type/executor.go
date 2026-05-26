// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_type

import (
	"context"

	"github.com/rapidaai/pkg/utils"
)

// Executor is the generic contract for session-scoped executors.
// P is the specific packet type the executor handles in Execute.
// Construction (with full dependency wiring) happens in each implementation's
// New<X>Executor function — there is no separate Initialize phase.
type Executor[P Packet] interface {
	Name() string
	Options() utils.Option
	Arguments() (map[string]string, error)
	Execute(ctx context.Context, packet P) error
	Close(ctx context.Context) error
}

// Typed interfaces for each concrete executor. Each embeds the generic Executor
// so it can extend the contract with executor-specific methods later.
type LLMExecutor interface {
	Executor[Packet]
}

type AnalysisExecutor interface {
	Executor[ExecuteAnalysisPacket]
}

type WebhookExecutor interface {
	Executor[ExecuteWebhookPacket]
}

type AuthenticationExecutor interface {
	Executor[ExecuteSessionAuthenticationPacket]
}

type EndOfSpeechExecutor interface {
	Executor[Packet]
}

type VoiceActivityDetectorExecutor interface {
	Executor[UserAudioReceivedPacket]
}

type VoiceDenoiserExecutor interface {
	Executor[DenoiseAudioPacket]
}
