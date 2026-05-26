// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package assistant_talk_api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	channel_pipeline "github.com/rapidaai/api/assistant-api/internal/channel/pipeline"
	"github.com/rapidaai/pkg/types"
)

// CallReciever handles incoming calls for the given assistant.
// Thin controller — business logic delegated to pipeline's handleCallReceived.
func (cApi *ConversationApi) CallReciever(c *gin.Context) {
	iAuth, isAuthenticated := types.GetAuthPrinciple(c)
	if !isAuthenticated {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthenticated request"})
		return
	}

	assistantID := c.Param("assistantId")
	if assistantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assistant ID"})
		return
	}
	assistantId, err := strconv.ParseUint(assistantID, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assistant ID"})
		return
	}

	// Pipeline handles: create conversation, answer provider, emit events
	result := cApi.channelPipeline.Run(c, channel_pipeline.CallReceivedPipeline{
		ID:          uuid.NewString(),
		Provider:    c.Param("telephony"),
		Auth:        iAuth,
		AssistantID: assistantId,
		GinContext:  c,
	})
	if result.Error != nil {
		cApi.logger.Errorf("failed to handle inbound call: %v", result.Error)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unable to initiate talker"})
	}
}

// CallTalkerByContext handles WebSocket connections for media streaming.
// Thin controller — pipeline handles: resolve context, create streamer/talker, Talk(), cleanup.
// contextId is read from URL path (:contextId) or query param (?contextId=).
// Query param fallback supports Asterisk chan_websocket which appends params via v() dialstring option.
func (cApi *ConversationApi) CallTalkerByContext(c *gin.Context) {
	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024, CheckOrigin: func(r *http.Request) bool { return true }}
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unable to upgrade connection"})
		return
	}

	contextID := c.Query("contextId")
	if contextID == "" {
		contextID = c.Param("contextId")
	}
	if contextID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing contextId"})
		return
	}

	// Pipeline handles: resolve context, create streamer, create talker,
	// create observer+hooks, hooks.OnBegin, Talk() (blocks), hooks.OnEnd, cleanup.
	result := cApi.channelPipeline.Run(c, channel_pipeline.SessionConnectedPipeline{
		ID:        contextID,
		ContextID: contextID,
		WebSocket: ws,
	})
	if result != nil && result.Error != nil {
		cApi.logger.Errorf("talk failed for context %s: %v", contextID, result.Error)
	}
}
