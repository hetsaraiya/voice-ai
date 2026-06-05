// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability_collector_conversationdb

import (
	"context"
	"errors"
	"testing"

	internal_conversation_entity "github.com/rapidaai/api/assistant-api/internal/entity/conversations"
	internal_message_gorm "github.com/rapidaai/api/assistant-api/internal/entity/messages"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/protos"
)

type conversationServiceStub struct {
	internal_services.AssistantConversationService

	metricAuth           types.SimplePrinciple
	metricAssistantID    uint64
	metricConversationID uint64
	metrics              []*types.Metric

	metadataAuth           types.SimplePrinciple
	metadataAssistantID    uint64
	metadataConversationID uint64
	metadata               []*types.Metadata

	messageMetricAuth           types.SimplePrinciple
	messageMetricConversationID uint64
	messageMetricMessageID      string
	messageMetrics              []*protos.Metric

	messageMetadataAuth           types.SimplePrinciple
	messageMetadataConversationID uint64
	messageMetadataMessageID      string
	messageMetadata               []*protos.Metadata
}

func (s *conversationServiceStub) ApplyConversationMetrics(
	_ context.Context,
	auth types.SimplePrinciple,
	assistantID uint64,
	conversationID uint64,
	metrics []*types.Metric,
) ([]*internal_conversation_entity.AssistantConversationMetric, error) {
	s.metricAuth = auth
	s.metricAssistantID = assistantID
	s.metricConversationID = conversationID
	s.metrics = metrics
	return nil, nil
}

func (s *conversationServiceStub) ApplyConversationMetadata(
	_ context.Context,
	auth types.SimplePrinciple,
	assistantID uint64,
	conversationID uint64,
	metadata []*types.Metadata,
) ([]*internal_conversation_entity.AssistantConversationMetadata, error) {
	s.metadataAuth = auth
	s.metadataAssistantID = assistantID
	s.metadataConversationID = conversationID
	s.metadata = metadata
	return nil, nil
}

func (s *conversationServiceStub) ApplyMessageMetrics(
	_ context.Context,
	auth types.SimplePrinciple,
	conversationID uint64,
	messageID string,
	metrics []*protos.Metric,
) ([]*internal_message_gorm.AssistantConversationMessageMetric, error) {
	s.messageMetricAuth = auth
	s.messageMetricConversationID = conversationID
	s.messageMetricMessageID = messageID
	s.messageMetrics = metrics
	return nil, nil
}

func (s *conversationServiceStub) ApplyMessageMetadata(
	_ context.Context,
	auth types.SimplePrinciple,
	conversationID uint64,
	messageID string,
	metadata []*protos.Metadata,
) ([]*internal_message_gorm.AssistantConversationMessageMetadata, error) {
	s.messageMetadataAuth = auth
	s.messageMetadataConversationID = conversationID
	s.messageMetadataMessageID = messageID
	s.messageMetadata = metadata
	return nil, nil
}

func TestCollectMetric_RequiresValidConversationScope(t *testing.T) {
	collector := New(Config{ConversationService: &conversationServiceStub{}})

	err := collector.Collect(context.Background(), observability.RecordMetric{
		CommonRecord: observability.CommonRecord{
			Scope: observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: 10},
			},
		},
		Metrics: []*protos.Metric{{Name: observability.MetricConversationDuration, Value: "1000"}},
	})
	if !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("expected scope error, got %v", err)
	}
}

func TestCollectMetric_EmptyMetricsAreNoop(t *testing.T) {
	collector := New(Config{ConversationService: &conversationServiceStub{}})
	if err := collector.Collect(context.Background(), observability.RecordMetric{}); err != nil {
		t.Fatalf("empty metrics should be noop, got %v", err)
	}
}

func TestCollectMetric_ForwardsConversationScopedRecords(t *testing.T) {
	service := &conversationServiceStub{}
	collector := New(Config{ConversationService: service})
	organizationID := uint64(1)
	projectID := uint64(2)
	userID := uint64(99)
	auth := &types.ServiceScope{
		UserId:         &userID,
		OrganizationId: &organizationID,
		ProjectId:      &projectID,
	}

	err := collector.Collect(context.Background(), observability.RecordMetric{
		CommonRecord: observability.CommonRecord{
			Auth: auth,
			Scope: observability.ConversationScope{
				AssistantScope: observability.AssistantScope{AssistantID: 10},
				ConversationID: 20,
			},
		},
		Metrics: []*protos.Metric{{
			Name:        observability.MetricConversationDuration,
			Value:       "1000",
			Description: "duration",
		}},
	})
	if err != nil {
		t.Fatalf("CollectMetric returned error: %v", err)
	}
	if service.metricAuth != auth || service.metricAssistantID != 10 || service.metricConversationID != 20 {
		t.Fatalf("unexpected call: auth=%v assistant=%d conversation=%d", service.metricAuth, service.metricAssistantID, service.metricConversationID)
	}
	if len(service.metrics) != 1 || service.metrics[0].Name != observability.MetricConversationDuration {
		t.Fatalf("unexpected metrics: %+v", service.metrics)
	}
}

func TestCollectMetadata_ForwardsMessageScopedRecords(t *testing.T) {
	service := &conversationServiceStub{}
	collector := New(Config{ConversationService: service})
	userID := uint64(99)
	auth := &types.ServiceScope{UserId: &userID}

	err := collector.Collect(context.Background(), observability.RecordMetadata{
		CommonRecord: observability.CommonRecord{
			Auth: auth,
			Scope: observability.MessageScope{
				ConversationScope: observability.ConversationScope{
					AssistantScope: observability.AssistantScope{AssistantID: 10},
					ConversationID: 20,
				},
				MessageID: "user-ctx-1",
				Role:      observability.MessageRoleUser,
			},
		},
		Metadata: []*protos.Metadata{{
			Key:   observability.MetadataLanguage,
			Value: "en",
		}},
	})
	if err != nil {
		t.Fatalf("CollectMetadata returned error: %v", err)
	}
	if service.messageMetadataAuth != auth || service.messageMetadataConversationID != 20 || service.messageMetadataMessageID != "user-ctx-1" {
		t.Fatalf("unexpected message metadata call: auth=%v conversation=%d message=%s", service.messageMetadataAuth, service.messageMetadataConversationID, service.messageMetadataMessageID)
	}
}

func TestCollectMetadata_AssistantScopeUnsupported(t *testing.T) {
	collector := New(Config{ConversationService: &conversationServiceStub{}})
	err := collector.Collect(context.Background(), observability.RecordMetadata{
		CommonRecord: observability.CommonRecord{
			Scope: observability.AssistantScope{AssistantID: 10},
		},
		Metadata: []*protos.Metadata{{Key: observability.MetadataLanguage, Value: "en"}},
	})
	if !errors.Is(err, ErrScopeUnsupported) {
		t.Fatalf("expected unsupported scope error, got %v", err)
	}
}

func TestConversionToServiceTypes(t *testing.T) {
	metrics := toServiceMetrics([]*protos.Metric{{
		Name:        observability.MetricConversationDuration,
		Value:       "1000",
		Description: "duration",
	}})
	if len(metrics) != 1 || metrics[0].Name != observability.MetricConversationDuration || metrics[0].Description != "duration" {
		t.Fatalf("unexpected service metrics: %+v", metrics)
	}

	metadata := toServiceMetadata([]*protos.Metadata{{
		Key:   observability.MetadataDisconnectReason,
		Value: "normal_clearing",
	}})
	if len(metadata) != 1 || metadata[0].Key != observability.MetadataDisconnectReason || metadata[0].Value != "normal_clearing" {
		t.Fatalf("unexpected service metadata: %+v", metadata)
	}
}
