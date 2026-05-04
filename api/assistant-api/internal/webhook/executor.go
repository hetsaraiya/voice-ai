// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package webhook

import (
	"context"
	"slices"
	"strconv"
	"time"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/clients/rest"
	"github.com/rapidaai/pkg/commons"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
)

// Executor defines webhook runtime behavior.
type Executor interface {
	Init(ctx context.Context, communication internal_type.Communication)
	Execute(ctx context.Context, packet internal_type.RunWebhookPacket) error
	Close(ctx context.Context)
}

type runtimeExecutor struct {
	logger   commons.Logger
	onPacket func(ctx context.Context, pkts ...internal_type.Packet) error
}

// NewExecutor creates a webhook executor.
func NewExecutor(logger commons.Logger) Executor {
	return &runtimeExecutor{logger: logger}
}

// Init wires live communication dependencies required by executor.
func (e *runtimeExecutor) Init(_ context.Context, communication internal_type.Communication) {
	e.onPacket = communication.OnPacket
}

// Execute runs webhook dispatch for packet event.
func (e *runtimeExecutor) Execute(ctx context.Context, packet internal_type.RunWebhookPacket) error {
	client := rest.NewRestClientWithConfig(packet.Webhook.GetUrl(), packet.Webhook.GetHeaders(), packet.Webhook.GetTimeoutSecond())
	startTime := time.Now()
	for retryCount := uint32(0); retryCount <= packet.Webhook.GetMaxRetryCount(); retryCount++ {
		switch packet.Webhook.GetMethod() {
		case "POST":
			response, err := client.Post(ctx, "", packet.Arguments, packet.Webhook.GetHeaders())
			if err != nil {
				e.logger.Warnw("Webhook execution failed", "url", packet.Webhook.GetUrl(), "error", err)
				if retryCount < packet.Webhook.GetMaxRetryCount() {
					time.Sleep(2 * time.Second)
				}
				continue
			}
			if !slices.Contains(packet.Webhook.GetRetryStatusCode(), strconv.Itoa(response.StatusCode)) {
				break
			}
			if retryCount < packet.Webhook.GetMaxRetryCount() {
				time.Sleep(2 * time.Second)
			}

			requestPayload, _ := utils.Serialize(packet.Arguments)
			responsePayload, _ := response.ToJSON()
			if err := e.onPacket(ctx, internal_type.WebhookLogCreatePacket{
				ContextID:       packet.ContextID,
				WebhookID:       packet.Webhook.Id,
				HTTPURL:         packet.Webhook.GetUrl(),
				HTTPMethod:      packet.Webhook.GetMethod(),
				Event:           packet.Event.Get(),
				ResponseStatus:  int64(response.StatusCode),
				TimeTaken:       int64(time.Since(startTime)),
				RetryCount:      retryCount,
				Status:          type_enums.RECORD_COMPLETE,
				RequestPayload:  requestPayload,
				ResponsePayload: responsePayload,
			}); err != nil {
				e.logger.Warnw("Failed to enqueue webhook log", "error", err)
			}
		case "PUT":
			response, err := client.Put(ctx, "", packet.Arguments, packet.Webhook.GetHeaders())
			if err != nil {
				e.logger.Warnw("Webhook execution failed", "url", packet.Webhook.GetUrl(), "error", err)
				if retryCount < packet.Webhook.GetMaxRetryCount() {
					time.Sleep(2 * time.Second)
				}
				continue
			}
			if !slices.Contains(packet.Webhook.GetRetryStatusCode(), strconv.Itoa(response.StatusCode)) {
				break
			}
			if retryCount < packet.Webhook.GetMaxRetryCount() {
				time.Sleep(2 * time.Second)
			}
			requestPayload, _ := utils.Serialize(packet.Arguments)
			responsePayload, _ := response.ToJSON()
			if err := e.onPacket(ctx, internal_type.WebhookLogCreatePacket{
				ContextID:       packet.ContextID,
				WebhookID:       packet.Webhook.Id,
				HTTPURL:         packet.Webhook.GetUrl(),
				HTTPMethod:      packet.Webhook.GetMethod(),
				Event:           packet.Event.Get(),
				ResponseStatus:  int64(response.StatusCode),
				TimeTaken:       int64(time.Since(startTime)),
				RetryCount:      retryCount,
				Status:          type_enums.RECORD_COMPLETE,
				RequestPayload:  requestPayload,
				ResponsePayload: responsePayload,
			}); err != nil {
				e.logger.Warnw("Failed to enqueue webhook log", "error", err)
			}
		case "PATCH":
			response, err := client.Patch(ctx, "", packet.Arguments, packet.Webhook.GetHeaders())
			if err != nil {
				e.logger.Warnw("Webhook execution failed", "url", packet.Webhook.GetUrl(), "error", err)
				if retryCount < packet.Webhook.GetMaxRetryCount() {
					time.Sleep(2 * time.Second)
				}
				continue
			}
			if !slices.Contains(packet.Webhook.GetRetryStatusCode(), strconv.Itoa(response.StatusCode)) {
				break
			}
			if retryCount < packet.Webhook.GetMaxRetryCount() {
				time.Sleep(2 * time.Second)
			}
			requestPayload, _ := utils.Serialize(packet.Arguments)
			responsePayload, _ := response.ToJSON()
			if err := e.onPacket(ctx, internal_type.WebhookLogCreatePacket{
				ContextID:       packet.ContextID,
				WebhookID:       packet.Webhook.Id,
				HTTPURL:         packet.Webhook.GetUrl(),
				HTTPMethod:      packet.Webhook.GetMethod(),
				Event:           packet.Event.Get(),
				ResponseStatus:  int64(response.StatusCode),
				TimeTaken:       int64(time.Since(startTime)),
				RetryCount:      retryCount,
				Status:          type_enums.RECORD_COMPLETE,
				RequestPayload:  requestPayload,
				ResponsePayload: responsePayload,
			}); err != nil {
				e.logger.Warnw("Failed to enqueue webhook log", "error", err)
			}
		default:
			response, err := client.Get(ctx, "", packet.Arguments, packet.Webhook.GetHeaders())
			if err != nil {
				e.logger.Warnw("Webhook execution failed", "url", packet.Webhook.GetUrl(), "error", err)
				if retryCount < packet.Webhook.GetMaxRetryCount() {
					time.Sleep(2 * time.Second)
				}
				continue
			}
			if !slices.Contains(packet.Webhook.GetRetryStatusCode(), strconv.Itoa(response.StatusCode)) {
				break
			}
			if retryCount < packet.Webhook.GetMaxRetryCount() {
				time.Sleep(2 * time.Second)
			}
			requestPayload, _ := utils.Serialize(packet.Arguments)
			responsePayload, _ := response.ToJSON()
			if err := e.onPacket(ctx, internal_type.WebhookLogCreatePacket{
				ContextID:       packet.ContextID,
				WebhookID:       packet.Webhook.Id,
				HTTPURL:         packet.Webhook.GetUrl(),
				HTTPMethod:      packet.Webhook.GetMethod(),
				Event:           packet.Event.Get(),
				ResponseStatus:  int64(response.StatusCode),
				TimeTaken:       int64(time.Since(startTime)),
				RetryCount:      retryCount,
				Status:          type_enums.RECORD_COMPLETE,
				RequestPayload:  requestPayload,
				ResponsePayload: responsePayload,
			}); err != nil {
				e.logger.Warnw("Failed to enqueue webhook log", "error", err)
			}
		}
	}
	return nil
}

// Close releases executor dependencies.
func (e *runtimeExecutor) Close(_ context.Context) {
	e.onPacket = nil
}
