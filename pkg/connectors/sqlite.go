// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package connectors

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	commons "github.com/rapidaai/pkg/commons"
	configs "github.com/rapidaai/pkg/configs"
)

type sqliteConnector struct {
	logger commons.Logger
	cfg    *configs.SQLiteConfig
	db     *gorm.DB
}

func NewSQLiteConnector(config *configs.SQLiteConfig, logger commons.Logger) SQLConnector {
	return &sqliteConnector{cfg: config, logger: logger}
}

func (sqlt *sqliteConnector) DB(ctx context.Context) *gorm.DB {
	return sqlt.db.WithContext(ctx)
}

func (sqlt *sqliteConnector) Connect(ctx context.Context) error {
	if err := ensureSQLiteParentDir(sqlt.cfg.Path); err != nil {
		return err
	}

	lgr := logger.Discard.LogMode(logger.Silent)
	db, err := gorm.Open(sqlite.Open(sqlt.cfg.Path), &gorm.Config{
		Logger: lgr,
	})
	if err != nil {
		sqlt.logger.Errorf("Failed to open sqlite connection %s.", err)
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		sqlt.logger.Errorf("Failed to create sqlite client connection pool %s.", err)
		return err
	}
	sqlDB.SetMaxIdleConns(sqlt.cfg.MaxIdealConnection)
	sqlDB.SetMaxOpenConns(sqlt.cfg.MaxOpenConnection)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		sqlt.logger.Errorf("Failed to enable sqlite foreign keys %s.", err)
		return err
	}

	sqlt.db = db
	return nil
}

func (sqlt *sqliteConnector) Name() string {
	return fmt.Sprintf("SQLITE sqlite3://%s", sqlt.cfg.Path)
}

func (sqlt *sqliteConnector) IsConnected(ctx context.Context) bool {
	if sqlt.db == nil {
		return false
	}
	db, err := sqlt.db.DB()
	if err != nil {
		sqlt.logger.Errorf("Failed to get sqlite client %s.", err)
		return false
	}
	if err := db.PingContext(ctx); err != nil {
		sqlt.logger.Errorf("Failed to ping sqlite client %s.", err)
		return false
	}
	return true
}

func (sqlt *sqliteConnector) Disconnect(ctx context.Context) error {
	sqlt.logger.Debug("Disconnecting sqlite client.")
	db, err := sqlt.db.DB()
	sqlt.db = nil
	if err != nil {
		sqlt.logger.Errorf("Disconnecting sqlite client %s.", err)
		return err
	}
	if err := db.Close(); err != nil {
		sqlt.logger.Debug("Disconnecting sqlite client %s.", err)
		return err
	}
	return nil
}

func (sqlt *sqliteConnector) Query(ctx context.Context, qry string, dest interface{}) error {
	tx := sqlt.db.WithContext(ctx).Raw(qry).Scan(dest)
	return tx.Error
}

func ensureSQLiteParentDir(path string) error {
	if path == "" || path == ":memory:" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
