// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package adapter_internal

import (
	"context"
	"slices"
	"strings"

	internal_condition "github.com/rapidaai/api/assistant-api/internal/condition"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/api/assistant-api/internal/variable"
	"github.com/rapidaai/pkg/utils"
)

// OnBeginConversation fires webhooks subscribed to ConversationBegin.
// Delegates to packet dispatcher infrastructure.
func (md *genericRequestor) OnBeginConversation(ctx context.Context) error {
	return md.onWebhookEvent(ctx, md.GetID(), utils.ConversationBegin)
}

// OnResumeConversation fires webhooks subscribed to ConversationResume.
func (md *genericRequestor) OnResumeConversation(ctx context.Context) error {
	return md.onWebhookEvent(ctx, md.GetID(), utils.ConversationResume)
}

// OnErrorConversation fires webhooks subscribed to ConversationFailed.
func (md *genericRequestor) OnErrorConversation(ctx context.Context) error {
	return md.onWebhookEvent(ctx, md.GetID(), utils.ConversationFailed)
}

// OnEndConversation runs analysis first and then webhook (completed event) via packets.
func (md *genericRequestor) OnEndConversation(ctx context.Context) error {
	return md.onWebhookEvent(ctx, md.GetID(), utils.ConversationCompleted)
}

func (r *genericRequestor) onWebhookEvent(ctx context.Context, contextID string, event utils.AssistantWebhookEvent) error {
	source := variable.NewCommunicationSource(r)
	registry := variable.NewDefaultRegistry().With("event", &variable.EventNamespace{})
	for _, webhook := range r.assistant.AssistantWebhooks {
		if !slices.Contains(webhook.GetAssistantEvents(), event.Get()) {
			continue
		}
		if !r.isWebhookAllowed(webhook, r.Conversation().Direction.String()) {
			continue
		}
		if err := r.OnPacket(ctx, internal_type.RunWebhookPacket{
			ContextID: contextID,
			Event:     event,
			Webhook:   webhook,
			Arguments: registry.Apply(webhook.GetBody(), source, variable.ResolveContext{Event: event.Get()}),
		}); err != nil {
			r.logger.Warnw("failed to enqueue webhook packet", "webhookID", webhook.Id, "event", event.Get(), "error", err)
		}
	}

	return nil
}

func (r *genericRequestor) isWebhookAllowed(
	webhook *internal_assistant_entity.AssistantWebhook,
	direction string,
) bool {
	if webhook == nil {
		return false
	}
	rawCondition := strings.TrimSpace(webhook.GetHeaders()["webhook.condition"])
	if rawCondition == "" {
		return true
	}
	parsed, parseErr := internal_condition.Parse(rawCondition)
	if parseErr != nil {
		r.logger.Warnf("invalid webhook.condition for webhook %d, excluding webhook: %v", webhook.Id, parseErr)
		return false
	}
	allowed, evalErr := parsed.Run(
		internal_condition.ConditionValue{RuleType: internal_condition.RuleTypeSource, Value: r.GetSource().Get()},
		internal_condition.ConditionValue{RuleType: internal_condition.RuleTypeMode, Value: r.GetMode().String()},
		internal_condition.ConditionValue{RuleType: internal_condition.RuleTypeDirection, Value: direction},
	)
	if evalErr != nil {
		r.logger.Warnf("invalid webhook.condition for webhook %d, excluding webhook: %v", webhook.Id, evalErr)
		return false
	}
	return allowed
}
