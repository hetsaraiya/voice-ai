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

func TestOpenSearchConfig_Validation(t *testing.T) {
	validate := validator.New()
	tests := []struct {
		name    string
		cfg     OpenSearchConfig
		wantErr bool
	}{
		{"valid", OpenSearchConfig{Schema: "https", Host: "localhost", MaxRetries: 3, MaxConnection: 10}, false},
		{"invalid missing schema", OpenSearchConfig{Host: "localhost", MaxRetries: 3, MaxConnection: 10}, true},
		{"invalid missing host", OpenSearchConfig{Schema: "https", MaxRetries: 3, MaxConnection: 10}, true},
		{"valid missing max_retries", OpenSearchConfig{Schema: "https", Host: "localhost", MaxConnection: 10}, false},
		{"valid missing max_connection", OpenSearchConfig{Schema: "https", Host: "localhost", MaxRetries: 3}, false},
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

func TestOpenSearchConfig_FromViper(t *testing.T) {
	v := viper.New()
	v.Set("schema", "https")
	v.Set("host", "opensearch.example.com")
	port := 9200
	v.Set("port", port)
	v.Set("auth.user", "user")
	v.Set("auth.password", "pass")
	v.Set("max_retries", 5)
	v.Set("max_connection", 20)

	var cfg OpenSearchConfig
	err := v.Unmarshal(&cfg)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if cfg.Schema != "https" {
		t.Errorf("Schema = %v, want https", cfg.Schema)
	}
	if cfg.Host != "opensearch.example.com" {
		t.Errorf("Host = %v, want opensearch.example.com", cfg.Host)
	}
	if cfg.Port == nil || *cfg.Port != 9200 {
		t.Errorf("Port = %v, want 9200", cfg.Port)
	}
	if cfg.Auth.User != "user" {
		t.Errorf("Auth.User = %v, want user", cfg.Auth.User)
	}
	if cfg.Auth.Password != "pass" {
		t.Errorf("Auth.Password = %v, want pass", cfg.Auth.Password)
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries = %v, want 5", cfg.MaxRetries)
	}
	if cfg.MaxConnection != 20 {
		t.Errorf("MaxConnection = %v, want 20", cfg.MaxConnection)
	}
}

func TestOpenSearchConfig_Defaults(t *testing.T) {
	v := viper.New()
	// no sets

	var cfg OpenSearchConfig
	err := v.Unmarshal(&cfg)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if cfg.Schema != "" {
		t.Errorf("Schema = %v, want empty", cfg.Schema)
	}
	if cfg.Host != "" {
		t.Errorf("Host = %v, want empty", cfg.Host)
	}
	if cfg.Port != nil {
		t.Errorf("Port should be nil")
	}
	if cfg.Auth.User != "" {
		t.Errorf("Auth.User = %v, want empty", cfg.Auth.User)
	}
	if cfg.Auth.Password != "" {
		t.Errorf("Auth.Password = %v, want empty", cfg.Auth.Password)
	}
	if cfg.MaxRetries != 0 {
		t.Errorf("MaxRetries = %v, want 0", cfg.MaxRetries)
	}
	if cfg.MaxConnection != 0 {
		t.Errorf("MaxConnection = %v, want 0", cfg.MaxConnection)
	}
}

func TestOpenSearchConfig_Configured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *OpenSearchConfig
		want bool
	}{
		{name: "nil", cfg: nil, want: false},
		{name: "missing schema", cfg: &OpenSearchConfig{Host: "localhost"}, want: false},
		{name: "missing host", cfg: &OpenSearchConfig{Schema: "http"}, want: false},
		{name: "blank values", cfg: &OpenSearchConfig{Schema: " ", Host: "\t"}, want: false},
		{name: "configured", cfg: &OpenSearchConfig{Schema: "http", Host: "localhost"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsValid(); got != tt.want {
				t.Fatalf("IsValid() = %t, want %t", got, tt.want)
			}
		})
	}
}
