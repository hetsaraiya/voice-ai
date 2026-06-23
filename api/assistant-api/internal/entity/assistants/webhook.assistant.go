// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_assistant_entity

import (
	gorm_model "github.com/rapidaai/pkg/models/gorm"
	gorm_types "github.com/rapidaai/pkg/models/gorm/types"
	"github.com/rapidaai/pkg/utils"
)

type AssistantWebhook struct {
	gorm_model.Audited
	gorm_model.Mutable
	gorm_model.Organizational

	AssistantId            uint64                    `json:"assistantId" gorm:"type:bigint;not null"`
	Provider               AssistantWebhookProvider  `json:"provider" gorm:"type:varchar(50);not null;default:http"`
	Description            string                    `json:"description" gorm:"type:text"`
	ExecutionPriority      uint32                    `json:"executionPriority" gorm:"type:int"`
	AssistantEvents        gorm_types.StringArray    `json:"assistantEvents" gorm:"type:text;not null;default:'[]'::text"`
	AssistantWebhookOption []*AssistantWebhookOption `json:"options" gorm:"foreignKey:AssistantWebhookId"`
}

type AssistantWebhookOption struct {
	gorm_model.Audited
	gorm_model.Mutable
	gorm_model.Metadata
	AssistantWebhookId uint64 `json:"assistantWebhookId" gorm:"type:bigint;size:20;not null"`
}

func (AssistantWebhookOption) TableName() string {
	return "assistant_webhook_options"
}

func (aa *AssistantWebhook) GetExecutionPriority() uint32 {
	return aa.ExecutionPriority
}

func (aa *AssistantWebhook) GetOptions() utils.Option {
	opts := make(utils.Option, len(aa.AssistantWebhookOption))
	for _, v := range aa.AssistantWebhookOption {
		opts[v.Key] = v.Value
	}
	return opts
}

func (aa *AssistantWebhook) GetAssistantEvents() []string {
	if aa.AssistantEvents == nil {
		return []string{}
	}
	return []string(aa.AssistantEvents)
}
