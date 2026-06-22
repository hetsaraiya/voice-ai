// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_analysis

import (
	"context"
	"fmt"

	internal_analysis_endpoint "github.com/rapidaai/api/assistant-api/internal/analysis/endpoint"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
)

// NewExecutor is the factory that returns an analysis executor implementation.
// Currently only the deployment-endpoint variant is supported; switch on the
// analysis artifact type when other transports are added.
func NewExecutor(
	logger commons.Logger,
	ctx context.Context,
	analysis *internal_assistant_entity.AssistantAnalysis,
	caller internal_type.InternalCaller,
) (internal_type.AnalysisExecutor, error) {
	switch analysis.Provider {
	case internal_assistant_entity.AssistantAnalysisProviderEndpoint:
		return internal_analysis_endpoint.NewExecutor(logger, ctx, analysis, caller)
	default:
		return nil, fmt.Errorf("analysis: unsupported executor type %q", analysis.Provider)
	}
}
