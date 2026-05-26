// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_twilio_telephony

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/rapidaai/api/assistant-api/config"
	internal_twilio "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/twilio/internal"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
	"github.com/twilio/twilio-go"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"
)

const twilioProvider = "twilio"

type twilioTelephony struct {
	appCfg *config.AssistantConfig
	logger commons.Logger
}

func NewTwilioTelephony(config *config.AssistantConfig, logger commons.Logger) (internal_type.Telephony, error) {
	return &twilioTelephony{
		appCfg: config,
		logger: logger,
	}, nil
}

func twilioClient(vaultCredential *protos.VaultCredential) (*twilio.RestClient, error) {
	clientParams, err := twilioClientParams(vaultCredential)
	if err != nil {
		return nil, err
	}
	return twilio.NewRestClientWithParams(*clientParams), nil
}

func twilioClientParams(vaultCredential *protos.VaultCredential) (*twilio.ClientParams, error) {
	if vaultCredential.GetValue() == nil {
		return nil, fmt.Errorf("vault credential value is nil")
	}
	vaultMap := vaultCredential.GetValue().AsMap()
	accountSid, ok := vaultMap["account_sid"]
	if !ok {
		return nil, fmt.Errorf("illegal vault config accountSid is not found")
	}
	authToken, ok := vaultMap["account_token"]
	if !ok {
		return nil, fmt.Errorf("illegal vault config account_token not found")
	}
	sid, ok := accountSid.(string)
	if !ok {
		return nil, fmt.Errorf("illegal vault config account_sid is not a string")
	}
	token, ok := authToken.(string)
	if !ok {
		return nil, fmt.Errorf("illegal vault config account_token is not a string")
	}
	return &twilio.ClientParams{
		Username: sid,
		Password: token,
	}, nil
}

func (tpc *twilioTelephony) CatchAllStatusCallback(ctx *gin.Context) (*internal_type.StatusInfo, error) {
	eventDetails := utils.Option{}
	if len(ctx.Request.URL.Query()) > 0 {
		for key, values := range ctx.Request.URL.Query() {
			if len(values) > 0 {
				eventDetails[key] = values[0]
			} else {
				eventDetails[key] = nil
			}
		}
	} else {
		body, err := ctx.GetRawData()
		if err != nil {
			tpc.logger.Errorf("failed to read event body with error %+v", err)
			return nil, fmt.Errorf("failed to read request body")
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			tpc.logger.Errorf("failed to parse body with error %+v", err)
			return nil, fmt.Errorf("failed to parse request body")
		}
		for key, value := range values {
			if len(value) > 0 {
				eventDetails[key] = value[0]
			} else {
				eventDetails[key] = nil
			}
		}
	}

	callback, err := internal_twilio.NewStatusCallback(eventDetails)
	if err != nil {
		tpc.logger.Errorf("failed to parse status callback: %+v", err)
		return nil, err
	}
	if !validator.NotBlank(callback.ChannelUUID) {
		tpc.logger.Errorf("call sid not found or invalid in catch-all payload")
		return nil, fmt.Errorf("call sid not found in callback")
	}
	return callback.StatusInfo(), nil
}

func (tpc *twilioTelephony) StatusCallback(c *gin.Context, auth types.SimplePrinciple, assistantId uint64, assistantConversationId uint64) (*internal_type.StatusInfo, error) {
	eventDetails := utils.Option{}
	if len(c.Request.URL.Query()) > 0 {
		for key, values := range c.Request.URL.Query() {
			if len(values) > 0 {
				eventDetails[key] = values[0]
			} else {
				eventDetails[key] = nil
			}
		}
	} else {
		body, err := c.GetRawData()
		if err != nil {
			tpc.logger.Errorf("failed to read event body with error %+v", err)
			return nil, fmt.Errorf("failed to read request body")
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			tpc.logger.Errorf("failed to parse body with error %+v", err)
			return nil, fmt.Errorf("failed to parse request body")
		}
		for key, value := range values {
			if len(value) > 0 {
				eventDetails[key] = value[0]
			} else {
				eventDetails[key] = nil
			}
		}
	}

	callback, err := internal_twilio.NewStatusCallback(eventDetails)
	if err != nil {
		tpc.logger.Errorf("failed to parse status callback: %+v", err)
		return nil, err
	}
	return callback.StatusInfo(), nil
}

func (tpc *twilioTelephony) OutboundCall(auth types.SimplePrinciple, toPhone string, fromPhone string, assistant *internal_assistant_entity.Assistant, assistantConversationId uint64, vaultCredential *protos.VaultCredential, opts utils.Option) (*internal_type.CallInfo, error) {
	info := &internal_type.CallInfo{Provider: twilioProvider}

	contextID, _ := opts.GetString("rapida.context_id")

	client, err := twilioClient(vaultCredential)
	if err != nil {
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("authentication error: %s", err.Error())
		return info, err
	}
	callParams := &openapi.CreateCallParams{}
	callParams.SetTo(toPhone)
	callParams.SetFrom(fromPhone)
	callParams.SetStatusCallback(
		fmt.Sprintf("https://%s/%s", tpc.appCfg.Assistant.Public, internal_type.GetContextEventPath(twilioProvider, contextID)),
	)
	callParams.SetStatusCallbackEvent([]string{
		"initiated", "ringing", "answered", "completed",
	})
	callParams.SetStatusCallbackMethod("POST")
	callParams.SetTwiml(
		tpc.CreateTwinML(
			tpc.appCfg.Assistant.Public,
			fmt.Sprintf("%d__%d", assistant.Id, assistantConversationId),
			internal_type.GetContextAnswerPath(twilioProvider, contextID),
			fmt.Sprintf("https://%s/%s", tpc.appCfg.Assistant.Public, internal_type.GetContextEventPath(twilioProvider, contextID)),
			assistant.Id,
			toPhone),
	)
	resp, err := client.Api.CreateCall(callParams)
	if err != nil || resp.Status == nil || resp.Sid == nil {
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("API error: %s", err.Error())
		return info, err
	}

	info.ChannelUUID = *resp.Sid
	info.Status = "SUCCESS"
	info.StatusInfo = internal_type.StatusInfo{Event: *resp.Status, Payload: resp}
	return info, nil
}

func (tpc *twilioTelephony) CreateTwinML(mediaServer string, name, path string, callback string, assistantId uint64, clientNumber string) string {
	return fmt.Sprintf(`
	    <Response>
		 	<Connect>
	        	<Stream url="wss://%s/%s" name="%s" statusCallback="%s" statusCallbackEvent="initiated ringing answered completed">
					<Parameter name="assistant_id" value="%d"/>
					<Parameter name="client_number" value="%s"/>
				</Stream>
			</Connect>
	    </Response>
	`,
		mediaServer,
		path,
		name,
		callback,
		assistantId,
		clientNumber,
	)
}

func (tpc *twilioTelephony) InboundCall(c *gin.Context, auth types.SimplePrinciple, assistantId uint64, clientNumber string, assistantConversationId uint64) error {
	contextID, _ := c.Get("contextId")
	ctxID := fmt.Sprintf("%v", contextID)

	c.Data(http.StatusOK, "text/xml", []byte(
		tpc.CreateTwinML(
			tpc.appCfg.Assistant.Public,
			fmt.Sprintf("%d__%d", assistantId, assistantConversationId),
			internal_type.GetContextAnswerPath("twilio", ctxID),
			fmt.Sprintf("https://%s/%s", tpc.appCfg.Assistant.Public, internal_type.GetContextEventPath("twilio", ctxID)),
			assistantId, clientNumber),
	))
	return nil
}

func (tpc *twilioTelephony) ReceiveCall(c *gin.Context) (*internal_type.CallInfo, error) {
	queryParams := make(map[string]string)
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			queryParams[key] = values[0]
		}
	}

	clientNumber, ok := queryParams["From"]
	if !ok || clientNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assistant ID"})
		return nil, fmt.Errorf("missing or empty 'from' query parameter")
	}

	info := &internal_type.CallInfo{
		CallerNumber: clientNumber,
		Provider:     twilioProvider,
		Status:       "SUCCESS",
		StatusInfo:   internal_type.StatusInfo{Event: "webhook", Payload: queryParams},
	}
	if v, ok := queryParams["To"]; ok && v != "" {
		info.FromNumber = v // DID that received the call (our number)
	}
	if v, ok := queryParams["CallSid"]; ok && v != "" {
		info.ChannelUUID = v
	}
	return info, nil
}
