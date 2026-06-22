// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_type

import (
	"context"

	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
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

// SyncExecutor is for session-scoped executors that must return data to the
// caller before the next packet in the flow can run.
type SyncExecutor[I any, O any] interface {
	Name() string
	Options() utils.Option
	Arguments() (map[string]string, error)
	Execute(ctx context.Context, input I) (O, error)
	Close(ctx context.Context) error
}

type AnalysisInput struct {
	ContextID      string
	Arguments      map[string]interface{}
	ConversationID uint64
	Auth           types.SimplePrinciple
}

type AnalysisOutput struct {
	Metadata *protos.Metadata
}

type AuthenticationInput struct {
	ContextID      string
	Arguments      map[string]interface{}
	Initialization *protos.ConversationInitialization
}

type AuthenticationOutput struct {
	Authenticated bool
	Arguments     map[string]interface{}
	Metadata      map[string]interface{}
	Options       map[string]interface{}
}

// Typed interfaces for each concrete executor.
type LLMExecutor interface {
	Executor[Packet]
}

type AnalysisExecutor interface {
	SyncExecutor[AnalysisInput, AnalysisOutput]
}

type AuthenticationExecutor interface {
	SyncExecutor[AuthenticationInput, AuthenticationOutput]
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
