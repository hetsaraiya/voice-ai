// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package assistant_deployment_api

import (
	"github.com/rapidaai/api/assistant-api/config"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	internal_assistant_service "github.com/rapidaai/api/assistant-api/internal/services/assistant"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	"github.com/rapidaai/pkg/storages"

	storage_files "github.com/rapidaai/pkg/storages/file-storage"
	"github.com/rapidaai/protos"
)

type AssistantDeploymentApi struct {
	cfg               *config.AssistantConfig
	logger            commons.Logger
	postgres          connectors.SQLConnector
	deploymentService internal_services.AssistantDeploymentService
	storage           storages.Storage
}

type assistantDeploymentGrpcApi struct {
	AssistantDeploymentApi
}

func NewAssistantDeploymentGRPCApi(config *config.AssistantConfig, logger commons.Logger,
	postgres connectors.SQLConnector,
) protos.AssistantDeploymentServiceServer {
	return &assistantDeploymentGrpcApi{
		AssistantDeploymentApi: AssistantDeploymentApi{
			cfg:               config,
			logger:            logger,
			postgres:          postgres,
			deploymentService: internal_assistant_service.NewAssistantDeploymentService(config, logger, postgres),
			storage:           storage_files.NewStorage(config.AssetStoreConfig, logger),
		},
	}
}

func NewAssistantDeploymentApi(config *config.AssistantConfig, logger commons.Logger,
	postgres connectors.PostgresConnector,
) *AssistantDeploymentApi {
	return &AssistantDeploymentApi{
		cfg:               config,
		logger:            logger,
		postgres:          postgres,
		deploymentService: internal_assistant_service.NewAssistantDeploymentService(config, logger, postgres),
		storage:           storage_files.NewStorage(config.AssetStoreConfig, logger),
	}
}
