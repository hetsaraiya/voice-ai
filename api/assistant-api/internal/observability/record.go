// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
)

type ScopeType string

const (
	ScopeProject      ScopeType = "project"
	ScopeAssistant    ScopeType = "assistant"
	ScopeConversation ScopeType = "conversation"
	ScopeMessage      ScopeType = "message"
)

type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

type Level string

const (
	LevelInfo     Level = "info"
	LevelError    Level = "error"
	LevelDebug    Level = "debug"
	LevelCritical Level = "critical"
)

type Attributes map[string]string

func (a Attributes) Clone() Attributes {
	if len(a) == 0 {
		return nil
	}
	cloned := make(Attributes, len(a))
	for key, value := range a {
		cloned[key] = value
	}
	return cloned
}

type Context struct {
	TraceID string
}

type GlobalScope struct {
	OrganizationID uint64
	ProjectID      uint64
}

type Scope interface {
	ScopeType() ScopeType
	GlobalScopeValue() GlobalScope
	WithGlobal(global GlobalScope) Scope
}

type ProjectScope struct {
	GlobalScope
}

func (ProjectScope) ScopeType() ScopeType {
	return ScopeProject
}

func (scope ProjectScope) GlobalScopeValue() GlobalScope {
	return scope.GlobalScope
}

func (scope ProjectScope) ContextID() string {
	return strconv.FormatUint(scope.ProjectID, 10)
}

func (scope ProjectScope) WithGlobal(global GlobalScope) Scope {
	scope.GlobalScope = global
	return scope
}

type AssistantScope struct {
	GlobalScope
	AssistantID uint64
}

func (AssistantScope) ScopeType() ScopeType {
	return ScopeAssistant
}

func (scope AssistantScope) GlobalScopeValue() GlobalScope {
	return scope.GlobalScope
}

func (scope AssistantScope) AssistantScopeID() uint64 {
	return scope.AssistantID
}

func (AssistantScope) ConversationScopeID() uint64 {
	return 0
}

func (AssistantScope) MessageScopeID() string {
	return ""
}

func (AssistantScope) MessageScopeRole() MessageRole {
	return ""
}

func (scope AssistantScope) ContextID() string {
	return strconv.FormatUint(scope.AssistantID, 10)
}

func (scope AssistantScope) WithGlobal(global GlobalScope) Scope {
	scope.GlobalScope = global
	return scope
}

type ConversationScope struct {
	AssistantScope
	ConversationID uint64
}

func (ConversationScope) ScopeType() ScopeType {
	return ScopeConversation
}

func (scope ConversationScope) GlobalScopeValue() GlobalScope {
	return scope.GlobalScope
}

func (scope ConversationScope) AssistantScopeID() uint64 {
	return scope.AssistantScope.AssistantID
}

func (scope ConversationScope) ConversationScopeID() uint64 {
	return scope.ConversationID
}

func (ConversationScope) MessageScopeID() string {
	return ""
}

func (ConversationScope) MessageScopeRole() MessageRole {
	return ""
}

func (scope ConversationScope) ContextID() string {
	return strconv.FormatUint(scope.ConversationID, 10)
}

func (scope ConversationScope) WithGlobal(global GlobalScope) Scope {
	scope.GlobalScope = global
	return scope
}

type MessageScope struct {
	ConversationScope
	MessageID string
	Role      MessageRole
}

func (MessageScope) ScopeType() ScopeType {
	return ScopeMessage
}

func (scope MessageScope) GlobalScopeValue() GlobalScope {
	return scope.GlobalScope
}

func (scope MessageScope) AssistantScopeID() uint64 {
	return scope.ConversationScope.AssistantScope.AssistantID
}

func (scope MessageScope) ConversationScopeID() uint64 {
	return scope.ConversationScope.ConversationID
}

func (scope MessageScope) MessageScopeID() string {
	return fmt.Sprintf("%s-%s", scope.MessageScopeRole(), scope.MessageID)
}

func (scope MessageScope) MessageScopeRole() MessageRole {
	return scope.Role
}

func (scope MessageScope) ContextID() string {
	return scope.MessageID
}

func (scope MessageScope) WithGlobal(global GlobalScope) Scope {
	scope.GlobalScope = global
	return scope
}

func ValidateScope(scope Scope) error {
	if scope == nil {
		return errors.New("observability: scope is required")
	}
	switch typed := scope.(type) {
	case ProjectScope:
		if typed.GlobalScopeValue().ProjectID == 0 {
			return errors.New("observability: project_id is required")
		}
		return nil
	case MessageScope:
		if typed.AssistantScopeID() == 0 {
			return errors.New("observability: assistant_id is required")
		}
		if typed.ConversationScopeID() == 0 {
			return errors.New("observability: conversation_id is required")
		}
		if !validator.NotBlank(typed.MessageScopeID()) {
			return errors.New("observability: message_id is required")
		}
		switch typed.MessageScopeRole() {
		case MessageRoleUser, MessageRoleAssistant:
			return nil
		default:
			return fmt.Errorf("observability: invalid message role %q", typed.MessageScopeRole())
		}
	case ConversationScope:
		if typed.AssistantScopeID() == 0 {
			return errors.New("observability: assistant_id is required")
		}
		if typed.ConversationScopeID() == 0 {
			return errors.New("observability: conversation_id is required")
		}
		return nil
	case AssistantScope:
		if typed.AssistantScopeID() == 0 {
			return errors.New("observability: assistant_id is required")
		}
		return nil
	default:
		return fmt.Errorf("observability: unsupported scope %T", scope)
	}
}

type Record interface {
	isRecord()
}

type RecordLog struct {
	ID         string
	Message    string
	Level      Level
	Attributes Attributes
	OccurredAt time.Time
}

func (RecordLog) isRecord() {}

type RecordEvent struct {
	ID         string
	Component  ComponentName
	Event      EventName
	Attributes Attributes
	OccurredAt time.Time
}

func NewConversationEventRecord(event EventName, attr Attributes) RecordEvent {
	return RecordEvent{
		Event:      event,
		Attributes: attr,
	}
}

func NewMessageEventRecord(messageID string, role MessageRole, event EventName, attr Attributes) RecordEvent {
	return RecordEvent{
		Event:      event,
		Attributes: attr,
	}
}

func NewMessageRecord(ctxID string, component ComponentName, event EventName, role MessageRole, attr Attributes) RecordEvent {
	rec := NewMessageEventRecord(ctxID, role, event, attr)
	rec.Component = component
	return rec
}

func (RecordEvent) isRecord() {}

type RecordMetric struct {
	ID         string
	Metrics    []*protos.Metric
	Attributes Attributes
	OccurredAt time.Time
}

func NewConversationMetricRecord(metrics []*protos.Metric) RecordMetric {
	return RecordMetric{
		Metrics: metrics,
	}
}

func NewMessageMetricRecord(messageID string, role MessageRole, metrics []*protos.Metric) RecordMetric {
	return RecordMetric{
		Metrics: metrics,
	}
}

func (RecordMetric) isRecord() {}

type RecordMetadata struct {
	ID         string
	Metadata   []*protos.Metadata
	OccurredAt time.Time
}

func NewConversationMetadataRecord(metadata []*protos.Metadata) RecordMetadata {
	return RecordMetadata{
		Metadata: metadata,
	}
}

func NewMessageMetadataRecord(messageID string, role MessageRole, metadata []*protos.Metadata) RecordMetadata {
	return RecordMetadata{
		Metadata: metadata,
	}
}

func (RecordMetadata) isRecord() {}

type RecordUsage struct {
	ID         string
	Component  ComponentName
	Provider   string
	Duration   time.Duration
	Attributes Attributes
	OccurredAt time.Time
}

func (RecordUsage) isRecord() {}

type RecordWebhook struct {
	ID         string
	Event      EventName
	Payload    map[string]interface{}
	OccurredAt time.Time
}

func (RecordWebhook) isRecord() {}
