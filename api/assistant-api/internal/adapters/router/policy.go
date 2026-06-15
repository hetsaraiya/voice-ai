// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package router

import (
	"context"
	"sync"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
)

type PacketPolicy interface {
	Route(ctx context.Context, packet internal_type.Packet, next func(context.Context, internal_type.Packet))
	Apply(policy internal_type.DispatchPolicy)
}

// RoutePolicy owns active dispatch policy state.
type RoutePolicy struct {
	mu            sync.RWMutex
	defaultAction internal_type.DispatchAction
	actions       map[internal_type.PacketName]internal_type.DispatchAction
}

func NewRoutePolicy() *RoutePolicy {
	return &RoutePolicy{
		defaultAction: internal_type.DispatchActionPassthrough,
		actions:       make(map[internal_type.PacketName]internal_type.DispatchAction),
	}
}

func (p *RoutePolicy) Route(ctx context.Context, packet internal_type.Packet, next func(context.Context, internal_type.Packet)) {
	p.mu.RLock()
	action, ok := p.actions[packet.PacketName()]
	if !ok {
		action = p.defaultAction
	}
	p.mu.RUnlock()

	switch action {
	case internal_type.DispatchActionIgnore:
		return
	case internal_type.DispatchActionPassthrough:
		next(ctx, packet)
	default:
		next(ctx, packet)
	}
}

func (p *RoutePolicy) Apply(policy internal_type.DispatchPolicy) {
	switch policy.Action {
	case internal_type.DispatchActionIgnore:
		p.mu.Lock()
		p.actions[policy.Target] = policy.Action
		p.mu.Unlock()
	case internal_type.DispatchActionPassthrough:
		p.mu.Lock()
		delete(p.actions, policy.Target)
		p.mu.Unlock()
	default:
		return
	}
}
