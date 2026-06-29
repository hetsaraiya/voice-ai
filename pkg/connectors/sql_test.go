package connectors

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commons "github.com/rapidaai/pkg/commons"
	configs "github.com/rapidaai/pkg/configs"
)

func TestNewSQLConnector_PostgresDispatch(t *testing.T) {
	logger, _ := commons.NewApplicationLogger()
	cfg := &configs.PostgresConfig{
		Host:              "localhost",
		DBName:            "app",
		Port:              5432,
		MaxIdleConnection: 1,
		MaxOpenConnection: 2,
		SslMode:           "disable",
		Auth:              configs.BasicAuth{User: "u", Password: "p"},
	}
	connector, err := NewSQLConnector(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, connector)
	assert.Contains(t, connector.Name(), "PSQL")
}

func TestNewSQLConnector_SQLiteDispatch(t *testing.T) {
	logger, _ := commons.NewApplicationLogger()
	cfg := &configs.SQLiteConfig{
		Path:              filepath.Join(t.TempDir(), "factory.db"),
		MaxIdleConnection: 1,
		MaxOpenConnection: 1,
	}
	connector, err := NewSQLConnector(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, connector)
	ctx := context.Background()
	require.NoError(t, connector.Connect(ctx))
	assert.Contains(t, connector.Name(), "SQLITE")
}

func TestNewSQLConnector_NilConfig(t *testing.T) {
	logger, _ := commons.NewApplicationLogger()
	_, err := NewSQLConnector(nil, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestNewSQLConnector_UnsupportedConfigType(t *testing.T) {
	logger, _ := commons.NewApplicationLogger()
	_, err := NewSQLConnector(unsupportedConfig{}, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported SQL config type")
}

type unsupportedConfig struct{}

func (unsupportedConfig) DriverName() string            { return "unsupported" }
func (unsupportedConfig) MigrationDriverName() string   { return "unsupported" }
func (unsupportedConfig) MigrationDSN() string          { return "" }
func (unsupportedConfig) DisplayName() string           { return "unsupported" }