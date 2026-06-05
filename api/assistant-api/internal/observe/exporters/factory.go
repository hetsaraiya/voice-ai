// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package observe_exporters

import (
	"context"

	"github.com/rapidaai/api/assistant-api/internal/observe"
	"github.com/rapidaai/config"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	"github.com/rapidaai/pkg/utils"
)

// GetExporter returns an EventExporter and MetricExporter for the given
// provider type. It mirrors the transformer factory's switch-based approach.
func GetExporter(
	ctx context.Context,
	logger commons.Logger,
	cfg *config.AppConfig,
	opensearch connectors.OpenSearchConnector,
	provider string,
	opts utils.Option,
) (observe.EventExporter, observe.MetricExporter, error) {
	// exp, err := providers.NewExporterFromOptions(
	// 	ctx,
	// 	provider,
	// 	map[string]interface{}(opts),
	// 	providers.FactoryDependencies{
	// 		Logger:     logger,
	// 		AppConfig:  cfg,
	// 		OpenSearch: opensearch,
	// 	},
	// )
	// if err != nil {
	// 	return nil, nil, err
	// }
	// if exp == nil {
	// 	return nil, nil, nil
	// }
	return nil, nil, nil
}
