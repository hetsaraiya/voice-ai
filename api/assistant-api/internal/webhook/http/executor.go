// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_webhook_http

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/clients/rest"
	"github.com/rapidaai/pkg/commons"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
)

const (
	WebhookOptionHTTPMethodKey       = "http_method"
	WebhookOptionHTTPURLKey          = "http_url"
	WebhookOptionHTTPHeadersKey      = "http_headers"
	WebhookOptionHTTPBodyKey         = "http_body"
	WebhookOptionRetryStatusCodesKey = "retry_status_codes"
	WebhookOptionMaxRetryCountKey    = "max_retry_count"
	WebhookOptionTimeoutSecondsKey   = "timeout_seconds"
)

type runtimeExecutor struct {
	logger   commons.Logger
	callback internal_type.Callback
	webhook  *internal_assistant_entity.AssistantWebhook
}

// NewExecutor creates a fully wired HTTP webhook executor.
func NewExecutor(logger commons.Logger, _ context.Context,
	webhook *internal_assistant_entity.AssistantWebhook,
	callback internal_type.Callback,
	_ internal_type.InternalCaller) (internal_type.WebhookExecutor, error) {
	return &runtimeExecutor{
		logger:   logger,
		callback: callback,
		webhook:  webhook,
	}, nil
}

func (e *runtimeExecutor) Name() string {
	return fmt.Sprintf("webhook-http-%d", e.webhook.Id)
}

func (e *runtimeExecutor) Options() utils.Option {
	return e.webhook.GetOptions()
}

func (e *runtimeExecutor) Arguments() (map[string]string, error) {
	return e.Options().GetStringMap(WebhookOptionHTTPBodyKey)
}

func (aa *runtimeExecutor) GetHeaders() map[string]string {
	opts, err := aa.Options().GetStringMap(WebhookOptionHTTPHeadersKey)
	if err != nil {
		return map[string]string{}
	}
	return opts
}

func (aa *runtimeExecutor) GetBody() map[string]string {
	opts, err := aa.Options().GetStringMap(WebhookOptionHTTPBodyKey)
	if err != nil {
		return map[string]string{}
	}
	return opts
}

func (aa *runtimeExecutor) GetMethod() string {
	raw, err := aa.Options().GetString(WebhookOptionHTTPMethodKey)
	if err != nil {
		return "POST"
	}
	return raw
}

func (aa *runtimeExecutor) GetUrl() string {
	raw, err := aa.Options().GetString(WebhookOptionHTTPURLKey)
	if err != nil {
		return ""
	}
	return raw
}

func (aa *runtimeExecutor) GetRetryStatusCode() []string {
	return aa.Options().GetStringSlice(WebhookOptionRetryStatusCodesKey)
}

func (aa *runtimeExecutor) GetAllowedEvents() []string {
	return aa.webhook.AssistantEvents
}

func (aa *runtimeExecutor) GetMaxRetryCount() uint32 {
	raw, err := aa.Options().GetUint32(WebhookOptionMaxRetryCountKey)
	if err != nil {
		return 0
	}
	return raw
}

func (aa *runtimeExecutor) GetTimeoutSecond() uint32 {
	raw, err := aa.Options().GetUint32(WebhookOptionTimeoutSecondsKey)
	if err != nil {
		return 0
	}
	return raw
}

// Execute runs webhook dispatch for packet event.
func (e *runtimeExecutor) Execute(ctx context.Context, packet internal_type.ExecuteWebhookPacket) error {
	if !slices.Contains(e.GetAllowedEvents(), packet.Event.Get()) {
		return nil
	}
	client := rest.NewRestClientWithConfig(e.GetUrl(), e.GetHeaders(), e.GetTimeoutSecond())
	startTime := time.Now()
	requestPayload := e.createRequestPayload(e.GetUrl(), e.GetMethod(), e.GetHeaders(), e.GetTimeoutSecond()*1000, packet.Arguments)
	for retryCount := uint32(0); retryCount <= e.GetMaxRetryCount(); retryCount++ {
		switch e.GetMethod() {
		case "POST":
			response, err := client.Post(ctx, "", packet.Arguments, e.GetHeaders())
			if err != nil {
				e.logger.Warnw("Webhook execution failed", "url", e.GetUrl(), "error", err)
				errorMessage := err.Error()
				e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_FAILED, 0, &errorMessage, requestPayload, nil)
				if retryCount < e.GetMaxRetryCount() {
					time.Sleep(2 * time.Second)
				}
				continue
			}

			isRetryable := slices.Contains(e.GetRetryStatusCode(), strconv.Itoa(response.StatusCode))
			if isRetryable {
				errorMessage := fmt.Sprintf("webhook: retryable status %d", response.StatusCode)
				e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_FAILED, int64(response.StatusCode), &errorMessage, requestPayload, response.Body)
				if retryCount < e.GetMaxRetryCount() {
					time.Sleep(2 * time.Second)
				}
				continue
			}
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				errorMessage := fmt.Sprintf("webhook: endpoint returned status %d", response.StatusCode)
				e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_FAILED, int64(response.StatusCode), &errorMessage, requestPayload, response.Body)
				return nil
			}
			e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_COMPLETE, int64(response.StatusCode), nil, requestPayload, response.Body)
			return nil
		case "PUT":
			response, err := client.Put(ctx, "", packet.Arguments, e.GetHeaders())
			if err != nil {
				e.logger.Warnw("Webhook execution failed", "url", e.GetUrl(), "error", err)
				errorMessage := err.Error()
				e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_FAILED, 0, &errorMessage, requestPayload, nil)
				if retryCount < e.GetMaxRetryCount() {
					time.Sleep(2 * time.Second)
				}
				continue
			}

			isRetryable := slices.Contains(e.GetRetryStatusCode(), strconv.Itoa(response.StatusCode))
			if isRetryable {
				errorMessage := fmt.Sprintf("webhook: retryable status %d", response.StatusCode)
				e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_FAILED, int64(response.StatusCode), &errorMessage, requestPayload, response.Body)
				if retryCount < e.GetMaxRetryCount() {
					time.Sleep(2 * time.Second)
				}
				continue
			}
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				errorMessage := fmt.Sprintf("webhook: endpoint returned status %d", response.StatusCode)
				e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_FAILED, int64(response.StatusCode), &errorMessage, requestPayload, response.Body)
				return nil
			}
			e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_COMPLETE, int64(response.StatusCode), nil, requestPayload, response.Body)
			return nil
		case "PATCH":
			response, err := client.Patch(ctx, "", packet.Arguments, e.GetHeaders())
			if err != nil {
				e.logger.Warnw("Webhook execution failed", "url", e.GetUrl(), "error", err)
				errorMessage := err.Error()
				e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_FAILED, 0, &errorMessage, requestPayload, nil)
				if retryCount < e.GetMaxRetryCount() {
					time.Sleep(2 * time.Second)
				}
				continue
			}

			isRetryable := slices.Contains(e.GetRetryStatusCode(), strconv.Itoa(response.StatusCode))
			if isRetryable {
				errorMessage := fmt.Sprintf("webhook: retryable status %d", response.StatusCode)
				e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_FAILED, int64(response.StatusCode), &errorMessage, requestPayload, response.Body)
				if retryCount < e.GetMaxRetryCount() {
					time.Sleep(2 * time.Second)
				}
				continue
			}
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				errorMessage := fmt.Sprintf("webhook: endpoint returned status %d", response.StatusCode)
				e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_FAILED, int64(response.StatusCode), &errorMessage, requestPayload, response.Body)
				return nil
			}
			e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_COMPLETE, int64(response.StatusCode), nil, requestPayload, response.Body)
			return nil
		default:
			response, err := client.Get(ctx, "", packet.Arguments, e.GetHeaders())
			if err != nil {
				e.logger.Warnw("Webhook execution failed", "url", e.GetUrl(), "error", err)
				errorMessage := err.Error()
				e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_FAILED, 0, &errorMessage, requestPayload, nil)
				if retryCount < e.GetMaxRetryCount() {
					time.Sleep(2 * time.Second)
				}
				continue
			}

			isRetryable := slices.Contains(e.GetRetryStatusCode(), strconv.Itoa(response.StatusCode))
			if isRetryable {
				errorMessage := fmt.Sprintf("webhook: retryable status %d", response.StatusCode)
				e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_FAILED, int64(response.StatusCode), &errorMessage, requestPayload, response.Body)
				if retryCount < e.GetMaxRetryCount() {
					time.Sleep(2 * time.Second)
				}
				continue
			}
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				errorMessage := fmt.Sprintf("webhook: endpoint returned status %d", response.StatusCode)
				e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_FAILED, int64(response.StatusCode), &errorMessage, requestPayload, response.Body)
				return nil
			}
			e.onCreateLog(ctx, packet, e.GetMethod(), startTime, retryCount, type_enums.RECORD_COMPLETE, int64(response.StatusCode), nil, requestPayload, response.Body)
			return nil
		}
	}
	return nil
}

func (e *runtimeExecutor) createRequestPayload(url, method string, headers map[string]string, timeoutMs uint32, body map[string]interface{}) []byte {
	payload := map[string]interface{}{
		"url":        url,
		"method":     method,
		"headers":    headers,
		"timeout_ms": timeoutMs,
		"body":       body,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		e.logger.Warnw("Failed to serialize webhook request payload snapshot", "error", err)
		return nil
	}
	return data
}

func (e *runtimeExecutor) onCreateLog(
	ctx context.Context,
	packet internal_type.ExecuteWebhookPacket,
	method string,
	startTime time.Time,
	retryCount uint32,
	status type_enums.RecordState,
	responseStatus int64,
	errorMessage *string,
	requestPayload []byte,
	responsePayload []byte,
) {
	sourceRefID := e.webhook.Id
	if err := e.callback.OnPacket(ctx, internal_type.HTTPLogCreatePacket{
		ContextID:       packet.ContextID,
		Source:          "webhook",
		SourceRefID:     sourceRefID,
		SourceEvent:     packet.Event.Get(),
		HTTPURL:         e.GetUrl(),
		HTTPMethod:      method,
		ResponseStatus:  responseStatus,
		TimeTaken:       int64(time.Since(startTime)),
		RetryCount:      retryCount,
		Status:          status,
		ErrorMessage:    errorMessage,
		RequestPayload:  requestPayload,
		ResponsePayload: responsePayload,
	}); err != nil {
		e.logger.Warnw("Failed to enqueue webhook log", "error", err)
	}
}

// Close releases executor dependencies.
func (e *runtimeExecutor) Close(_ context.Context) error {
	e.callback = nil
	return nil
}
