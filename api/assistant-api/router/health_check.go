// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package assistant_router

import (
	"github.com/gin-gonic/gin"
	healthCheckApi "github.com/rapidaai/api/assistant-api/api/health"
	"github.com/rapidaai/api/assistant-api/config"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
)

func HealthCheckRoutes(cfg *config.AssistantConfig, engine *gin.Engine, logger commons.Logger, postgres connectors.PostgresConnector) {
	logger.Info("Internal HealthCheckRoutes and Connectors added to engine.")
	apiv1 := engine.Group("")
	hcApi := healthCheckApi.New(cfg, logger, postgres)
	{
		apiv1.GET("/readiness/", hcApi.Readiness)
		apiv1.GET("/healthz/", hcApi.Healthz)
	}
}
