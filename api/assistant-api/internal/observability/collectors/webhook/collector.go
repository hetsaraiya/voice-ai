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
	"github.com/rapidaai/pkg/utils"
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
	HTTPLogService          internal_services.AssistantHTTPLogService
}

type Collector struct {
	logger                  commons.Logger
	auth                    types.SimplePrinciple
	assistantID             uint64
	assistantWebhookService internal_services.AssistantWebhookService
	httpLogService          internal_services.AssistantHTTPLogService
	webhooks                []*internal_assistant_entity.AssistantWebhook
	webhooksLoaded          bool
	mu                      sync.Mutex
}

func New(_ context.Context, config Config) observability.Collector {
	if config.Auth == nil || config.AssistantWebhookService == nil || config.HTTPLogService == nil {
		return observability.NoopCollector{}
	}
	return &Collector{
		logger:                  config.Logger,
		auth:                    config.Auth,
		assistantID:             config.AssistantID,
		assistantWebhookService: config.AssistantWebhookService,
		httpLogService:          config.HTTPLogService,
	}
}

func (c *Collector) Key() string {
	if !validator.NonNil(c) {
		return "webhook"
	}
	return "webhook:" + strconv.FormatUint(c.assistantID, 10)
}

func (c *Collector) Collect(ctx context.Context, scope observability.Scope, _ observability.Context, record observability.Record) error {
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
	webhookEventPayload := map[string]interface{}{}
	if len(webhookRecord.Payload) > 0 {
		webhookEventPayload = make(map[string]interface{}, len(webhookRecord.Payload))
		for key, value := range webhookRecord.Payload {
			webhookEventPayload[key] = value
		}
	}

	var webhookErrors []error
	for _, assistantWebhook := range assistantWebhooks {
		if !c.shouldSend(assistantWebhook, webhookRecord.Event.String()) {
			continue
		}
		if err := c.send(ctx, scope, assistantWebhook, webhookRecord.Event.String(), webhookRecord.ContextID, webhookEventPayload); err != nil {
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
	if _, err := internal_assistant_entity.NewAssistantWebhookProvider(string(assistantWebhook.Provider)); err != nil {
		return false
	}
	return slices.Contains(assistantWebhook.GetAssistantEvents(), eventName)
}

func (c *Collector) send(ctx context.Context, scope observability.Scope, assistantWebhook *internal_assistant_entity.AssistantWebhook, webhookEventName string, webhookContextID string, webhookPayload map[string]interface{}) error {
	webhookOptions := assistantWebhook.GetOptions()
	webhookHTTPMethod, err := webhookOptions.GetString(WebhookOptionHTTPMethodKey)
	if err != nil || !validator.NotBlank(webhookHTTPMethod) {
		webhookHTTPMethod = http.MethodPost
	}
	webhookHTTPMethod = strings.ToUpper(strings.TrimSpace(webhookHTTPMethod))

	webhookHTTPURL, _ := webhookOptions.GetString(WebhookOptionHTTPURLKey)
	if !validator.NotBlank(webhookHTTPURL) {
		return fmt.Errorf("observability webhook: http_url is required for webhook %d", assistantWebhook.Id)
	}

	webhookHTTPHeaders, err := webhookOptions.GetStringMap(WebhookOptionHTTPHeadersKey)
	if err != nil {
		webhookHTTPHeaders = map[string]string{}
	}

	webhookMaxRetryCount, err := webhookOptions.GetUint32(WebhookOptionMaxRetryCountKey)
	if err != nil {
		webhookMaxRetryCount = 0
	}

	webhookTimeoutSeconds, err := webhookOptions.GetUint32(WebhookOptionTimeoutSecondsKey)
	if err != nil {
		webhookTimeoutSeconds = 0
	}
	webhookRetryStatusCodes := webhookOptions.GetStringSlice(WebhookOptionRetryStatusCodesKey)

	webhookHTTPClient := rest.NewRestClientWithConfig(webhookHTTPURL, webhookHTTPHeaders, webhookTimeoutSeconds)
	webhookAssistantID := uint64(0)
	var webhookConversationID *uint64
	switch typedScope := scope.(type) {
	case observability.MessageScope:
		webhookAssistantID = typedScope.AssistantScopeID()
		scopeConversationID := typedScope.ConversationScopeID()
		webhookConversationID = &scopeConversationID
		if !validator.NotBlank(webhookContextID) {
			webhookContextID = typedScope.ContextID()
		}
	case observability.ConversationScope:
		webhookAssistantID = typedScope.AssistantScopeID()
		scopeConversationID := typedScope.ConversationScopeID()
		webhookConversationID = &scopeConversationID
		if !validator.NotBlank(webhookContextID) {
			webhookContextID = typedScope.ContextID()
		}
	case observability.AssistantScope:
		webhookAssistantID = typedScope.AssistantScopeID()
		if !validator.NotBlank(webhookContextID) {
			webhookContextID = typedScope.ContextID()
		}
	}
	if webhookAssistantID == 0 {
		webhookAssistantID = c.assistantID
	}
	webhookRequestBody := map[string]interface{}{
		"assistant": map[string]interface{}{
			"id": webhookAssistantID,
		},
		"data":  webhookPayload,
		"event": webhookEventName,
	}
	if webhookConversationID != nil && *webhookConversationID != 0 {
		webhookRequestBody["conversation"] = map[string]interface{}{
			"id": *webhookConversationID,
		}
	}

	for webhookRetryCount := uint32(0); webhookRetryCount <= webhookMaxRetryCount; webhookRetryCount++ {
		webhookAttemptStartTime := time.Now()
		webhookRequestPayload, webhookRequestPayloadMarshalError := json.Marshal(map[string]interface{}{
			"url":        webhookHTTPURL,
			"method":     webhookHTTPMethod,
			"headers":    webhookHTTPHeaders,
			"timeout_ms": webhookTimeoutSeconds * 1000,
			"body":       webhookRequestBody,
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

		var webhookResponse *rest.APIResponse
		var webhookSendError error
		switch webhookHTTPMethod {
		case http.MethodPut:
			webhookResponse, webhookSendError = webhookHTTPClient.Put(ctx, "", webhookRequestBody, webhookHTTPHeaders)
		case http.MethodPatch:
			webhookResponse, webhookSendError = webhookHTTPClient.Patch(ctx, "", webhookRequestBody, webhookHTTPHeaders)
		case http.MethodGet:
			webhookResponse, webhookSendError = webhookHTTPClient.Get(ctx, "", webhookRequestBody, webhookHTTPHeaders)
		default:
			webhookResponse, webhookSendError = webhookHTTPClient.Post(ctx, "", webhookRequestBody, webhookHTTPHeaders)
		}
		if webhookSendError != nil {
			webhookErrorMessageValue := webhookSendError.Error()
			webhookHTTPLogStatus = type_enums.RECORD_FAILED
			webhookErrorMessage = &webhookErrorMessageValue
			webhookReturnError = webhookSendError
			shouldRetryWebhook = webhookRetryCount < webhookMaxRetryCount
		} else {
			webhookResponseStatus = int64(webhookResponse.StatusCode)
			webhookResponsePayload = webhookResponse.Body
			if utils.MatchAnyString(webhookRetryStatusCodes, strconv.Itoa(webhookResponse.StatusCode)) {
				webhookErrorMessageValue := fmt.Sprintf("observability webhook: retryable status %d", webhookResponse.StatusCode)
				webhookHTTPLogStatus = type_enums.RECORD_FAILED
				webhookErrorMessage = &webhookErrorMessageValue
				webhookReturnError = fmt.Errorf("observability webhook: retryable status %d", webhookResponse.StatusCode)
				shouldRetryWebhook = webhookRetryCount < webhookMaxRetryCount
			} else if webhookResponse.StatusCode < 200 || webhookResponse.StatusCode >= 300 {
				webhookErrorMessageValue := fmt.Sprintf("observability webhook: endpoint returned status %d", webhookResponse.StatusCode)
				webhookHTTPLogStatus = type_enums.RECORD_FAILED
				webhookErrorMessage = &webhookErrorMessageValue
				webhookReturnError = fmt.Errorf("observability webhook: endpoint returned status %d", webhookResponse.StatusCode)
			}
		}

		if _, recordWebhookRequestLogError := c.httpLogService.CreateLog(
			ctx,
			c.auth,
			"webhook",
			assistantWebhook.Id,
			webhookEventName,
			webhookContextID,
			webhookAssistantID,
			webhookConversationID,
			webhookHTTPURL,
			webhookHTTPMethod,
			webhookResponseStatus,
			int64(time.Since(webhookAttemptStartTime)),
			webhookRetryCount,
			webhookHTTPLogStatus,
			webhookErrorMessage,
			webhookRequestPayload,
			webhookResponsePayload,
		); recordWebhookRequestLogError != nil && validator.NonNil(c.logger) {
			c.logger.Warnw("observability webhook request log record failed", "webhookID", assistantWebhook.Id, "event", webhookEventName, "error", recordWebhookRequestLogError)
		}

		if shouldRetryWebhook {
			time.Sleep(2 * time.Second)
			continue
		}
		return webhookReturnError
	}
	return nil
}
