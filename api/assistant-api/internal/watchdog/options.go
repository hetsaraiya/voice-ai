// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package watchdog

import (
	"context"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
)

type onPacketOption struct {
	onPacket func(context.Context, ...internal_type.Packet) error
}

func WithOnPacket(onPacket func(context.Context, ...internal_type.Packet) error) onPacketOption {
	return onPacketOption{onPacket: onPacket}
}

func (option onPacketOption) applyTTSCompletionOptions(options *TTSCompletionOptions) {
	options.OnPacket = option.onPacket
}

func (option onPacketOption) applyIdleTimeoutOptions(options *IdleTimeoutOptions) {
	options.OnPacket = option.onPacket
}

type packetContextOption struct {
	ctx context.Context
}

func WithPacketContext(ctx context.Context) packetContextOption {
	return packetContextOption{ctx: ctx}
}

func (option packetContextOption) applyTTSCompletionOptions(options *TTSCompletionOptions) {
	options.PacketContext = option.ctx
}

func (option packetContextOption) applyIdleTimeoutOptions(options *IdleTimeoutOptions) {
	options.PacketContext = option.ctx
}

type recordScopeOption struct {
	scope internal_type.ObservabilityRecordScope
}

func WithRecordScope(scope internal_type.ObservabilityRecordScope) recordScopeOption {
	return recordScopeOption{scope: scope}
}

func (option recordScopeOption) applyTTSCompletionOptions(options *TTSCompletionOptions) {
	options.RecordScope = option.scope
}

func (option recordScopeOption) applyIdleTimeoutOptions(options *IdleTimeoutOptions) {
	options.RecordScope = option.scope
}
