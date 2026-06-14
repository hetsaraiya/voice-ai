// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package adapter_internal

import (
	"context"

	adapter_channel "github.com/rapidaai/api/assistant-api/internal/adapters/channel"
	adapter_router "github.com/rapidaai/api/assistant-api/internal/adapters/router"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/utils"
)

// =============================================================================
// OnPacket — enqueue into the priority channel
// =============================================================================

func (r *genericRequestor) OnPacket(ctx context.Context, pkts ...internal_type.Packet) error {
	for _, p := range pkts {
		route := adapter_router.Classify(p)
		switch route {
		case adapter_router.RouteControl:
			r.channels.OnControl(adapter_channel.Envelope{Ctx: ctx, Pkt: p})
		case adapter_router.RouteBootstrap:
			r.channels.OnBootstrap(adapter_channel.Envelope{Ctx: ctx, Pkt: p})
		case adapter_router.RouteIngress:
			r.channels.OnIngress(adapter_channel.Envelope{Ctx: ctx, Pkt: p})
		case adapter_router.RouteEgress:
			r.channels.OnEgress(adapter_channel.Envelope{Ctx: ctx, Pkt: p})
		case adapter_router.RouteData:
			r.channels.OnData(adapter_channel.Envelope{Ctx: ctx, Pkt: p})
		case adapter_router.RouteBackground:
			r.channels.OnBackground(adapter_channel.Envelope{Ctx: ctx, Pkt: p})
		default:
			r.channels.OnBackground(adapter_channel.Envelope{Ctx: ctx, Pkt: p})
		}
	}
	return nil
}

// =============================================================================
// Dispatchers — one goroutine per priority channel
// =============================================================================

func (r *genericRequestor) runCriticalDispatcher(ctx context.Context) {
	r.channels.RunControl(ctx, func(e adapter_channel.Envelope) {
		r.dispatch(e.Ctx, e.Pkt)
	})
}

func (r *genericRequestor) runBootstrapDispatcher(ctx context.Context) {
	r.channels.RunBootstrap(ctx, func(e adapter_channel.Envelope) {
		r.dispatch(e.Ctx, e.Pkt)
	})
}

func (r *genericRequestor) runInputDispatcher(ctx context.Context) {
	r.channels.RunIngress(ctx, func(e adapter_channel.Envelope) {
		r.dispatch(e.Ctx, e.Pkt)
	})
}

func (r *genericRequestor) runOutputDispatcher(ctx context.Context) {
	r.channels.RunEgress(ctx, func(e adapter_channel.Envelope) {
		r.dispatch(e.Ctx, e.Pkt)
	})
}

func (r *genericRequestor) runDataDispatcher(ctx context.Context) {
	r.channels.RunData(ctx, func(e adapter_channel.Envelope) {
		r.dispatch(e.Ctx, e.Pkt)
	})
}

func (r *genericRequestor) runLowDispatcher(ctx context.Context) {
	r.channels.RunBackground(ctx, func(e adapter_channel.Envelope) {
		r.dispatch(e.Ctx, e.Pkt)
	})
}

// =============================================================================
// dispatch — routes a single packet to its handler
// =============================================================================

func (r *genericRequestor) dispatch(ctx context.Context, p internal_type.Packet) {
	r.dispatchRoute.Route(ctx, p, func(ctx context.Context, p internal_type.Packet) {
		switch p.(type) {
		case internal_type.AsyncPacket:
			utils.Go(ctx, func() {
				_ = adapter_router.DispatchPacket(ctx, p, requestorDispatchHandler{r: r})
			})
		default:
			_ = adapter_router.DispatchPacket(ctx, p, requestorDispatchHandler{r: r})
		}
	})
}
