// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package configs

import "github.com/rapidaai/pkg/validator"

type OpenSearchConfig struct {
	Schema        string    `mapstructure:"schema" validate:"required"`
	Host          string    `mapstructure:"host" validate:"required"`
	Port          *int      `mapstructure:"port"`
	Auth          BasicAuth `mapstructure:"auth"`
	MaxRetries    int       `mapstructure:"max_retries"`
	MaxConnection int       `mapstructure:"max_connection"`
}

func (cfg *OpenSearchConfig) IsValid() bool {
	return validator.NonNil(cfg) &&
		validator.NotBlank(cfg.Schema) &&
		validator.NotBlank(cfg.Host)
}

func (cfg *OpenSearchConfig) ToMap() map[string]interface{} {
	if !cfg.IsValid() {
		return nil
	}
	return map[string]interface{}{
		"schema":         cfg.Schema,
		"host":           cfg.Host,
		"port":           cfg.Port,
		"auth":           cfg.Auth,
		"max_retries":    cfg.MaxRetries,
		"max_connection": cfg.MaxConnection,
	}
}
