// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_telephony_base

import (
	"testing"

	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/utils"
	"github.com/stretchr/testify/require"
)

func newTestLogger(t *testing.T) commons.Logger {
	t.Helper()
	l, err := commons.NewApplicationLogger(
		commons.Level("error"),
		commons.Name("base-streamer-test"),
		commons.EnableFile(false),
	)
	require.NoError(t, err)
	return l
}

func metadataString(t *testing.T, md map[string]interface{}, key string) string {
	t.Helper()
	v, ok := md[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	require.Truef(t, ok, "metadata key %q is not a string: %T", key, v)
	return s
}

func TestCreateConnectionRequest_EmitsAllClientKeys(t *testing.T) {
	cc := &callcontext.CallContext{
		AssistantID:    1,
		ConversationID: 42,
		Direction:      "outbound",
		Provider:       "sip",
		CallerNumber:   "15551234567",
		FromNumber:     "15557654321",
		ChannelUUID:    "live-call-id",
		ContextID:      "ctx-uuid-123",
	}
	base := NewBaseTelephonyStreamer(newTestLogger(t), cc, nil)

	req := base.CreateConnectionRequest()
	require.NotNil(t, req)

	md, err := utils.AnyMapToInterfaceMap(req.GetMetadata())
	require.NoError(t, err)

	require.Equal(t, "outbound", metadataString(t, md, "client.direction"))
	require.Equal(t, "sip", metadataString(t, md, "client.channel"))
	require.Equal(t, "15551234567", metadataString(t, md, "client.phone"))
	require.Equal(t, "15557654321", metadataString(t, md, "client.assistant_phone"))
	require.Equal(t, "live-call-id", metadataString(t, md, "client.provider_call_id"))
	require.Equal(t, "ctx-uuid-123", metadataString(t, md, "client.context_id"))
}

func TestCreateConnectionRequest_OmitsEmptyOptionalFields(t *testing.T) {
	// CallerNumber / FromNumber empty (defensive — e.g. degraded fallback path).
	cc := &callcontext.CallContext{
		Direction: "inbound",
		Provider:  "sip",
		ContextID: "ctx-1",
	}
	base := NewBaseTelephonyStreamer(newTestLogger(t), cc, nil)

	req := base.CreateConnectionRequest()
	md, err := utils.AnyMapToInterfaceMap(req.GetMetadata())
	require.NoError(t, err)

	_, hasPhone := md["client.phone"]
	require.False(t, hasPhone, "client.phone should be omitted when CallerNumber empty")
	_, hasAssistantPhone := md["client.assistant_phone"]
	require.False(t, hasAssistantPhone, "client.assistant_phone should be omitted when FromNumber empty")
	_, hasProviderCallID := md["client.provider_call_id"]
	require.False(t, hasProviderCallID, "client.provider_call_id should be omitted when ChannelUUID empty")

	// Required-when-known fields still emitted.
	require.Equal(t, "inbound", metadataString(t, md, "client.direction"))
	require.Equal(t, "sip", metadataString(t, md, "client.channel"))
	require.Equal(t, "ctx-1", metadataString(t, md, "client.context_id"))
}
