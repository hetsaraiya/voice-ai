// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_exotel_telephony

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rapidaai/api/assistant-api/config"
	internal_exotel "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/exotel/internal"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"

	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
)

const exotelProvider = "exotel"

type exotelTelephony struct {
	logger commons.Logger
	appCfg *config.AssistantConfig
}

func NewExotelTelephony(config *config.AssistantConfig, logger commons.Logger) (internal_type.Telephony, error) {
	return &exotelTelephony{
		logger: logger,
		appCfg: config,
	}, nil
}

func (exo *exotelTelephony) CatchAllStatusCallback(ctx *gin.Context) (*internal_type.StatusInfo, error) {
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
		if err := ctx.Request.ParseForm(); err == nil && len(ctx.Request.PostForm) > 0 {
			for key, values := range ctx.Request.PostForm {
				if len(values) > 0 {
					eventDetails[key] = values[0]
				} else {
					eventDetails[key] = nil
				}
			}
		} else {
			form, err := ctx.MultipartForm()
			if err != nil {
				exo.logger.Errorf("failed to parse callback form-data with error %+v", err)
				return nil, fmt.Errorf("failed to parse callback form-data")
			}
			for key, values := range form.Value {
				if len(values) > 0 {
					eventDetails[key] = values[0]
				} else {
					eventDetails[key] = nil
				}
			}
		}
	}

	callback, err := internal_exotel.NewStatusCallback(eventDetails)
	if err != nil {
		exo.logger.Errorf("failed to parse status callback: %+v", err)
		return nil, err
	}
	if !validator.NotBlank(callback.ChannelUUID) {
		exo.logger.Errorf("call sid not found or invalid in catch-all payload")
		return nil, fmt.Errorf("call sid not found in callback")
	}
	return callback.StatusInfo(), nil
}

func (exo *exotelTelephony) StatusCallback(c *gin.Context, auth types.SimplePrinciple, assistantId uint64, assistantConversationId uint64) (*internal_type.StatusInfo, error) {
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
		if err := c.Request.ParseForm(); err == nil && len(c.Request.PostForm) > 0 {
			for key, values := range c.Request.PostForm {
				if len(values) > 0 {
					eventDetails[key] = values[0]
				} else {
					eventDetails[key] = nil
				}
			}
		} else {
			form, err := c.MultipartForm()
			if err != nil {
				exo.logger.Errorf("failed to parse callback form-data with error %+v", err)
				return nil, fmt.Errorf("failed to parse callback form-data")
			}
			for key, values := range form.Value {
				if len(values) > 0 {
					eventDetails[key] = values[0]
				} else {
					eventDetails[key] = nil
				}
			}
		}
	}
	callback, err := internal_exotel.NewStatusCallback(eventDetails)
	if err != nil {
		exo.logger.Errorf("failed to parse status callback: %+v", err)
		return nil, err
	}
	return callback.StatusInfo(), nil
}

func (exo *exotelTelephony) ClientUrl(vaultCredential *protos.VaultCredential, opts utils.Option) (*string, error) {
	if vaultCredential.GetValue() == nil {
		return nil, fmt.Errorf("vault credential value is nil")
	}
	vaultMap := vaultCredential.GetValue().AsMap()
	accountSid, ok := vaultMap["account_sid"]
	if !ok {
		return nil, fmt.Errorf("illegal vault config accountSid is not found")
	}
	clientId, ok := vaultMap["client_id"]
	if !ok {
		return nil, fmt.Errorf("illegal vault config client_id not found")
	}
	authToken, ok := vaultMap["client_secret"]
	if !ok {
		return nil, fmt.Errorf("illegal vault config client_secret not found")
	}
	sid, _ := accountSid.(string)
	cid, _ := clientId.(string)
	token, _ := authToken.(string)
	if sid == "" || cid == "" || token == "" {
		return nil, fmt.Errorf("illegal vault config: credentials must be non-empty strings")
	}
	return utils.Ptr(fmt.Sprintf("https://%s:%s@api.exotel.com/v1/Accounts/%s/Calls/connect.json",
		cid, token, sid)), nil
}

func (exo *exotelTelephony) AppUrl(vaultCredential *protos.VaultCredential, opts utils.Option) (*string, error) {
	if vaultCredential.GetValue() == nil {
		return nil, fmt.Errorf("vault credential value is nil")
	}
	vaultMap := vaultCredential.GetValue().AsMap()
	accountSid, ok := vaultMap["account_sid"]
	if !ok {
		return nil, fmt.Errorf("illegal vault config accountSid is not found")
	}
	app_id, err := opts.GetString("app_id")
	if err != nil {
		return nil, fmt.Errorf("illegal app_id option is not found")
	}
	sid, _ := accountSid.(string)
	if sid == "" {
		return nil, fmt.Errorf("illegal vault config account_sid must be a non-empty string")
	}
	return utils.Ptr(fmt.Sprintf("http://my.exotel.com/%s/exoml/start_voice/%s", sid, app_id)), nil
}

func (exo *exotelTelephony) OutboundCall(
	auth types.SimplePrinciple,
	toPhone string,
	fromPhone string,
	assistant *internal_assistant_entity.Assistant, assistantConversationId uint64,
	vaultCredential *protos.VaultCredential,
	opts utils.Option) (*internal_type.CallInfo, error) {
	info := &internal_type.CallInfo{Provider: exotelProvider}

	clientUrl, err := exo.ClientUrl(vaultCredential, opts)
	if err != nil {
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("Failed to build url, check credentials: %s", err.Error())
		return info, err
	}

	appUrl, err := exo.AppUrl(vaultCredential, opts)
	if err != nil {
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("Failed to build app url: %s", err.Error())
		return info, err
	}

	contextID, _ := opts.GetString("rapida.context_id")

	formData := url.Values{}
	formData.Set("From", toPhone)
	formData.Set("CallerId", fromPhone)
	formData.Set("To", fromPhone)
	formData.Set("Url", *appUrl)
	formData.Set("StatusCallback", fmt.Sprintf("https://%s/%s", exo.appCfg.Assistant.Public, internal_type.GetContextEventPath(exotelProvider, contextID)))
	formData.Set("CustomField", internal_type.GetContextAnswerPath(exotelProvider, contextID))

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("POST", *clientUrl, strings.NewReader(formData.Encode()))
	if err != nil {
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("request creation error: %s", err.Error())
		return info, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("API error: %s", err.Error())
		return info, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("failed to read response: %s", err.Error())
		return info, err
	}
	if resp.StatusCode != http.StatusOK {
		exo.logger.Errorf("Unexpected HTTP Status: %d, Response Body: %s\n", resp.StatusCode, string(bodyBytes))
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
		info.StatusInfo = internal_type.StatusInfo{Event: "Failed", Payload: string(bodyBytes)}
		return info, fmt.Errorf("status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var jsonResponse internal_exotel.MakeCallResponse
	if err := json.Unmarshal(bodyBytes, &jsonResponse); err != nil {
		info.Status = "FAILED"
		info.ErrorMessage = fmt.Sprintf("failed to decode response: %s", err.Error())
		info.StatusInfo = internal_type.StatusInfo{Event: jsonResponse.Call.Status, Payload: "Failed to decode response"}
		return info, err
	}

	info.ChannelUUID = jsonResponse.Call.Sid
	info.Status = "SUCCESS"
	info.StatusInfo = internal_type.StatusInfo{Event: jsonResponse.Call.Status, Payload: jsonResponse}
	return info, nil
}

func (exo *exotelTelephony) InboundCall(c *gin.Context, auth types.SimplePrinciple, assistantId uint64, clientNumber string, assistantConversationId uint64) error {
	contextID, _ := c.Get("contextId")
	ctxID := fmt.Sprintf("%v", contextID)

	response := map[string]string{
		"url": fmt.Sprintf("wss://%s/%s",
			exo.appCfg.Assistant.Public,
			internal_type.GetContextAnswerPath("exotel", ctxID)),
	}
	c.JSON(http.StatusOK, response)
	return nil
}

func (exo *exotelTelephony) ReceiveCall(c *gin.Context) (*internal_type.CallInfo, error) {
	queryParams := make(map[string]string)
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			queryParams[key] = values[0]
		}
	}

	// Exotel outbound redirect: when CustomField is present, this is the
	// callback from Exotel's outbound flow asking for the WebSocket URL.
	// Respond with the URL and return nil CallInfo so the pipeline skips
	// normal inbound call setup.
	socketUrl, ok := queryParams["CustomField"]
	if ok {
		response := map[string]string{"url": fmt.Sprintf("wss://%s/%s", exo.appCfg.Assistant.Public, socketUrl)}
		c.JSON(http.StatusOK, response)
		return nil, nil
	}

	clientNumber, ok := queryParams["CallFrom"]
	if !ok || clientNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid caller"})
		return nil, fmt.Errorf("missing or empty 'from' query parameter")
	}

	info := &internal_type.CallInfo{
		CallerNumber: clientNumber,
		Provider:     exotelProvider,
		Status:       "SUCCESS",
		StatusInfo:   internal_type.StatusInfo{Event: "webhook", Payload: queryParams},
	}
	if v, ok := queryParams["CallTo"]; ok && v != "" {
		info.FromNumber = v
	}
	if v, ok := queryParams["CallSid"]; ok && v != "" {
		info.ChannelUUID = v
	}
	return info, nil
}
