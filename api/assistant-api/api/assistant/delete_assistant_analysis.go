// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package assistant_api

import (
	"context"
	"errors"

	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	assistant_api "github.com/rapidaai/protos"
)

func (assistantApi *assistantGrpcApi) DeleteAssistantAnalysis(ctx context.Context, cer *assistant_api.DeleteAssistantAnalysisRequest) (*assistant_api.GetAssistantAnalysisResponse, error) {
	iAuth, isAuthenticated := types.GetSimplePrincipleGRPC(ctx)
	if !isAuthenticated || !iAuth.HasProject() {
		assistantApi.logger.Errorf("unauthenticated request for DeleteAssistantAnalysisRequest")
		return utils.Error[assistant_api.GetAssistantAnalysisResponse](
			errors.New("unauthenticated request for DeleteAssistantAnalysisRequest"),
			"Please provider valid service credentials to perfom DeleteAssistantAnalysisRequest, read docs @ docs.rapida.ai",
		)
	}
	analysis, err := assistantApi.assistantAnalysisService.Delete(ctx,
		iAuth,
		cer.GetId(), cer.GetAssistantId())
	if err != nil {
		return utils.Error[assistant_api.GetAssistantAnalysisResponse](
			err,
			"Unable to update assistant analysis, please try again in sometime",
		)
	}
	out := &assistant_api.AssistantAnalysis{}
	err = utils.Cast(analysis, out)
	if err != nil {
		assistantApi.logger.Errorf("unable to cast the assistant analysis to the response object")
	}
	return utils.Success[assistant_api.GetAssistantAnalysisResponse, *assistant_api.AssistantAnalysis](out)

}
