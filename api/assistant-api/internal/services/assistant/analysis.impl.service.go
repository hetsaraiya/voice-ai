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
	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/protos"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type assistantAnalysisService struct {
	logger   commons.Logger
	postgres connectors.PostgresConnector
}

func NewAssistantAnalysisService(logger commons.Logger, postgres connectors.PostgresConnector) internal_services.AssistantAnalysisService {
	return &assistantAnalysisService{
		logger:   logger,
		postgres: postgres,
	}
}

// Get implements internal_services.AssistantAnalysisService.
func (eService *assistantAnalysisService) Get(ctx context.Context, auth types.SimplePrinciple, analysisId, assistantId uint64) (*internal_assistant_entity.AssistantAnalysis, error) {
	start := time.Now()
	db := eService.postgres.DB(ctx)
	var analysis *internal_assistant_entity.AssistantAnalysis
	tx := db.Preload("AssistantAnalysisOption", "status = ?", type_enums.RECORD_ACTIVE).
		Where("id = ? AND assistant_id = ?", analysisId, assistantId).
		Where("organization_id = ? AND project_id = ?", *auth.GetCurrentOrganizationId(), *auth.GetCurrentProjectId()).
		First(&analysis)
	if tx.Error != nil {
		eService.logger.Benchmark("AnalysisService.Get", time.Since(start))
		eService.logger.Errorf("not able to find any analysis %v", tx.Error)
		return nil, tx.Error
	}
	eService.logger.Benchmark("AnalysisService.Get", time.Since(start))
	return analysis, nil
}

func (eService *assistantAnalysisService) Create(ctx context.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
	provider string,
	name string,
	options []*protos.Metadata,
	executionPriority uint32,
	description *string,
) (*internal_assistant_entity.AssistantAnalysis, error) {
	start := time.Now()
	db := eService.postgres.DB(ctx)
	desc := ""
	if description != nil {
		desc = *description
	}
	provider = internal_assistant_entity.NormalizeAssistantAnalysisProvider(provider)
	analysis := &internal_assistant_entity.AssistantAnalysis{
		AssistantId:       assistantId,
		Provider:          provider,
		Description:       desc,
		Name:              name,
		ExecutionPriority: executionPriority,
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
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&analysis).Error; err != nil {
			return err
		}
		if _, err := eService.createOptions(ctx, tx, auth, analysis.Id, options); err != nil {
			return err
		}
		return tx.Preload("AssistantAnalysisOption", "status = ?", type_enums.RECORD_ACTIVE).
			Where("id = ?", analysis.Id).
			First(&analysis).Error
	})
	if err != nil {
		eService.logger.Benchmark("assistantAnalysisService.Create", time.Since(start))
		eService.logger.Errorf("error while creating analysis %v", err)
		return nil, err
	}
	eService.logger.Benchmark("assistantAnalysisService.Create", time.Since(start))
	return analysis, nil
}

func (eService *assistantAnalysisService) Update(ctx context.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
	analysisId uint64,
	provider string,
	name string,
	options []*protos.Metadata,
	executionPriority uint32,
	description *string,
) (*internal_assistant_entity.AssistantAnalysis, error) {
	start := time.Now()
	db := eService.postgres.DB(ctx)
	desc := ""
	if description != nil {
		desc = *description
	}
	provider = internal_assistant_entity.NormalizeAssistantAnalysisProvider(provider)
	patch := &internal_assistant_entity.AssistantAnalysis{
		Provider:          provider,
		Description:       desc,
		Name:              name,
		ExecutionPriority: executionPriority,
		Mutable: gorm_models.Mutable{
			UpdatedBy: *auth.GetUserId(),
		},
	}
	var out *internal_assistant_entity.AssistantAnalysis
	err := db.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&internal_assistant_entity.AssistantAnalysis{}).
			Where("id = ? AND assistant_id = ? AND organization_id = ? AND project_id = ? AND status = ?",
				analysisId,
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
			return errors.New("assistant analysis not found")
		}
		if err := eService.archiveOptions(ctx, tx, auth, analysisId); err != nil {
			return err
		}
		if _, err := eService.createOptions(ctx, tx, auth, analysisId, options); err != nil {
			return err
		}
		return tx.Preload("AssistantAnalysisOption", "status = ?", type_enums.RECORD_ACTIVE).
			Where("id = ? AND assistant_id = ? AND organization_id = ? AND project_id = ?",
				analysisId,
				assistantId,
				*auth.GetCurrentOrganizationId(),
				*auth.GetCurrentProjectId(),
			).
			First(&out).Error
	})
	if err != nil {
		eService.logger.Benchmark("assistantAnalysisService.Update", time.Since(start))
		eService.logger.Errorf("error while updating analysis %v", err)
		return nil, err
	}
	eService.logger.Benchmark("assistantAnalysisService.Update", time.Since(start))
	return out, nil
}

func (eService *assistantAnalysisService) Delete(ctx context.Context,
	auth types.SimplePrinciple,
	analysisId uint64,
	assistantId uint64,
) (*internal_assistant_entity.AssistantAnalysis, error) {
	start := time.Now()
	db := eService.postgres.DB(ctx)
	patch := &internal_assistant_entity.AssistantAnalysis{
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_ARCHIEVE,
			UpdatedBy: *auth.GetUserId(),
		},
	}
	var out *internal_assistant_entity.AssistantAnalysis
	err := db.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&internal_assistant_entity.AssistantAnalysis{}).
			Where("id = ? AND assistant_id = ? AND organization_id = ? AND project_id = ? AND status = ?",
				analysisId,
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
			return errors.New("assistant analysis not found")
		}
		if err := eService.archiveOptions(ctx, tx, auth, analysisId); err != nil {
			return err
		}
		return tx.Where("id = ? AND assistant_id = ? AND organization_id = ? AND project_id = ?",
			analysisId,
			assistantId,
			*auth.GetCurrentOrganizationId(),
			*auth.GetCurrentProjectId(),
		).
			First(&out).Error
	})
	if err != nil {
		eService.logger.Benchmark("assistantAnalysisService.Delete", time.Since(start))
		eService.logger.Errorf("error while deleting analysis %v", err)
		return nil, err
	}
	eService.logger.Benchmark("assistantAnalysisService.Delete", time.Since(start))
	return out, nil
}

// GetAll implements internal_services.AssistantAnalysisService.
func (eService *assistantAnalysisService) GetAll(ctx context.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
	criterias []*protos.Criteria,
	paginate *protos.Paginate) (int64, []*internal_assistant_entity.AssistantAnalysis, error) {
	start := time.Now()
	db := eService.postgres.DB(ctx)
	var (
		analysises []*internal_assistant_entity.AssistantAnalysis
		cnt        int64
	)
	qry := db.Model(internal_assistant_entity.AssistantAnalysis{}).
		Preload("AssistantAnalysisOption", "status = ?", type_enums.RECORD_ACTIVE)
	qry = qry.
		Where(
			"assistant_id = ? AND organization_id = ? AND project_id = ? AND status = ?",
			assistantId,
			*auth.GetCurrentOrganizationId(),
			*auth.GetCurrentProjectId(),
			type_enums.RECORD_ACTIVE,
		)
	for _, ct := range criterias {
		qry = qry.Where(fmt.Sprintf("%s %s ?", ct.GetKey(), ct.GetLogic()), ct.GetValue())
	}
	tx := qry.
		Scopes(gorm_models.
			Paginate(gorm_models.
				NewPaginated(
					int(paginate.GetPage()),
					int(paginate.GetPageSize()),
					&cnt,
					qry))).
		Order(clause.OrderByColumn{
			Column: clause.Column{Name: "created_date"},
			Desc:   true,
		}).Find(&analysises)

	if tx.Error != nil {
		eService.logger.Errorf("not able to find any Webhooks %v", tx.Error)
		return cnt, nil, tx.Error
	}
	eService.logger.Benchmark("AnalysisService.GetAll", time.Since(start))
	return cnt, analysises, nil
}

func (eService *assistantAnalysisService) archiveOptions(
	ctx context.Context,
	tx *gorm.DB,
	auth types.SimplePrinciple,
	analysisId uint64,
) error {
	patch := &internal_assistant_entity.AssistantAnalysisOption{
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_ARCHIEVE,
			UpdatedBy: *auth.GetUserId(),
		},
	}
	return tx.WithContext(ctx).
		Where("assistant_analysis_id = ? AND status = ?", analysisId, type_enums.RECORD_ACTIVE).
		Updates(patch).
		Error
}

func (eService *assistantAnalysisService) createOptions(
	ctx context.Context,
	tx *gorm.DB,
	auth types.SimplePrinciple,
	analysisId uint64,
	options []*protos.Metadata,
) ([]*internal_assistant_entity.AssistantAnalysisOption, error) {
	if len(options) == 0 {
		return []*internal_assistant_entity.AssistantAnalysisOption{}, nil
	}

	out := make([]*internal_assistant_entity.AssistantAnalysisOption, 0, len(options))
	for _, opt := range options {
		if opt == nil {
			continue
		}
		out = append(out, &internal_assistant_entity.AssistantAnalysisOption{
			AssistantAnalysisId: analysisId,
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
		return []*internal_assistant_entity.AssistantAnalysisOption{}, nil
	}

	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "key"},
			{Name: "assistant_analysis_id"},
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
