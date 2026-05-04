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
	gorm_generator "github.com/rapidaai/pkg/models/gorm/generators"
	gorm_types "github.com/rapidaai/pkg/models/gorm/types"
	"github.com/rapidaai/pkg/storages"
	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
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
	storage storages.Storage) internal_services.AssistantWebhookService {
	return &assistantWebhookService{
		logger:   logger,
		postgres: postgres,
		storage:  storage,
	}
}

// Get implements internal_services.AssistantWebhookService.
func (eService *assistantWebhookService) Get(ctx context.Context, auth types.SimplePrinciple, webhookId, assistantId uint64) (*internal_assistant_entity.AssistantWebhook, error) {
	start := time.Now()
	db := eService.postgres.DB(ctx)
	var Webhook *internal_assistant_entity.AssistantWebhook
	tx := db.Preload("AssistantWebhookOption", "status = ?", type_enums.RECORD_ACTIVE).
		Where("id = ? AND assistant_id = ?", webhookId, assistantId).
		Where("organization_id = ? AND project_id = ?", *auth.GetCurrentOrganizationId(), *auth.GetCurrentProjectId()).
		First(&Webhook)
	if tx.Error != nil {
		eService.logger.Benchmark("WebhookService.Get", time.Since(start))
		eService.logger.Errorf("not able to find any webhook %v", tx.Error)
		return nil, tx.Error
	}
	eService.logger.Benchmark("WebhookService.Get", time.Since(start))
	return Webhook, nil
}

func (eService *assistantWebhookService) Create(ctx context.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
	assistantEvents []string,
	options []*protos.Metadata,
	executionPriority uint32,
	description *string,
) (*internal_assistant_entity.AssistantWebhook, error) {
	start := time.Now()
	db := eService.postgres.DB(ctx)
	desc := ""
	if description != nil {
		desc = *description
	}
	webhook := &internal_assistant_entity.AssistantWebhook{
		AssistantId:       assistantId,
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
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&webhook).Error; err != nil {
			return err
		}
		if _, err := eService.createOptions(ctx, tx, auth, webhook.Id, options); err != nil {
			return err
		}
		return tx.Preload("AssistantWebhookOption", "status = ?", type_enums.RECORD_ACTIVE).
			Where("id = ?", webhook.Id).
			First(&webhook).Error
	})
	if err != nil {
		eService.logger.Benchmark("eService.Create", time.Since(start))
		eService.logger.Errorf("error while creating webhook %v", err)
		return nil, err
	}
	eService.logger.Benchmark("eService.Create", time.Since(start))
	return webhook, nil
}

func (eService *assistantWebhookService) Update(ctx context.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
	webhookId uint64,
	assistantEvents []string,
	options []*protos.Metadata,
	executionPriority uint32,
	description *string,
) (*internal_assistant_entity.AssistantWebhook, error) {
	start := time.Now()
	db := eService.postgres.DB(ctx)
	desc := ""
	if description != nil {
		desc = *description
	}
	webhook := &internal_assistant_entity.AssistantWebhook{
		Description:       desc,
		ExecutionPriority: executionPriority,
		AssistantEvents:   gorm_types.StringArray(assistantEvents),
		Mutable: gorm_models.Mutable{
			UpdatedBy: *auth.GetUserId(),
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
			Updates(&webhook)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected == 0 {
			return errors.New("assistant webhook not found")
		}
		if err := eService.archiveOptions(ctx, tx, auth, webhookId); err != nil {
			return err
		}
		if _, err := eService.createOptions(ctx, tx, auth, webhookId, options); err != nil {
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
		eService.logger.Benchmark("assistantWebhookService.Update", time.Since(start))
		eService.logger.Errorf("error while creating webhook %v", err)
		return nil, err
	}
	eService.logger.Benchmark("assistantWebhookService.Update", time.Since(start))
	return out, nil
}

func (eService *assistantWebhookService) Delete(ctx context.Context,
	auth types.SimplePrinciple,
	webhookId uint64,
	assistantId uint64,
) (*internal_assistant_entity.AssistantWebhook, error) {
	start := time.Now()
	db := eService.postgres.DB(ctx)
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
		if err := eService.archiveOptions(ctx, tx, auth, webhookId); err != nil {
			return err
		}
		return tx.Where("id = ? AND assistant_id = ? AND organization_id = ? AND project_id = ?",
			webhookId,
			assistantId,
			*auth.GetCurrentOrganizationId(),
			*auth.GetCurrentProjectId(),
		).
			First(&out).Error
	})
	if err != nil {
		eService.logger.Benchmark("assistantWebhookService.Delete", time.Since(start))
		eService.logger.Errorf("error while creating webhook %v", err)
		return nil, err
	}
	eService.logger.Benchmark("assistantWebhookService.Delete", time.Since(start))
	return out, nil
}

// GetAll implements internal_services.AssistantWebhookService.
func (eService *assistantWebhookService) GetAll(ctx context.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
	criterias []*protos.Criteria,
	paginate *protos.Paginate) (int64, []*internal_assistant_entity.AssistantWebhook, error) {
	start := time.Now()
	db := eService.postgres.DB(ctx)
	var (
		Webhooks []*internal_assistant_entity.AssistantWebhook
		cnt      int64
	)
	qry := db.Model(internal_assistant_entity.AssistantWebhook{}).
		Preload("AssistantWebhookOption", "status = ?", type_enums.RECORD_ACTIVE)
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
		}).Find(&Webhooks)

	if tx.Error != nil {
		eService.logger.Errorf("not able to find any Webhooks %v", tx.Error)
		return cnt, nil, tx.Error
	}
	eService.logger.Benchmark("WebhookService.GetAll", time.Since(start))
	return cnt, Webhooks, nil
}

func (eService *assistantWebhookService) archiveOptions(
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
		Updates(patch).
		Error
}

func (eService *assistantWebhookService) createOptions(
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
		if opt == nil {
			continue
		}
		if opt.GetKey() == "" || opt.GetKey() == internal_assistant_entity.WebhookOptionAssistantEventsKey {
			continue
		}
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

func (eService *assistantWebhookService) CreateLog(
	ctx context.Context,
	auth types.SimplePrinciple,
	webhookId uint64,
	assistantId, conversationId uint64,
	httpUrl string,
	httpMethod string,
	event string,
	responseStatus int64,
	timeTaken int64,
	retryCount uint32,
	status type_enums.RecordState,
	request, response []byte,
) (*internal_assistant_entity.AssistantWebhookLog, error) {
	start := time.Now()
	db := eService.postgres.DB(ctx)
	s3Prefix := eService.ObjectPrefix(*auth.GetCurrentOrganizationId(), *auth.GetCurrentProjectId())
	_auditId := gorm_generator.ID()
	utils.Go(ctx, func() {
		key := eService.ObjectKey(s3Prefix, _auditId, "request.json")
		eService.storage.Store(ctx, key, request)
	})

	utils.Go(ctx, func() {
		key := eService.ObjectKey(s3Prefix, _auditId, "response.json")
		eService.storage.Store(ctx, key, response)
	})

	webhookLog := &internal_assistant_entity.AssistantWebhookLog{
		Audited: gorm_models.Audited{
			Id: _auditId,
		},
		HttpMethod:              httpMethod,
		HttpUrl:                 httpUrl,
		AssistantId:             assistantId,
		WebhookId:               webhookId,
		AssistantConversationId: conversationId,
		AssetPrefix:             s3Prefix,
		ResponseStatus:          responseStatus,
		Event:                   event,
		TimeTaken:               timeTaken,
		Organizational: gorm_models.Organizational{
			ProjectId:      *auth.GetCurrentProjectId(),
			OrganizationId: *auth.GetCurrentOrganizationId(),
		},
		Mutable: gorm_models.Mutable{
			Status: status,
		},
	}
	tx := db.Create(&webhookLog)
	if tx.Error != nil {
		eService.logger.Benchmark("eService.Create", time.Since(start))
		eService.logger.Errorf("error while creating webhook log %v", tx.Error)
		return nil, tx.Error
	}
	eService.logger.Benchmark("eService.Create", time.Since(start))
	return webhookLog, nil
}

func (eService *assistantWebhookService) GetLog(
	ctx context.Context,
	auth types.SimplePrinciple,
	projectId uint64,
	webhookLogId uint64) (*internal_assistant_entity.AssistantWebhookLog, error) {
	start := time.Now()
	db := eService.postgres.DB(ctx)
	var wkg *internal_assistant_entity.AssistantWebhookLog
	tx := db.Where("id = ? AND organization_id = ? AND project_id = ?", webhookLogId, *auth.GetCurrentOrganizationId(), projectId).
		First(&wkg)
	if tx.Error != nil {
		eService.logger.Benchmark("WebhookService.GetLog", time.Since(start))
		eService.logger.Errorf("not able to find any webhook %v", tx.Error)
		return nil, tx.Error
	}
	eService.logger.Benchmark("WebhookService.GetLog", time.Since(start))
	return wkg, nil
}

func (eService *assistantWebhookService) GetAllLog(
	ctx context.Context,
	auth types.SimplePrinciple,
	projectId uint64,
	criterias []*protos.Criteria,
	paginate *protos.Paginate,
	order *protos.Ordering) (int64, []*internal_assistant_entity.AssistantWebhookLog, error) {
	start := time.Now()
	db := eService.postgres.DB(ctx)
	var (
		webhookLogs []*internal_assistant_entity.AssistantWebhookLog
		cnt         int64
	)
	qry := db.Model(internal_assistant_entity.AssistantWebhookLog{})
	qry = qry.
		Where("organization_id = ? AND project_id = ? ", *auth.GetCurrentOrganizationId(), projectId)
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
		}).Find(&webhookLogs)

	if tx.Error != nil {
		eService.logger.Errorf("not able to find any Webhooks %v", tx.Error)
		return cnt, nil, tx.Error
	}
	eService.logger.Benchmark("WebhookService.GetAllLog", time.Since(start))
	return cnt, webhookLogs, nil
}

func (eService *assistantWebhookService) ObjectPrefix(orgId, projectId uint64) string {
	return fmt.Sprintf("%d/%d/webhook", orgId, projectId)
}

func (eService *assistantWebhookService) ObjectKey(keyPrefix string, auditId uint64, objName string) string {
	return fmt.Sprintf("%s/%d__%s", keyPrefix, auditId, objName)
}

func (eService *assistantWebhookService) GetLogObject(
	ctx context.Context,
	organizationId,
	projectId, webhookLogId uint64) (requestData []byte, responseData []byte, err error) {
	keyPrefix := eService.ObjectPrefix(organizationId, projectId)
	responseKey := eService.ObjectKey(keyPrefix, webhookLogId, "response.json")
	requestKey := eService.ObjectKey(keyPrefix, webhookLogId, "request.json")

	type _fileStruct struct {
		Key   string
		Data  []byte
		Error error
	}

	responseChan := make(chan _fileStruct)
	requestChan := make(chan _fileStruct)

	go func(key string) {
		eService.logger.Debugf("Getting key from s3 %s", key)
		result := eService.storage.Get(ctx, key)
		if result.Error != nil {
			eService.logger.Errorf("error downloading goroutine: %v", result.Error)
			responseChan <- _fileStruct{Key: key, Error: result.Error}
			close(responseChan)
			return
		}
		responseChan <- _fileStruct{Key: key, Data: result.Data}
		close(responseChan)
	}(responseKey)

	go func(key string) {
		eService.logger.Debugf("Getting key from s3 %s", key)
		result := eService.storage.Get(ctx, key)
		if result.Error != nil {
			eService.logger.Errorf("error downloading goroutine: %v", result.Error)
			requestChan <- _fileStruct{Key: key, Error: result.Error}
			close(requestChan)
			return
		}
		requestChan <- _fileStruct{Key: key, Data: result.Data}
		close(requestChan)

	}(requestKey)

	for result := range responseChan {
		if result.Error != nil {
			eService.logger.Errorf("error downloading/parsing response: %v", result.Error)
			break
		}
		responseData = result.Data
	}

	for result := range requestChan {
		if result.Error != nil {
			eService.logger.Errorf("error downloading/parsing request: %v", result.Error)
			break
		}
		requestData = result.Data
	}

	return requestData, responseData, nil
}
