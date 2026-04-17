package endpoint_router

import (
	"google.golang.org/grpc"

	endpoint_api "github.com/rapidaai/api/endpoint-api/api"
	"github.com/rapidaai/api/endpoint-api/config"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	"github.com/rapidaai/protos"
)

func EndpointReaderApiRoute(
	Cfg *config.EndpointConfig,
	S *grpc.Server,
	Logger commons.Logger,
	Postgres connectors.SQLConnector,
	Redis connectors.RedisConnector,

) {
	protos.RegisterEndpointServiceServer(S, endpoint_api.NewEndpointGRPCApi(Cfg, Logger, Postgres, Redis))
}

func InvokeApiRoute(
	Cfg *config.EndpointConfig,
	S *grpc.Server,
	Logger commons.Logger,
	Postgres connectors.SQLConnector,
	Redis connectors.RedisConnector,
) {
	protos.RegisterDeploymentServer(S, endpoint_api.NewInvokerGRPCApi(Cfg, Logger, Postgres, Redis))
}
