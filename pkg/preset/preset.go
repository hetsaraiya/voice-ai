// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

// Package preset applies default values to request definitions.
package preset

import "github.com/rapidaai/protos"

// AssistantDefinition applies default values to an assistant definition.
func AssistantDefinition(assistant *protos.AssistantDefinition) {
	if assistant != nil && assistant.GetVersion() == "" {
		assistant.Version = "latest"
	}
}
