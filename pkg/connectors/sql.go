// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package connectors

import (
	commons "github.com/rapidaai/pkg/commons"
	configs "github.com/rapidaai/pkg/configs"
)

func NewSQLConnector(config configs.SQLConfig, logger commons.Logger) SQLConnector {
	switch cfg := config.(type) {
	case *configs.PostgresConfig:
		return NewPostgresConnector(cfg, logger)
	case *configs.SQLiteConfig:
		return NewSQLiteConnector(cfg, logger)
	default:
		return nil
	}
}
