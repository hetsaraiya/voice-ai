// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package core

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	internal_inbound "github.com/rapidaai/api/assistant-api/sip/internal/inbound"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInboundCall_InvalidSDPRejectsWithoutSession(t *testing.T) {
	server := newServerForCommandTests(t)
	request := newSIPRequest(sip.INVITE, "inbound-invalid-sdp")
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 400, transaction.lastStatus())
	_, exists := server.GetSession("inbound-invalid-sdp")
	assert.False(t, exists)
}

func TestInboundCall_InvalidIdentityRejectsWithoutSession(t *testing.T) {
	cases := []struct {
		name         string
		callID       string
		removeHeader string
	}{
		{name: "missing call id", callID: "inbound-missing-call-id", removeHeader: "Call-ID"},
		{name: "missing from", callID: "inbound-missing-from", removeHeader: "From"},
		{name: "missing to", callID: "inbound-missing-to", removeHeader: "To"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newServerForCommandTests(t)
			request := newInboundInviteRequest(tc.callID)
			for request.RemoveHeader(tc.removeHeader) {
			}
			transaction := newTestServerTx()

			server.handleInvite(request, transaction)

			require.NotEmpty(t, transaction.responses)
			assert.Equal(t, 400, transaction.lastStatus())
			if tc.removeHeader != "Call-ID" {
				_, exists := server.GetSession(tc.callID)
				assert.False(t, exists)
			}
		})
	}
}

func TestInboundCall_ConfigRejectDoesNotCreateSession(t *testing.T) {
	server := newServerForCommandTests(t)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		return &SIPError{Code: 403, Message: "forbidden", Err: ErrAuthRequired}
	}})
	request := newInboundInviteRequest("inbound-config-reject")
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 403, transaction.lastStatus())
	_, exists := server.GetSession("inbound-config-reject")
	assert.False(t, exists)
}

func TestInboundCall_MiddlewareErrorRejectsWithoutSession(t *testing.T) {
	server := newServerForCommandTests(t)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		return errors.New("resolver unavailable")
	}})
	request := newInboundInviteRequest("inbound-config-error")
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 500, transaction.lastStatus())
	_, exists := server.GetSession("inbound-config-error")
	assert.False(t, exists)
}

func TestInboundCall_InvalidSessionConfigRejectsWithoutSession(t *testing.T) {
	server := newServerForCommandTests(t)
	invalidConfig := bridgeTestConfig()
	invalidConfig.RTPPortRangeStart = 20000
	invalidConfig.RTPPortRangeEnd = 10000
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = invalidConfig
		return nil
	}})
	request := newInboundInviteRequest("inbound-session-config-invalid")
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 500, transaction.lastStatus())
	_, exists := server.GetSession("inbound-session-config-invalid")
	assert.False(t, exists)
}

func TestInboundCall_UnsupportedCodecRejectsWithoutFallback(t *testing.T) {
	server := newServerForCommandTests(t)
	request := newInboundInviteRequest("inbound-unsupported-codec")
	request.SetBody([]byte(unsupportedInboundOfferSDP()))
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 488, transaction.lastStatus())
	_, exists := server.GetSession("inbound-unsupported-codec")
	assert.False(t, exists)
}

func TestInboundCall_UnsupportedContentTypeRejects415(t *testing.T) {
	server := newServerForCommandTests(t)
	request := newInboundInviteRequest("inbound-unsupported-content")
	for request.RemoveHeader("Content-Type") {
	}
	request.AppendHeader(sip.NewHeader("Content-Type", "application/json"))
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 415, transaction.lastStatus())
	_, exists := server.GetSession("inbound-unsupported-content")
	assert.False(t, exists)
}

func TestInboundCall_MissingContentTypeRejects415(t *testing.T) {
	server := newServerForCommandTests(t)
	request := newInboundInviteRequest("inbound-missing-content")
	for request.RemoveHeader("Content-Type") {
	}
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 415, transaction.lastStatus())
	_, exists := server.GetSession("inbound-missing-content")
	assert.False(t, exists)
}

func TestInboundCall_InvalidRemoteRTPAddressRejects400(t *testing.T) {
	server := newServerForCommandTests(t)
	request := newInboundInviteRequest("inbound-invalid-rtp-ip")
	request.SetBody([]byte(inboundOfferSDPWithMedia("not-an-ip", 19000, "0 101")))
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 400, transaction.lastStatus())
	_, exists := server.GetSession("inbound-invalid-rtp-ip")
	assert.False(t, exists)
}

func TestInboundCall_MissingRemoteRTPAddressRejects400(t *testing.T) {
	server := newServerForCommandTests(t)
	request := newInboundInviteRequest("inbound-missing-rtp-ip")
	request.SetBody([]byte(inboundOfferSDPWithoutConnection()))
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 400, transaction.lastStatus())
	_, exists := server.GetSession("inbound-missing-rtp-ip")
	assert.False(t, exists)
}

func TestInboundCall_DisabledRemoteRTPAddressRejects488(t *testing.T) {
	server := newServerForCommandTests(t)
	request := newInboundInviteRequest("inbound-disabled-rtp-ip")
	request.SetBody([]byte(inboundOfferSDPWithMedia("0.0.0.0", 19000, "0 101")))
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 488, transaction.lastStatus())
	_, exists := server.GetSession("inbound-disabled-rtp-ip")
	assert.False(t, exists)
}

func TestInboundCall_InvalidRemoteRTPPortRejects400(t *testing.T) {
	server := newServerForCommandTests(t)
	request := newInboundInviteRequest("inbound-invalid-rtp-port")
	request.SetBody([]byte(inboundOfferSDPWithMedia("127.0.0.1", 70000, "0 101")))
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 400, transaction.lastStatus())
	_, exists := server.GetSession("inbound-invalid-rtp-port")
	assert.False(t, exists)
}

func TestInboundCall_MissingRemoteRTPPortRejects400(t *testing.T) {
	server := newServerForCommandTests(t)
	request := newInboundInviteRequest("inbound-missing-rtp-port")
	request.SetBody([]byte(inboundOfferSDPWithRawMedia("127.0.0.1", "m=audio RTP/AVP 0 101")))
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 400, transaction.lastStatus())
	_, exists := server.GetSession("inbound-missing-rtp-port")
	assert.False(t, exists)
}

func TestInboundCall_NoPayloadTypesRejects488(t *testing.T) {
	server := newServerForCommandTests(t)
	request := newInboundInviteRequest("inbound-no-payloads")
	request.SetBody([]byte(inboundOfferSDPWithMedia("127.0.0.1", 19000, "")))
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 488, transaction.lastStatus())
	_, exists := server.GetSession("inbound-no-payloads")
	assert.False(t, exists)
}

func TestInboundCall_RTPAllocationFailureEndsLifecycle(t *testing.T) {
	server := newServerForCommandTests(t)
	server.rtpAllocator = failingInboundRTPAllocator{err: errors.New("rtp exhausted")}
	server.newRTPHandler = testOutboundRTPHandler
	request := newInboundInviteRequest("inbound-rtp-failed")
	transaction := newActiveTestServerTx()
	inboundCall := newInboundCall(server, request, transaction)

	require.NoError(t, inboundCall.loadIdentity())
	require.NoError(t, inboundCall.parseMediaOffer())
	inboundCall.resolvedConfig = inboundResolvedConfig{config: bridgeTestConfig()}
	require.NoError(t, inboundCall.createSession())
	server.registerSession(inboundCall.session, inboundCall.identity.callID)
	require.NoError(t, inboundCall.createDialog())

	err := inboundCall.setupRTP()
	require.Error(t, err)
	inboundCall.failSetup(503, internal_inbound.FailureRTP, LifecycleReasonInboundInviteFailed, err)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 503, transaction.lastStatus())
	_, exists := server.GetSession("inbound-rtp-failed")
	assert.False(t, exists)
}

func TestInboundCall_RTPHandlerCreationFailureEndsLifecycle(t *testing.T) {
	server := newServerForCommandTests(t)
	server.rtpAllocator = &testRTPAllocator{nextPort: 19000}
	server.newRTPHandler = func(context.Context, *RTPConfig) (*RTPHandler, error) {
		return nil, errors.New("rtp bind failed")
	}
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	request := newInboundInviteRequest("inbound-rtp-handler-failed")
	transaction := newActiveTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 503, transaction.lastStatus())
	_, exists := server.GetSession("inbound-rtp-handler-failed")
	assert.False(t, exists)
}

func TestInboundCall_DialogSetupFailureDoesNotSendManualFinalResponse(t *testing.T) {
	server := newServerForCommandTests(t)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	request := newInboundInviteRequest("inbound-dialog-create-failed")
	transaction := newTestServerTx()

	server.handleInvite(request, transaction)

	assert.Empty(t, transaction.responses)
	_, exists := server.GetSession("inbound-dialog-create-failed")
	assert.False(t, exists)
}

func TestInboundCall_TryingResponseFailureEndsLifecycle(t *testing.T) {
	server := newServerForCommandTests(t)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	request := newInboundInviteRequest("inbound-trying-failed")
	transaction := newFailingStatusServerTx(100)

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 500, transaction.lastStatus())
	assertSIPStatus(t, transaction.responses, 100)
	_, exists := server.GetSession("inbound-trying-failed")
	assert.False(t, exists)
}

func TestInboundCall_RingingResponseFailureEndsLifecycle(t *testing.T) {
	server := newServerForCommandTests(t)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	request := newInboundInviteRequest("inbound-ringing-failed")
	transaction := newFailingStatusServerTx(180)

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 500, transaction.lastStatus())
	assertSIPStatus(t, transaction.responses, 100)
	assertSIPStatus(t, transaction.responses, 180)
	_, exists := server.GetSession("inbound-ringing-failed")
	assert.False(t, exists)
}

func TestInboundCall_CancelBeforeSessionCreationStopsSetup(t *testing.T) {
	server := newServerForCommandTests(t)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		cancelRequest := newSIPRequest(sip.CANCEL, "inbound-cancel-before-session")
		cancelTransaction := newTestServerTx()
		server.handleCancel(cancelRequest, cancelTransaction)
		require.NotEmpty(t, cancelTransaction.responses)
		assert.Equal(t, 200, cancelTransaction.lastStatus())
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	request := newInboundInviteRequest("inbound-cancel-before-session")
	transaction := newActiveTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 487, transaction.lastStatus())
	assertNoSIPStatus(t, transaction.responses, 200)
	_, exists := server.GetSession("inbound-cancel-before-session")
	assert.False(t, exists)
}

func TestInboundCall_ApplicationReadyBeforeAnswerAndMediaStart(t *testing.T) {
	server := newServerForCommandTests(t)
	server.rtpAllocator = &testRTPAllocator{nextPort: 19000}
	server.newRTPHandler = inboundNoopRTPHandler(server)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	phaseOrder := make([]string, 0, 2)
	transaction := newActiveAckableTestServerTx()
	server.SetOnApplicationReady(func(session *Session, _, _ string) error {
		phaseOrder = append(phaseOrder, "application_ready")
		assert.Equal(t, 180, transaction.lastStatus())
		assert.Equal(t, InboundSetupPhaseMediaAllocated, session.GetInboundSetupPhase())
		return nil
	})
	server.SetOnInvite(func(session *Session, _, _ string) error {
		phaseOrder = append(phaseOrder, "call_start")
		assert.Equal(t, InboundSetupPhaseMediaFlowing, session.GetInboundSetupPhase())
		return nil
	})
	request := newInboundInviteRequest("inbound-ready-before-answer")
	transaction.PushACK(newACKRequest("inbound-ready-before-answer"))

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 200, transaction.lastStatus())
	assert.Equal(t, []string{"application_ready", "call_start"}, phaseOrder)
	session, exists := server.GetSession("inbound-ready-before-answer")
	require.True(t, exists)
	assert.Equal(t, CallStateConnected, session.GetState())
	assert.Equal(t, InboundSetupPhaseMediaFlowing, session.GetInboundSetupPhase())
}

func TestInboundCall_ProvisionalResponsesBeforeAnswer(t *testing.T) {
	server := newServerForCommandTests(t)
	server.rtpAllocator = &testRTPAllocator{nextPort: 19000}
	server.newRTPHandler = inboundNoopRTPHandler(server)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	transaction := newActiveAckableTestServerTx()
	request := newInboundInviteRequest("inbound-provisional-order")
	transaction.PushACK(newACKRequest("inbound-provisional-order"))

	server.handleInvite(request, transaction)

	require.GreaterOrEqual(t, len(transaction.responses), 3)
	assert.Equal(t, 100, transaction.responses[0].StatusCode)
	assert.Equal(t, 180, transaction.responses[1].StatusCode)
	assert.Equal(t, 200, transaction.responses[2].StatusCode)
}

func TestInboundCall_StartsRTPAfterACK(t *testing.T) {
	server := newServerForCommandTests(t)
	server.rtpAllocator = &testRTPAllocator{nextPort: 19000}
	server.newRTPHandler = inboundNoopRTPHandler(server)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	transaction := newActiveAckableTestServerTx()
	request := newInboundInviteRequest("inbound-rtp-before-ack")
	done := make(chan struct{})

	go func() {
		defer close(done)
		server.handleInvite(request, transaction)
	}()

	require.Eventually(t, func() bool {
		return transaction.lastStatus() == 200
	}, time.Second, time.Millisecond)
	session, exists := server.GetSession("inbound-rtp-before-ack")
	require.True(t, exists)
	require.NotNil(t, session.GetRTPHandler())
	assert.Equal(t, InboundSetupPhaseAnswered, session.GetInboundSetupPhase())

	transaction.PushACK(newACKRequest("inbound-rtp-before-ack"))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for inbound INVITE handler")
	}
	assert.Equal(t, InboundSetupPhaseMediaFlowing, session.GetInboundSetupPhase())
}

func TestInboundCall_RetransmitsRingingUntilAnswer(t *testing.T) {
	server := newServerForCommandTests(t)
	server.inboundRingingInterval = 5 * time.Millisecond
	server.rtpAllocator = &testRTPAllocator{nextPort: 19000}
	server.newRTPHandler = inboundNoopRTPHandler(server)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	applicationReady := make(chan struct{})
	server.SetOnApplicationReady(func(_ *Session, _, _ string) error {
		<-applicationReady
		return nil
	})
	transaction := newActiveAckableTestServerTx()
	request := newInboundInviteRequest("inbound-ringing-retransmit")
	done := make(chan struct{})

	go func() {
		defer close(done)
		server.handleInvite(request, transaction)
	}()

	require.Eventually(t, func() bool {
		return transaction.statusCount(180) >= 2
	}, time.Second, time.Millisecond)
	assert.Equal(t, 0, transaction.statusCount(200))

	close(applicationReady)
	transaction.PushACK(newACKRequest("inbound-ringing-retransmit"))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for inbound INVITE handler")
	}
	ringingCount := transaction.statusCount(180)
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, ringingCount, transaction.statusCount(180))
	assert.Equal(t, 200, transaction.lastStatus())
}

func TestInboundCall_WaitsForApplicationReadyBeforeAnswer(t *testing.T) {
	server := newServerForCommandTests(t)
	server.rtpAllocator = &testRTPAllocator{nextPort: 19000}
	server.newRTPHandler = inboundNoopRTPHandler(server)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	applicationReady := make(chan struct{})
	server.SetOnApplicationReady(func(_ *Session, _, _ string) error {
		<-applicationReady
		return nil
	})
	transaction := newActiveAckableTestServerTx()
	request := newInboundInviteRequest("inbound-waits-application-ready")
	done := make(chan struct{})

	go func() {
		defer close(done)
		server.handleInvite(request, transaction)
	}()

	require.Eventually(t, func() bool {
		return transaction.lastStatus() == 180
	}, time.Second, time.Millisecond)
	assertNoSIPStatus(t, transaction.responses, 200)

	close(applicationReady)
	transaction.PushACK(newACKRequest("inbound-waits-application-ready"))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for inbound INVITE handler")
	}
	assert.Equal(t, 200, transaction.lastStatus())
}

func TestInboundCall_MinRingPolicyDelaysAnswer(t *testing.T) {
	server := newServerForCommandTests(t)
	server.rtpAllocator = &testRTPAllocator{nextPort: 19000}
	server.newRTPHandler = inboundNoopRTPHandler(server)
	minRingDuration := 25 * time.Millisecond
	config := bridgeTestConfig()
	config.InboundAnswerMode = InboundAnswerModeAfterMinRingDuration
	config.InboundMinRingDuration = minRingDuration
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = config
		return nil
	}})
	transaction := newActiveAckableTestServerTx()
	request := newInboundInviteRequest("inbound-min-ring")
	transaction.PushACK(newACKRequest("inbound-min-ring"))

	startedAt := time.Now()
	server.handleInvite(request, transaction)

	assert.GreaterOrEqual(t, time.Since(startedAt), minRingDuration)
	assert.Equal(t, 200, transaction.lastStatus())
}

func TestInboundCall_MinRingConfigRequiresDuration(t *testing.T) {
	server := newServerForCommandTests(t)
	config := bridgeTestConfig()
	config.InboundAnswerMode = InboundAnswerModeAfterMinRingDuration
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = config
		return nil
	}})
	transaction := newTestServerTx()
	request := newInboundInviteRequest("inbound-min-ring-requires-duration")

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 500, transaction.lastStatus())
	assertNoSIPStatus(t, transaction.responses, 200)
	session, exists := server.GetSession("inbound-min-ring-requires-duration")
	assert.False(t, exists)
	assert.Nil(t, session)
}

func TestInboundCall_AnswersAfterApplicationReadyWithoutAssistantAudio(t *testing.T) {
	server := newServerForCommandTests(t)
	server.rtpAllocator = &testRTPAllocator{nextPort: 19000}
	server.newRTPHandler = inboundNoopRTPHandler(server)
	config := bridgeTestConfig()
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = config
		return nil
	}})
	applicationReadyCalled := false
	server.SetOnApplicationReady(func(_ *Session, _, _ string) error {
		applicationReadyCalled = true
		return nil
	})
	transaction := newActiveAckableTestServerTx()
	request := newInboundInviteRequest("inbound-application-ready-no-audio")
	transaction.PushACK(newACKRequest("inbound-application-ready-no-audio"))

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.True(t, applicationReadyCalled)
	assert.Equal(t, 200, transaction.lastStatus())
	session, exists := server.GetSession("inbound-application-ready-no-audio")
	require.True(t, exists)
	timings := session.GetInboundSetupTimings()
	assert.True(t, timings.FirstAssistantAudioReadyAt.IsZero())
	assert.True(t, timings.FirstAssistantAudioSentAt.IsZero())
	metrics := session.GetInboundLatencyMetrics()
	assert.NotContains(t, metrics, "assistant_audio_ready_to_answer_ms")
	assert.NotContains(t, metrics, "answer_to_first_assistant_audio_sent_ms")
}

func TestInboundCall_UDPFinalResponseRetransmitsUntilACKTimeout(t *testing.T) {
	server := newServerForCommandTests(t)
	server.inboundACKTimeout = 35 * time.Millisecond
	server.inboundFinalResponseRetryInitial = 5 * time.Millisecond
	server.inboundFinalResponseRetryMax = 5 * time.Millisecond
	server.rtpAllocator = &testRTPAllocator{nextPort: 19000}
	server.newRTPHandler = inboundNoopRTPHandler(server)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	transaction := newActiveAckableTestServerTx()
	request := newInboundInviteRequest("inbound-200-retry")

	server.handleInvite(request, transaction)

	okResponses := 0
	for _, response := range transaction.responses {
		if response.StatusCode == 200 {
			okResponses++
		}
	}
	assert.GreaterOrEqual(t, okResponses, 2)
	_, exists := server.GetSession("inbound-200-retry")
	assert.False(t, exists)
}

func TestInboundCall_RecordsInboundLatencyMetrics(t *testing.T) {
	server := newServerForCommandTests(t)
	server.rtpAllocator = &testRTPAllocator{nextPort: 19000}
	server.newRTPHandler = inboundNoopRTPHandler(server)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	transaction := newActiveAckableTestServerTx()
	request := newInboundInviteRequest("inbound-latency-metrics")
	transaction.PushACK(newACKRequest("inbound-latency-metrics"))

	server.handleInvite(request, transaction)

	session, exists := server.GetSession("inbound-latency-metrics")
	require.True(t, exists)
	metrics := session.GetInboundLatencyMetrics()
	assert.Contains(t, metrics, "invite_to_100_ms")
	assert.Contains(t, metrics, "invite_to_180_ms")
	assert.Contains(t, metrics, "180_to_200_ms")
	assert.Contains(t, metrics, "200_to_ack_ms")
}

func TestInboundCall_ApplicationReadinessFailureRejectsBeforeAnswer(t *testing.T) {
	server := newServerForCommandTests(t)
	server.rtpAllocator = &testRTPAllocator{nextPort: 19000}
	server.newRTPHandler = inboundNoopRTPHandler(server)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	server.SetOnApplicationReady(func(_ *Session, _, _ string) error {
		return errors.New("assistant not ready")
	})
	server.SetOnInvite(func(_ *Session, _, _ string) error {
		t.Fatal("onInvite should not run when application readiness fails")
		return nil
	})
	request := newInboundInviteRequest("inbound-readiness-failed")
	transaction := newActiveAckableTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 503, transaction.lastStatus())
	for _, response := range transaction.responses {
		assert.NotEqual(t, 200, response.StatusCode)
	}
	_, exists := server.GetSession("inbound-readiness-failed")
	assert.False(t, exists)
}

func TestInboundCall_ACKTimeoutCleansPreparedApplication(t *testing.T) {
	server := newServerForCommandTests(t)
	server.inboundACKTimeout = time.Millisecond
	server.rtpAllocator = &testRTPAllocator{nextPort: 19000}
	server.newRTPHandler = inboundNoopRTPHandler(server)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	cleanupCount := 0
	server.SetOnApplicationReady(func(_ *Session, _, _ string) error {
		return nil
	})
	server.SetOnApplicationCleanup(func(_ *Session) {
		cleanupCount++
	})
	server.SetOnInvite(func(_ *Session, _, _ string) error {
		t.Fatal("onInvite should not run when ACK timeout fails")
		return nil
	})
	request := newInboundInviteRequest("inbound-ack-timeout-cleanup")
	transaction := newActiveAckableTestServerTx()

	server.handleInvite(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 200, transaction.lastStatus())
	assert.Equal(t, 1, cleanupCount)
	_, exists := server.GetSession("inbound-ack-timeout-cleanup")
	assert.False(t, exists)
}

func TestInboundCall_CancelWhileWaitingForACKDoesNotSend487After200(t *testing.T) {
	server := newServerForCommandTests(t)
	server.rtpAllocator = &testRTPAllocator{nextPort: 19000}
	server.newRTPHandler = inboundNoopRTPHandler(server)
	server.SetMiddlewares([]Middleware{func(ctx *SIPRequestContext) error {
		ctx.Config = bridgeTestConfig()
		return nil
	}})
	request := newInboundInviteRequest("inbound-cancel-waiting-ack")
	transaction := newActiveAckableTestServerTx()
	done := make(chan struct{})

	go func() {
		defer close(done)
		server.handleInvite(request, transaction)
	}()

	require.Eventually(t, func() bool {
		return transaction.lastStatus() == 200
	}, time.Second, time.Millisecond)

	cancelTransaction := newTestServerTx()
	server.handleCancel(newSIPRequest(sip.CANCEL, "inbound-cancel-waiting-ack"), cancelTransaction)

	require.NotEmpty(t, cancelTransaction.responses)
	assert.Equal(t, 481, cancelTransaction.lastStatus())
	assertNoSIPStatus(t, transaction.responses, 487)
	transaction.PushACK(newACKRequest("inbound-cancel-waiting-ack"))

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for inbound INVITE handler")
	}
	session, exists := server.GetSession("inbound-cancel-waiting-ack")
	require.True(t, exists)
	assert.Equal(t, CallStateConnected, session.GetState())
}

func TestInboundCall_CancelAfterSessionRegistrationEndsLifecycle(t *testing.T) {
	server := newServerForCommandTests(t)
	request := newInboundInviteRequest("inbound-cancel-registered")
	transaction := newActiveTestServerTx()
	inboundCall := newInboundCall(server, request, transaction)

	require.NoError(t, inboundCall.loadIdentity())
	require.NoError(t, inboundCall.parseMediaOffer())
	inboundCall.resolvedConfig = inboundResolvedConfig{config: bridgeTestConfig()}
	require.NoError(t, inboundCall.createSession())
	server.registerSession(inboundCall.session, inboundCall.identity.callID)
	server.setPendingInvite(inboundCall.identity.callID, request, transaction)
	server.markInviteCancelled(inboundCall.identity.callID)

	cancelled := inboundCall.cancelIfRequested(LifecycleReasonInviteCancelled)

	assert.True(t, cancelled)
	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 487, transaction.lastStatus())
	assertNoSIPStatus(t, transaction.responses, 200)
	assert.True(t, inboundCall.session.IsEnded())
	assert.Equal(t, CallStateCancelled, inboundCall.session.GetState())
	_, exists := server.GetSession("inbound-cancel-registered")
	assert.False(t, exists)
}

func TestInboundCall_CancelAfterRTPOwnershipReleasesPort(t *testing.T) {
	server := newServerForCommandTests(t)
	rtpAllocator := &testRTPAllocator{nextPort: 19000}
	server.rtpAllocator = rtpAllocator
	request := newInboundInviteRequest("inbound-cancel-rtp")
	transaction := newActiveTestServerTx()
	inboundCall := newInboundCall(server, request, transaction)

	require.NoError(t, inboundCall.loadIdentity())
	require.NoError(t, inboundCall.parseMediaOffer())
	inboundCall.resolvedConfig = inboundResolvedConfig{config: bridgeTestConfig()}
	require.NoError(t, inboundCall.createSession())
	inboundCall.session.SetLocalRTP("127.0.0.1", rtpAllocator.nextPort)
	inboundCall.session.SetRTPHandler(&RTPHandler{})
	server.registerSession(inboundCall.session, inboundCall.identity.callID)
	server.setPendingInvite(inboundCall.identity.callID, request, transaction)
	server.markInviteCancelled(inboundCall.identity.callID)

	cancelled := inboundCall.cancelIfRequested(LifecycleReasonInviteCancelledBeforeAnswer)

	assert.True(t, cancelled)
	assertNoSIPStatus(t, transaction.responses, 200)
	assert.True(t, inboundCall.session.IsEnded())
	assert.Equal(t, rtpAllocator.nextPort, rtpAllocator.releasePort)
	_, exists := server.GetSession("inbound-cancel-rtp")
	assert.False(t, exists)
}

func TestSIPCommand_CANCEL_ConnectedInboundReturns481(t *testing.T) {
	server := newServerForCommandTests(t)
	session := newTestSession(t, "inbound-cancel-connected", CallDirectionInbound)
	server.registerSession(session, "inbound-cancel-connected")
	require.True(t, server.TransitionCall(session, CallStateRinging, LifecycleReasonInboundInviteRinging))
	require.True(t, server.TransitionCall(session, CallStateConnected, LifecycleReasonInboundInviteAnswered))

	request := newSIPRequest(sip.CANCEL, "inbound-cancel-connected")
	transaction := newTestServerTx()

	server.handleCancel(request, transaction)

	require.NotEmpty(t, transaction.responses)
	assert.Equal(t, 481, transaction.lastStatus())
	assert.False(t, session.IsEnded())
}

type failingInboundRTPAllocator struct {
	err error
}

type activeTestServerTx struct {
	*testServerTx
}

func newActiveTestServerTx() *activeTestServerTx {
	return &activeTestServerTx{testServerTx: newTestServerTx()}
}

func (t *activeTestServerTx) OnTerminate(_ sip.FnTxTerminate) bool {
	return true
}

func (t *activeTestServerTx) OnCancel(_ sip.FnTxCancel) bool {
	return true
}

type activeAckableTestServerTx struct {
	*activeTestServerTx
	acks chan *sip.Request
}

func newActiveAckableTestServerTx() *activeAckableTestServerTx {
	return &activeAckableTestServerTx{
		activeTestServerTx: newActiveTestServerTx(),
		acks:               make(chan *sip.Request, 2),
	}
}

func (t *activeAckableTestServerTx) Acks() <-chan *sip.Request {
	return t.acks
}

func (t *activeAckableTestServerTx) PushACK(req *sip.Request) {
	t.acks <- req
}

func (a failingInboundRTPAllocator) Allocate() (int, error) {
	return 0, a.err
}

func (a failingInboundRTPAllocator) Release(_ int) {}

func (a failingInboundRTPAllocator) InUse() (int, error) {
	return 0, nil
}

func (a failingInboundRTPAllocator) ReleaseAll(_ context.Context) {}

func assertNoSIPStatus(t *testing.T, responses []*sip.Response, statusCode int) {
	t.Helper()
	for _, response := range responses {
		assert.NotEqual(t, statusCode, response.StatusCode)
	}
}

func assertSIPStatus(t *testing.T, responses []*sip.Response, statusCode int) {
	t.Helper()
	for _, response := range responses {
		if response.StatusCode == statusCode {
			return
		}
	}
	t.Fatalf("expected SIP status %d in responses", statusCode)
}

func inboundNoopRTPHandler(server *Server) RTPHandlerFactory {
	return func(_ context.Context, cfg *RTPConfig) (*RTPHandler, error) {
		handler := newTestRTPHandler()
		handler.localIP = cfg.LocalIP
		handler.localPort = cfg.LocalPort
		handler.codec = &CodecPCMU
		handler.logger = server.logger
		return handler, nil
	}
}

func unsupportedInboundOfferSDP() string {
	return inboundOfferSDPWithMedia("127.0.0.1", 19000, "18 101")
}

func inboundOfferSDPWithMedia(ip string, port int, payloadTypes string) string {
	mediaLine := "m=audio " + fmt.Sprintf("%d RTP/AVP", port)
	if payloadTypes != "" {
		mediaLine += " " + payloadTypes
	}
	return inboundOfferSDPWithRawMedia(ip, mediaLine)
}

func inboundOfferSDPWithoutConnection() string {
	return "v=0\r\n" +
		"o=caller 1 1 IN IP4 127.0.0.1\r\n" +
		"s=call\r\n" +
		"t=0 0\r\n" +
		"m=audio 19000 RTP/AVP 0 101\r\n" +
		"a=rtpmap:0 PCMU/8000\r\n" +
		"a=rtpmap:101 telephone-event/8000\r\n" +
		"a=sendrecv\r\n"
}

func inboundOfferSDPWithRawMedia(ip string, mediaLine string) string {
	return "v=0\r\n" +
		"o=caller 1 1 IN IP4 127.0.0.1\r\n" +
		"s=call\r\n" +
		"c=IN IP4 " + ip + "\r\n" +
		"t=0 0\r\n" +
		mediaLine + "\r\n" +
		"a=rtpmap:0 PCMU/8000\r\n" +
		"a=rtpmap:101 telephone-event/8000\r\n" +
		"a=sendrecv\r\n"
}
