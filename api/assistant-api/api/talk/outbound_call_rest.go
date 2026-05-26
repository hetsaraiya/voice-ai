// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package assistant_talk_api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	channel_pipeline "github.com/rapidaai/api/assistant-api/internal/channel/pipeline"
	"github.com/rapidaai/openapi"
	"github.com/rapidaai/pkg/preset"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
)

// CreatePhoneCallRest initiates an outbound phone call from the REST API.
func (cApi *ConversationApi) CreatePhoneCallRest(c *gin.Context) {
	auth, isAuthenticated := types.GetAuthPrinciple(c)
	if !isAuthenticated {
		c.JSON(http.StatusForbidden, openapi.ErrorResponse{
			Code:    utils.Ptr(int32(401)),
			Success: utils.Ptr(false),
			Error: &openapi.Error{
				ErrorCode:    utils.Ptr(openapi.Uint64String("401")),
				ErrorMessage: utils.Ptr("unauthenticated request"),
				HumanMessage: utils.Ptr("Unauthenticated request, please try again with valid authentication."),
			},
		})
		return
	}

	var ir openapi.CreatePhoneCallRequest
	if err := c.ShouldBindJSON(&ir); err != nil {
		c.JSON(http.StatusBadRequest, openapi.ErrorResponse{
			Code:    utils.Ptr(int32(400)),
			Success: utils.Ptr(false),
			Error: &openapi.Error{
				ErrorCode:    utils.Ptr(openapi.Uint64String("400")),
				ErrorMessage: utils.Ptr(err.Error()),
				HumanMessage: utils.Ptr("Invalid request."),
			},
		})
		return
	}

	if !validator.NonNil(ir.ToNumber) || !validator.NotBlank(*ir.ToNumber) {
		c.JSON(http.StatusBadRequest, openapi.ErrorResponse{
			Code:    utils.Ptr(int32(200)),
			Success: utils.Ptr(false),
			Error: &openapi.Error{
				ErrorCode:    utils.Ptr(openapi.Uint64String("200")),
				ErrorMessage: utils.Ptr("missing to_phone parameter"),
				HumanMessage: utils.Ptr("Please provide the required to_phone parameter."),
			},
		})
		return
	}

	var assistantID uint64
	version := ""
	if validator.NonNil(ir.Assistant) {
		if validator.NonNil(ir.Assistant.AssistantId) {
			assistantID, _ = strconv.ParseUint(*ir.Assistant.AssistantId, 10, 64)
		}
		if validator.NonNil(ir.Assistant.Version) {
			version = *ir.Assistant.Version
		}
	}
	assistant := &protos.AssistantDefinition{
		AssistantId: assistantID,
		Version:     version,
	}
	preset.AssistantDefinition(assistant)
	if !validator.OfAssistantDefinition(assistant) {
		c.JSON(http.StatusBadRequest, openapi.ErrorResponse{
			Code:    utils.Ptr(int32(200)),
			Success: utils.Ptr(false),
			Error: &openapi.Error{
				ErrorCode:    utils.Ptr(openapi.Uint64String("200")),
				ErrorMessage: utils.Ptr("invalid assistant"),
				HumanMessage: utils.Ptr("Please provide a valid assistant."),
			},
		})
		return
	}

	fromNumber := ""
	if validator.NonNil(ir.FromNumber) {
		fromNumber = *ir.FromNumber
	}
	var metadata map[string]interface{}
	if validator.NonNil(ir.Metadata) {
		metadata = *ir.Metadata
	}
	var args map[string]interface{}
	if validator.NonNil(ir.Args) {
		args = *ir.Args
	}
	var opts map[string]interface{}
	if validator.NonNil(ir.Options) {
		opts = *ir.Options
	}

	result := cApi.channelPipeline.Run(c, channel_pipeline.OutboundRequestedPipeline{
		ID:          fmt.Sprintf("%d", assistant.GetAssistantId()),
		Auth:        auth,
		AssistantID: assistant.GetAssistantId(),
		Version:     assistant.GetVersion(),
		ToPhone:     *ir.ToNumber,
		FromPhone:   fromNumber,
		Metadata:    metadata,
		Args:        args,
		Options:     opts,
	})
	if result.Error != nil {
		cApi.logger.Errorf("outbound call failed: %v", result.Error)
		c.JSON(http.StatusInternalServerError, openapi.ErrorResponse{
			Code:    utils.Ptr(int32(500)),
			Success: utils.Ptr(false),
			Error: &openapi.Error{
				ErrorCode:    utils.Ptr(openapi.Uint64String("500")),
				ErrorMessage: utils.Ptr(result.Error.Error()),
				HumanMessage: utils.Ptr("Failed to initiate outbound call"),
			},
		})
		return
	}

	cApi.logger.Infof("outbound call dispatched: contextId=%s, conversationId=%d",
		result.ContextID, result.ConversationID)

	c.JSON(http.StatusOK, openapi.CreatePhoneCallResponse{
		Code:    utils.Ptr(int32(200)),
		Success: utils.Ptr(true),
		Data: &openapi.AssistantConversation{
			Id: utils.Ptr(openapi.Uint64String(strconv.FormatUint(result.ConversationID, 10))),
		},
	})
}
