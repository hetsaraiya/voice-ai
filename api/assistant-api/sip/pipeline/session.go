// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package sip_pipeline

import (
	"context"
	"fmt"

	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/api/assistant-api/internal/observability/collectors"
	observability_collector_conversationdb "github.com/rapidaai/api/assistant-api/internal/observability/collectors/conversationdb"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
)

func (d *Dispatcher) createConversation(ctx context.Context, stage sip_infra.SessionEstablishedPipeline) (uint64, error) {
	dirEnum := type_enums.DIRECTION_INBOUND
	if stage.Direction == sip_infra.CallDirectionOutbound {
		dirEnum = type_enums.DIRECTION_OUTBOUND
	}

	callerNumber := sip_infra.ExtractDIDFromURI(stage.FromURI)
	if callerNumber == "" {
		callerNumber = stage.FromURI
	}

	assistant := stage.Session.GetAssistant()
	if assistant == nil && d.assistantService != nil {
		var err error
		assistant, err = d.assistantService.Get(ctx, stage.Auth, stage.AssistantID, utils.GetVersionDefinition("latest"),
			&internal_services.GetAssistantOption{InjectPhoneDeployment: true})
		if err != nil {
			return 0, fmt.Errorf("failed to load assistant %d: %w", stage.AssistantID, err)
		}
	}

	assistantID := stage.AssistantID
	var assistantProviderID uint64
	if assistant != nil {
		assistantID = assistant.Id
		assistantProviderID = assistant.AssistantProviderId
	}
	if d.assistantConversationService == nil {
		return 0, fmt.Errorf("assistant conversation service not configured")
	}
	conversation, err := d.assistantConversationService.CreateConversation(
		ctx, stage.Auth, callerNumber, assistantID, assistantProviderID, dirEnum, utils.PhoneCall,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create conversation: %w", err)
	}
	return conversation.Id, nil
}

func (d *Dispatcher) ensureCallContext(ctx context.Context, stage sip_infra.SessionEstablishedPipeline, conversationID uint64) (*callcontext.CallContext, error) {
	if d.callContextStore == nil {
		return nil, nil
	}

	callID := stage.Session.GetCallID()
	dirStr := string(stage.Direction)
	if stage.Direction == sip_infra.CallDirectionOutbound {
		contextID := stage.Session.GetContextID()
		if contextID == "" {
			return reconstructCallContext(stage.Auth, stage.AssistantID, conversationID, dirStr, callID, "", stage.FromURI, stage.ToURI), nil
		}
		if claimed, err := d.callContextStore.Claim(ctx, contextID); err == nil {
			return claimed, nil
		}
		if loaded, err := d.callContextStore.Get(ctx, contextID); err == nil {
			return loaded, nil
		}
		return reconstructCallContext(stage.Auth, stage.AssistantID, conversationID, dirStr, callID, contextID, stage.FromURI, stage.ToURI), nil
	}

	callContext := &callcontext.CallContext{
		AssistantID:    stage.AssistantID,
		ConversationID: conversationID,
		AuthToken:      stage.Auth.GetCurrentToken(),
		AuthType:       stage.Auth.Type().String(),
		Direction:      dirStr,
		Provider:       "sip",
		CallerNumber:   extractDIDOrRaw(stage.FromURI),
		FromNumber:     extractDIDOrRaw(stage.ToURI),
		ChannelUUID:    callID,
	}
	if pid := stage.Auth.GetCurrentProjectId(); pid != nil {
		callContext.ProjectID = *pid
	}
	if oid := stage.Auth.GetCurrentOrganizationId(); oid != nil {
		callContext.OrganizationID = *oid
	}
	if assistant := stage.Session.GetAssistant(); assistant != nil {
		callContext.AssistantProviderId = assistant.AssistantProviderId
	}
	if _, err := d.callContextStore.Save(ctx, callContext); err != nil {
		d.logger.Warnw("failed to persist inbound call context — continuing in-memory",
			"call_id", callID, "error", err)
		return callContext, nil
	}
	if _, err := d.callContextStore.Claim(ctx, callContext.ContextID); err != nil {
		d.logger.Debugw("inbound claim non-fatal", "call_id", callID, "error", err)
	}
	return callContext, nil
}

func (d *Dispatcher) setupCall(ctx context.Context, stage sip_infra.SessionEstablishedPipeline, conversationID uint64, cc *callcontext.CallContext) (*CallSetupResult, error) {
	assistant := stage.Session.GetAssistant()
	if assistant == nil && d.assistantService != nil {
		var err error
		assistant, err = d.assistantService.Get(ctx, stage.Auth, stage.AssistantID, utils.GetVersionDefinition("latest"),
			&internal_services.GetAssistantOption{InjectPhoneDeployment: true})
		if err != nil {
			return nil, fmt.Errorf("failed to load assistant %d: %w", stage.AssistantID, err)
		}
	}

	result := &CallSetupResult{
		AssistantID:    stage.AssistantID,
		ConversationID: conversationID,
		CallContext:    cc,
	}
	if assistant != nil {
		result.AssistantID = assistant.Id
		result.AssistantProviderId = assistant.AssistantProviderId
	}
	if stage.Auth != nil {
		result.AuthToken = stage.Auth.GetCurrentToken()
		result.AuthType = stage.Auth.Type().String()
		if stage.Auth.GetCurrentProjectId() != nil {
			result.ProjectID = *stage.Auth.GetCurrentProjectId()
		}
		if stage.Auth.GetCurrentOrganizationId() != nil {
			result.OrganizationID = *stage.Auth.GetCurrentOrganizationId()
		}
	}

	return result, nil
}

func (d *Dispatcher) createObserver(ctx context.Context, setup *CallSetupResult, auth types.SimplePrinciple) observability.Recorder {
	otelCollectors := make([]observability.Collector, 0)
	if d.assistantConversationService != nil {
		otelCollectors = append(otelCollectors, observability_collector_conversationdb.New(observability_collector_conversationdb.Config{
			Logger:              d.logger,
			ConversationService: d.assistantConversationService,
		}))
	}
	otelCollectors = append(otelCollectors, collectors.NewWithEnv(ctx, d.logger, d.assistantConfig)...)
	return observability.New(
		observability.WithLogger(d.logger),
		observability.WithAuth(auth),
		observability.WithGlobalScope(observability.GlobalScope{
			ProjectID:      setup.ProjectID,
			OrganizationID: setup.OrganizationID,
		}),
		observability.WithContext(ctx),
		observability.WithGracePeriod(),
		observability.WithCollectors(otelCollectors...),
	)
}

func reconstructCallContext(
	auth types.SimplePrinciple,
	assistantID uint64,
	conversationID uint64,
	direction string,
	callID string,
	contextID string,
	fromURI string,
	toURI string,
) *callcontext.CallContext {
	callContext := &callcontext.CallContext{
		AssistantID:    assistantID,
		ConversationID: conversationID,
		AuthToken:      auth.GetCurrentToken(),
		AuthType:       auth.Type().String(),
		Direction:      direction,
		Provider:       "sip",
		ChannelUUID:    callID,
		ContextID:      contextID,
	}
	if direction == string(sip_infra.CallDirectionOutbound) {
		callContext.CallerNumber = extractDIDOrRaw(toURI)
		callContext.FromNumber = extractDIDOrRaw(fromURI)
	} else {
		callContext.CallerNumber = extractDIDOrRaw(fromURI)
		callContext.FromNumber = extractDIDOrRaw(toURI)
	}
	if pid := auth.GetCurrentProjectId(); pid != nil {
		callContext.ProjectID = *pid
	}
	if oid := auth.GetCurrentOrganizationId(); oid != nil {
		callContext.OrganizationID = *oid
	}
	return callContext
}

func extractDIDOrRaw(uri string) string {
	if uri == "" {
		return ""
	}
	if did := sip_infra.ExtractDIDFromURI(uri); did != "" {
		return did
	}
	return uri
}
