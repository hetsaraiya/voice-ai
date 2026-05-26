// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package preset

import (
	"testing"

	"github.com/rapidaai/protos"
)

func TestAssistantDefinition(t *testing.T) {
	t.Run("sets latest for empty version", func(t *testing.T) {
		assistant := &protos.AssistantDefinition{AssistantId: 1}

		AssistantDefinition(assistant)

		if assistant.GetVersion() != "latest" {
			t.Fatalf("version = %q, want latest", assistant.GetVersion())
		}
	})

	t.Run("keeps provided version", func(t *testing.T) {
		assistant := &protos.AssistantDefinition{AssistantId: 1, Version: "vrsn_12"}

		AssistantDefinition(assistant)

		if assistant.GetVersion() != "vrsn_12" {
			t.Fatalf("version = %q, want vrsn_12", assistant.GetVersion())
		}
	})

	t.Run("nil assistant", func(t *testing.T) {
		AssistantDefinition(nil)
	})
}
