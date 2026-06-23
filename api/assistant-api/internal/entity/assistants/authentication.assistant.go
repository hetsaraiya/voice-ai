// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_assistant_entity

import (
	gorm_model "github.com/rapidaai/pkg/models/gorm"
	"github.com/rapidaai/pkg/utils"
)

type AssistantAuthentication struct {
	gorm_model.Audited
	gorm_model.Mutable
	gorm_model.Organizational
	AssistantId                   uint64                           `json:"assistantId" gorm:"type:bigint;size:20;not null"`
	Provider                      AssistantAuthenticationProvider  `json:"provider" gorm:"type:varchar(50);not null;default:http"`
	FailBehavior                  string                           `json:"failBehavior" gorm:"type:string;size:20;not null;default:block"`
	TimeoutMs                     uint64                           `json:"timeoutMs" gorm:"type:bigint;not null;default:5000"`
	AssistantAuthenticationOption []*AssistantAuthenticationOption `json:"options" gorm:"foreignKey:AssistantAuthenticationId"`
}

func (AssistantAuthentication) TableName() string {
	return "assistant_authentications"
}

func (a *AssistantAuthentication) GetOptions() utils.Option {
	opts := make(utils.Option, len(a.AssistantAuthenticationOption))
	for _, v := range a.AssistantAuthenticationOption {
		opts[v.Key] = v.Value
	}
	return opts
}

type AssistantAuthenticationOption struct {
	gorm_model.Audited
	gorm_model.Mutable
	gorm_model.Metadata
	AssistantAuthenticationId uint64 `json:"assistantAuthenticationId" gorm:"type:bigint;size:20;not null"`
}

func (AssistantAuthenticationOption) TableName() string {
	return "assistant_authentication_options"
}
