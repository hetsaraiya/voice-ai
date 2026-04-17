// Rapida – Open Source Voice AI Orchestration Platform
// Copyright (C) 2023-2025 Prashant Srivastav <prashant@rapida.ai>
// Licensed under a modified GPL-2.0. See the LICENSE file for details.
package integration_routers

import (
	"google.golang.org/grpc"

	integrationApi "github.com/rapidaai/api/integration-api/api"
	"github.com/rapidaai/api/integration-api/config"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	"github.com/rapidaai/protos"
)

// all the provider routes
func ProviderApiRoute(Cfg *config.IntegrationConfig, S *grpc.Server, Logger commons.Logger, Postgres connectors.SQLConnector) {
	protos.RegisterUnifiedProviderServiceServer(S, integrationApi.NewUnifiedProviderGRPC(Cfg, Logger, Postgres))
}

// audit logging api route
func AuditLoggingApiRoute(
	Cfg *config.IntegrationConfig,
	S *grpc.Server,
	Logger commons.Logger,
	Postgres connectors.SQLConnector,
) {
	protos.RegisterAuditLoggingServiceServer(S, integrationApi.NewAuditLoggingGRPC(Cfg, Logger, Postgres))
}
