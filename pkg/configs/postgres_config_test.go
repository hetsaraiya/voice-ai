// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package configs

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

func TestPostgresConfig_Validation(t *testing.T) {
	validate := validator.New()
	tests := []struct {
		name    string
		cfg     PostgresConfig
		wantErr bool
	}{
		{"valid", PostgresConfig{Host: "localhost", DBName: "test", MaxIdleConnection: 5, MaxOpenConnection: 10, SslMode: "disable"}, false},
		{"invalid missing host", PostgresConfig{DBName: "test", MaxIdleConnection: 5, MaxOpenConnection: 10, SslMode: "disable"}, true},
		{"invalid missing db_name", PostgresConfig{Host: "localhost", MaxIdleConnection: 5, MaxOpenConnection: 10, SslMode: "disable"}, true},
		{"zero pool sizes allowed", PostgresConfig{Host: "localhost", DBName: "test", MaxIdleConnection: 0, MaxOpenConnection: 0, SslMode: "disable"}, false},
		{"invalid missing ssl_mode", PostgresConfig{Host: "localhost", DBName: "test", MaxIdleConnection: 5, MaxOpenConnection: 10}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validation error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPostgresConfig_FromViper(t *testing.T) {
	v := viper.New()
	v.Set("host", "db.example.com")
	v.Set("port", 5432)
	v.Set("auth.user", "postgres")
	v.Set("auth.password", "secret")
	v.Set("db_name", "mydb")
	v.Set("max_idle_connection", 10)
	v.Set("max_open_connection", 20)
	v.Set("ssl_mode", "require")

	var cfg PostgresConfig
	err := v.Unmarshal(&cfg)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if cfg.Host != "db.example.com" {
		t.Errorf("Host = %v, want db.example.com", cfg.Host)
	}
	if cfg.Port != 5432 {
		t.Errorf("Port = %v, want 5432", cfg.Port)
	}
	if cfg.Auth.User != "postgres" {
		t.Errorf("Auth.User = %v, want postgres", cfg.Auth.User)
	}
	if cfg.Auth.Password != "secret" {
		t.Errorf("Auth.Password = %v, want secret", cfg.Auth.Password)
	}
	if cfg.DBName != "mydb" {
		t.Errorf("DBName = %v, want mydb", cfg.DBName)
	}
	if cfg.MaxIdleConnection != 10 {
		t.Errorf("MaxIdleConnection = %v, want 10", cfg.MaxIdleConnection)
	}
	if cfg.MaxOpenConnection != 20 {
		t.Errorf("MaxOpenConnection = %v, want 20", cfg.MaxOpenConnection)
	}
	if cfg.SslMode != "require" {
		t.Errorf("SslMode = %v, want require", cfg.SslMode)
	}
	if cfg.SLCache != nil {
		t.Errorf("SLCache should be nil")
	}
}

func TestPostgresConfig_Defaults(t *testing.T) {
	v := viper.New()
	// no sets

	var cfg PostgresConfig
	err := v.Unmarshal(&cfg)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if cfg.Host != "" {
		t.Errorf("Host = %v, want empty", cfg.Host)
	}
	if cfg.Port != 0 {
		t.Errorf("Port = %v, want 0", cfg.Port)
	}
	if cfg.Auth.User != "" {
		t.Errorf("Auth.User = %v, want empty", cfg.Auth.User)
	}
	if cfg.Auth.Password != "" {
		t.Errorf("Auth.Password = %v, want empty", cfg.Auth.Password)
	}
	if cfg.DBName != "" {
		t.Errorf("DBName = %v, want empty", cfg.DBName)
	}
	if cfg.MaxIdleConnection != 0 {
		t.Errorf("MaxIdleConnection = %v, want 0", cfg.MaxIdleConnection)
	}
	if cfg.MaxOpenConnection != 0 {
		t.Errorf("MaxOpenConnection = %v, want 0", cfg.MaxOpenConnection)
	}
	if cfg.SslMode != "" {
		t.Errorf("SslMode = %v, want empty", cfg.SslMode)
	}
	if cfg.SLCache != nil {
		t.Errorf("SLCache should be nil")
	}
}
