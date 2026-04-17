package configs

import (
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
		v.Set("POSTGRES__MAX_IDEAL_CONNECTION", 5)
		v.Set("POSTGRES__SSL_MODE", "disable")

		cfg, err := ResolveSQLConfig(v, validate,
			&PostgresConfig{
				Host:               "localhost",
				DBName:             "app",
				Auth:               BasicAuth{User: "user", Password: "pass"},
				Port:               5432,
				MaxOpenConnection:  10,
				MaxIdealConnection: 5,
				SslMode:            "disable",
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
		v.Set("SQLITE__MAX_IDEAL_CONNECTION", 1)

		cfg, err := ResolveSQLConfig(v, validate, nil, &SQLiteConfig{
			Path:               "/tmp/rapida.db",
			MaxOpenConnection:  1,
			MaxIdealConnection: 1,
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
		cfg := SelectedSQLConfig(&PostgresConfig{DBName: "app"}, nil)
		if cfg == nil || cfg.DriverName() != "postgres" {
			t.Fatalf("SelectedSQLConfig() = %#v, want postgres config", cfg)
		}
	})

	t.Run("sqlite", func(t *testing.T) {
		cfg := SelectedSQLConfig(nil, &SQLiteConfig{Path: "/tmp/rapida.db"})
		if cfg == nil || cfg.DriverName() != "sqlite" {
			t.Fatalf("SelectedSQLConfig() = %#v, want sqlite config", cfg)
		}
	})

	t.Run("multiple panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("SelectedSQLConfig() did not panic when both configs were provided")
			}
		}()
		_ = SelectedSQLConfig(&PostgresConfig{DBName: "app"}, &SQLiteConfig{Path: "/tmp/rapida.db"})
	})
}
