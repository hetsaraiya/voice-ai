// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package channel_telephony

import (
	"context"
	"fmt"
	"time"

	"github.com/rapidaai/api/assistant-api/config"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	internal_telephony_base "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/base"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	web_client "github.com/rapidaai/pkg/clients/web"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
)

type OutboundDispatcher struct {
	cfg                    *config.AssistantConfig
	store                  callcontext.Store
	logger                 commons.Logger
	vaultClient            web_client.VaultClient
	assistantService       internal_services.AssistantService
	conversationService    internal_services.AssistantConversationService
	telephonyOpt           TelephonyOption
	outboundConnectTimeout time.Duration
}

func NewOutboundDispatcher(deps TelephonyDispatcherDeps) *OutboundDispatcher {
	return &OutboundDispatcher{
		cfg:                    deps.Cfg,
		store:                  deps.Store,
		logger:                 deps.Logger,
		vaultClient:            deps.VaultClient,
		assistantService:       deps.AssistantService,
		conversationService:    deps.ConversationService,
		telephonyOpt:           deps.TelephonyOpt,
		outboundConnectTimeout: defaultOutboundConnectTimeout,
	}
}

func (d *OutboundDispatcher) Dispatch(ctx context.Context, contextID string) error {
	cc, err := d.store.Get(ctx, contextID)
	if err != nil {
		d.logger.Errorf("outbound dispatcher: failed to resolve call context %s: %v", contextID, err)
		return err
	}

	d.logger.Infof("outbound dispatcher[%s]: processing contextId=%s, assistant=%d, conversation=%d",
		cc.Provider, cc.ContextID, cc.AssistantID, cc.ConversationID)

	if err := d.performOutbound(ctx, cc); err != nil {
		d.logger.Errorf("outbound dispatcher[%s]: call failed for contextId=%s: %v", cc.Provider, contextID, err)
		d.persistOutboundSetupFailure(ctx, cc, err)
		return err
	}

	d.logger.Infof("outbound dispatcher[%s]: call initiated for contextId=%s", cc.Provider, contextID)

	// The answer monitor must outlive the API request that initiated the call.
	callMonitorContext := context.WithoutCancel(ctx)
	go d.monitorCallConnect(callMonitorContext, contextID, cc)

	return nil
}

const defaultOutboundConnectTimeout = 2 * time.Minute

// monitorCallConnect marks unclaimed outbound calls as no-answer after the provider timeout.
func (d *OutboundDispatcher) monitorCallConnect(ctx context.Context, contextID string, cc *callcontext.CallContext) {
	outboundConnectTimeout := d.providerOutboundConnectTimeout(cc.Provider)
	select {
	case <-ctx.Done():
		return
	case <-time.After(outboundConnectTimeout):
	}

	currentCallContext, err := d.store.Get(ctx, contextID)
	if err != nil {
		return
	}
	if currentCallContext.Status != callcontext.StatusPending {
		return // Already claimed or processed
	}

	d.logger.Warnw("Outbound call not answered within timeout, marking as failed",
		"contextId", contextID,
		"provider", cc.Provider,
		"timeout", outboundConnectTimeout)

	d.persistOutboundConnectTimeout(ctx, currentCallContext)

	if d.conversationService != nil {
		auth := cc.ToAuth()
		d.conversationService.PersistMetrics(ctx, auth, cc.AssistantID, cc.ConversationID, []*types.Metric{
			{Name: "status", Value: "FAILED", Description: "no_answer_timeout"},
		})
	}
}

func (d *OutboundDispatcher) providerOutboundConnectTimeout(provider string) time.Duration {
	timeout := d.outboundConnectTimeout
	if timeout <= 0 {
		timeout = defaultOutboundConnectTimeout
	}
	if provider == SIP.String() && d.cfg != nil && d.cfg.SIPConfig != nil && d.cfg.SIPConfig.InviteTimeout > 0 {
		return d.cfg.SIPConfig.InviteTimeout + 15*time.Second
	}
	return timeout
}

func (d *OutboundDispatcher) performOutbound(ctx context.Context, cc *callcontext.CallContext) error {
	telephony, err := GetTelephony(Telephony(cc.Provider), d.cfg, d.logger, d.telephonyOpt)
	if err != nil {
		return fmt.Errorf("telephony provider %s not available: %w", cc.Provider, err)
	}

	auth := cc.ToAuth()

	assistant, err := d.assistantService.Get(ctx, auth, cc.AssistantID, nil, &internal_services.GetAssistantOption{InjectPhoneDeployment: true})
	if err != nil {
		return fmt.Errorf("failed to load assistant %d: %w", cc.AssistantID, err)
	}
	if !assistant.IsPhoneDeploymentEnable() {
		return fmt.Errorf("phone deployment not enabled for assistant %d", cc.AssistantID)
	}

	credentialID, err := assistant.AssistantPhoneDeployment.GetOptions().GetUint64("rapida.credential_id")
	if err != nil {
		return fmt.Errorf("failed to get credential ID: %w", err)
	}

	vltC, err := d.vaultClient.GetCredential(ctx, auth, credentialID)
	if err != nil {
		return fmt.Errorf("failed to get vault credential: %w", err)
	}

	opts := assistant.AssistantPhoneDeployment.GetOptions()
	opts["rapida.context_id"] = cc.ContextID

	statusReporter := d.newProviderCallStatusReporter(cc.ContextID)
	callInfo, callErr := telephony.OutboundCall(ctx, auth, cc.CallerNumber, cc.FromNumber, assistant, cc.ConversationID, vltC, statusReporter, opts)
	if callErr != nil {
		d.logger.Errorf("outbound dispatcher[%s]: telephony call failed for contextId=%s: %v", cc.Provider, cc.ContextID, callErr)
	}
	if callInfo == nil {
		return callErr
	}

	if callInfo.ChannelUUID != "" {
		if updateErr := d.store.UpdateField(ctx, cc.ContextID, "channel_uuid", callInfo.ChannelUUID); updateErr != nil {
			d.logger.Warnf("outbound dispatcher[%s]: failed to store channel UUID: %v", cc.Provider, updateErr)
		}
	}

	return callErr
}

func (d *OutboundDispatcher) newProviderCallStatusReporter(contextID string) internal_type.ProviderCallStatusReporter {
	return func(update internal_type.ProviderCallStatusUpdate) {
		if update.ChannelUUID != "" {
			if err := d.store.UpdateField(context.Background(), contextID, "channel_uuid", update.ChannelUUID); err != nil {
				d.logger.Warnw("Failed to persist outbound channel UUID",
					"contextId", contextID,
					"channel_uuid", update.ChannelUUID,
					"error", err)
			}
		}
		if update.CallStatus == "" {
			return
		}
		err := d.store.UpdateCallStatus(context.Background(), contextID, callcontext.CallStatusUpdate{
			CallStatus:         update.CallStatus,
			CallError:          update.ErrorMessage,
			FailureClass:       update.FailureClass,
			FailureReason:      update.FailureReason,
			DisconnectReason:   update.DisconnectReason,
			Retryable:          update.Retryable,
			ProviderStatusCode: update.ProviderStatusCode,
		})
		if err != nil {
			d.logger.Warnw("Failed to persist outbound status",
				"contextId", contextID,
				"call_status", update.CallStatus,
				"failure_class", update.FailureClass,
				"error", err)
		}
	}
}

func (d *OutboundDispatcher) persistOutboundSetupFailure(ctx context.Context, cc *callcontext.CallContext, setupErr error) {
	if cc == nil || setupErr == nil {
		return
	}
	d.updateOutboundFailureIfNotTerminal(ctx, cc.ContextID, callcontext.CallStatusUpdate{
		CallStatus:       internal_telephony_base.OutboundCallStatusFailed,
		CallError:        setupErr.Error(),
		FailureClass:     internal_telephony_base.OutboundFailureClassSetup,
		FailureReason:    "outbound setup failed",
		DisconnectReason: internal_telephony_base.OutboundDisconnectReasonSetupFailed,
	})
}

func (d *OutboundDispatcher) persistOutboundConnectTimeout(ctx context.Context, cc *callcontext.CallContext) {
	if cc == nil {
		return
	}
	d.updateOutboundFailureIfNotTerminal(ctx, cc.ContextID, callcontext.CallStatusUpdate{
		CallStatus:       internal_telephony_base.OutboundCallStatusFailed,
		FailureClass:     internal_telephony_base.OutboundFailureClassNoAnswer,
		FailureReason:    internal_telephony_base.OutboundFailureReasonNoAnswer,
		DisconnectReason: internal_telephony_base.OutboundDisconnectReasonNoAnswer,
		Retryable:        true,
	})
}

func (d *OutboundDispatcher) updateOutboundFailureIfNotTerminal(ctx context.Context, contextID string, status callcontext.CallStatusUpdate) {
	current, err := d.store.Get(ctx, contextID)
	if err == nil && callContextHasTerminalOutboundStatus(current) {
		return
	}
	if err := d.store.UpdateCallStatus(ctx, contextID, status); err != nil {
		d.logger.Warnw("Failed to persist outbound failure status",
			"contextId", contextID,
			"call_status", status.CallStatus,
			"failure_class", status.FailureClass,
			"error", err)
	}
}

func callContextHasTerminalOutboundStatus(cc *callcontext.CallContext) bool {
	if cc == nil {
		return false
	}
	switch cc.Status {
	case callcontext.StatusFailed, callcontext.StatusCompleted:
		return true
	}
	switch cc.CallStatus {
	case internal_telephony_base.OutboundCallStatusFailed, internal_telephony_base.OutboundCallStatusCancelled:
		return true
	default:
		return false
	}
}
