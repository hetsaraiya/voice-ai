// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package channel_pipeline

import (
	"context"
	"fmt"

	channel_telephony "github.com/rapidaai/api/assistant-api/internal/channel/telephony"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	"github.com/rapidaai/pkg/commons"
)

const (
	signalChSize  = 64
	setupChSize   = 256
	mediaChSize   = 256
	controlChSize = 512
)

// PipelineResult carries the outcome of a sync pipeline stage back to the caller.
type PipelineResult struct {
	ContextID      string
	ConversationID uint64
	Error          error

	Provider     string
	CallerNumber string
	CallStatus   string
	CallEvent    string
	Extra        map[string]string // provider-specific metadata
}

type callEnvelope struct {
	ctx context.Context
	p   Pipeline
}

// Dispatcher routes channel call lifecycle stages to priority-based goroutines.
//
//	signal  — disconnect, completed, failed
//	setup   — call received, conversation created
//	media   — session connected, initialized, active
//	control — events, metrics
type Dispatcher struct {
	logger commons.Logger

	signalCh  chan callEnvelope
	setupCh   chan callEnvelope
	mediaCh   chan callEnvelope
	controlCh chan callEnvelope

	inboundDispatcher   *channel_telephony.InboundDispatcher
	outboundDispatcher  *channel_telephony.OutboundDispatcher
	conversationService internal_services.AssistantConversationService
	assistantService    internal_services.AssistantService
}

// DispatcherOptions holds dependencies for creating a channel dispatcher.
type DispatcherOptions struct {
	logger              commons.Logger
	inboundDispatcher   *channel_telephony.InboundDispatcher
	outboundDispatcher  *channel_telephony.OutboundDispatcher
	conversationService internal_services.AssistantConversationService
	assistantService    internal_services.AssistantService
}

type FuncOption func(*DispatcherOptions)

func WithLogger(logger commons.Logger) FuncOption {
	return func(options *DispatcherOptions) {
		options.logger = logger
	}
}

func WithInboundDispatcher(dispatcher *channel_telephony.InboundDispatcher) FuncOption {
	return func(options *DispatcherOptions) {
		options.inboundDispatcher = dispatcher
	}
}

func WithOutboundDispatcher(dispatcher *channel_telephony.OutboundDispatcher) FuncOption {
	return func(options *DispatcherOptions) {
		options.outboundDispatcher = dispatcher
	}
}

func WithConversationService(conversationService internal_services.AssistantConversationService) FuncOption {
	return func(options *DispatcherOptions) {
		options.conversationService = conversationService
	}
}

func WithAssistantService(assistantService internal_services.AssistantService) FuncOption {
	return func(options *DispatcherOptions) {
		options.assistantService = assistantService
	}
}

func NewDispatcher(opts ...FuncOption) *Dispatcher {
	var cfg DispatcherOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Dispatcher{
		logger:              cfg.logger,
		inboundDispatcher:   cfg.inboundDispatcher,
		outboundDispatcher:  cfg.outboundDispatcher,
		conversationService: cfg.conversationService,
		signalCh:            make(chan callEnvelope, signalChSize),
		setupCh:             make(chan callEnvelope, setupChSize),
		mediaCh:             make(chan callEnvelope, mediaChSize),
		controlCh:           make(chan callEnvelope, controlChSize),
	}
}

func (d *Dispatcher) Start(ctx context.Context) {
	go d.runDispatcher(ctx, d.signalCh)
	go d.runDispatcher(ctx, d.setupCh)
	go d.runDispatcher(ctx, d.mediaCh)
	go d.runDispatcher(ctx, d.controlCh)
	d.logger.Infow("Channel pipeline dispatcher started")
}

// OnPipeline enqueues a pipeline stage asynchronously (fire-and-forget).
func (d *Dispatcher) OnPipeline(ctx context.Context, stages ...Pipeline) {
	for _, s := range stages {
		d.enqueue(ctx, s)
	}
}

// Run executes a sync pipeline stage inline on the caller's goroutine.
// For CallReceived, SessionConnected, and OutboundRequested the handler runs
// sequentially without channels or goroutines. All other stage types are
// forwarded to OnPipeline (async fire-and-forget) and an empty result is returned.
func (d *Dispatcher) Run(ctx context.Context, stage Pipeline) *PipelineResult {
	switch v := stage.(type) {
	case CallReceivedPipeline:
		return d.runInboundCall(ctx, v)
	case SessionConnectedPipeline:
		return d.runSession(ctx, v)
	case OutboundRequestedPipeline:
		return d.runOutbound(ctx, v)
	default:
		d.OnPipeline(ctx, stage)
		return &PipelineResult{}
	}
}

func (d *Dispatcher) enqueue(ctx context.Context, s Pipeline) {
	e := callEnvelope{ctx: ctx, p: s}
	switch s.(type) {
	case DisconnectRequestedPipeline, CallCompletedPipeline, CallFailedPipeline:
		d.signalCh <- e
	case EventEmittedPipeline, MetricEmittedPipeline:
		d.controlCh <- e
	default:
		d.logger.Warnw("OnPipeline: unrouted type", "type", fmt.Sprintf("%T", s))
		d.controlCh <- e
	}
}

func (d *Dispatcher) runDispatcher(ctx context.Context, ch chan callEnvelope) {
	for {
		select {
		case <-ctx.Done():
			d.drain(ch)
			return
		case e := <-ch:
			d.dispatch(e)
		}
	}
}

func (d *Dispatcher) drain(ch chan callEnvelope) {
	for {
		select {
		case e := <-ch:
			d.dispatch(e)
		default:
			return
		}
	}
}

func (d *Dispatcher) dispatch(e callEnvelope) {
	ctx := e.ctx

	switch v := e.p.(type) {
	case DisconnectRequestedPipeline:
		d.handleDisconnectRequested(ctx, v)
	case CallCompletedPipeline:
		d.handleCallCompleted(ctx, v)
	case CallFailedPipeline:
		d.handleCallFailed(ctx, v)
	case EventEmittedPipeline:
		d.handleEventEmitted(ctx, v)
	case MetricEmittedPipeline:
		d.handleMetricEmitted(ctx, v)
	default:
		d.logger.Warnw("dispatch: unknown pipeline type", "type", fmt.Sprintf("%T", e.p))
	}
}
