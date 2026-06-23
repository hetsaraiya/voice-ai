// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_assistant_entity

import "fmt"

type AssistantAuthenticationProvider string
type AssistantWebhookProvider string
type AssistantAnalysisProvider string

const (
	AssistantAuthenticationProviderHTTP AssistantAuthenticationProvider = "http"
	AssistantWebhookProviderHTTP        AssistantWebhookProvider        = "http"
	AssistantAnalysisProviderEndpoint   AssistantAnalysisProvider       = "endpoint"
)

func NewAssistantAuthenticationProvider(provider string) (AssistantAuthenticationProvider, error) {
	switch AssistantAuthenticationProvider(provider) {
	case AssistantAuthenticationProviderHTTP:
		return AssistantAuthenticationProviderHTTP, nil
	default:
		return "", fmt.Errorf("invalid assistant authentication provider %q", provider)
	}
}

func NewAssistantWebhookProvider(provider string) (AssistantWebhookProvider, error) {
	switch AssistantWebhookProvider(provider) {
	case AssistantWebhookProviderHTTP:
		return AssistantWebhookProviderHTTP, nil
	default:
		return "", fmt.Errorf("invalid assistant webhook provider %q", provider)
	}
}

func NewAssistantAnalysisProvider(provider string) (AssistantAnalysisProvider, error) {
	switch AssistantAnalysisProvider(provider) {
	case AssistantAnalysisProviderEndpoint:
		return AssistantAnalysisProviderEndpoint, nil
	default:
		return "", fmt.Errorf("invalid assistant analysis provider %q", provider)
	}
}
