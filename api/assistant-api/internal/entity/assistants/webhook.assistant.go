// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_assistant_entity

import (
	"encoding/json"
	"strings"

	gorm_model "github.com/rapidaai/pkg/models/gorm"
	gorm_types "github.com/rapidaai/pkg/models/gorm/types"
	"github.com/rapidaai/pkg/utils"
)

const (
	WebhookOptionAssistantEventsKey  = "assistant_events"
	WebhookOptionHTTPMethodKey       = "http_method"
	WebhookOptionHTTPURLKey          = "http_url"
	WebhookOptionHTTPHeadersKey      = "http_headers"
	WebhookOptionHTTPBodyKey         = "http_body"
	WebhookOptionRetryStatusCodesKey = "retry_status_codes"
	WebhookOptionMaxRetryCountKey    = "max_retry_count"
	WebhookOptionTimeoutSecondsKey   = "timeout_seconds"
)

type AssistantWebhook struct {
	gorm_model.Audited
	gorm_model.Mutable
	gorm_model.Organizational

	AssistantId            uint64                    `json:"assistantId" gorm:"type:bigint;not null"`
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

func (aa *AssistantWebhook) GetHeaders() map[string]string {
	opts, err := aa.GetOptions().GetStringMap(WebhookOptionHTTPHeadersKey)
	if err != nil {
		return map[string]string{}
	}
	return opts
}

func (aa *AssistantWebhook) GetBody() map[string]string {
	opts, err := aa.GetOptions().GetStringMap(WebhookOptionHTTPBodyKey)
	if err != nil {
		return map[string]string{}
	}
	return opts
}

func (aa *AssistantWebhook) GetMethod() string {
	raw, err := aa.GetOptions().GetString(WebhookOptionHTTPMethodKey)
	if err != nil {
		return "POST"
	}
	return raw
}

func (aa *AssistantWebhook) GetUrl() string {
	raw, err := aa.GetOptions().GetString(WebhookOptionHTTPURLKey)
	if err != nil {
		return ""
	}
	return raw
}

func (aa *AssistantWebhook) GetRetryStatusCode() []string {
	return aa.getStringSliceOption(WebhookOptionRetryStatusCodesKey)
}

func (aa *AssistantWebhook) GetMaxRetryCount() uint32 {
	raw, err := aa.GetOptions().GetUint32(WebhookOptionMaxRetryCountKey)
	if err != nil {
		return 0
	}
	return raw
}

func (aa *AssistantWebhook) GetTimeoutSecond() uint32 {
	raw, err := aa.GetOptions().GetUint32(WebhookOptionTimeoutSecondsKey)
	if err != nil {
		return 0
	}
	return raw
}

func (aa *AssistantWebhook) getStringSliceOption(key string) []string {
	raw, err := aa.GetOptions().GetString(key)
	if err != nil {
		return []string{}
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []string{}
	}

	parsed := []string{}
	if json.Unmarshal([]byte(trimmed), &parsed) == nil {
		return parsed
	}

	if strings.Contains(trimmed, ",") {
		out := []string{}
		for _, item := range strings.Split(trimmed, ",") {
			part := strings.TrimSpace(item)
			if part != "" {
				out = append(out, part)
			}
		}
		return out
	}

	return []string{trimmed}
}

type AssistantWebhookLog struct {
	gorm_model.Audited
	gorm_model.Mutable
	gorm_model.Organizational
	WebhookId               uint64 `json:"webhookId" gorm:"type:bigint"`
	HttpMethod              string `json:"httpMethod" gorm:"type:string;size:200;not null"`
	HttpUrl                 string `json:"httpUrl" gorm:"type:string;size:400;not null"`
	AssistantId             uint64 `json:"assistantId" gorm:"type:bigint"`
	AssistantConversationId uint64 `json:"assistantConversationId" gorm:"type:bigint"`
	Event                   string `json:"event" gorm:"type:string;size:200;not null"`
	AssetPrefix             string `json:"assetPrefix" gorm:"type:string;size:200;not null"`
	ResponseStatus          int64  `json:"responseStatus" gorm:"type:bigint;size:10"`
	TimeTaken               int64  `json:"timeTaken" gorm:"type:bigint;size:20"`
	RetryCount              uint32 `json:"retryCount" gorm:"type:bigint;size:20"`
}
