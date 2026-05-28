// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_llm_agentkit

import (
	"github.com/rapidaai/pkg/commons"
)

func NewAgentKitAssistantExecutor(logger commons.Logger) *agentkitExecutor {
	return &agentkitExecutor{logger: logger}
}

func (e *agentkitExecutor) Name() string { return "agentkit" }
