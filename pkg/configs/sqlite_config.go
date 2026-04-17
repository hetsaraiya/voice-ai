// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package configs

type SQLiteConfig struct {
	Path               string `mapstructure:"path" validate:"required"`
	MaxIdealConnection int    `mapstructure:"max_ideal_connection" validate:"required"`
	MaxOpenConnection  int    `mapstructure:"max_open_connection" validate:"required"`
}
