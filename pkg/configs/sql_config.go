// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package configs

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

var (
	ErrNoSQLConfigConfigured      = errors.New("no SQL configuration found; set exactly one of POSTGRES__* or SQLITE__*")
	ErrMultipleSQLConfigsDetected = errors.New("multiple SQL configurations found; set only one of POSTGRES__* or SQLITE__*")
)

type SQLConfig interface {
	DriverName() string
	MigrationDriverName() string
	MigrationDSN() string
	DisplayName() string
}

func (c *PostgresConfig) DriverName() string {
	return "postgres"
}

func (c *PostgresConfig) MigrationDriverName() string {
	return "postgres"
}

func (c *PostgresConfig) MigrationDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.Auth.User,
		c.Auth.Password,
		c.Host,
		c.Port,
		c.DBName,
		c.SslMode,
	)
}

func (c *PostgresConfig) DisplayName() string {
	return fmt.Sprintf("postgres://%s:%d/%s", c.Host, c.Port, c.DBName)
}

func (c *SQLiteConfig) DriverName() string {
	return "sqlite"
}

func (c *SQLiteConfig) MigrationDriverName() string {
	return "sqlite3"
}

func (c *SQLiteConfig) MigrationDSN() string {
	return "sqlite3://" + c.Path
}

func (c *SQLiteConfig) DisplayName() string {
	return "sqlite3://" + c.Path
}

func ResolveSQLConfig(v *viper.Viper, validate *validator.Validate, postgres *PostgresConfig, sqlite *SQLiteConfig) (SQLConfig, error) {
	postgresConfigured, sqliteConfigured := detectSQLPrefixes(v)

	switch {
	case postgresConfigured && sqliteConfigured:
		return nil, ErrMultipleSQLConfigsDetected
	case postgresConfigured:
		if postgres == nil {
			postgres = &PostgresConfig{}
		}
		if err := validate.Struct(postgres); err != nil {
			return nil, fmt.Errorf("invalid POSTGRES configuration: %w", err)
		}
		return postgres, nil
	case sqliteConfigured:
		if sqlite == nil {
			sqlite = &SQLiteConfig{}
		}
		if err := validate.Struct(sqlite); err != nil {
			return nil, fmt.Errorf("invalid SQLITE configuration: %w", err)
		}
		return sqlite, nil
	default:
		return nil, ErrNoSQLConfigConfigured
	}
}

func SelectedSQLConfig(postgres *PostgresConfig, sqlite *SQLiteConfig) SQLConfig {
	if postgres != nil && sqlite != nil {
		panic("SelectedSQLConfig requires exactly one SQL config; both postgres and sqlite were provided")
	}
	if sqlite != nil {
		return sqlite
	}
	return postgres
}

func detectSQLPrefixes(v *viper.Viper) (bool, bool) {
	return hasConfigPrefix(v, "POSTGRES"), hasConfigPrefix(v, "SQLITE")
}

func hasConfigPrefix(v *viper.Viper, prefix string) bool {
	prefix = strings.ToUpper(prefix) + "__"
	for _, key := range v.AllKeys() {
		normalizedKey := strings.ToUpper(strings.ReplaceAll(key, ".", "__"))
		if strings.HasPrefix(normalizedKey, prefix) {
			return true
		}
	}
	for _, env := range os.Environ() {
		name, _, found := strings.Cut(env, "=")
		if found && strings.HasPrefix(strings.ToUpper(name), prefix) {
			return true
		}
	}
	return false
}
