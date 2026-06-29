// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package configs

type SQLiteConfig struct {
	Path              string `mapstructure:"path" validate:"required"`
	MaxIdleConnection int    `mapstructure:"max_idle_connection" validate:"gte=0"`
	MaxOpenConnection int    `mapstructure:"max_open_connection" validate:"gte=0"`
}