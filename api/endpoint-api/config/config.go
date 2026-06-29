// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package config

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"

	"github.com/rapidaai/config"
	"github.com/rapidaai/pkg/configs"
)

// Application config structure
type EndpointConfig struct {
	config.AppConfig `mapstructure:",squash"`
	PostgresConfig   *configs.PostgresConfig  `mapstructure:"postgres"`
	SQLiteConfig     *configs.SQLiteConfig    `mapstructure:"sqlite"`
	RedisConfig      configs.RedisConfig      `mapstructure:"redis" validate:"required"`
	AssetStoreConfig configs.AssetStoreConfig `mapstructure:"asset_store" validate:"required"`
}

// reading config and intializing configs for application
func InitConfig() (*viper.Viper, error) {
	vConfig := viper.New()

	path := os.Getenv("ENV_PATH")
	if path != "" {
		log.Printf("config path %v", path)
		vConfig.SetConfigFile(path)
	} else {
		vConfig.AddConfigPath("./env/")
		vConfig.SetConfigName("endpoint")
		vConfig.SetConfigType("yaml")
	}

	if err := vConfig.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			if path != "" {
				return nil, fmt.Errorf("read config file from ENV_PATH: %w", err)
			}
			return vConfig, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	return vConfig, nil
}

// Getting application config from viper
func GetApplicationConfig(v *viper.Viper) (*EndpointConfig, error) {
	var config EndpointConfig
	err := v.Unmarshal(&config)
	if err != nil {
		return nil, fmt.Errorf("unmarshal application config: %w", err)
	}

	validate := validator.New()
	sqlConfig, err := configs.ResolveSQLConfig(v, validate, config.PostgresConfig, config.SQLiteConfig)
	if err != nil {
		return nil, fmt.Errorf("resolve SQL config: %w", err)
	}
	switch cfg := sqlConfig.(type) {
	case *configs.PostgresConfig:
		config.PostgresConfig = cfg
		config.SQLiteConfig = nil
	case *configs.SQLiteConfig:
		config.SQLiteConfig = cfg
		config.PostgresConfig = nil
	}

	err = validate.Struct(&config)
	if err != nil {
		return nil, fmt.Errorf("validate application config: %w", err)
	}
	return &config, nil
}

func (c *EndpointConfig) SQLConfig() (configs.SQLConfig, error) {
	cfg, err := configs.SelectedSQLConfig(c.PostgresConfig, c.SQLiteConfig)
	if err != nil {
		return nil, fmt.Errorf("select SQL config: %w", err)
	}
	return cfg, nil
}
