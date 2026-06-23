// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_assistant_entity

import "testing"

func TestNewAssistantAuthenticationProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		want     AssistantAuthenticationProvider
		wantErr  bool
	}{
		{name: "valid http", provider: string(AssistantAuthenticationProviderHTTP), want: AssistantAuthenticationProviderHTTP},
		{name: "uppercase is invalid", provider: "HTTP", wantErr: true},
		{name: "leading space is invalid", provider: " http", wantErr: true},
		{name: "trailing space is invalid", provider: "http ", wantErr: true},
		{name: "empty is invalid", provider: "", wantErr: true},
		{name: "unknown is invalid", provider: "endpoint", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewAssistantAuthenticationProvider(tt.provider)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewAssistantAuthenticationProvider(%q) expected error", tt.provider)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewAssistantAuthenticationProvider(%q) unexpected error: %v", tt.provider, err)
			}
			if got != tt.want {
				t.Fatalf("NewAssistantAuthenticationProvider(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestNewAssistantWebhookProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		want     AssistantWebhookProvider
		wantErr  bool
	}{
		{name: "valid http", provider: string(AssistantWebhookProviderHTTP), want: AssistantWebhookProviderHTTP},
		{name: "uppercase is invalid", provider: "HTTP", wantErr: true},
		{name: "leading space is invalid", provider: " http", wantErr: true},
		{name: "trailing space is invalid", provider: "http ", wantErr: true},
		{name: "empty is invalid", provider: "", wantErr: true},
		{name: "unknown is invalid", provider: "endpoint", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewAssistantWebhookProvider(tt.provider)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewAssistantWebhookProvider(%q) expected error", tt.provider)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewAssistantWebhookProvider(%q) unexpected error: %v", tt.provider, err)
			}
			if got != tt.want {
				t.Fatalf("NewAssistantWebhookProvider(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestNewAssistantAnalysisProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		want     AssistantAnalysisProvider
		wantErr  bool
	}{
		{name: "valid endpoint", provider: string(AssistantAnalysisProviderEndpoint), want: AssistantAnalysisProviderEndpoint},
		{name: "uppercase is invalid", provider: "ENDPOINT", wantErr: true},
		{name: "leading space is invalid", provider: " endpoint", wantErr: true},
		{name: "trailing space is invalid", provider: "endpoint ", wantErr: true},
		{name: "empty is invalid", provider: "", wantErr: true},
		{name: "unknown is invalid", provider: "http", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewAssistantAnalysisProvider(tt.provider)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewAssistantAnalysisProvider(%q) expected error", tt.provider)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewAssistantAnalysisProvider(%q) unexpected error: %v", tt.provider, err)
			}
			if got != tt.want {
				t.Fatalf("NewAssistantAnalysisProvider(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}
