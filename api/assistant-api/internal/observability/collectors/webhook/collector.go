// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package webhook

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/pkg/clients/rest"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/validator"
)

const (
	WebhookOptionHTTPMethodKey       = "http_method"
	WebhookOptionHTTPURLKey          = "http_url"
	WebhookOptionHTTPHeadersKey      = "http_headers"
	WebhookOptionRetryStatusCodesKey = "retry_status_codes"
	WebhookOptionMaxRetryCountKey    = "max_retry_count"
	WebhookOptionTimeoutSecondsKey   = "timeout_seconds"
)

type Config struct {
	Logger   commons.Logger
	Webhooks []*internal_assistant_entity.AssistantWebhook
}

type Collector struct {
	logger   commons.Logger
	webhooks []*internal_assistant_entity.AssistantWebhook
}

func New(cfg Config) observability.Collector {
	if !validator.NotEmpty(cfg.Webhooks) {
		return observability.NoopCollector{}
	}
	return &Collector{
		logger:   cfg.Logger,
		webhooks: append([]*internal_assistant_entity.AssistantWebhook(nil), cfg.Webhooks...),
	}
}

func (c *Collector) Collect(ctx context.Context, record observability.Record) error {
	webhookRecord, ok := record.(observability.RecordWebhook)
	if !ok {
		return nil
	}
	if !validator.NonNil(c) || !validator.NotEmpty(c.webhooks) {
		return nil
	}
	payload := webhookPayload(webhookRecord)

	var errs []error
	for _, assistantWebhook := range c.webhooks {
		if !c.shouldSend(assistantWebhook, webhookRecord.Event.String()) {
			continue
		}
		if err := c.send(ctx, assistantWebhook, payload); err != nil {
			errs = append(errs, err)
			if validator.NonNil(c.logger) {
				c.logger.Warnw("observability webhook failed", "webhookID", assistantWebhook.Id, "event", webhookRecord.Event.String(), "error", err)
			}
		}
	}
	return errors.Join(errs...)
}

func (c *Collector) Close(context.Context) error {
	return nil
}

func (c *Collector) shouldSend(assistantWebhook *internal_assistant_entity.AssistantWebhook, eventName string) bool {
	if !validator.NonNil(assistantWebhook) || !validator.NotBlank(eventName) {
		return false
	}
	if internal_assistant_entity.NormalizeAssistantWebhookProvider(assistantWebhook.Provider) != internal_assistant_entity.AssistantWebhookProviderHTTP {
		return false
	}
	return slices.Contains(assistantWebhook.GetAssistantEvents(), eventName)
}

func (c *Collector) send(ctx context.Context, assistantWebhook *internal_assistant_entity.AssistantWebhook, payload map[string]interface{}) error {
	cfg := httpConfigFromWebhook(assistantWebhook)
	if !validator.NotBlank(cfg.URL) {
		return fmt.Errorf("observability webhook: http_url is required for webhook %d", assistantWebhook.Id)
	}

	client := rest.NewRestClientWithConfig(cfg.URL, cfg.Headers, cfg.TimeoutSeconds)
	for retryCount := uint32(0); retryCount <= cfg.MaxRetryCount; retryCount++ {
		response, err := sendHTTPRequest(ctx, client, cfg.Method, payload, cfg.Headers)
		if err != nil {
			if retryCount < cfg.MaxRetryCount {
				time.Sleep(2 * time.Second)
				continue
			}
			return err
		}
		if isRetryableStatus(response.StatusCode, cfg.RetryStatusCodes) {
			if retryCount < cfg.MaxRetryCount {
				time.Sleep(2 * time.Second)
				continue
			}
			return fmt.Errorf("observability webhook: retryable status %d", response.StatusCode)
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("observability webhook: endpoint returned status %d", response.StatusCode)
		}
		return nil
	}
	return nil
}

type httpConfig struct {
	URL              string
	Method           string
	Headers          map[string]string
	RetryStatusCodes []string
	MaxRetryCount    uint32
	TimeoutSeconds   uint32
}

func httpConfigFromWebhook(assistantWebhook *internal_assistant_entity.AssistantWebhook) httpConfig {
	opts := assistantWebhook.GetOptions()
	method, err := opts.GetString(WebhookOptionHTTPMethodKey)
	if err != nil || !validator.NotBlank(method) {
		method = http.MethodPost
	}
	url, _ := opts.GetString(WebhookOptionHTTPURLKey)
	headers, err := opts.GetStringMap(WebhookOptionHTTPHeadersKey)
	if err != nil {
		headers = map[string]string{}
	}
	maxRetryCount, err := opts.GetUint32(WebhookOptionMaxRetryCountKey)
	if err != nil {
		maxRetryCount = 0
	}
	timeoutSeconds, err := opts.GetUint32(WebhookOptionTimeoutSecondsKey)
	if err != nil {
		timeoutSeconds = 0
	}
	return httpConfig{
		URL:              url,
		Method:           strings.ToUpper(strings.TrimSpace(method)),
		Headers:          headers,
		RetryStatusCodes: opts.GetStringSlice(WebhookOptionRetryStatusCodesKey),
		MaxRetryCount:    maxRetryCount,
		TimeoutSeconds:   timeoutSeconds,
	}
}

func sendHTTPRequest(ctx context.Context, client *rest.RestClient, method string, payload map[string]interface{}, headers map[string]string) (*rest.APIResponse, error) {
	switch method {
	case http.MethodPut:
		return client.Put(ctx, "", payload, headers)
	case http.MethodPatch:
		return client.Patch(ctx, "", payload, headers)
	case http.MethodGet:
		return client.Get(ctx, "", payload, headers)
	default:
		return client.Post(ctx, "", payload, headers)
	}
}

func isRetryableStatus(statusCode int, retryStatusCodes []string) bool {
	return slices.Contains(retryStatusCodes, strconv.Itoa(statusCode))
}

func webhookPayload(record observability.RecordWebhook) map[string]interface{} {
	if len(record.Payload) == 0 {
		return map[string]interface{}{}
	}
	payload := make(map[string]interface{}, len(record.Payload))
	for key, value := range record.Payload {
		payload[key] = value
	}
	if record.Scope != nil {
		payload["scope"] = record.Scope.ScopeType()
		payload["context_id"] = record.Scope.ContextID()
	}
	return payload
}
