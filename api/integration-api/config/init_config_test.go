// Copyright (c) 2023-2025 RapidaAI
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestInitConfig_ENVPathMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	t.Setenv("ENV_PATH", missing)

	_, err := InitConfig()
	if err == nil {
		t.Fatal("expected error when ENV_PATH points to missing file")
	}
	if !strings.Contains(err.Error(), "read config") {
		t.Fatalf("expected wrapped read error, got %v", err)
	}
}

func TestInitConfig_ENVPathInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("not: valid: yaml: [[["), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV_PATH", path)

	_, err := InitConfig()
	if err == nil {
		t.Fatal("expected read config error for invalid yaml")
	}
	if !strings.Contains(err.Error(), "read config") {
		t.Fatalf("expected wrapped read error, got %v", err)
	}
}

func TestInitConfig_DefaultPathMissingAllowed(t *testing.T) {
	t.Setenv("ENV_PATH", "")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	v, err := InitConfig()
	if err != nil {
		t.Fatalf("expected nil error when default config file is missing, got %v", err)
	}
	if v == nil {
		t.Fatal("expected viper instance")
	}
}

func TestIntegrationConfig_SQLConfig_Postgres(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(baseIntegrationYAML)); err != nil {
		t.Fatal(err)
	}
	cfg, err := GetApplicationConfig(v)
	if err != nil {
		t.Fatal(err)
	}
	sql, err := cfg.SQLConfig()
	if err != nil {
		t.Fatal(err)
	}
	if sql.DriverName() != "postgres" {
		t.Fatalf("DriverName() = %q, want postgres", sql.DriverName())
	}
}

func TestIntegrationConfig_SQLConfig_NoBackend(t *testing.T) {
	cfg := &IntegrationConfig{}
	_, err := cfg.SQLConfig()
	if err == nil {
		t.Fatal("expected error when no SQL backend configured")
	}
	if !strings.Contains(err.Error(), "select SQL config") {
		t.Fatalf("expected select SQL config wrap, got %v", err)
	}
}