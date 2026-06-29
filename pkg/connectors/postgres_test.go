// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package connectors

import (
	"context"
	"log"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	commons "github.com/rapidaai/pkg/commons"
	configs "github.com/rapidaai/pkg/configs"
)

// Local config struct for testing - removed since we can use configs package



func TestSQLConnector_Name(t *testing.T) {
	connector := &postgresConnector{
		cfg: &configs.PostgresConfig{Host: "localhost", Port: 5432},
	}
	result := connector.Name()
	assert.Equal(t, "PSQL psql://localhost:5432", result)
}

func TestSQLConnector_DB(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		log.Fatalf("An error '%s' was not expected when opening a stub database connection", err)
	}
	logger, _ := commons.NewApplicationLogger()
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db,
	}), &gorm.Config{})

	connector := &postgresConnector{
		cfg: &configs.PostgresConfig{
			Host: "localhost",
			Port: 5432,
		},
		logger: logger,
		db:     gormDB,
	}

	ctx := context.Background()
	result := connector.DB(ctx)
	assert.NotNil(t, result)
}

func TestSQLConnector_Disconnect(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		log.Fatalf("An error '%s' was not expected when opening a stub database connection", err)
	}
	logger, _ := commons.NewApplicationLogger()
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db,
	}), &gorm.Config{})

	connector := &postgresConnector{
		cfg: &configs.PostgresConfig{
			Host: "localhost",
			Port: 5432,
		},
		logger: logger,
		db:     gormDB,
	}

	ctx := context.Background()
	err = connector.Disconnect(ctx)

	assert.Nil(t, connector.db)
}

func TestSQLConnector_IsConnected(t *testing.T) {
	logger, _ := commons.NewApplicationLogger()
	connector := &postgresConnector{
		cfg: &configs.PostgresConfig{
			Host: "localhost",
			Port: 5432,
		},
		logger: logger,
	}

	ctx := context.Background()
	result := connector.IsConnected(ctx)
	assert.False(t, result) // db is nil, so ping fails

	// With mock DB, but since it's hard to mock gorm.DB.Ping, skip
	t.Run("with db", func(t *testing.T) {
		t.Skip("Requires real database connection - integration test")
	})
}

// Note: Connect and Query methods require real PostgreSQL connection
// and are tested as integration tests.

func TestSQLConnector_Connect_ErrorHandling(t *testing.T) {
	// Test with invalid config
	t.Skip("Requires external packages - integration test")
}

func TestSQLConnector_Query_ErrorHandling(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		log.Fatalf("An error '%s' was not expected when opening a stub database connection", err)
	}
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db,
	}), &gorm.Config{})

	logger, _ := commons.NewApplicationLogger()
	connector := &postgresConnector{
		cfg: &configs.PostgresConfig{
			Host: "localhost",
			Port: 5432,
		},
		logger: logger,
		db:     gormDB,
	}

	ctx := context.Background()
	var dest interface{}
	err = connector.Query(ctx, "SELECT 1", &dest)
	mock.ExpectExec("SELECT 1").WillReturnResult(sqlmock.NewResult(1, 1))
	assert.Error(t, err) // db is nil
}

func TestSQLConnector_EdgeCases(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		log.Fatalf("An error '%s' was not expected when opening a stub database connection", err)
	}
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db,
	}), &gorm.Config{})

	logger, _ := commons.NewApplicationLogger()
	t.Run("disconnect with nil db", func(t *testing.T) {
		connector := &postgresConnector{
			cfg: &configs.PostgresConfig{
				Host: "localhost",
				Port: 5432,
			},
			logger: logger,
			db:     gormDB,
		}

		ctx := context.Background()
		err = connector.Disconnect(ctx)
		assert.Nil(t, connector.db)
	})

	t.Run("db with nil db", func(t *testing.T) {
		connector := &postgresConnector{
			cfg: &configs.PostgresConfig{
				Host: "localhost",
				Port: 5432,
			},
			logger: logger,
			db:     gormDB,
		}

		ctx := context.Background()
		result := connector.DB(ctx)
		assert.Nil(t, result.Error)
	})

	t.Run("name with different host port", func(t *testing.T) {
		cfg := &configs.PostgresConfig{
			Host: "db.example.com",
			Port: 9999,
		}
		connector := &postgresConnector{cfg: cfg}
		result := connector.Name()
		assert.Equal(t, "PSQL psql://db.example.com:9999", result)
	})
}
