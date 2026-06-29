package config

import (
	"log"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"

	config "github.com/rapidaai/config"
	"github.com/rapidaai/pkg/configs"
)

// Application config structure
type IntegrationConfig struct {
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
		vConfig.SetConfigName("integration")
		vConfig.SetConfigType("yaml")
	}

	if err := vConfig.ReadInConfig(); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Error while reading the config: %v", err)
		}
	}

	return vConfig, nil
}

// Getting application config from viper
func GetApplicationConfig(v *viper.Viper) (*IntegrationConfig, error) {
	var config IntegrationConfig
	err := v.Unmarshal(&config)
	if err != nil {
		log.Printf("%+v\n", err)
		return nil, err
	}

	// valdating the app config
	validate := validator.New()
	sqlConfig, err := configs.ResolveSQLConfig(v, validate, config.PostgresConfig, config.SQLiteConfig)
	if err != nil {
		log.Printf("%+v\n", err)
		return nil, err
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
		log.Printf("%+v\n", err)
		return nil, err
	}
	return &config, nil
}

func (c *IntegrationConfig) SQLConfig() (configs.SQLConfig, error) {
	return configs.SelectedSQLConfig(c.PostgresConfig, c.SQLiteConfig)
}
