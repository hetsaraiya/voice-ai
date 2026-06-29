// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package connectors

import (
	"fmt"

	commons "github.com/rapidaai/pkg/commons"
	configs "github.com/rapidaai/pkg/configs"
)

func NewSQLConnector(config configs.SQLConfig, logger commons.Logger) (SQLConnector, error) {
	if config == nil {
		return nil, fmt.Errorf("sql config is nil")
	}
	switch cfg := config.(type) {
	case *configs.PostgresConfig:
		return NewPostgresConnector(cfg, logger), nil
	case *configs.SQLiteConfig:
		return NewSQLiteConnector(cfg, logger), nil
	default:
		return nil, fmt.Errorf("unsupported SQL config type %T; expected *PostgresConfig or *SQLiteConfig", config)
	}
}