// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_vonage_telephony

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rapidaai/api/assistant-api/config"
	internal_vonage "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/vonage/internal"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
	"github.com/vonage/vonage-go-sdk"
	"github.com/vonage/vonage-go-sdk/ncco"
)

const vonageProvider = "vonage"

type vonageTelephony struct {
	appCfg *config.AssistantConfig
	logger commons.Logger
}

func NewVonageTelephony(config *config.AssistantConfig, logger commons.Logger) (internal_type.Telephony, error) {
	return &vonageTelephony{
		logger: logger,
		appCfg: config,
	}, nil
}

func vonageAuth(vaultCredential *protos.VaultCredential) (vonage.Auth, error) {
	if vaultCredential.GetValue() == nil {
		return nil, fmt.Errorf("vault credential value is nil")
	}
	vaultMap := vaultCredential.GetValue().AsMap()
	privateKey, ok := vaultMap["private_key"]
	if !ok {
		return nil, fmt.Errorf("illegal vault config privateKey is not found")
	}
	applicationId, ok := vaultMap["application_id"]
	if !ok {
		return nil, fmt.Errorf("illegal vault config application_id is not found")
	}
	pk, ok := privateKey.(string)
	if !ok {
		return nil, fmt.Errorf("illegal vault config private_key is not a string")
	}
	appID, ok := applicationId.(string)
	if !ok {
		return nil, fmt.Errorf("illegal vault config application_id is not a string")
	}
	clientAuth, err := vonage.CreateAuthFromAppPrivateKey(appID, []byte(pk))
	if err != nil {
		return nil, err
	}
	return clientAuth, nil
}

func (vng *vonageTelephony) CatchAllStatusCallback(ctx *gin.Context) (*internal_type.StatusInfo, error) {
	eventDetails := map[string]interface{}{}
	for key, values := range ctx.Request.URL.Query() {
		if len(values) > 0 {
			eventDetails[key] = values[0]
		} else {
			eventDetails[key] = nil
		}
	}

	callback, err := internal_vonage.NewStatusCallback(eventDetails)
	if err != nil {
		vng.logger.Errorf("failed to parse status callback: %+v", err)
		return nil, err
	}
	if !validator.NotBlank(callback.ChannelUUID) {
		vng.logger.Errorf("uuid not found or invalid in catch-all payload")
		return nil, fmt.Errorf("uuid not found in callback")
	}

	vng.logger.Debugf("catch-all event processed | status: %s, payload: %+v", callback.Status, eventDetails)
	return callback.StatusInfo(), nil
}

func (vng *vonageTelephony) StatusCallback(c *gin.Context, auth types.SimplePrinciple, assistantId uint64, assistantConversationId uint64) (*internal_type.StatusInfo, error) {
	var payload map[string]interface{}
	if len(c.Request.URL.Query()) > 0 {
		payload = make(map[string]interface{})
		for key, values := range c.Request.URL.Query() {
			if len(values) > 0 {
				payload[key] = values[0]
			} else {
				payload[key] = nil
			}
		}
	} else {
		body, err := c.GetRawData()
		if err != nil {
			vng.logger.Errorf("failed to read request body with error %+v", err)
			return nil, fmt.Errorf("failed to read request body")
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			vng.logger.Errorf("failed to parse request body: %+v", err)
			return nil, fmt.Errorf("failed to parse request body")
		}
	}

	callback, err := internal_vonage.NewStatusCallback(payload)
	if err != nil {
		vng.logger.Errorf("failed to parse status callback: %+v", err)
		return nil, err
	}
	vng.logger.Debugf("event processed | status: %s, payload: %+v", callback.Status, payload)
	return callback.StatusInfo(), nil
}

func (vng *vonageTelephony) OutboundCall(
	auth types.SimplePrinciple,
	toPhone string,
	fromPhone string,
	assistant *internal_assistant_entity.Assistant, assistantConversationId uint64,
	vaultCredential *protos.VaultCredential,
	opts utils.Option,
) (*internal_type.CallInfo, error) {
	info := &internal_type.CallInfo{Provider: vonageProvider}

	cAuth, err := vonageAuth(vaultCredential)
	if err != nil {
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("authentication error: %s", err.Error())
		return info, err
	}
	ct := vonage.NewVoiceClient(cAuth)

	contextID, _ := opts.GetString("rapida.context_id")

	connectAction := ncco.Ncco{}
	nccoConnect := ncco.ConnectAction{
		EventType: "synchronous",
		EventUrl:  []string{fmt.Sprintf("https://%s/%s", vng.appCfg.Assistant.Public, internal_type.GetContextEventPath(vonageProvider, contextID))},
		Endpoint: []ncco.Endpoint{ncco.WebSocketEndpoint{
			Uri: fmt.Sprintf("wss://%s/%s",
				vng.appCfg.Assistant.Public,
				internal_type.GetContextAnswerPath(vonageProvider, contextID)),
			ContentType: "audio/l16;rate=16000",
		}},
	}
	connectAction.AddAction(nccoConnect)
	result, vErr, apiError := ct.CreateCall(
		vonage.CreateCallOpts{
			From:        vonage.CallFrom{Type: "phone", Number: fromPhone},
			To:          vonage.CallTo{Type: "phone", Number: toPhone},
			Ncco:        connectAction,
			EventUrl:    []string{fmt.Sprintf("https://%s/%s", vng.appCfg.Assistant.Public, internal_type.GetContextEventPath(vonageProvider, contextID))},
			EventMethod: "GET",
		})

	if apiError != nil {
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("API error: %s", apiError.Error())
		return info, apiError
	}

	if vErr.Error != nil {
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("Calling error: %v", vErr.Error)
		return info, fmt.Errorf("failed to create call")
	}

	info.ChannelUUID = result.Uuid
	info.Status = "SUCCESS"
	info.StatusInfo = internal_type.StatusInfo{Event: result.Status, Payload: result}
	info.Extra = map[string]string{
		"conversation_uuid": result.ConversationUuid,
	}
	return info, nil
}

func (vng *vonageTelephony) InboundCall(c *gin.Context, auth types.SimplePrinciple, assistantId uint64, clientNumber string, assistantConversationId uint64) error {
	contextID, _ := c.Get("contextId")
	ctxID := fmt.Sprintf("%v", contextID)

	c.JSON(http.StatusOK, []gin.H{
		{
			"action":    "connect",
			"eventType": "synchronous",
			"eventUrl":  []string{fmt.Sprintf("https://%s/%s", vng.appCfg.Assistant.Public, internal_type.GetContextEventPath("vonage", ctxID))},
			"endpoint": []gin.H{
				{
					"type": "websocket",
					"uri": fmt.Sprintf("wss://%s/%s",
						vng.appCfg.Assistant.Public,
						internal_type.GetContextAnswerPath("vonage", ctxID)),
					"content-type": "audio/l16;rate=16000",
				},
			},
		},
	})
	return nil
}

func (vng *vonageTelephony) ReceiveCall(c *gin.Context) (*internal_type.CallInfo, error) {
	queryParams := make(map[string]string)
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			queryParams[key] = values[0]
		}
	}

	clientNumber, ok := queryParams["from"]
	if !ok || clientNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assistant ID"})
		return nil, fmt.Errorf("missing or empty 'from' query parameter")
	}

	info := &internal_type.CallInfo{
		CallerNumber: clientNumber,
		Provider:     vonageProvider,
		Status:       "SUCCESS",
		StatusInfo:   internal_type.StatusInfo{Event: "webhook", Payload: queryParams},
		Extra:        make(map[string]string),
	}

	if v, ok := queryParams["to"]; ok && v != "" {
		info.FromNumber = v
	}
	if v, ok := queryParams["conversation_uuid"]; ok && v != "" {
		info.Extra["conversation_uuid"] = v
	}
	if v, ok := queryParams["uuid"]; ok && v != "" {
		info.ChannelUUID = v
	}
	return info, nil
}
