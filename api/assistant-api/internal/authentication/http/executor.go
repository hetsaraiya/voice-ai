// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_authentication_http

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/clients/rest"
	"github.com/rapidaai/pkg/commons"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
)

const (
	OptionHTTPURLKey     = "http_url"
	OptionHTTPMethodKey  = "http_method"
	OptionHTTPHeadersKey = "http_headers"
	OptionHTTPBodyKey    = "http_body"

	ResponseArgumentsKey   = "arguments"
	ResponseArgumentsKeyV1 = "args"
	ResponseMetadataKey    = "metadata"
	ResponseOptionsKey     = "options"

	FailBehaviorBlock = "block"
	FailBehaviorAllow = "allow"
)

type runtimeExecutor struct {
	logger        commons.Logger
	callback      internal_type.Callback
	authenticator *internal_assistant_entity.AssistantAuthentication
}

// NewExecutor creates a fully wired HTTP authentication executor.
func NewExecutor(logger commons.Logger, _ context.Context, authenticator *internal_assistant_entity.AssistantAuthentication, callback internal_type.Callback, _ internal_type.InternalCaller) (internal_type.AuthenticationExecutor, error) {
	return &runtimeExecutor{
		logger:        logger,
		callback:      callback,
		authenticator: authenticator,
	}, nil
}

func (e *runtimeExecutor) Name() string {
	return string(e.authenticator.Provider)
}

func (e *runtimeExecutor) Options() utils.Option {
	return e.authenticator.GetOptions()
}

func (e *runtimeExecutor) Arguments() (map[string]string, error) {
	return e.authenticator.GetOptions().GetStringMap(OptionHTTPBodyKey)
}

// Execute runs authentication against the configured endpoint.
func (e *runtimeExecutor) Execute(ctx context.Context, input internal_type.AuthenticationInput) (internal_type.AuthenticationOutput, error) {
	auth := e.authenticator
	url, err := auth.GetOptions().GetString(OptionHTTPURLKey)
	if err != nil || url == "" {
		return internal_type.AuthenticationOutput{}, fmt.Errorf("authentication: missing %s", OptionHTTPURLKey)
	}
	method := "POST"
	if m, err := auth.GetOptions().GetString(OptionHTTPMethodKey); err == nil && m != "" {
		method = m
	}
	method = strings.ToUpper(method)

	headers := map[string]string{}
	if h, err := auth.GetOptions().GetStringMap(OptionHTTPHeadersKey); err == nil {
		headers = h
	}

	timeout := auth.TimeoutMs
	if timeout == 0 {
		timeout = 5000
	}

	client := rest.NewRestClientWithConfig(url, headers, uint32(timeout/1000))
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	startTime := time.Now()
	requestPayload := e.createRequestPayload(url, method, headers, uint32(timeout), input.Arguments)
	sourceRefID := auth.Id
	response, err := e.send(callCtx, client, method, input.Arguments, headers)
	if err != nil {
		errMsg := err.Error()
		e.onCreateLog(ctx, input.ContextID, url, method, sourceRefID, startTime, type_enums.RECORD_FAILED, 0, &errMsg, requestPayload, nil)
		if auth.FailBehavior == FailBehaviorAllow {
			e.logger.Warnw("authentication failed, allowing due to fail_behavior=allow", "url", url, "error", err)
			return internal_type.AuthenticationOutput{
				Authenticated: false,
			}, nil
		}
		return internal_type.AuthenticationOutput{}, fmt.Errorf("authentication: request failed: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		errMsg := fmt.Sprintf("authentication: endpoint returned status %d", response.StatusCode)
		e.onCreateLog(ctx, input.ContextID, url, method, sourceRefID, startTime, type_enums.RECORD_FAILED, int64(response.StatusCode), &errMsg, requestPayload, response.Body)

		if auth.FailBehavior == FailBehaviorAllow {
			e.logger.Warnw("authentication returned non-2xx, allowing due to fail_behavior=allow",
				"url", url, "status", response.StatusCode)
			return internal_type.AuthenticationOutput{
				Authenticated: false,
			}, nil
		}
		return internal_type.AuthenticationOutput{}, fmt.Errorf("authentication: endpoint returned status %d", response.StatusCode)
	}
	e.onCreateLog(ctx, input.ContextID, url, method, sourceRefID, startTime, type_enums.RECORD_COMPLETE, int64(response.StatusCode), nil, requestPayload, response.Body)
	result := internal_type.AuthenticationOutput{
		Authenticated: true,
	}
	if parsed, err := response.ToMap(); err == nil {
		if args, ok := parsed[ResponseArgumentsKeyV1].(map[string]interface{}); ok {
			result.Arguments = args
		}
		if args, ok := parsed[ResponseArgumentsKey].(map[string]interface{}); ok {
			result.Arguments = args
		}
		if metadata, ok := parsed[ResponseMetadataKey].(map[string]interface{}); ok {
			result.Metadata = metadata
		}
		if options, ok := parsed[ResponseOptionsKey].(map[string]interface{}); ok {
			result.Options = options
		}
	}

	return result, nil
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
		e.logger.Warnw("Failed to serialize authentication request payload snapshot", "error", err)
		return nil
	}
	return data
}

// Close releases executor dependencies.
func (e *runtimeExecutor) Close(_ context.Context) error {
	e.callback = nil
	return nil
}

func (e *runtimeExecutor) send(ctx context.Context, client *rest.RestClient, method string, body map[string]interface{}, headers map[string]string) (*rest.APIResponse, error) {
	switch method {
	case "POST":
		return client.Post(ctx, "", body, headers)
	case "PUT":
		return client.Put(ctx, "", body, headers)
	case "PATCH":
		return client.Patch(ctx, "", body, headers)
	default:
		return client.Get(ctx, "", body, headers)
	}
}

func (e *runtimeExecutor) onCreateLog(
	ctx context.Context,
	contextID string,
	url string,
	method string,
	sourceRefID uint64,
	startTime time.Time,
	status type_enums.RecordState,
	responseStatus int64,
	errorMessage *string,
	requestPayload []byte,
	responsePayload []byte,
) {
	if err := e.callback.OnPacket(ctx, internal_type.HTTPLogCreatePacket{
		ContextID:       contextID,
		Source:          "authentication",
		SourceRefID:     sourceRefID,
		SourceEvent:     "session_authentication",
		HTTPURL:         url,
		HTTPMethod:      method,
		ResponseStatus:  responseStatus,
		TimeTaken:       int64(time.Since(startTime)),
		RetryCount:      0,
		Status:          status,
		ErrorMessage:    errorMessage,
		RequestPayload:  requestPayload,
		ResponsePayload: responsePayload,
	}); err != nil {
		e.logger.Warnw("Failed to enqueue authentication http log", "error", err)
	}
}
