// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_assistant_service

import (
	"context"
	"errors"
	"fmt"
	"time"

	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	gorm_models "github.com/rapidaai/pkg/models/gorm"
	gorm_types "github.com/rapidaai/pkg/models/gorm/types"
	"github.com/rapidaai/pkg/storages"
	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/protos"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type assistantWebhookService struct {
	logger   commons.Logger
	postgres connectors.PostgresConnector
	storage  storages.Storage
}

func NewAssistantWebhookService(
	logger commons.Logger,
	postgres connectors.PostgresConnector,
	storage storages.Storage,
) internal_services.AssistantWebhookService {
	return &assistantWebhookService{
		logger:   logger,
		postgres: postgres,
		storage:  storage,
	}
}

func (s *assistantWebhookService) Get(
	ctx context.Context,
	auth types.SimplePrinciple,
	webhookId uint64,
	assistantId uint64,
) (*internal_assistant_entity.AssistantWebhook, error) {
	start := time.Now()
	db := s.postgres.DB(ctx)

	var webhook *internal_assistant_entity.AssistantWebhook
	tx := db.Preload("AssistantWebhookOption", "status = ?", type_enums.RECORD_ACTIVE).
		Where("id = ? AND assistant_id = ?", webhookId, assistantId).
		Where("organization_id = ? AND project_id = ?", *auth.GetCurrentOrganizationId(), *auth.GetCurrentProjectId()).
		First(&webhook)
	if tx.Error != nil {
		s.logger.Benchmark("WebhookService.Get", time.Since(start))
		s.logger.Errorf("not able to find any webhook %v", tx.Error)
		return nil, tx.Error
	}

	s.logger.Benchmark("WebhookService.Get", time.Since(start))
	return webhook, nil
}

func (s *assistantWebhookService) Create(
	ctx context.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
	provider string,
	assistantEvents []string,
	options []*protos.Metadata,
	executionPriority uint32,
	description *string,
) (*internal_assistant_entity.AssistantWebhook, error) {
	start := time.Now()
	db := s.postgres.DB(ctx)

	desc := ""
	if description != nil {
		desc = *description
	}
	webhookProvider, err := internal_assistant_entity.NewAssistantWebhookProvider(provider)
	if err != nil {
		s.logger.Benchmark("WebhookService.Create", time.Since(start))
		s.logger.Errorf("error while creating webhook %v", err)
		return nil, err
	}

	webhook := &internal_assistant_entity.AssistantWebhook{
		AssistantId:       assistantId,
		Provider:          webhookProvider,
		Description:       desc,
		ExecutionPriority: executionPriority,
		AssistantEvents:   gorm_types.StringArray(assistantEvents),
		Organizational: gorm_models.Organizational{
			ProjectId:      *auth.GetCurrentProjectId(),
			OrganizationId: *auth.GetCurrentOrganizationId(),
		},
		Mutable: gorm_models.Mutable{
			CreatedBy: *auth.GetUserId(),
			UpdatedBy: *auth.GetUserId(),
			Status:    type_enums.RECORD_ACTIVE,
		},
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&webhook).Error; err != nil {
			return err
		}
		if _, err := s.createOptions(ctx, tx, auth, webhook.Id, options); err != nil {
			return err
		}
		return tx.Preload("AssistantWebhookOption", "status = ?", type_enums.RECORD_ACTIVE).
			Where("id = ?", webhook.Id).
			First(&webhook).Error
	})
	if err != nil {
		s.logger.Benchmark("WebhookService.Create", time.Since(start))
		s.logger.Errorf("error while creating webhook %v", err)
		return nil, err
	}

	s.logger.Benchmark("WebhookService.Create", time.Since(start))
	return webhook, nil
}

func (s *assistantWebhookService) Update(
	ctx context.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
	webhookId uint64,
	provider string,
	assistantEvents []string,
	options []*protos.Metadata,
	executionPriority uint32,
	description *string,
) (*internal_assistant_entity.AssistantWebhook, error) {
	start := time.Now()
	db := s.postgres.DB(ctx)

	desc := ""
	if description != nil {
		desc = *description
	}
	webhookProvider, err := internal_assistant_entity.NewAssistantWebhookProvider(provider)
	if err != nil {
		s.logger.Benchmark("WebhookService.Update", time.Since(start))
		s.logger.Errorf("error while updating webhook %v", err)
		return nil, err
	}

	patch := &internal_assistant_entity.AssistantWebhook{
		Provider:          webhookProvider,
		Description:       desc,
		ExecutionPriority: executionPriority,
		AssistantEvents:   gorm_types.StringArray(assistantEvents),
		Mutable: gorm_models.Mutable{
			UpdatedBy: *auth.GetUserId(),
		},
	}

	var out *internal_assistant_entity.AssistantWebhook
	err = db.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&internal_assistant_entity.AssistantWebhook{}).
			Where("id = ? AND assistant_id = ? AND organization_id = ? AND project_id = ? AND status = ?",
				webhookId,
				assistantId,
				*auth.GetCurrentOrganizationId(),
				*auth.GetCurrentProjectId(),
				type_enums.RECORD_ACTIVE,
			).
			Updates(patch)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected == 0 {
			return errors.New("assistant webhook not found")
		}
		if err := s.archiveOptions(ctx, tx, auth, webhookId); err != nil {
			return err
		}
		if _, err := s.createOptions(ctx, tx, auth, webhookId, options); err != nil {
			return err
		}
		return tx.Preload("AssistantWebhookOption", "status = ?", type_enums.RECORD_ACTIVE).
			Where("id = ? AND assistant_id = ? AND organization_id = ? AND project_id = ?",
				webhookId,
				assistantId,
				*auth.GetCurrentOrganizationId(),
				*auth.GetCurrentProjectId(),
			).
			First(&out).Error
	})
	if err != nil {
		s.logger.Benchmark("WebhookService.Update", time.Since(start))
		s.logger.Errorf("error while updating webhook %v", err)
		return nil, err
	}

	s.logger.Benchmark("WebhookService.Update", time.Since(start))
	return out, nil
}

func (s *assistantWebhookService) Delete(
	ctx context.Context,
	auth types.SimplePrinciple,
	webhookId uint64,
	assistantId uint64,
) (*internal_assistant_entity.AssistantWebhook, error) {
	start := time.Now()
	db := s.postgres.DB(ctx)

	patch := &internal_assistant_entity.AssistantWebhook{
		Mutable: gorm_models.Mutable{
			UpdatedBy: *auth.GetUserId(),
			Status:    type_enums.RECORD_ARCHIEVE,
		},
	}

	var out *internal_assistant_entity.AssistantWebhook
	err := db.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&internal_assistant_entity.AssistantWebhook{}).
			Where("id = ? AND assistant_id = ? AND organization_id = ? AND project_id = ? AND status = ?",
				webhookId,
				assistantId,
				*auth.GetCurrentOrganizationId(),
				*auth.GetCurrentProjectId(),
				type_enums.RECORD_ACTIVE,
			).
			Updates(patch)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected == 0 {
			return errors.New("assistant webhook not found")
		}
		if err := s.archiveOptions(ctx, tx, auth, webhookId); err != nil {
			return err
		}
		return tx.Where("id = ? AND assistant_id = ? AND organization_id = ? AND project_id = ?",
			webhookId,
			assistantId,
			*auth.GetCurrentOrganizationId(),
			*auth.GetCurrentProjectId(),
		).First(&out).Error
	})
	if err != nil {
		s.logger.Benchmark("WebhookService.Delete", time.Since(start))
		s.logger.Errorf("error while deleting webhook %v", err)
		return nil, err
	}

	s.logger.Benchmark("WebhookService.Delete", time.Since(start))
	return out, nil
}

func (s *assistantWebhookService) GetAll(
	ctx context.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
	criterias []*protos.Criteria,
	paginate *protos.Paginate,
) (int64, []*internal_assistant_entity.AssistantWebhook, error) {
	start := time.Now()
	db := s.postgres.DB(ctx)

	var (
		webhooks []*internal_assistant_entity.AssistantWebhook
		cnt      int64
	)

	qry := db.Model(internal_assistant_entity.AssistantWebhook{}).
		Preload("AssistantWebhookOption", "status = ?", type_enums.RECORD_ACTIVE)
	qry = qry.Where(
		"assistant_id = ? AND organization_id = ? AND project_id = ? AND status = ?",
		assistantId,
		*auth.GetCurrentOrganizationId(),
		*auth.GetCurrentProjectId(),
		type_enums.RECORD_ACTIVE,
	)
	for _, ct := range criterias {
		qry = qry.Where(fmt.Sprintf("%s %s ?", ct.GetKey(), ct.GetLogic()), ct.GetValue())
	}

	tx := qry.Scopes(
		gorm_models.Paginate(
			gorm_models.NewPaginated(
				int(paginate.GetPage()),
				int(paginate.GetPageSize()),
				&cnt,
				qry,
			),
		),
	).Order(clause.OrderByColumn{
		Column: clause.Column{Name: "created_date"},
		Desc:   true,
	}).Find(&webhooks)
	if tx.Error != nil {
		s.logger.Errorf("not able to find any webhooks %v", tx.Error)
		return cnt, nil, tx.Error
	}

	s.logger.Benchmark("WebhookService.GetAll", time.Since(start))
	return cnt, webhooks, nil
}

func (s *assistantWebhookService) archiveOptions(
	ctx context.Context,
	tx *gorm.DB,
	auth types.SimplePrinciple,
	webhookId uint64,
) error {
	patch := &internal_assistant_entity.AssistantWebhookOption{
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_ARCHIEVE,
			UpdatedBy: *auth.GetUserId(),
		},
	}
	return tx.WithContext(ctx).
		Where("assistant_webhook_id = ? AND status = ?", webhookId, type_enums.RECORD_ACTIVE).
		Updates(patch).Error
}

func (s *assistantWebhookService) createOptions(
	ctx context.Context,
	tx *gorm.DB,
	auth types.SimplePrinciple,
	webhookId uint64,
	options []*protos.Metadata,
) ([]*internal_assistant_entity.AssistantWebhookOption, error) {
	if len(options) == 0 {
		return []*internal_assistant_entity.AssistantWebhookOption{}, nil
	}
	out := make([]*internal_assistant_entity.AssistantWebhookOption, 0, len(options))
	for _, opt := range options {
		out = append(out, &internal_assistant_entity.AssistantWebhookOption{
			AssistantWebhookId: webhookId,
			Metadata: gorm_models.Metadata{
				Key:   opt.GetKey(),
				Value: opt.GetValue(),
			},
			Mutable: gorm_models.Mutable{
				Status:    type_enums.RECORD_ACTIVE,
				CreatedBy: *auth.GetUserId(),
				UpdatedBy: *auth.GetUserId(),
			},
		})
	}
	if len(out) == 0 {
		return []*internal_assistant_entity.AssistantWebhookOption{}, nil
	}

	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "key"},
			{Name: "assistant_webhook_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"value",
			"status",
			"updated_by",
			"updated_date",
		}),
	}).Create(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
