// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_telephony_base

import (
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
)

const (
	OutboundCallStatusInitiated = "initiated"
	OutboundCallStatusFailed    = "failed"
	OutboundCallStatusCancelled = "cancelled"

	OutboundFailureClassAuthentication   = "authentication"
	OutboundFailureClassConfiguration    = "configuration"
	OutboundFailureClassProviderAPI      = "provider_api"
	OutboundFailureClassProviderResponse = "provider_response"
	OutboundFailureClassHealthGate       = "health_gate"
	OutboundFailureClassRequestCancelled = "request_cancelled"
	OutboundFailureClassRequestCreation  = "request_creation"
	OutboundFailureClassRequestPayload   = "request_payload"
	OutboundFailureClassSetup            = "setup"
	OutboundFailureClassNoAnswer         = "no_answer"

	OutboundDisconnectReasonSetupFailed      = "outbound_setup_failed"
	OutboundDisconnectReasonHealthGate       = "outbound_health_gate_failed"
	OutboundDisconnectReasonRequestCancelled = "outbound_request_cancelled"
	OutboundDisconnectReasonNoAnswer         = "outbound_no_answer"

	OutboundFailureReasonNoAnswer = "outbound_no_answer"
)

func ReportOutboundInitiated(statusReporter internal_type.ProviderCallStatusReporter, channelUUID string) {
	if statusReporter == nil {
		return
	}
	statusReporter(internal_type.ProviderCallStatusUpdate{
		ChannelUUID: channelUUID,
		CallStatus:  OutboundCallStatusInitiated,
	})
}

func ReportOutboundFailure(
	statusReporter internal_type.ProviderCallStatusReporter,
	failureClass string,
	failureReason string,
	disconnectReason string,
	err error,
	providerStatusCode int,
) {
	if statusReporter == nil {
		return
	}
	errorMessage := failureReason
	if err != nil {
		errorMessage = err.Error()
	}
	statusReporter(internal_type.ProviderCallStatusUpdate{
		CallStatus:         OutboundCallStatusFailed,
		ErrorMessage:       errorMessage,
		FailureClass:       failureClass,
		FailureReason:      failureReason,
		DisconnectReason:   disconnectReason,
		ProviderStatusCode: providerStatusCode,
	})
}
