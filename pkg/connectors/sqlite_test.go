package connectors

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commons "github.com/rapidaai/pkg/commons"
	configs "github.com/rapidaai/pkg/configs"
)

func TestSQLiteConnector_Lifecycle(t *testing.T) {
	logger, _ := commons.NewApplicationLogger()
	path := filepath.Join(t.TempDir(), "data", "rapida.db")
	connector := NewSQLiteConnector(&configs.SQLiteConfig{
		Path:               path,
		MaxIdealConnection: 1,
		MaxOpenConnection:  1,
	}, logger)

	ctx := context.Background()
	require.NoError(t, connector.Connect(ctx))
	assert.True(t, connector.IsConnected(ctx))
	assert.Equal(t, "SQLITE sqlite3://"+path, connector.Name())

	db := connector.DB(ctx)
	require.NoError(t, db.Exec("CREATE TABLE test_items (name TEXT NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO test_items(name) VALUES (?)", "sqlite").Error)

	var rows []struct {
		Name string
	}
	require.NoError(t, connector.Query(ctx, "SELECT name FROM test_items", &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "sqlite", rows[0].Name)
	assert.FileExists(t, path)

	require.NoError(t, connector.Disconnect(ctx))
	assert.False(t, connector.IsConnected(ctx))
}

func TestNewSQLConnector_SQLite(t *testing.T) {
	logger, _ := commons.NewApplicationLogger()
	connector := NewSQLConnector(&configs.SQLiteConfig{
		Path:               filepath.Join(t.TempDir(), "rapida.db"),
		MaxIdealConnection: 1,
		MaxOpenConnection:  1,
	}, logger)
	require.NotNil(t, connector)
}

func TestEnsureSQLiteParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "sqlite.db")
	require.NoError(t, ensureSQLiteParentDir(path))
	_, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
}
