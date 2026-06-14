// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_assistant_service

import (
	"context"
	"time"

	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	gorm_models "github.com/rapidaai/pkg/models/gorm"
	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/protos"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type assistantAuthenticationService struct {
	logger   commons.Logger
	postgres connectors.PostgresConnector
}

func NewAssistantAuthenticationService(
	logger commons.Logger,
	postgres connectors.PostgresConnector,
) internal_services.AssistantAuthenticationService {
	return &assistantAuthenticationService{
		logger:   logger,
		postgres: postgres,
	}
}

func (s *assistantAuthenticationService) Get(
	ctx context.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
) (*internal_assistant_entity.AssistantAuthentication, error) {
	start := time.Now()
	db := s.postgres.DB(ctx)

	//
	var out *internal_assistant_entity.AssistantAuthentication
	tx := db.Preload("AssistantAuthenticationOption", "status = ?", type_enums.RECORD_ACTIVE).
		Where("assistant_id = ? AND organization_id = ? AND project_id = ? AND status IN ?",
			assistantId,
			*auth.GetCurrentOrganizationId(),
			*auth.GetCurrentProjectId(),
			[]type_enums.RecordState{
				type_enums.RECORD_ACTIVE,
				type_enums.RECORD_INACTIVE,
			}).
		Last(&out)
	if tx.Error != nil {
		s.logger.Benchmark("AssistantAuthenticationService.Get", time.Since(start))
		return nil, tx.Error
	}
	s.logger.Benchmark("AssistantAuthenticationService.Get", time.Since(start))
	return out, nil
}

func (s *assistantAuthenticationService) Create(
	ctx context.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
	provider string,
	status string,
	failBehavior string,
	timeoutMs uint64,
	options []*protos.Metadata,
) (*internal_assistant_entity.AssistantAuthentication, error) {
	start := time.Now()
	db := s.postgres.DB(ctx)

	if failBehavior == "" {
		failBehavior = "block"
	}
	if timeoutMs == 0 {
		timeoutMs = 5000
	}
	var out *internal_assistant_entity.AssistantAuthentication
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where(
			"id = ? AND organization_id = ? AND project_id = ? AND status = ?",
			assistantId,
			*auth.GetCurrentOrganizationId(),
			*auth.GetCurrentProjectId(),
			type_enums.RECORD_ACTIVE,
		).First(&internal_assistant_entity.Assistant{}).Error; err != nil {
			return err
		}

		if err := s.archiveCurrentConfigs(ctx, tx, auth, assistantId); err != nil {
			return err
		}
		created := &internal_assistant_entity.AssistantAuthentication{
			AssistantId:  assistantId,
			Provider:     internal_assistant_entity.NormalizeAssistantAuthenticationProvider(provider),
			FailBehavior: failBehavior,
			TimeoutMs:    timeoutMs,
			Organizational: gorm_models.Organizational{
				ProjectId:      *auth.GetCurrentProjectId(),
				OrganizationId: *auth.GetCurrentOrganizationId(),
			},
			Mutable: gorm_models.Mutable{
				Status:    type_enums.ToRecordState(status),
				CreatedBy: *auth.GetUserId(),
				UpdatedBy: *auth.GetUserId(),
			},
		}
		if err := tx.Create(created).Error; err != nil {
			return err
		}

		if _, err := s.createOptions(ctx, tx, auth, created.Id, options); err != nil {
			return err
		}

		return tx.Preload("AssistantAuthenticationOption", "status = ?", type_enums.RECORD_ACTIVE).
			Where("id = ?", created.Id).
			First(&out).Error
	})
	if err != nil {
		s.logger.Benchmark("AssistantAuthenticationService.Create", time.Since(start))
		return nil, err
	}

	s.logger.Benchmark("AssistantAuthenticationService.Create", time.Since(start))
	return out, nil
}

func (s *assistantAuthenticationService) Disable(
	ctx context.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
) (*internal_assistant_entity.AssistantAuthentication, error) {
	start := time.Now()
	db := s.postgres.DB(ctx)

	var out *internal_assistant_entity.AssistantAuthentication
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where(
			"id = ? AND organization_id = ? AND project_id = ? AND status = ?",
			assistantId,
			*auth.GetCurrentOrganizationId(),
			*auth.GetCurrentProjectId(),
			type_enums.RECORD_ACTIVE,
		).First(&internal_assistant_entity.Assistant{}).Error; err != nil {
			return err
		}

		var current *internal_assistant_entity.AssistantAuthentication
		_ = tx.WithContext(ctx).
			Preload("AssistantAuthenticationOption", "status = ?", type_enums.RECORD_ACTIVE).
			Where("assistant_id = ? AND organization_id = ? AND project_id = ? AND status IN ?",
				assistantId,
				*auth.GetCurrentOrganizationId(),
				*auth.GetCurrentProjectId(),
				[]type_enums.RecordState{
					type_enums.RECORD_ACTIVE,
					type_enums.RECORD_INACTIVE,
				}).
			Order(clause.OrderByColumn{
				Column: clause.Column{Name: "created_date"},
				Desc:   true,
			}).
			First(&current).Error

		if err := s.archiveCurrentConfigs(ctx, tx, auth, assistantId); err != nil {
			return err
		}

		failBehavior := "block"
		timeoutMs := uint64(5000)
		provider := internal_assistant_entity.AssistantAuthenticationProviderHTTP
		var options []*protos.Metadata
		if current != nil {
			provider = internal_assistant_entity.NormalizeAssistantAuthenticationProvider(current.Provider)
			if current.FailBehavior != "" {
				failBehavior = current.FailBehavior
			}
			if current.TimeoutMs > 0 {
				timeoutMs = current.TimeoutMs
			}
			options = make([]*protos.Metadata, 0, len(current.AssistantAuthenticationOption))
			for _, opt := range current.AssistantAuthenticationOption {
				options = append(options, &protos.Metadata{
					Key:   opt.Key,
					Value: opt.Value,
				})
			}
		}

		created := &internal_assistant_entity.AssistantAuthentication{
			AssistantId:  assistantId,
			Provider:     provider,
			FailBehavior: failBehavior,
			TimeoutMs:    timeoutMs,
			Organizational: gorm_models.Organizational{
				ProjectId:      *auth.GetCurrentProjectId(),
				OrganizationId: *auth.GetCurrentOrganizationId(),
			},
			Mutable: gorm_models.Mutable{
				Status:    type_enums.RECORD_INACTIVE,
				CreatedBy: *auth.GetUserId(),
				UpdatedBy: *auth.GetUserId(),
			},
		}
		if err := tx.Create(created).Error; err != nil {
			return err
		}

		if _, err := s.createOptions(ctx, tx, auth, created.Id, options); err != nil {
			return err
		}

		return tx.Preload("AssistantAuthenticationOption", "status = ?", type_enums.RECORD_ACTIVE).
			Where("id = ?", created.Id).
			First(&out).Error
	})
	if err != nil {
		s.logger.Benchmark("AssistantAuthenticationService.Disable", time.Since(start))
		return nil, err
	}
	s.logger.Benchmark("AssistantAuthenticationService.Disable", time.Since(start))
	return out, nil
}

func (s *assistantAuthenticationService) archiveCurrentConfigs(
	ctx context.Context,
	tx *gorm.DB,
	auth types.SimplePrinciple,
	assistantId uint64,
) error {
	var current []*internal_assistant_entity.AssistantAuthentication
	if err := tx.WithContext(ctx).
		Where("assistant_id = ? AND organization_id = ? AND project_id = ? AND status IN ?",
			assistantId,
			*auth.GetCurrentOrganizationId(),
			*auth.GetCurrentProjectId(),
			[]type_enums.RecordState{
				type_enums.RECORD_ACTIVE,
				type_enums.RECORD_INACTIVE,
			}).
		Find(&current).Error; err != nil {
		return err
	}

	if len(current) == 0 {
		return nil
	}

	ids := make([]uint64, 0, len(current))
	for _, cfg := range current {
		ids = append(ids, cfg.Id)
	}

	if err := tx.WithContext(ctx).
		Model(&internal_assistant_entity.AssistantAuthentication{}).
		Where("id IN ?", ids).
		Updates(&internal_assistant_entity.AssistantAuthentication{
			Mutable: gorm_models.Mutable{
				Status:    type_enums.RECORD_ARCHIEVE,
				UpdatedBy: *auth.GetUserId(),
			},
		}).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).
		Where("assistant_authentication_id IN ? AND status = ?", ids, type_enums.RECORD_ACTIVE).
		Updates(&internal_assistant_entity.AssistantAuthenticationOption{
			Mutable: gorm_models.Mutable{
				Status:    type_enums.RECORD_ARCHIEVE,
				UpdatedBy: *auth.GetUserId(),
			},
		}).
		Error
}

func (s *assistantAuthenticationService) createOptions(
	ctx context.Context,
	tx *gorm.DB,
	auth types.SimplePrinciple,
	authenticationId uint64,
	options []*protos.Metadata,
) ([]*internal_assistant_entity.AssistantAuthenticationOption, error) {
	if len(options) == 0 {
		return []*internal_assistant_entity.AssistantAuthenticationOption{}, nil
	}
	out := make([]*internal_assistant_entity.AssistantAuthenticationOption, 0, len(options))
	for _, opt := range options {
		out = append(out, &internal_assistant_entity.AssistantAuthenticationOption{
			AssistantAuthenticationId: authenticationId,
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

	if err := tx.WithContext(ctx).Create(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
