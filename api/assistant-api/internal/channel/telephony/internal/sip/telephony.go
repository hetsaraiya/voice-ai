// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_sip_telephony

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rapidaai/api/assistant-api/config"
	internal_telephony_base "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/base"
	internal_sip "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/sip/internal"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

type sipTelephony struct {
	appCfg       *config.AssistantConfig
	logger       commons.Logger
	sharedServer *sip_infra.Server
}

func NewSIPTelephony(cfg *config.AssistantConfig, logger commons.Logger, sipServer *sip_infra.Server) (internal_type.Telephony, error) {
	return &sipTelephony{
		appCfg:       cfg,
		logger:       logger,
		sharedServer: sipServer,
	}, nil
}

func (t *sipTelephony) parseConfig(vaultCredential *protos.VaultCredential) (*sip_infra.Config, error) {
	cfg, err := sip_infra.ParseConfigFromVault(vaultCredential)
	if err != nil {
		return nil, err
	}

	if cfg.Port <= 0 {
		cfg.Port = internal_sip.DefaultOutboundSIPPort
	}

	if t.appCfg.SIPConfig != nil {
		cfg.ApplyOperationalDefaults(
			t.appCfg.SIPConfig.Port,
			sip_infra.Transport(t.appCfg.SIPConfig.Transport),
			t.appCfg.SIPConfig.RTPPortRangeStart,
			t.appCfg.SIPConfig.RTPPortRangeEnd,
		)
		cfg.ApplyTimeoutDefaults(
			t.appCfg.SIPConfig.RegisterTimeout,
			t.appCfg.SIPConfig.InviteTimeout,
			t.appCfg.SIPConfig.SessionTimeout,
		)
		inboundConfig := t.appCfg.SIPConfig.Inbound
		cfg.ApplyInboundAnswerDefaults(
			sip_infra.InboundAnswerMode(inboundConfig.AnswerMode),
			inboundConfig.MinRingDuration,
			inboundConfig.MaxRingDuration,
			inboundConfig.ACKTimeout,
			inboundConfig.AssistantAudioReadyTimeout,
			inboundConfig.RequireAssistantAudioReady,
		)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (t *sipTelephony) StatusCallback(
	c *gin.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
	assistantConversationId uint64,
) (*internal_type.StatusInfo, error) {
	payload := make(map[string]interface{})
	if body, err := c.GetRawData(); err == nil && len(body) > 0 {
		if json.Unmarshal(body, &payload) != nil {
			if formErr := c.Request.ParseForm(); formErr == nil {
				for k, v := range c.Request.PostForm {
					payload[k] = v[0]
				}
			}
		}
	}
	if len(payload) == 0 {
		for k, v := range c.Request.URL.Query() {
			payload[k] = v[0]
		}
	}

	eventType, _ := payload["event"].(string)
	callID, _ := payload["call_id"].(string)

	t.logger.Debug("SIP status callback received",
		"event", eventType,
		"call_id", callID,
		"assistant_id", assistantId,
		"conversation_id", assistantConversationId)

	return &internal_type.StatusInfo{Event: eventType, Payload: payload}, nil
}

func (t *sipTelephony) CatchAllStatusCallback(ctx *gin.Context) (*internal_type.StatusInfo, error) {
	return nil, nil
}

func (t *sipTelephony) OutboundCall(
	ctx context.Context,
	auth types.SimplePrinciple,
	toPhone string,
	fromPhone string,
	assistant *internal_assistant_entity.Assistant,
	assistantConversationId uint64,
	vaultCredential *protos.VaultCredential,
	statusReporter internal_type.ProviderCallStatusReporter,
	opts utils.Option,
) (*internal_type.CallInfo, error) {
	info := &internal_type.CallInfo{Provider: internal_sip.Provider}

	cfg, err := t.parseConfig(vaultCredential)
	if err != nil {
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("config error: %s", err.Error())
		internal_telephony_base.ReportOutboundFailure(
			statusReporter,
			internal_telephony_base.OutboundFailureClassConfiguration,
			"invalid SIP outbound configuration",
			internal_telephony_base.OutboundDisconnectReasonSetupFailed,
			err,
			0,
		)
		return info, err
	}

	contextID, _ := opts.GetString("rapida.context_id")
	fromUser := strings.TrimSpace(fromPhone)
	assistantID := outboundAssistantID(assistant)
	if assistant != nil && assistant.AssistantPhoneDeployment != nil {
		if deploymentPhone, phoneErr := assistant.AssistantPhoneDeployment.GetOptions().GetString("phone"); phoneErr == nil && deploymentPhone != "" {
			fromUser = strings.TrimSpace(deploymentPhone)
		}
	}

	if t.sharedServer == nil {
		err := fmt.Errorf("shared SIP server not available")
		info.Status = "FAILED"
		info.ErrorMessage = "SIP server not initialized"
		t.logger.Warnw("SIP outbound call blocked before setup",
			"context_id", contextID,
			"assistant_id", assistantID,
			"conversation_id", assistantConversationId,
			"to_user", strings.TrimSpace(toPhone),
			"from_user", fromUser,
			"trunk_address", cfg.Server,
			"reason", "server_not_initialized")
		internal_telephony_base.ReportOutboundFailure(
			statusReporter,
			internal_telephony_base.OutboundFailureClassHealthGate,
			"sip server not initialized",
			internal_telephony_base.OutboundDisconnectReasonHealthGate,
			err,
			0,
		)
		return info, err
	}
	if !t.sharedServer.IsRunning() {
		err := fmt.Errorf("shared SIP server is not running")
		info.Status = "FAILED"
		info.ErrorMessage = "SIP server not running"
		t.logger.Warnw("SIP outbound call blocked before setup",
			"context_id", contextID,
			"assistant_id", assistantID,
			"conversation_id", assistantConversationId,
			"to_user", strings.TrimSpace(toPhone),
			"from_user", fromUser,
			"trunk_address", cfg.Server,
			"reason", "server_not_running")
		internal_telephony_base.ReportOutboundFailure(
			statusReporter,
			internal_telephony_base.OutboundFailureClassHealthGate,
			"sip server not running",
			internal_telephony_base.OutboundDisconnectReasonHealthGate,
			err,
			0,
		)
		return info, err
	}

	t.logger.Infow("SIP outbound call setup requested",
		"context_id", contextID,
		"assistant_id", assistantID,
		"conversation_id", assistantConversationId,
		"to_user", strings.TrimSpace(toPhone),
		"from_user", fromUser,
		"trunk_address", cfg.Server,
		"trunk_port", cfg.Port,
		"transport", cfg.GetTransport(),
		"ringing_timeout_ms", cfg.InviteTimeout.Milliseconds(),
		"max_call_duration_ms", cfg.SessionTimeout.Milliseconds(),
		"outbound_health_gate", outboundHealthGateEnabled(t.appCfg))

	if outboundHealthGateEnabled(t.appCfg) {
		healthSnapshot := t.sharedServer.HealthSnapshot()
		if !healthSnapshot.Ready {
			err := fmt.Errorf("SIP outbound health gate failed: %s", healthSnapshot.Reason)
			info.Status = "FAILED"
			info.ErrorMessage = err.Error()
			t.logger.Warnw("SIP outbound call blocked by health gate",
				"context_id", contextID,
				"assistant_id", assistantID,
				"conversation_id", assistantConversationId,
				"to_user", strings.TrimSpace(toPhone),
				"from_user", fromUser,
				"trunk_address", cfg.Server,
				"health_reason", healthSnapshot.Reason,
				"active_calls", healthSnapshot.ActiveCalls,
				"rtp_ports_in_use", healthSnapshot.RTPPortsInUse)
			internal_telephony_base.ReportOutboundFailure(
				statusReporter,
				internal_telephony_base.OutboundFailureClassHealthGate,
				healthSnapshot.Reason,
				internal_telephony_base.OutboundDisconnectReasonHealthGate,
				err,
				0,
			)
			return info, err
		}
	}

	session, err := t.sharedServer.MakeCall(ctx, cfg, toPhone, fromUser, sip_infra.MakeCallOptions{
		Auth:               auth,
		Assistant:          assistant,
		ConversationID:     assistantConversationId,
		ContextID:          contextID,
		VaultCredential:    vaultCredential,
		CallStatusObserver: statusReporter,
	})
	if err != nil {
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("call error: %s", err.Error())
		internal_telephony_base.ReportOutboundFailure(
			statusReporter,
			internal_telephony_base.OutboundFailureClassSetup,
			"sip outbound setup failed",
			internal_telephony_base.OutboundDisconnectReasonSetupFailed,
			err,
			0,
		)
		return info, err
	}
	t.logger.Infow("SIP outbound call initiated",
		"context_id", contextID,
		"to_user", strings.TrimSpace(toPhone),
		"from_user", fromUser,
		"trunk_address", cfg.Server,
		"trunk_port", cfg.Port,
		"transport", cfg.GetTransport(),
		"call_id", session.GetCallID(),
		"assistant_id", assistantID,
		"conversation_id", assistantConversationId)

	return newOutboundInitiatedCallInfo(session, toPhone, fromUser, assistantID, assistantConversationId), nil
}

func outboundAssistantID(assistant *internal_assistant_entity.Assistant) uint64 {
	if assistant == nil {
		return 0
	}
	return assistant.Id
}

func outboundHealthGateEnabled(appCfg *config.AssistantConfig) bool {
	if appCfg == nil || appCfg.SIPConfig == nil || appCfg.SIPConfig.OutboundHealthGate == nil {
		return true
	}
	return *appCfg.SIPConfig.OutboundHealthGate
}

func newOutboundInitiatedCallInfo(session *sip_infra.Session, toPhone string, fromUser string, assistantID uint64, assistantConversationID uint64) *internal_type.CallInfo {
	initiatedStatus := string(sip_infra.OutboundCallStatusInitiated)
	callID := session.GetCallID()
	return &internal_type.CallInfo{
		Provider:    internal_sip.Provider,
		ChannelUUID: callID,
		Status:      initiatedStatus,
		StatusInfo: internal_type.StatusInfo{
			Event: initiatedStatus,
			Payload: map[string]interface{}{
				"to":              toPhone,
				"from":            fromUser,
				"call_id":         callID,
				"assistant_id":    assistantID,
				"conversation_id": assistantConversationID,
			},
		},
		Extra: map[string]string{
			"telephony.status": initiatedStatus,
		},
	}
}

func (t *sipTelephony) InboundCall(
	c *gin.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
	clientNumber string,
	assistantConversationId uint64,
) error {
	c.JSON(http.StatusOK, gin.H{
		"status":          "ready",
		"assistant_id":    assistantId,
		"conversation_id": assistantConversationId,
		"client_number":   clientNumber,
		"message":         "SIP inbound call ready - connect via SIP signaling",
	})
	return nil
}

func (t *sipTelephony) ReceiveCall(c *gin.Context) (*internal_type.CallInfo, error) {
	clientNumber := c.Query("from")
	if clientNumber == "" {
		clientNumber = c.Query("caller")
	}
	if clientNumber == "" {
		return nil, fmt.Errorf("missing caller information")
	}

	dialedNumber := c.Query("to")
	if dialedNumber == "" {
		dialedNumber = c.Query("called")
	}
	if dialedNumber == "" {
		dialedNumber = c.Query("destination")
	}

	queryParams := make(map[string]string, len(c.Request.URL.Query()))
	for key, values := range c.Request.URL.Query() {
		queryParams[key] = values[0]
	}

	info := &internal_type.CallInfo{
		CallerNumber: clientNumber,
		FromNumber:   dialedNumber,
		Provider:     internal_sip.Provider,
		Status:       "SUCCESS",
		StatusInfo:   internal_type.StatusInfo{Event: "webhook", Payload: queryParams},
	}
	if callID := c.Query("call_id"); callID != "" {
		info.ChannelUUID = callID
	}
	return info, nil
}
