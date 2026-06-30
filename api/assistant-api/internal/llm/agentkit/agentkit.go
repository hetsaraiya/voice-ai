// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_llm_agentkit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type agentkitExecutor struct {
	logger           commons.Logger
	ctx              context.Context
	cancel           context.CancelCauseFunc
	connection       *AgentkitConnection
	stateMu          sync.RWMutex
	activeContextID  string
	requestStartedAt time.Time
}

type options struct {
	ctx           context.Context
	logger        commons.Logger
	communication internal_type.Communication
	configuration *protos.ConversationInitialization
}

type Option func(*options)

func WithContext(ctx context.Context) Option {
	return func(options *options) {
		options.ctx = ctx
	}
}

func WithLogger(logger commons.Logger) Option {
	return func(options *options) {
		options.logger = logger
	}
}

func WithCommunication(communication internal_type.Communication) Option {
	return func(options *options) {
		options.communication = communication
	}
}

func WithConfiguration(configuration *protos.ConversationInitialization) Option {
	return func(options *options) {
		options.configuration = configuration
	}
}

func New(opts ...Option) (*agentkitExecutor, error) {
	options := &options{ctx: context.Background()}
	for _, opt := range opts {
		opt(options)
	}
	if options.ctx == nil {
		options.ctx = context.Background()
	}
	if options.communication == nil {
		return nil, ErrAgentkitCommunicationRequired
	}
	if options.configuration == nil {
		return nil, ErrAgentkitConfigurationRequired
	}
	if options.communication.Assistant() == nil {
		return nil, ErrAgentkitAssistantRequired
	}
	if options.communication.Assistant().AssistantProviderAgentkit == nil {
		return nil, ErrAgentkitProviderConfigurationRequired
	}

	start := time.Now()
	executorCtx, cancel := context.WithCancelCause(options.ctx)
	executor := &agentkitExecutor{
		logger:     options.logger,
		ctx:        executorCtx,
		cancel:     cancel,
		connection: NewAgentkitConnection(options.communication.Assistant().AssistantProviderAgentkit),
	}

	if options.communication.Assistant().AssistantProviderAgentkit.TLSVerification != nil &&
		*options.communication.Assistant().AssistantProviderAgentkit.TLSVerification == TLSVerificationSkipVerify {
		executor.logger.Warnf("Using insecure TLS (skipping certificate verification)")
	}

	if err := executor.connection.Connect(); err != nil {
		options.communication.OnPacket(options.ctx, internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: fmt.Sprintf("%s: error while initialization %s", executor.Name(), err.Error()),
				Attributes: observability.Attributes{
					"component":  observability.ComponentLLM.String(),
					"provider":   executor.Name(),
					"options":    observability.AttributeValue(executor.connection.GetOption()),
					"url":        options.communication.Assistant().AssistantProviderAgentkit.Url,
					"error":      err.Error(),
					"error_type": fmt.Sprintf("%T", err),
				},
				OccurredAt: time.Now(),
			},
		})
		executor.Close(options.ctx)
		return nil, fmt.Errorf("%w: %w", ErrAgentkitInitializationConnect, err)
	}

	if err := executor.connection.OpenTalkStream(executor.ctx); err != nil {
		options.communication.OnPacket(options.ctx, internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: fmt.Sprintf("%s: error while initialization %s", executor.Name(), err.Error()),
				Attributes: observability.Attributes{
					"component":  observability.ComponentLLM.String(),
					"provider":   executor.Name(),
					"options":    observability.AttributeValue(executor.connection.GetOption()),
					"url":        options.communication.Assistant().AssistantProviderAgentkit.Url,
					"error":      err.Error(),
					"error_type": fmt.Sprintf("%T", err),
				},
				OccurredAt: time.Now(),
			},
		})
		executor.Close(options.ctx)
		return nil, fmt.Errorf("%w: %w", ErrAgentkitInitializationOpenTalkStream, err)
	}

	utils.Go(executor.ctx, func() {
		executor.Read(executor.ctx, options.communication, executor.connection)
	})

	if err := executor.connection.Send(&protos.TalkInput{
		Request: &protos.TalkInput_Initialization{
			Initialization: &protos.ConversationInitialization{
				AssistantConversationId: options.communication.Conversation().Id,
				Assistant: &protos.AssistantDefinition{
					AssistantId: options.communication.Assistant().AssistantProviderAgentkit.AssistantId,
					Version:     utils.GetVersionString(options.communication.Assistant().AssistantProviderAgentkit.Id),
				},
				Args: options.configuration.GetArgs(), Metadata: options.configuration.GetMetadata(),
				Options: options.configuration.GetOptions(), StreamMode: options.configuration.GetStreamMode(),
				UserIdentity: options.configuration.GetUserIdentity(), Time: timestamppb.Now(),
			},
		},
	}); err != nil {
		options.communication.OnPacket(options.ctx, internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: fmt.Sprintf("%s: error while initialization %s", executor.Name(), err.Error()),
				Attributes: observability.Attributes{
					"component":  observability.ComponentLLM.String(),
					"provider":   executor.Name(),
					"options":    observability.AttributeValue(executor.connection.GetOption()),
					"url":        options.communication.Assistant().AssistantProviderAgentkit.Url,
					"error":      err.Error(),
					"error_type": fmt.Sprintf("%T", err),
				},
				OccurredAt: time.Now(),
			},
		})
		executor.Close(options.ctx)
		return nil, fmt.Errorf("%w: %w", ErrAgentkitInitializationSend, err)
	}

	options.communication.OnPacket(options.ctx,
		internal_type.ObservabilityMetricRecordPacket{
			Scope:  internal_type.ObservabilityRecordScopeConversation,
			Record: observability.NewMetricLLMInitLatencyMs(time.Since(start), observability.Attributes{"provider": executor.Name()}),
		},
		internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelInfo,
				Message: fmt.Sprintf("%s: initialization completed", executor.Name()),
				Attributes: observability.Attributes{
					"component": observability.ComponentLLM.String(),
					"provider":  executor.Name(),
					"url":       options.communication.Assistant().AssistantProviderAgentkit.Url,
					"options":   observability.AttributeValue(executor.connection.GetOption()),
				},
				OccurredAt: time.Now(),
			},
		},
	)
	return executor, nil
}

func (e *agentkitExecutor) Close(ctx context.Context) error {
	e.stateMu.Lock()
	activeConnection := e.connection
	e.stateMu.Unlock()

	if validator.NonNil(e.cancel) {
		e.cancel(context.Canceled)
	}
	if validator.NonNil(activeConnection) {
		if err := e.connection.Close(); err != nil {
			e.logger.Warnf("failed to close agentkit connection: %v", err)
		}
	}
	e.connection = nil
	e.activeContextID = ""
	e.requestStartedAt = time.Time{}
	return nil
}

func (e *agentkitExecutor) Name() string { return "agentkit" }
