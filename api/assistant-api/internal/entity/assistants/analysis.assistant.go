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

type AssistantAnalysis struct {
	gorm_model.Audited
	gorm_model.Mutable
	gorm_model.Organizational

	AssistantId             uint64                     `json:"assistantId" gorm:"type:bigint;not null"`
	Provider                AssistantAnalysisProvider  `json:"provider" gorm:"type:varchar(50);not null;default:endpoint"`
	Name                    string                     `json:"name" gorm:"type:text"`
	Description             string                     `json:"description" gorm:"type:text"`
	ExecutionPriority       uint32                     `json:"executionPriority" gorm:"type:int"`
	AssistantAnalysisOption []*AssistantAnalysisOption `json:"options" gorm:"foreignKey:AssistantAnalysisId"`
}

type AssistantAnalysisOption struct {
	gorm_model.Audited
	gorm_model.Mutable
	gorm_model.Metadata
	AssistantAnalysisId uint64 `json:"assistantAnalysisId" gorm:"type:bigint;size:20;not null"`
}

func (AssistantAnalysisOption) TableName() string {
	return "assistant_analysis_options"
}

func (aa *AssistantAnalysis) GetName() string {
	return aa.Name
}
func (aa *AssistantAnalysis) GetExecutionPriority() uint32 {
	return aa.ExecutionPriority
}

func (aa *AssistantAnalysis) GetOptions() utils.Option {
	opts := make(utils.Option, len(aa.AssistantAnalysisOption))
	for _, v := range aa.AssistantAnalysisOption {
		opts[v.Key] = v.Value
	}
	return opts
}
