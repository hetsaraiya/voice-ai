// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

const baseIntegrationYAML = `
service_name: "Integration_Service"
host: "0.0.0.0"
port: 9004
secret: "rpd_pks"
log_level: "debug"
env: "development"

postgres:
  host: "localhost"
  port: 5432
  db_name: "integration_db"
  auth:
    user: "rapida_user"
    password: "rapida_db_password"
  max_open_connection: 10
  max_idle_connection: 10
  ssl_mode: "disable"

redis:
  host: "127.0.0.1"
  port: 6379
  db: 0
  max_connection: 10
  auth:
    user: ""
    password: ""

asset_store:
  storage_type: "local"
  storage_path_prefix: "/tmp/rapida-data/assets/integration"

integration:
  host: "localhost:9004"
endpoint:
  host: "localhost:9005"
assistant:
  host: "localhost:9007"
web:
  host: "localhost:9001"
document:
  host: "http://localhost:9010"
ui:
  host: "http://localhost:3000"
`

func TestInitConfig(t *testing.T) {
	configPath := filepath.Join(os.TempDir(), "integration_test.yaml")
	err := os.WriteFile(configPath, []byte(baseIntegrationYAML), 0o644)
	if err != nil {
		t.Fatalf("Failed to create mock config file: %v", err)
	}
	defer os.Remove(configPath)

	os.Setenv("ENV_PATH", configPath)
	defer os.Unsetenv("ENV_PATH")

	vConfig, err := InitConfig()
	if err != nil {
		t.Fatalf("InitConfig returned an error: %v", err)
	}
	if vConfig == nil {
		t.Fatalf("vConfig is nil")
	}
	if vConfig.ConfigFileUsed() != configPath {
		t.Errorf("Expected config file used to be %v, but got %v", configPath, vConfig.ConfigFileUsed())
	}

	appConfig, err := GetApplicationConfig(vConfig)
	if err != nil {
		t.Fatalf("GetApplicationConfig returned an error: %v", err)
	}
	if appConfig.PostgresConfig == nil || appConfig.PostgresConfig.DBName != "integration_db" {
		t.Fatalf("Expected PostgresConfig.DBName to be 'integration_db', got %#v", appConfig.PostgresConfig)
	}
	if appConfig.Assistant.Host != "localhost:9007" {
		t.Errorf("Expected Assistant.Host to be 'localhost:9007', but got %v", appConfig.Assistant.Host)
	}
}

func TestGetApplicationConfig(t *testing.T) {
	vConfig := viper.New()
	vConfig.SetConfigType("yaml")
	if err := vConfig.ReadConfig(strings.NewReader(baseIntegrationYAML)); err != nil {
		t.Fatalf("ReadConfig returned an error: %v", err)
	}

	appConfig, err := GetApplicationConfig(vConfig)
	if err != nil {
		t.Fatalf("GetApplicationConfig returned an error: %v", err)
	}
	if appConfig == nil {
		t.Fatalf("appConfig is nil")
	}

	if appConfig.PostgresConfig == nil {
		t.Fatalf("Expected PostgresConfig to be populated, got nil")
	} else if appConfig.PostgresConfig.DBName != "integration_db" {
		t.Errorf("Expected PostgresConfig.DBName to be 'integration_db', but got %q", appConfig.PostgresConfig.DBName)
	}
	if appConfig.AssetStoreConfig.StorageType != "local" {
		t.Errorf("Expected AssetStoreConfig.StorageType to be 'local', but got %v", appConfig.AssetStoreConfig.StorageType)
	}
	if appConfig.RedisConfig.Host != "127.0.0.1" || appConfig.RedisConfig.Port != 6379 {
		t.Errorf("Redis Config mismatch: Host=%v, Port=%v", appConfig.RedisConfig.Host, appConfig.RedisConfig.Port)
	}
	if appConfig.Integration.Host != "localhost:9004" {
		t.Errorf("Expected Integration.Host to be 'localhost:9004', but got %v", appConfig.Integration.Host)
	}
	if appConfig.Endpoint.Host != "localhost:9005" {
		t.Errorf("Expected Endpoint.Host to be 'localhost:9005', but got %v", appConfig.Endpoint.Host)
	}
	if appConfig.Assistant.Host != "localhost:9007" {
		t.Errorf("Expected Assistant.Host to be 'localhost:9007', but got %v", appConfig.Assistant.Host)
	}
	if appConfig.Web.Host != "localhost:9001" {
		t.Errorf("Expected Web.Host to be 'localhost:9001', but got %v", appConfig.Web.Host)
	}
	if appConfig.Document.Host != "http://localhost:9010" {
		t.Errorf("Expected Document.Host to be 'http://localhost:9010', but got %v", appConfig.Document.Host)
	}
	if appConfig.Ui.Host != "http://localhost:3000" {
		t.Errorf("Expected Ui.Host to be 'http://localhost:3000', but got %v", appConfig.Ui.Host)
	}
}

func TestGetApplicationConfig_SQLite(t *testing.T) {
	vConfig := viper.NewWithOptions(viper.KeyDelimiter("__"))
	vConfig.Set("SERVICE_NAME", "integration-api")
	vConfig.Set("HOST", "0.0.0.0")
	vConfig.Set("PORT", 9004)
	vConfig.Set("LOG_LEVEL", "debug")
	vConfig.Set("SECRET", "rpd_pks")
	vConfig.Set("ENV", "development")

	vConfig.Set("SQLITE__PATH", filepath.Join(t.TempDir(), "integration.db"))
	vConfig.Set("SQLITE__MAX_OPEN_CONNECTION", 1)
	vConfig.Set("SQLITE__MAX_IDLE_CONNECTION", 1)

	vConfig.Set("REDIS__HOST", "localhost")
	vConfig.Set("REDIS__PORT", "6379")
	vConfig.Set("REDIS__MAX_CONNECTION", 5)

	vConfig.Set("ASSET_STORE__STORAGE_TYPE", "local")
	vConfig.Set("ASSET_STORE__STORAGE_PATH_PREFIX", os.Getenv("HOME")+"/rapida-data/assets/integration")

	vConfig.Set("INTEGRATION__HOST", "localhost:9004")
	vConfig.Set("ENDPOINT__HOST", "localhost:9005")
	vConfig.Set("ASSISTANT__HOST", "localhost:9007")
	vConfig.Set("WEB__HOST", "localhost:9001")
	vConfig.Set("DOCUMENT__HOST", "http://localhost:9010")
	vConfig.Set("UI__HOST", "http://localhost:3000")

	appConfig, err := GetApplicationConfig(vConfig)
	if err != nil {
		t.Fatalf("GetApplicationConfig returned an error: %v", err)
	}
	if appConfig.SQLiteConfig == nil || appConfig.SQLiteConfig.Path == "" {
		t.Fatalf("expected SQLiteConfig to be populated, got %#v", appConfig.SQLiteConfig)
	}
	if appConfig.PostgresConfig != nil {
		t.Fatalf("expected PostgresConfig to be nil when sqlite is selected")
	}
}

func TestGetApplicationConfig_MultipleSQLConfigs(t *testing.T) {
	vConfig := viper.NewWithOptions(viper.KeyDelimiter("__"))
	vConfig.Set("SERVICE_NAME", "integration-api")
	vConfig.Set("HOST", "0.0.0.0")
	vConfig.Set("PORT", 9004)
	vConfig.Set("LOG_LEVEL", "debug")
	vConfig.Set("SECRET", "rpd_pks")
	vConfig.Set("ENV", "development")

	vConfig.Set("POSTGRES__HOST", "localhost")
	vConfig.Set("POSTGRES__DB_NAME", "integration_db")
	vConfig.Set("POSTGRES__AUTH__USER", "rapida_user")
	vConfig.Set("POSTGRES__AUTH__PASSWORD", "rapida_db_password")
	vConfig.Set("POSTGRES__PORT", 5432)
	vConfig.Set("POSTGRES__MAX_OPEN_CONNECTION", 10)
	vConfig.Set("POSTGRES__MAX_IDLE_CONNECTION", 10)
	vConfig.Set("POSTGRES__SSL_MODE", "disable")

	vConfig.Set("SQLITE__PATH", filepath.Join(t.TempDir(), "integration.db"))
	vConfig.Set("SQLITE__MAX_OPEN_CONNECTION", 1)
	vConfig.Set("SQLITE__MAX_IDLE_CONNECTION", 1)

	vConfig.Set("REDIS__HOST", "localhost")
	vConfig.Set("REDIS__PORT", "6379")
	vConfig.Set("REDIS__MAX_CONNECTION", 5)
	vConfig.Set("ASSET_STORE__STORAGE_TYPE", "local")
	vConfig.Set("ASSET_STORE__STORAGE_PATH_PREFIX", os.Getenv("HOME")+"/rapida-data/assets/integration")
	vConfig.Set("INTEGRATION__HOST", "localhost:9004")
	vConfig.Set("ENDPOINT__HOST", "localhost:9005")
	vConfig.Set("ASSISTANT__HOST", "localhost:9007")
	vConfig.Set("WEB__HOST", "localhost:9001")
	vConfig.Set("DOCUMENT__HOST", "http://localhost:9010")
	vConfig.Set("UI__HOST", "http://localhost:3000")

	_, err := GetApplicationConfig(vConfig)
	if err == nil || !strings.Contains(err.Error(), "set only one") {
		t.Fatalf("expected multi-sql validation error, got %v", err)
	}
}
