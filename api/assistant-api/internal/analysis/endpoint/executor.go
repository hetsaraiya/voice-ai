// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_analysis_endpoint

import (
	"context"
	"encoding/json"
	"fmt"

	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	endpoint_client_builders "github.com/rapidaai/pkg/clients/endpoint/builders"
	"github.com/rapidaai/pkg/commons"
	rapida_types "github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

const (
	AnalysisOptionEndpointIDKey      = "endpoint_id"
	AnalysisOptionEndpointVersionKey = "endpoint_version"
	AnalysisOptionEndpointParamsKey  = "endpoint_parameters"
)

type runtimeExecutor struct {
	logger       commons.Logger
	caller       internal_type.InternalCaller
	analysis     *internal_assistant_entity.AssistantAnalysis
	inputBuilder endpoint_client_builders.InputInvokeBuilder
}

// NewExecutor creates a fully wired endpoint-based analysis executor.
func NewExecutor(
	logger commons.Logger,
	_ context.Context,
	analysis *internal_assistant_entity.AssistantAnalysis,
	caller internal_type.InternalCaller,
) (internal_type.AnalysisExecutor, error) {
	return &runtimeExecutor{
		logger:       logger,
		caller:       caller,
		analysis:     analysis,
		inputBuilder: endpoint_client_builders.NewInputInvokeBuilder(logger),
	}, nil
}

func (e *runtimeExecutor) Name() string {
	return fmt.Sprintf("endpoint-analysis-%s", e.analysis.GetName())
}

func (e *runtimeExecutor) Options() utils.Option {
	return e.analysis.GetOptions()
}

func (e *runtimeExecutor) Arguments() (map[string]string, error) {
	return e.analysis.GetOptions().GetStringMap(AnalysisOptionEndpointParamsKey)
}

func (e *runtimeExecutor) GetEndpointId() (uint64, error) {
	return e.analysis.GetOptions().GetUint64(AnalysisOptionEndpointIDKey)
}

func (e *runtimeExecutor) GetEndpointVersion() (string, error) {
	return e.analysis.GetOptions().GetString(AnalysisOptionEndpointVersionKey)
}

// Execute runs one analysis and returns its metadata to the finalization flow.
func (e *runtimeExecutor) Execute(ctx context.Context, input internal_type.AnalysisInput) (internal_type.AnalysisOutput, error) {
	endpointID, err := e.GetEndpointId()
	if err != nil {
		return internal_type.AnalysisOutput{}, fmt.Errorf("failed to get endpoint ID: %w", err)
	}
	endpointVersion, err := e.GetEndpointVersion()
	if err != nil {
		return internal_type.AnalysisOutput{}, fmt.Errorf("failed to get endpoint version: %w", err)
	}
	response, err := e.caller.DeploymentCaller().Invoke(
		ctx,
		input.Auth,
		e.inputBuilder.Invoke(
			&protos.EndpointDefinition{
				EndpointId: endpointID,
				Version:    endpointVersion,
			},
			e.inputBuilder.Arguments(input.Arguments, nil),
			nil,
			nil,
		),
	)
	if err != nil {
		return internal_type.AnalysisOutput{}, err
	}
	if !response.GetSuccess() || len(response.GetData()) == 0 {
		return internal_type.AnalysisOutput{}, fmt.Errorf("empty response from endpoint")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(response.GetData()[0]), &parsed); err != nil {
		parsed = map[string]interface{}{"result": response.GetData()[0]}
	}

	metadata := rapida_types.NewMetadata(fmt.Sprintf("analysis.%s", e.analysis.GetName()), parsed)
	return internal_type.AnalysisOutput{
		Metadata: &protos.Metadata{Key: metadata.Key, Value: metadata.Value},
	}, nil
}

// Close releases executor dependencies.
func (e *runtimeExecutor) Close(_ context.Context) error {
	e.caller = nil
	return nil
}
