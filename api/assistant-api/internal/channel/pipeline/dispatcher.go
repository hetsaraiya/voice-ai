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

type Dispatcher struct {
	logger commons.Logger

	inboundDispatcher    *channel_telephony.InboundDispatcher
	outboundDispatcher   *channel_telephony.OutboundDispatcher
	conversationService  internal_services.AssistantConversationService
	assistantService     internal_services.AssistantService
	webhookService       internal_services.AssistantWebhookService
	httpLogService       internal_services.AssistantHTTPLogService
	assistantToolService internal_services.AssistantToolService
}

// DispatcherOptions holds dependencies for creating a channel dispatcher.
type DispatcherOptions struct {
	logger               commons.Logger
	inboundDispatcher    *channel_telephony.InboundDispatcher
	outboundDispatcher   *channel_telephony.OutboundDispatcher
	conversationService  internal_services.AssistantConversationService
	assistantService     internal_services.AssistantService
	webhookService       internal_services.AssistantWebhookService
	httpLogService       internal_services.AssistantHTTPLogService
	assistantToolService internal_services.AssistantToolService
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

func WithWebhookService(webhookService internal_services.AssistantWebhookService) FuncOption {
	return func(options *DispatcherOptions) {
		options.webhookService = webhookService
	}
}

func WithHTTPLogService(httpLogService internal_services.AssistantHTTPLogService) FuncOption {
	return func(options *DispatcherOptions) {
		options.httpLogService = httpLogService
	}
}

func WithAssistantToolService(assistantToolService internal_services.AssistantToolService) FuncOption {
	return func(options *DispatcherOptions) {
		options.assistantToolService = assistantToolService
	}
}

func NewDispatcher(opts ...FuncOption) *Dispatcher {
	var cfg DispatcherOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Dispatcher{
		logger:               cfg.logger,
		inboundDispatcher:    cfg.inboundDispatcher,
		outboundDispatcher:   cfg.outboundDispatcher,
		conversationService:  cfg.conversationService,
		assistantService:     cfg.assistantService,
		webhookService:       cfg.webhookService,
		httpLogService:       cfg.httpLogService,
		assistantToolService: cfg.assistantToolService,
	}
}

// Run executes a sync pipeline stage inline on the caller's goroutine.
func (d *Dispatcher) Run(ctx context.Context, stage Pipeline) *PipelineResult {
	switch v := stage.(type) {
	case CallReceivedPipeline:
		return d.runInboundCall(ctx, v)
	case SessionConnectedPipeline:
		return d.runSession(ctx, v)
	case OutboundRequestedPipeline:
		return d.runOutbound(ctx, v)
	default:
		d.logger.Warnw("Run: unsupported pipeline type", "type", fmt.Sprintf("%T", stage))
		return &PipelineResult{}
	}
}
