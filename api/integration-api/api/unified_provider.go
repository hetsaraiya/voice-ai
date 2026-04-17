// Rapida – Open Source Voice AI Orchestration Platform
// Copyright (C) 2023-2025 Prashant Srivastav <prashant@rapida.ai>
// Licensed under a modified GPL-2.0. See the LICENSE file for details.
package integration_api

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	config "github.com/rapidaai/api/integration-api/config"
	internal_callers "github.com/rapidaai/api/integration-api/internal/caller"
	internal_types "github.com/rapidaai/api/integration-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	protos "github.com/rapidaai/protos"
)

type unifiedProviderGRPCApi struct {
	integrationApi
}

func NewUnifiedProviderGRPC(cfg *config.IntegrationConfig, logger commons.Logger, postgres connectors.SQLConnector) protos.UnifiedProviderServiceServer {
	return &unifiedProviderGRPCApi{
		integrationApi: NewInegrationApi(cfg, logger, postgres),
	}
}

func (u *unifiedProviderGRPCApi) Chat(c context.Context, req *protos.ChatRequest) (*protos.ChatResponse, error) {
	return u.integrationApi.Chat(c, req, req.GetProviderName())
}

func (u *unifiedProviderGRPCApi) StreamChat(stream protos.UnifiedProviderService_StreamChatServer) error {
	u.logger.Debugf("Bidirectional stream chat opened for unified provider")
	return u.integrationApi.StreamChatBidirectionalUnified(
		stream.Context(),
		u.logger,
		stream,
	)
}

func (u *unifiedProviderGRPCApi) Embedding(c context.Context, req *protos.EmbeddingRequest) (*protos.EmbeddingResponse, error) {
	providerName := strings.ToLower(req.GetProviderName())
	if providerName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "providerName is required")
	}
	caller, err := internal_callers.GetEmbeddingCaller(u.logger, providerName, req.GetCredential())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	return u.integrationApi.Embedding(c, req, providerName, caller)
}

func (u *unifiedProviderGRPCApi) Reranking(c context.Context, req *protos.RerankingRequest) (*protos.RerankingResponse, error) {
	providerName := strings.ToLower(req.GetProviderName())
	if providerName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "providerName is required")
	}
	caller, err := internal_callers.GetRerankingCaller(u.logger, providerName, req.GetCredential())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	return u.integrationApi.Reranking(c, req, providerName, caller)
}

func (u *unifiedProviderGRPCApi) VerifyCredential(c context.Context, req *protos.VerifyCredentialRequest) (*protos.VerifyCredentialResponse, error) {
	providerName := strings.ToLower(req.GetProviderName())
	if providerName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "providerName is required")
	}
	verifier, err := internal_callers.GetVerifier(u.logger, providerName, req.GetCredential())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	st, err := verifier.CredentialVerifier(c, &internal_types.CredentialVerifierOptions{})
	if err != nil {
		u.logger.Errorf("verify credential response with error %v", err)
		return &protos.VerifyCredentialResponse{
			Code:         401,
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}
	return &protos.VerifyCredentialResponse{
		Code:     200,
		Success:  true,
		Response: st,
	}, nil
}
