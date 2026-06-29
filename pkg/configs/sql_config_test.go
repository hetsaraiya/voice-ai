package configs

import (
	"net/url"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

func TestResolveSQLConfig(t *testing.T) {
	validate := validator.New()

	t.Run("postgres", func(t *testing.T) {
		v := viper.NewWithOptions(viper.KeyDelimiter("__"))
		v.Set("POSTGRES__HOST", "localhost")
		v.Set("POSTGRES__DB_NAME", "app")
		v.Set("POSTGRES__AUTH__USER", "user")
		v.Set("POSTGRES__AUTH__PASSWORD", "pass")
		v.Set("POSTGRES__PORT", 5432)
		v.Set("POSTGRES__MAX_OPEN_CONNECTION", 10)
		v.Set("POSTGRES__MAX_IDLE_CONNECTION", 5)
		v.Set("POSTGRES__SSL_MODE", "disable")

		cfg, err := ResolveSQLConfig(v, validate,
			&PostgresConfig{
				Host:              "localhost",
				DBName:            "app",
				Auth:              BasicAuth{User: "user", Password: "pass"},
				Port:              5432,
				MaxOpenConnection: 10,
				MaxIdleConnection: 5,
				SslMode:           "disable",
			},
			nil,
		)
		if err != nil {
			t.Fatalf("ResolveSQLConfig returned error: %v", err)
		}
		if cfg.DriverName() != "postgres" {
			t.Fatalf("DriverName() = %q, want %q", cfg.DriverName(), "postgres")
		}
	})

	t.Run("sqlite", func(t *testing.T) {
		v := viper.NewWithOptions(viper.KeyDelimiter("__"))
		v.Set("SQLITE__PATH", "/tmp/rapida.db")
		v.Set("SQLITE__MAX_OPEN_CONNECTION", 1)
		v.Set("SQLITE__MAX_IDLE_CONNECTION", 1)

		cfg, err := ResolveSQLConfig(v, validate, nil, &SQLiteConfig{
			Path:              "/tmp/rapida.db",
			MaxOpenConnection: 1,
			MaxIdleConnection: 1,
		})
		if err != nil {
			t.Fatalf("ResolveSQLConfig returned error: %v", err)
		}
		if cfg.DriverName() != "sqlite" {
			t.Fatalf("DriverName() = %q, want %q", cfg.DriverName(), "sqlite")
		}
	})

	t.Run("missing", func(t *testing.T) {
		v := viper.NewWithOptions(viper.KeyDelimiter("__"))
		_, err := ResolveSQLConfig(v, validate, nil, nil)
		if err != ErrNoSQLConfigConfigured {
			t.Fatalf("ResolveSQLConfig() error = %v, want %v", err, ErrNoSQLConfigConfigured)
		}
	})

	t.Run("multiple", func(t *testing.T) {
		v := viper.NewWithOptions(viper.KeyDelimiter("__"))
		v.Set("POSTGRES__HOST", "localhost")
		v.Set("SQLITE__PATH", "/tmp/rapida.db")

		_, err := ResolveSQLConfig(v, validate, &PostgresConfig{}, &SQLiteConfig{})
		if err != ErrMultipleSQLConfigsDetected {
			t.Fatalf("ResolveSQLConfig() error = %v, want %v", err, ErrMultipleSQLConfigsDetected)
		}
	})
}

func TestSelectedSQLConfig(t *testing.T) {
	t.Run("postgres", func(t *testing.T) {
		cfg, err := SelectedSQLConfig(&PostgresConfig{DBName: "app"}, nil)
		if err != nil || cfg == nil || cfg.DriverName() != "postgres" {
			t.Fatalf("SelectedSQLConfig() = %#v, %v, want postgres config", cfg, err)
		}
	})

	t.Run("sqlite", func(t *testing.T) {
		cfg, err := SelectedSQLConfig(nil, &SQLiteConfig{Path: "/tmp/rapida.db"})
		if err != nil || cfg == nil || cfg.DriverName() != "sqlite" {
			t.Fatalf("SelectedSQLConfig() = %#v, %v, want sqlite config", cfg, err)
		}
	})

	t.Run("multiple", func(t *testing.T) {
		_, err := SelectedSQLConfig(&PostgresConfig{DBName: "app"}, &SQLiteConfig{Path: "/tmp/rapida.db"})
		if err != ErrMultipleSQLConfigsDetected {
			t.Fatalf("SelectedSQLConfig() error = %v, want %v", err, ErrMultipleSQLConfigsDetected)
		}
	})

	t.Run("none", func(t *testing.T) {
		cfg, err := SelectedSQLConfig(nil, nil)
		if err != ErrNoSQLConfigConfigured {
			t.Fatalf("SelectedSQLConfig() error = %v, want %v", err, ErrNoSQLConfigConfigured)
		}
		if cfg != nil {
			t.Fatalf("SelectedSQLConfig() = %#v, want nil config", cfg)
		}
	})
}

func TestPostgresConfig_MigrationDSN(t *testing.T) {
	cfg := &PostgresConfig{
		Host:              "localhost",
		Port:              5432,
		DBName:            "mydb",
		Auth:              BasicAuth{User: "user@x", Password: "p:a ss"},
		SslMode:           "disable",
		MaxIdleConnection: 1,
		MaxOpenConnection: 2,
	}
	dsn := cfg.MigrationDSN()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if u.Scheme != "postgres" {
		t.Fatalf("scheme = %q", u.Scheme)
	}
	if !strings.Contains(dsn, "mydb") {
		t.Fatalf("dsn missing db name: %s", dsn)
	}
	if u.Query().Get("sslmode") != "disable" {
		t.Fatalf("sslmode = %q", u.Query().Get("sslmode"))
	}
}

func TestSQLiteConfig_MigrationDSN(t *testing.T) {
	cfg := &SQLiteConfig{Path: "/tmp/rapida.db", MaxIdleConnection: 1, MaxOpenConnection: 1}
	want := "sqlite3:///tmp/rapida.db"
	if got := cfg.MigrationDSN(); got != want {
		t.Fatalf("MigrationDSN() = %q, want %q", got, want)
	}
}