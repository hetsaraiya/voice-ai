// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_callcontext

import (
	"time"

	gorm_generator "github.com/rapidaai/pkg/models/gorm/generators"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	"gorm.io/gorm"
)

// Call context status constants.
//
//	PENDING → CLAIMED
//
// Save creates with PENDING. Claim transitions to CLAIMED when the media path
// consumes the context (typically at session establishment). Get reads
// regardless of status, since async callbacks may arrive after claim.
const (
	StatusPending   = "pending"   // Context created, awaiting media-path consumption
	StatusClaimed   = "claimed"   // Context consumed (media-path bound, or call ended unclaimed)
	StatusCompleted = "completed" // Provider reported a terminal completed state
)

// CallContext holds all the information needed to resolve a call session.
// It bridges the gap between the HTTP call-setup request (inbound webhook or outbound gRPC)
// and the AudioSocket/WebSocket connection that follows.
//
// Stored in Postgres (call_contexts table). The status field provides atomic
// claiming: only one caller can transition pending→claimed.
type CallContext struct {
	Id             uint64    `json:"id" gorm:"type:bigint;primaryKey;<-:create"`
	ContextID      string    `json:"contextId" gorm:"column:context_id;type:varchar(36);not null;uniqueIndex"`
	Status         string    `json:"status" gorm:"column:status;type:varchar(20);not null;default:pending"`
	AssistantID    uint64    `json:"assistantId" gorm:"column:assistant_id;type:bigint;not null"`
	ConversationID uint64    `json:"conversationId" gorm:"column:conversation_id;type:bigint;not null"`
	ProjectID      uint64    `json:"projectId" gorm:"column:project_id;type:bigint;not null;default:0"`
	OrganizationID uint64    `json:"organizationId" gorm:"column:organization_id;type:bigint;not null;default:0"`
	AuthToken      string    `json:"-" gorm:"column:auth_token;type:text;not null;default:''"`
	AuthType       string    `json:"authType" gorm:"column:auth_type;type:varchar(50);not null;default:''"`
	Provider       string    `json:"provider" gorm:"column:provider;type:varchar(50);not null;default:''"`
	Direction      string    `json:"direction" gorm:"column:direction;type:varchar(20);not null;default:''"`
	CallerNumber   string    `json:"callerNumber" gorm:"column:caller_number;type:varchar(50);not null;default:''"`
	FromNumber     string    `json:"fromNumber" gorm:"column:from_number;type:varchar(50);not null;default:''"`
	CreatedDate    time.Time `json:"createdDate" gorm:"type:timestamp;not null;default:NOW();<-:create"`
	UpdatedDate    time.Time `json:"updatedDate" gorm:"type:timestamp;default:null"`

	// AssistantProviderId is the version identifier for the assistant provider.
	AssistantProviderId uint64 `json:"assistantProviderId" gorm:"column:assistant_provider_id;type:bigint;not null;default:0"`

	// ChannelUUID is the provider-specific call identifier (Twilio CallSid, Vonage UUID,
	// Asterisk channel ID, SIP Call-ID, etc.).
	ChannelUUID string `json:"channelUuid" gorm:"column:channel_uuid;type:varchar(200);not null;default:''"`
}

func (CallContext) TableName() string {
	return "call_contexts"
}

func (cc *CallContext) BeforeCreate(tx *gorm.DB) (err error) {
	if cc.Id <= 0 {
		cc.Id = gorm_generator.ID()
	}
	if cc.CreatedDate.IsZero() {
		cc.CreatedDate = time.Now()
	}
	return nil
}

// ToAuth converts the CallContext into a SimplePrinciple for use in service calls.
func (cc *CallContext) ToAuth() types.SimplePrinciple {
	auth := &types.ServiceScope{
		CurrentToken: cc.AuthToken,
	}
	if cc.ProjectID != 0 {
		auth.ProjectId = utils.Ptr(cc.ProjectID)
	}
	if cc.OrganizationID != 0 {
		auth.OrganizationId = utils.Ptr(cc.OrganizationID)
	}
	return auth
}
