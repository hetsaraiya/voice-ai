// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package assistant_api

import (
	"github.com/rapidaai/api/assistant-api/config"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	internal_assistant_service "github.com/rapidaai/api/assistant-api/internal/services/assistant"
	internal_knowledge_service "github.com/rapidaai/api/assistant-api/internal/services/knowledge"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	storage_files "github.com/rapidaai/pkg/storages/file-storage"
	"github.com/rapidaai/protos"
)

type assistantApi struct {
	cfg                       *config.AssistantConfig
	logger                    commons.Logger
	postgres                  connectors.SQLConnector
	redis                     connectors.RedisConnector
	opensearch                connectors.OpenSearchConnector
	vectordb                  connectors.VectorConnector
	assistantService          internal_services.AssistantService
	knowledgeDocumentService  internal_services.KnowledgeDocumentService
	conversactionService      internal_services.AssistantConversationService
	assistantHTTPLogService   internal_services.AssistantHTTPLogService
	assistantConfigService    internal_services.AssistantConfigurationService
	assistantToolService      internal_services.AssistantToolService
	assistantKnowledgeService internal_services.AssistantKnowledgeService
}

type assistantGrpcApi struct {
	assistantApi
	protos.UnimplementedAssistantServiceServer
}

func NewAssistantGRPCApi(config *config.AssistantConfig, logger commons.Logger,
	postgres connectors.SQLConnector,
	redis connectors.RedisConnector,
	opensearch connectors.OpenSearchConnector,
	vectordb connectors.VectorConnector,

) protos.AssistantServiceServer {
	var knowledgeDocSvc internal_services.KnowledgeDocumentService
	if opensearch != nil {
		knowledgeDocSvc = internal_knowledge_service.NewKnowledgeDocumentService(config, logger, postgres, opensearch)
	}
	return &assistantGrpcApi{
		assistantApi: assistantApi{
			cfg:                       config,
			logger:                    logger,
			postgres:                  postgres,
			redis:                     redis,
			opensearch:                opensearch,
			vectordb:                  vectordb,
			assistantService:          internal_assistant_service.NewAssistantService(config, logger, postgres, opensearch),
			knowledgeDocumentService:  knowledgeDocSvc,
			conversactionService:      internal_assistant_service.NewAssistantConversationService(logger, postgres, storage_files.NewStorage(config.AssetStoreConfig, logger)),
			assistantHTTPLogService:   internal_assistant_service.NewAssistantHTTPLogService(logger, postgres, storage_files.NewStorage(config.AssetStoreConfig, logger)),
			assistantConfigService:    internal_assistant_service.NewAssistantConfigurationService(logger, postgres),
			assistantToolService:      internal_assistant_service.NewAssistantToolService(logger, postgres, storage_files.NewStorage(config.AssetStoreConfig, logger)),
			assistantKnowledgeService: internal_assistant_service.NewAssistantKnowledgeService(logger, postgres, storage_files.NewStorage(config.AssetStoreConfig, logger)),
		},
	}
}
