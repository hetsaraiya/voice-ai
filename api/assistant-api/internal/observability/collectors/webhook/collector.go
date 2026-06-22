// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	"github.com/rapidaai/pkg/clients/rest"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
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
	Logger                  commons.Logger
	Auth                    types.SimplePrinciple
	AssistantID             uint64
	AssistantWebhookService internal_services.AssistantWebhookService
	Recorder                observability.Recorder
}

type Collector struct {
	logger                  commons.Logger
	auth                    types.SimplePrinciple
	assistantID             uint64
	assistantWebhookService internal_services.AssistantWebhookService
	webhooks                []*internal_assistant_entity.AssistantWebhook
	webhooksLoaded          bool
	recorder                observability.Recorder
	mu                      sync.Mutex
}

func New(_ context.Context, config Config) observability.Collector {
	if config.Auth == nil || config.AssistantID == 0 || config.AssistantWebhookService == nil {
		return observability.NoopCollector{}
	}
	return &Collector{
		logger:                  config.Logger,
		auth:                    config.Auth,
		assistantID:             config.AssistantID,
		assistantWebhookService: config.AssistantWebhookService,
		recorder:                config.Recorder,
	}
}

func (c *Collector) Key() string {
	if !validator.NonNil(c) || c.assistantID == 0 {
		return "webhook"
	}
	return "webhook:" + strconv.FormatUint(c.assistantID, 10)
}

func (c *Collector) Collect(ctx context.Context, scope observability.Scope, observabilityContext observability.Context, record observability.Record) error {
	webhookRecord, ok := record.(observability.RecordWebhook)
	if !ok {
		return nil
	}
	if !validator.NonNil(c) {
		return nil
	}
	assistantWebhooks, err := c.assistantWebhooks(ctx)
	if err != nil {
		return err
	}
	if !validator.NotEmpty(assistantWebhooks) {
		return nil
	}

	webhookEventPayload := webhookPayload(webhookRecord)

	var webhookErrors []error
	for _, assistantWebhook := range assistantWebhooks {
		if !c.shouldSend(assistantWebhook, webhookRecord.Event.String()) {
			continue
		}
		if err := c.send(ctx, scope, observabilityContext, assistantWebhook, webhookRecord.Event.String(), webhookRecord.ContextID, webhookEventPayload); err != nil {
			webhookErrors = append(webhookErrors, err)
			if validator.NonNil(c.logger) {
				c.logger.Warnw("observability webhook failed", "webhookID", assistantWebhook.Id, "event", webhookRecord.Event.String(), "error", err)
			}
		}
	}
	return errors.Join(webhookErrors...)
}

func (c *Collector) Close(context.Context) error {
	return nil
}

func (c *Collector) assistantWebhooks(ctx context.Context) ([]*internal_assistant_entity.AssistantWebhook, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.webhooksLoaded {
		return c.webhooks, nil
	}

	_, assistantWebhooks, err := c.assistantWebhookService.GetAll(ctx, c.auth, c.assistantID, nil, &protos.Paginate{})
	if err != nil {
		if validator.NonNil(c.logger) {
			c.logger.Warnw("observability webhook load failed", "assistantID", c.assistantID, "error", err)
		}
		return nil, err
	}
	c.webhooks = append([]*internal_assistant_entity.AssistantWebhook(nil), assistantWebhooks...)
	c.webhooksLoaded = true
	return c.webhooks, nil
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

func (c *Collector) send(ctx context.Context, scope observability.Scope, _ observability.Context, assistantWebhook *internal_assistant_entity.AssistantWebhook, webhookEventName string, webhookContextID string, webhookPayload map[string]interface{}) error {
	webhookHTTPConfig := httpConfigFromWebhook(assistantWebhook)
	if !validator.NotBlank(webhookHTTPConfig.URL) {
		return fmt.Errorf("observability webhook: http_url is required for webhook %d", assistantWebhook.Id)
	}

	webhookHTTPClient := rest.NewRestClientWithConfig(webhookHTTPConfig.URL, webhookHTTPConfig.Headers, webhookHTTPConfig.TimeoutSeconds)
	webhookAssistantID := uint64(0)
	var webhookRequestLogScope observability.Scope
	switch typedScope := scope.(type) {
	case observability.MessageScope:
		webhookAssistantID = typedScope.AssistantScopeID()
		if !validator.NotBlank(webhookContextID) {
			webhookContextID = typedScope.ContextID()
		}
		webhookRequestLogScope = typedScope
	case observability.ConversationScope:
		webhookAssistantID = typedScope.AssistantScopeID()
		if !validator.NotBlank(webhookContextID) {
			webhookContextID = typedScope.ContextID()
		}
		webhookRequestLogScope = typedScope
	case observability.AssistantScope:
		webhookAssistantID = typedScope.AssistantScopeID()
		if !validator.NotBlank(webhookContextID) {
			webhookContextID = typedScope.ContextID()
		}
		webhookRequestLogScope = typedScope
	}
	shouldRecordWebhookRequestLog := c.recorder != nil && webhookRequestLogScope != nil && webhookAssistantID != 0

	for webhookRetryCount := uint32(0); webhookRetryCount <= webhookHTTPConfig.MaxRetryCount; webhookRetryCount++ {
		webhookAttemptStartTime := time.Now()
		webhookRequestPayload, webhookRequestPayloadMarshalError := json.Marshal(map[string]interface{}{
			"url":        webhookHTTPConfig.URL,
			"method":     webhookHTTPConfig.Method,
			"headers":    webhookHTTPConfig.Headers,
			"timeout_ms": webhookHTTPConfig.TimeoutSeconds * 1000,
			"body":       webhookPayload,
		})
		if webhookRequestPayloadMarshalError != nil {
			webhookRequestPayload = nil
		}

		webhookHTTPLogStatus := type_enums.RECORD_COMPLETE
		webhookResponseStatus := int64(0)
		var webhookErrorMessage *string
		var webhookResponsePayload []byte
		var webhookReturnError error
		shouldRetryWebhook := false

		webhookResponse, webhookSendError := sendHTTPRequest(ctx, webhookHTTPClient, webhookHTTPConfig.Method, webhookPayload, webhookHTTPConfig.Headers)
		if webhookSendError != nil {
			webhookErrorMessageValue := webhookSendError.Error()
			webhookHTTPLogStatus = type_enums.RECORD_FAILED
			webhookErrorMessage = &webhookErrorMessageValue
			webhookReturnError = webhookSendError
			shouldRetryWebhook = webhookRetryCount < webhookHTTPConfig.MaxRetryCount
		} else {
			webhookResponseStatus = int64(webhookResponse.StatusCode)
			webhookResponsePayload = webhookResponse.Body
			if isRetryableStatus(webhookResponse.StatusCode, webhookHTTPConfig.RetryStatusCodes) {
				webhookErrorMessageValue := fmt.Sprintf("observability webhook: retryable status %d", webhookResponse.StatusCode)
				webhookHTTPLogStatus = type_enums.RECORD_FAILED
				webhookErrorMessage = &webhookErrorMessageValue
				webhookReturnError = fmt.Errorf("observability webhook: retryable status %d", webhookResponse.StatusCode)
				shouldRetryWebhook = webhookRetryCount < webhookHTTPConfig.MaxRetryCount
			} else if webhookResponse.StatusCode < 200 || webhookResponse.StatusCode >= 300 {
				webhookErrorMessageValue := fmt.Sprintf("observability webhook: endpoint returned status %d", webhookResponse.StatusCode)
				webhookHTTPLogStatus = type_enums.RECORD_FAILED
				webhookErrorMessage = &webhookErrorMessageValue
				webhookReturnError = fmt.Errorf("observability webhook: endpoint returned status %d", webhookResponse.StatusCode)
			}
		}

		if shouldRecordWebhookRequestLog {
			if recordWebhookRequestLogError := c.recorder.Record(
				ctx,
				webhookRequestLogScope,
				observability.RecordRequestLog{
					Source:          "webhook",
					SourceRefID:     assistantWebhook.Id,
					SourceEvent:     webhookEventName,
					ContextID:       webhookContextID,
					HTTPURL:         webhookHTTPConfig.URL,
					HTTPMethod:      webhookHTTPConfig.Method,
					ResponseStatus:  webhookResponseStatus,
					TimeTaken:       int64(time.Since(webhookAttemptStartTime)),
					RetryCount:      webhookRetryCount,
					Status:          webhookHTTPLogStatus,
					ErrorMessage:    webhookErrorMessage,
					RequestPayload:  webhookRequestPayload,
					ResponsePayload: webhookResponsePayload,
				},
			); recordWebhookRequestLogError != nil && validator.NonNil(c.logger) {
				c.logger.Warnw("observability webhook request log record failed", "webhookID", assistantWebhook.Id, "event", webhookEventName, "error", recordWebhookRequestLogError)
			}
		}

		if shouldRetryWebhook {
			time.Sleep(2 * time.Second)
			continue
		}
		return webhookReturnError
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
	return payload
}
