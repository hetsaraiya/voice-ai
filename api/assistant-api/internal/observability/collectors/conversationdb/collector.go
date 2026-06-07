// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability_collector_conversationdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
)

var (
	ErrLoggerRequired   = errors.New("conversationdb: logger is required")
	ErrPostgresRequired = errors.New("conversationdb: postgres is required")
	ErrAuthRequired     = errors.New("conversationdb: auth is required")
	ErrScopeRequired    = errors.New("conversationdb: assistant_id and conversation_id are required")
	ErrMessageRequired  = errors.New("conversationdb: conversation_id and message_id are required")
	ErrScopeUnsupported = errors.New("conversationdb: scope is not supported")
)

type Config struct {
	Logger              commons.Logger
	ConversationService internal_services.AssistantConversationService
}

type Collector struct {
	logger  commons.Logger
	service internal_services.AssistantConversationService
}

func New(cfg Config) observability.Collector {
	return &Collector{
		logger:  cfg.Logger,
		service: cfg.ConversationService,
	}
}

func (c *Collector) Key() string {
	return "conversationdb"
}

func (c *Collector) Collect(ctx context.Context, scope observability.Scope, record observability.Record) error {
	switch typed := record.(type) {
	case observability.RecordMetric:
		return c.collectMetrics(ctx, scope, typed)
	case observability.RecordMetadata:
		return c.collectMetadata(ctx, scope, typed)
	default:
		return nil
	}
}

func (c *Collector) Close(context.Context) error {
	return nil
}

func (c *Collector) collectMetrics(ctx context.Context, scope observability.Scope, record observability.RecordMetric) error {
	if !validator.NotEmpty(record.Metrics) {
		return nil
	}
	if err := validateCollector(c); err != nil {
		return err
	}
	auth, err := authFromContext(ctx)
	if err != nil {
		return err
	}

	switch scope := scope.(type) {
	case observability.MessageScope:
		if err := validateMessageScope(scope); err != nil {
			return err
		}
		_, err := c.service.CreateOrUpdateMessageMetrics(
			ctx,
			auth,
			scope.ConversationScopeID(),
			scope.MessageScopeID(),
			record.Metrics,
		)
		return err
	case observability.ConversationScope:
		if err := validateConversationScope(scope); err != nil {
			return err
		}
		_, err := c.service.CreateOrUpdateConversationMetrics(
			ctx,
			auth,
			scope.AssistantScopeID(),
			scope.ConversationScopeID(),
			toServiceMetrics(record.Metrics),
		)
		return err
	case observability.AssistantScope:
		return fmt.Errorf("%w: %s", ErrScopeUnsupported, observability.ScopeAssistant)
	default:
		return fmt.Errorf("%w: %T", ErrScopeUnsupported, scope)
	}
}

func (c *Collector) collectMetadata(ctx context.Context, scope observability.Scope, record observability.RecordMetadata) error {
	if !validator.NotEmpty(record.Metadata) {
		return nil
	}
	if err := validateCollector(c); err != nil {
		return err
	}
	auth, err := authFromContext(ctx)
	if err != nil {
		return err
	}

	switch scope := scope.(type) {
	case observability.MessageScope:
		if err := validateMessageScope(scope); err != nil {
			return err
		}
		_, err := c.service.CreateOrUpdateMessageMetadata(
			ctx,
			auth,
			scope.ConversationScopeID(),
			scope.MessageScopeID(),
			record.Metadata,
		)
		return err
	case observability.ConversationScope:
		if err := validateConversationScope(scope); err != nil {
			return err
		}
		_, err := c.service.CreateOrUpdateConversationMetadata(
			ctx,
			auth,
			scope.AssistantScopeID(),
			scope.ConversationScopeID(),
			toServiceMetadata(record.Metadata),
		)
		return err
	case observability.AssistantScope:
		return fmt.Errorf("%w: %s", ErrScopeUnsupported, observability.ScopeAssistant)
	default:
		return fmt.Errorf("%w: %T", ErrScopeUnsupported, scope)
	}
}

func authFromContext(ctx context.Context) (types.SimplePrinciple, error) {
	auth, ok := types.GetSimplePrincipleGRPC(ctx)
	if !ok || !validator.NonNil(auth) {
		return nil, ErrAuthRequired
	}
	return auth, nil
}

func toServiceMetrics(metrics []*protos.Metric) []*types.Metric {
	converted := make([]*types.Metric, 0, len(metrics))
	for _, metric := range metrics {
		converted = append(converted, &types.Metric{
			Name:        metric.Name,
			Value:       metric.Value,
			Description: metric.Description,
		})
	}
	return converted
}

func toServiceMetadata(metadata []*protos.Metadata) []*types.Metadata {
	converted := make([]*types.Metadata, 0, len(metadata))
	for _, item := range metadata {
		converted = append(converted, types.NewMetadata(item.Key, item.Value))
	}
	return converted
}

func validateCollector(collector *Collector) error {
	if !validator.NonNil(collector) || !validator.NonNil(collector.service) {
		return ErrPostgresRequired
	}
	return nil
}

func validateConversationScope(scope observability.ConversationScope) error {
	if scope.AssistantScopeID() == 0 || scope.ConversationScopeID() == 0 {
		return fmt.Errorf("%w: assistant_id=%d conversation_id=%d", ErrScopeRequired, scope.AssistantScopeID(), scope.ConversationScopeID())
	}
	return nil
}

func validateMessageScope(scope observability.MessageScope) error {
	if scope.ConversationScopeID() == 0 || !validator.NotBlank(scope.MessageScopeID()) {
		return fmt.Errorf("%w: conversation_id=%d message_id=%q", ErrMessageRequired, scope.ConversationScopeID(), scope.MessageScopeID())
	}
	return nil
}
