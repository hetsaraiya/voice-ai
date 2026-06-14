// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package router

import (
	"context"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
)

type RouteQueue interface {
	FlushControlMatching(func(internal_type.Packet) bool) int
	FlushBootstrapMatching(func(internal_type.Packet) bool) int
	FlushIngressMatching(func(internal_type.Packet) bool) int
	FlushEgressMatching(func(internal_type.Packet) bool) int
	FlushDataMatching(func(internal_type.Packet) bool) int
	FlushBackgroundMatching(func(internal_type.Packet) bool) int
}

type DispatchRoute struct {
	policy PacketPolicy
	queue  RouteQueue
}

func NewDispatchRoute(policy PacketPolicy, queue RouteQueue) *DispatchRoute {
	return &DispatchRoute{
		policy: policy,
		queue:  queue,
	}
}

func (r *DispatchRoute) Route(ctx context.Context, packet internal_type.Packet, next func(context.Context, internal_type.Packet)) {
	r.policy.Route(ctx, packet, next)
}

func (r *DispatchRoute) ApplyPolicy(policy internal_type.DispatchPolicy) {
	if policy.Target == internal_type.PacketNameDispatchPolicy {
		return
	}

	r.policy.Apply(policy)

	switch policy.Action {
	case internal_type.DispatchActionIgnore:
		r.drain(policy.Target)
	case internal_type.DispatchActionPassthrough:
	default:
		return
	}
}

func (r *DispatchRoute) drain(target internal_type.PacketName) {
	matchTarget := func(queued internal_type.Packet) bool {
		return queued.PacketName() == target
	}
	switch ClassifyName(target) {
	case RouteControl:
		r.queue.FlushControlMatching(matchTarget)
	case RouteBootstrap:
		r.queue.FlushBootstrapMatching(matchTarget)
	case RouteIngress:
		r.queue.FlushIngressMatching(matchTarget)
	case RouteEgress:
		r.queue.FlushEgressMatching(matchTarget)
	case RouteData:
		r.queue.FlushDataMatching(matchTarget)
	case RouteBackground:
		r.queue.FlushBackgroundMatching(matchTarget)
	default:
		r.queue.FlushBackgroundMatching(matchTarget)
	}
}
