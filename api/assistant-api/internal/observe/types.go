// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package observe

import (
	"github.com/rapidaai/pkg/telemetry"
)

type SessionMeta struct {
	AssistantID             uint64
	AssistantConversationID uint64
	ProjectID               uint64
	OrganizationID          uint64
}

type EventRecord = telemetry.EventRecord
type MetricRecord = telemetry.MetricRecord
