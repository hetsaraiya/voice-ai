// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_callcontext

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	"github.com/rapidaai/pkg/validator"
)

// Store provides operations to save and retrieve call contexts from the SQL backend.
//
// Call contexts bridge the HTTP call-setup request and the media connection.
// Save creates PENDING; Claim atomically marks the context consumed by the media
// path; call status updates can mark unclaimed setup attempts FAILED.
type Store interface {
	Save(ctx context.Context, cc *CallContext) (string, error)
	Get(ctx context.Context, contextID string) (*CallContext, error)
	GetByChannelUUID(ctx context.Context, provider string, assistantID uint64, channelUUID string) (*CallContext, error)
	Claim(ctx context.Context, contextID string) (*CallContext, error)
	UpdateField(ctx context.Context, contextID, field, value string) error
	UpdateCallStatus(ctx context.Context, contextID string, status CallStatusUpdate) error
}

type CallStatusUpdate struct {
	ExpectedCallStatus string
	CallStatus         string
	CallError          string
	FailureClass       string
	FailureReason      string
	DisconnectReason   string
	Retryable          bool
	ProviderStatusCode int
}

type sqlStore struct {
	sql    connectors.SQLConnector
	logger commons.Logger
}

func NewStore(sql connectors.SQLConnector, logger commons.Logger) Store {
	return &sqlStore{
		sql:    sql,
		logger: logger,
	}
}

func (s *sqlStore) Save(ctx context.Context, cc *CallContext) (string, error) {
	if cc.ContextID == "" {
		cc.ContextID = uuid.New().String()
	}
	cc.Status = StatusPending

	db := s.sql.DB(ctx)
	if err := db.Create(cc).Error; err != nil {
		return "", fmt.Errorf("failed to save call context %s: %w", cc.ContextID, err)
	}

	s.logger.Infof("saved call context: contextId=%s, assistant=%d, conversation=%d, direction=%s",
		cc.ContextID, cc.AssistantID, cc.ConversationID, cc.Direction)

	return cc.ContextID, nil
}

func (s *sqlStore) Get(ctx context.Context, contextID string) (*CallContext, error) {
	db := s.sql.DB(ctx)
	var cc CallContext
	if err := db.Where("context_id = ?", contextID).First(&cc).Error; err != nil {
		return nil, fmt.Errorf("call context not found: %s: %w", contextID, err)
	}
	return &cc, nil
}

func (s *sqlStore) GetByChannelUUID(ctx context.Context, provider string, assistantID uint64, channelUUID string) (*CallContext, error) {
	if !validator.NotBlank(provider) {
		return nil, fmt.Errorf("provider is required to get call context by channel uuid")
	}
	if !validator.AllNonZero(assistantID) {
		return nil, fmt.Errorf("assistant id is required to get call context by channel uuid")
	}
	if !validator.NotBlank(channelUUID) {
		return nil, fmt.Errorf("channel uuid is required to get call context")
	}

	db := s.sql.DB(ctx)
	var cc CallContext
	if err := db.
		Where("provider = ? AND assistant_id = ? AND channel_uuid = ?", provider, assistantID, channelUUID).
		Order("created_date DESC").
		First(&cc).Error; err != nil {
		return nil, fmt.Errorf("call context not found for provider=%s assistant=%d channel_uuid=%s: %w", provider, assistantID, channelUUID, err)
	}
	return &cc, nil
}

// Claim atomically transitions PENDING → CLAIMED in a single query.
func (s *sqlStore) Claim(ctx context.Context, contextID string) (*CallContext, error) {
	db := s.sql.DB(ctx)

	var cc CallContext
	result := db.Raw(`
		UPDATE call_contexts
		SET status = ?, updated_date = ?
		WHERE context_id = ? AND status = ?
		RETURNING *`,
		StatusClaimed, time.Now(), contextID, StatusPending,
	).Scan(&cc)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to claim call context %s: %w", contextID, result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("call context %s not found or already claimed", contextID)
	}

	s.logger.Debugf("claimed call context: contextId=%s, assistant=%d, conversation=%d",
		cc.ContextID, cc.AssistantID, cc.ConversationID)

	return &cc, nil
}

func (s *sqlStore) UpdateField(ctx context.Context, contextID, field, value string) error {
	db := s.sql.DB(ctx)

	allowed := map[string]bool{
		"channel_uuid": true,
		"status":       true,
		"provider":     true,
	}
	if !allowed[field] {
		return fmt.Errorf("field %q is not updatable on call context", field)
	}

	query := db.Model(&CallContext{}).Where("context_id = ?", contextID)
	if field == "status" && value == StatusClaimed {
		query = query.Where("status NOT IN ?", []string{StatusFailed, StatusCompleted, StatusCancelled})
	}
	result := query.Update(field, value)

	if result.Error != nil {
		return fmt.Errorf("failed to update field %s on call context %s: %w", field, contextID, result.Error)
	}

	s.logger.Debugf("updated call context field: contextId=%s, %s=%s", contextID, field, value)
	return nil
}

func (s *sqlStore) UpdateCallStatus(ctx context.Context, contextID string, status CallStatusUpdate) error {
	db := s.sql.DB(ctx)
	updates := map[string]interface{}{
		"call_status":          status.CallStatus,
		"call_error":           status.CallError,
		"failure_class":        status.FailureClass,
		"failure_reason":       status.FailureReason,
		"disconnect_reason":    status.DisconnectReason,
		"retryable":            status.Retryable,
		"provider_status_code": status.ProviderStatusCode,
		"updated_date":         time.Now(),
	}
	if status.CallStatus == CallStatusFailed || status.CallStatus == CallStatusCancelled {
		updates["status"] = StatusFailed
	}
	if status.CallStatus == CallStatusCompleted {
		updates["status"] = StatusCompleted
	}

	query := db.Model(&CallContext{}).Where("context_id = ?", contextID)
	if status.ExpectedCallStatus != "" {
		query = query.Where("call_status = ?", status.ExpectedCallStatus)
	}
	result := query.Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update call status on call context %s: %w", contextID, result.Error)
	}

	s.logger.Debugf("updated call status: contextId=%s, call_status=%s, failure_class=%s", contextID, status.CallStatus, status.FailureClass)
	return nil
}
