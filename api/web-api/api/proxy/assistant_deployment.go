package web_proxy_api

import (
	"context"
	"errors"

	assistant_client "github.com/rapidaai/pkg/clients/workflow"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	protos "github.com/rapidaai/protos"

	web_api "github.com/rapidaai/api/web-api/api"
	config "github.com/rapidaai/api/web-api/config"
	commons "github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
)

type webAssistantDeploymentApi struct {
	web_api.WebApi
	cfg             *config.WebAppConfig
	logger          commons.Logger
	postgres        connectors.SQLConnector
	redis           connectors.RedisConnector
	assistantClient assistant_client.AssistantServiceClient
}

type webAssistantDeploymentGRPCApi struct {
	webAssistantDeploymentApi
}

// GetAssistantApiDeployment implements protos.AssistantDeploymentServiceServer.
func (w *webAssistantDeploymentGRPCApi) GetAssistantApiDeployment(c context.Context, iRequest *protos.GetAssistantDeploymentRequest) (*protos.GetAssistantApiDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetSimplePrincipleGRPC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.GetAssistantApiDeployment(c, iAuth, iRequest)
}

func (w *webAssistantDeploymentGRPCApi) GetAllAssistantApiDeployment(c context.Context, iRequest *protos.GetAllAssistantDeploymentRequest) (*protos.GetAllAssistantApiDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetSimplePrincipleGRPC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	page, data, err := w.assistantClient.GetAllAssistantApiDeployment(c, iAuth, iRequest.GetAssistantId(), iRequest.GetCriterias(), iRequest.GetPaginate())
	if err != nil {
		return nil, err
	}
	return utils.PaginatedSuccess[protos.GetAllAssistantApiDeploymentResponse, []*protos.AssistantApiDeployment](
		page.GetTotalItem(), page.GetCurrentPage(),
		data)
}

func (w *webAssistantDeploymentGRPCApi) DisableAssistantApiDeployment(c context.Context, iRequest *protos.GetAssistantDeploymentRequest) (*protos.GetAssistantApiDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetAuthPrincipleGPRC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.DisableAssistantApiDeployment(c, iAuth, iRequest)
}

// GetAssistantDebuggerDeployment implements protos.AssistantDeploymentServiceServer.
func (w *webAssistantDeploymentGRPCApi) GetAssistantDebuggerDeployment(c context.Context, iRequest *protos.GetAssistantDeploymentRequest) (*protos.GetAssistantDebuggerDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetSimplePrincipleGRPC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.GetAssistantDebuggerDeployment(c, iAuth, iRequest)
}

func (w *webAssistantDeploymentGRPCApi) GetAllAssistantDebuggerDeployment(c context.Context, iRequest *protos.GetAllAssistantDeploymentRequest) (*protos.GetAllAssistantDebuggerDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetSimplePrincipleGRPC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	page, data, err := w.assistantClient.GetAllAssistantDebuggerDeployment(c, iAuth, iRequest.GetAssistantId(), iRequest.GetCriterias(), iRequest.GetPaginate())
	if err != nil {
		return nil, err
	}
	return utils.PaginatedSuccess[protos.GetAllAssistantDebuggerDeploymentResponse, []*protos.AssistantDebuggerDeployment](
		page.GetTotalItem(), page.GetCurrentPage(),
		data)
}

func (w *webAssistantDeploymentGRPCApi) DisableAssistantDebuggerDeployment(c context.Context, iRequest *protos.GetAssistantDeploymentRequest) (*protos.GetAssistantDebuggerDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetAuthPrincipleGPRC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.DisableAssistantDebuggerDeployment(c, iAuth, iRequest)
}

// GetAssistantPhoneDeployment implements protos.AssistantDeploymentServiceServer.
func (w *webAssistantDeploymentGRPCApi) GetAssistantPhoneDeployment(c context.Context, iRequest *protos.GetAssistantDeploymentRequest) (*protos.GetAssistantPhoneDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetSimplePrincipleGRPC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.GetAssistantPhoneDeployment(c, iAuth, iRequest)
}

func (w *webAssistantDeploymentGRPCApi) GetAllAssistantPhoneDeployment(c context.Context, iRequest *protos.GetAllAssistantDeploymentRequest) (*protos.GetAllAssistantPhoneDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetSimplePrincipleGRPC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	page, data, err := w.assistantClient.GetAllAssistantPhoneDeployment(c, iAuth, iRequest.GetAssistantId(), iRequest.GetCriterias(), iRequest.GetPaginate())
	if err != nil {
		return nil, err
	}
	return utils.PaginatedSuccess[protos.GetAllAssistantPhoneDeploymentResponse, []*protos.AssistantPhoneDeployment](
		page.GetTotalItem(), page.GetCurrentPage(),
		data)
}

func (w *webAssistantDeploymentGRPCApi) DisableAssistantPhoneDeployment(c context.Context, iRequest *protos.GetAssistantDeploymentRequest) (*protos.GetAssistantPhoneDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetAuthPrincipleGPRC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.DisableAssistantPhoneDeployment(c, iAuth, iRequest)
}

// GetAssistantWebpluginDeployment implements protos.AssistantDeploymentServiceServer.
func (w *webAssistantDeploymentGRPCApi) GetAssistantWebpluginDeployment(c context.Context, iRequest *protos.GetAssistantDeploymentRequest) (*protos.GetAssistantWebpluginDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetSimplePrincipleGRPC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.GetAssistantWebpluginDeployment(c, iAuth, iRequest)
}

func (w *webAssistantDeploymentGRPCApi) GetAllAssistantWebpluginDeployment(c context.Context, iRequest *protos.GetAllAssistantDeploymentRequest) (*protos.GetAllAssistantWebpluginDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetSimplePrincipleGRPC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	page, data, err := w.assistantClient.GetAllAssistantWebpluginDeployment(c, iAuth, iRequest.GetAssistantId(), iRequest.GetCriterias(), iRequest.GetPaginate())
	if err != nil {
		return nil, err
	}
	return utils.PaginatedSuccess[protos.GetAllAssistantWebpluginDeploymentResponse, []*protos.AssistantWebpluginDeployment](
		page.GetTotalItem(), page.GetCurrentPage(),
		data)
}

func (w *webAssistantDeploymentGRPCApi) DisableAssistantWebpluginDeployment(c context.Context, iRequest *protos.GetAssistantDeploymentRequest) (*protos.GetAssistantWebpluginDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetAuthPrincipleGPRC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.DisableAssistantWebpluginDeployment(c, iAuth, iRequest)
}

// GetAssistantWhatsappDeployment implements protos.AssistantDeploymentServiceServer.
func (w *webAssistantDeploymentGRPCApi) GetAssistantWhatsappDeployment(c context.Context, iRequest *protos.GetAssistantDeploymentRequest) (*protos.GetAssistantWhatsappDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetSimplePrincipleGRPC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.GetAssistantWhatsappDeployment(c, iAuth, iRequest)
}

func (w *webAssistantDeploymentGRPCApi) GetAllAssistantWhatsappDeployment(c context.Context, iRequest *protos.GetAllAssistantDeploymentRequest) (*protos.GetAllAssistantWhatsappDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetSimplePrincipleGRPC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	page, data, err := w.assistantClient.GetAllAssistantWhatsappDeployment(c, iAuth, iRequest.GetAssistantId(), iRequest.GetCriterias(), iRequest.GetPaginate())
	if err != nil {
		return nil, err
	}
	return utils.PaginatedSuccess[protos.GetAllAssistantWhatsappDeploymentResponse, []*protos.AssistantWhatsappDeployment](
		page.GetTotalItem(), page.GetCurrentPage(),
		data)
}

func (w *webAssistantDeploymentGRPCApi) DisableAssistantWhatsappDeployment(c context.Context, iRequest *protos.GetAssistantDeploymentRequest) (*protos.GetAssistantWhatsappDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetAuthPrincipleGPRC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.DisableAssistantWhatsappDeployment(c, iAuth, iRequest)
}

func (w *webAssistantDeploymentGRPCApi) CreateAssistantApiDeployment(c context.Context, iRequest *protos.CreateAssistantDeploymentRequest) (*protos.GetAssistantApiDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetAuthPrincipleGPRC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.CreateAssistantApiDeployment(c, iAuth, iRequest)
}

// CreateAssistantDebuggerDeployment implements protos.AssistantDeploymentServiceServer.
func (w *webAssistantDeploymentGRPCApi) CreateAssistantDebuggerDeployment(c context.Context, iRequest *protos.CreateAssistantDeploymentRequest) (*protos.GetAssistantDebuggerDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetAuthPrincipleGPRC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.CreateAssistantDebuggerDeployment(c, iAuth, iRequest)
}

// CreateAssistantPhoneDeployment implements protos.AssistantDeploymentServiceServer.
func (w *webAssistantDeploymentGRPCApi) CreateAssistantPhoneDeployment(c context.Context, iRequest *protos.CreateAssistantDeploymentRequest) (*protos.GetAssistantPhoneDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetAuthPrincipleGPRC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.CreateAssistantPhoneDeployment(c, iAuth, iRequest)
}

// CreateAssistantWebpluginDeployment implements protos.AssistantDeploymentServiceServer.
func (w *webAssistantDeploymentGRPCApi) CreateAssistantWebpluginDeployment(c context.Context, iRequest *protos.CreateAssistantDeploymentRequest) (*protos.GetAssistantWebpluginDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetAuthPrincipleGPRC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.CreateAssistantWebpluginDeployment(c, iAuth, iRequest)
}

// CreateAssistantWhatsappDeployment implements protos.AssistantDeploymentServiceServer.
func (w *webAssistantDeploymentGRPCApi) CreateAssistantWhatsappDeployment(c context.Context, iRequest *protos.CreateAssistantDeploymentRequest) (*protos.GetAssistantWhatsappDeploymentResponse, error) {
	iAuth, isAuthenticated := types.GetAuthPrincipleGPRC(c)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	return w.assistantClient.CreateAssistantWhatsappDeployment(c, iAuth, iRequest)
}

// G
func NewAssistantDeploymentGRPCApi(config *config.WebAppConfig, logger commons.Logger, postgres connectors.SQLConnector, redis connectors.RedisConnector) protos.AssistantDeploymentServiceServer {
	return &webAssistantDeploymentGRPCApi{
		webAssistantDeploymentApi{
			WebApi:          web_api.NewWebApi(config, logger, postgres, redis),
			cfg:             config,
			logger:          logger,
			postgres:        postgres,
			redis:           redis,
			assistantClient: assistant_client.NewAssistantServiceClientGRPC(&config.AppConfig, logger, redis),
		},
	}
}
