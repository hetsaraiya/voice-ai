// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_type

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

// StatusInfo is the structured response returned by status/event callbacks.
// It carries the event name and raw payload from the provider.
type StatusInfo struct {
	// Event is the status/event name from the provider callback
	// (e.g. "completed", "ringing", "answered", "stream-started", "channel_destroyed").
	Event string

	// ChannelUUID is the provider-specific call identifier from the callback.
	ChannelUUID string

	// Error is set when the provider callback represents a failed terminal state.
	Error *StatusError

	// Duration is the provider-reported call duration, when present.
	// A pointer keeps explicit zero-duration callbacks distinguishable from
	// callbacks that did not include a duration.
	Duration *time.Duration

	// Price is the provider-reported call price, when present.
	Price string

	// Payload is the raw event payload from the provider (parsed body, form data, etc.).
	Payload interface{}
}

type StatusError struct {
	Error  string
	Reason string
}

// CallInfo is the structured response returned by ReceiveCall and OutboundCall.
// Providers populate plain data fields; the dispatcher owns telemetry construction.
// When a new provider needs extra data, add a field here (or use Extra).
type CallInfo struct {
	// ChannelUUID is the provider-specific call identifier
	// (Twilio CallSid, Vonage UUID, Asterisk channel ID, SIP Call-ID, etc.)
	ChannelUUID string

	// CallerNumber is the resolved caller/client phone number.
	// Inbound: the caller's number. Outbound: the "to" number.
	CallerNumber string

	// FromNumber is the originating number for outbound calls.
	FromNumber string

	// Direction is "inbound" or "outbound".
	Direction string

	// Status is the call status string (e.g. "SUCCESS", "FAILED", "initiated").
	Status string

	// StatusInfo carries the event name and payload for the call.
	// For OutboundCall: the initial event (e.g. "initiated", "channel_created").
	// For ReceiveCall: typically "webhook" with query params as payload.
	StatusInfo StatusInfo

	// ErrorMessage is set when the provider call fails. The dispatcher uses this
	// to build a telephony.error metadata entry.
	ErrorMessage string

	// Provider is the telephony provider name (twilio, vonage, exotel, asterisk, sip).
	Provider string

	// Extra holds provider-specific fields that don't warrant a top-level field.
	// Examples: vonage "conversation_uuid", sip "telephony.status".
	// If a field is used by multiple providers, promote it to a top-level field.
	Extra map[string]string
}

// Telephony defines the interface that all telephony providers must implement.
// Providers return structured data — they never construct telemetry.
// The dispatcher is responsible for converting CallInfo/StatusInfo into telemetry.
type Telephony interface {

	// StatusCallback handles a status/event callback for a conversation.
	StatusCallback(ctx *gin.Context, auth types.SimplePrinciple, assistantId, assistantConversationId uint64) (*StatusInfo, error)
	// CatchAllStatusCallback handles a catch-all event callback.
	CatchAllStatusCallback(ctx *gin.Context) (*StatusInfo, error)

	// ReceiveCall processes an incoming call webhook and returns structured call info.
	ReceiveCall(c *gin.Context) (*CallInfo, error)
	// OutboundCall places an outbound call and returns structured call info.
	OutboundCall(auth types.SimplePrinciple, toPhone string, fromPhone string, assistant *internal_assistant_entity.Assistant, assistantConversationId uint64, vaultCredential *protos.VaultCredential, opts utils.Option) (*CallInfo, error)
	// InboundCall instructs the provider to answer/connect the inbound call.
	InboundCall(c *gin.Context, auth types.SimplePrinciple, assistantId uint64, clientNumber string, assistantConversationId uint64) error
}

// GetContextAnswerPath returns the contextId-based WebSocket path for media streaming.
// Route: GET /:telephony/ctx/:contextId
func GetContextAnswerPath(provider, contextID string) string {
	return fmt.Sprintf("v1/talk/%s/ctx/%s", provider, contextID)
}

// GetContextEventPath returns the contextId-based event callback path for status updates.
// Route: GET/POST /:telephony/ctx/:contextId/event
func GetContextEventPath(provider, contextID string) string {
	return fmt.Sprintf("v1/talk/%s/ctx/%s/event", provider, contextID)
}
