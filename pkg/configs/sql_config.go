// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package configs

import (
	"errors"
	"fmt"
	"net/url"
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
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.Auth.User, c.Auth.Password),
		Host:   fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:   "/" + c.DBName,
	}
	q := url.Values{}
	q.Set("sslmode", c.SslMode)
	u.RawQuery = q.Encode()
	return u.String()
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

func SelectedSQLConfig(postgres *PostgresConfig, sqlite *SQLiteConfig) (SQLConfig, error) {
	switch {
	case postgres != nil && sqlite != nil:
		return nil, ErrMultipleSQLConfigsDetected
	case sqlite != nil:
		return sqlite, nil
	case postgres != nil:
		return postgres, nil
	default:
		return nil, ErrNoSQLConfigConfigured
	}
}

func detectSQLPrefixes(v *viper.Viper) (bool, bool) {
	return hasConfigPrefix(v, "POSTGRES"), hasConfigPrefix(v, "SQLITE")
}

func hasConfigPrefix(v *viper.Viper, prefix string) bool {
	prefix = strings.ToUpper(prefix) + "__"
	for _, key := range v.AllKeys() {
		normalizedKey := strings.ToUpper(strings.ReplaceAll(key, ".", "__"))
		if strings.HasPrefix(normalizedKey, prefix) && v.IsSet(key) {
			return true
		}
	}
	return false
}