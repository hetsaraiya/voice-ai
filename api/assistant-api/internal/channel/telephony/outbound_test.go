package channel_telephony

import (
	"context"
	"testing"
	"time"

	"github.com/rapidaai/api/assistant-api/config"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	internal_telephony_base "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/base"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
)

type outboundDispatcherTestStore struct {
	callContext *callcontext.CallContext
	lastStatus  callcontext.CallStatusUpdate
	updateCount int
}

func (s *outboundDispatcherTestStore) Save(ctx context.Context, cc *callcontext.CallContext) (string, error) {
	return cc.ContextID, nil
}

func (s *outboundDispatcherTestStore) Get(ctx context.Context, contextID string) (*callcontext.CallContext, error) {
	return s.callContext, nil
}

func (s *outboundDispatcherTestStore) GetByChannelUUID(ctx context.Context, provider string, assistantID uint64, channelUUID string) (*callcontext.CallContext, error) {
	return nil, nil
}

func (s *outboundDispatcherTestStore) Claim(ctx context.Context, contextID string) (*callcontext.CallContext, error) {
	s.callContext.Status = callcontext.StatusClaimed
	return s.callContext, nil
}

func (s *outboundDispatcherTestStore) UpdateField(ctx context.Context, contextID, field, value string) error {
	if field == "status" {
		s.callContext.Status = value
	}
	if field == "channel_uuid" {
		s.callContext.ChannelUUID = value
	}
	return nil
}

func (s *outboundDispatcherTestStore) UpdateCallStatus(ctx context.Context, contextID string, status callcontext.CallStatusUpdate) error {
	s.lastStatus = status
	s.updateCount++
	s.callContext.CallStatus = status.CallStatus
	s.callContext.CallError = status.CallError
	s.callContext.FailureClass = status.FailureClass
	s.callContext.FailureReason = status.FailureReason
	s.callContext.DisconnectReason = status.DisconnectReason
	s.callContext.Retryable = status.Retryable
	s.callContext.ProviderStatusCode = status.ProviderStatusCode
	if status.CallStatus == callcontext.StatusFailed || status.CallStatus == "cancelled" {
		s.callContext.Status = callcontext.StatusFailed
	}
	return nil
}

func newOutboundDispatcherTestLogger(t *testing.T) commons.Logger {
	t.Helper()
	logger, err := commons.NewApplicationLogger(
		commons.EnableConsole(true),
		commons.EnableFile(false),
		commons.Level("error"),
	)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return logger
}

func TestOutboundDispatcher_PersistSetupFailureDoesNotOverwriteTerminalProviderStatus(t *testing.T) {
	store := &outboundDispatcherTestStore{
		callContext: &callcontext.CallContext{
			ContextID:    "ctx-terminal",
			Status:       callcontext.StatusFailed,
			CallStatus:   internal_telephony_base.OutboundCallStatusFailed,
			FailureClass: internal_telephony_base.OutboundFailureClassProviderAPI,
		},
	}
	dispatcher := &OutboundDispatcher{
		store:  store,
		logger: newOutboundDispatcherTestLogger(t),
	}

	dispatcher.persistOutboundSetupFailure(context.Background(), store.callContext, assertOutboundError{})

	if store.updateCount != 0 {
		t.Fatalf("expected terminal provider status to be preserved, got %d updates", store.updateCount)
	}
	if store.callContext.FailureClass != internal_telephony_base.OutboundFailureClassProviderAPI {
		t.Fatalf("expected provider failure class preserved, got %q", store.callContext.FailureClass)
	}
}

func TestOutboundDispatcher_PersistSetupFailureDoesNotOverwriteCancelledProviderStatus(t *testing.T) {
	store := &outboundDispatcherTestStore{
		callContext: &callcontext.CallContext{
			ContextID:        "ctx-cancelled",
			Status:           callcontext.StatusFailed,
			CallStatus:       internal_telephony_base.OutboundCallStatusCancelled,
			FailureClass:     internal_telephony_base.OutboundFailureClassRequestCancelled,
			DisconnectReason: internal_telephony_base.OutboundDisconnectReasonRequestCancelled,
		},
	}
	dispatcher := &OutboundDispatcher{
		store:  store,
		logger: newOutboundDispatcherTestLogger(t),
	}

	dispatcher.persistOutboundSetupFailure(context.Background(), store.callContext, assertOutboundError{})

	if store.updateCount != 0 {
		t.Fatalf("expected cancelled provider status to be preserved, got %d updates", store.updateCount)
	}
	if store.callContext.CallStatus != internal_telephony_base.OutboundCallStatusCancelled {
		t.Fatalf("expected cancelled call status preserved, got %q", store.callContext.CallStatus)
	}
	if store.callContext.DisconnectReason != internal_telephony_base.OutboundDisconnectReasonRequestCancelled {
		t.Fatalf("expected cancelled disconnect reason preserved, got %q", store.callContext.DisconnectReason)
	}
}

func TestOutboundDispatcher_PersistSetupFailureUsesProviderNeutralStatus(t *testing.T) {
	store := &outboundDispatcherTestStore{
		callContext: &callcontext.CallContext{
			ContextID: "ctx-setup",
			Status:    callcontext.StatusPending,
		},
	}
	dispatcher := &OutboundDispatcher{
		store:  store,
		logger: newOutboundDispatcherTestLogger(t),
	}

	dispatcher.persistOutboundSetupFailure(context.Background(), store.callContext, assertOutboundError{})

	if store.callContext.Status != callcontext.StatusFailed {
		t.Fatalf("expected context failed, got %q", store.callContext.Status)
	}
	if store.lastStatus.FailureClass != internal_telephony_base.OutboundFailureClassSetup {
		t.Fatalf("expected setup failure class, got %q", store.lastStatus.FailureClass)
	}
	if store.lastStatus.DisconnectReason != internal_telephony_base.OutboundDisconnectReasonSetupFailed {
		t.Fatalf("expected setup disconnect reason, got %q", store.lastStatus.DisconnectReason)
	}
}

func TestOutboundDispatcher_StatusReporterPersistsFailureDetails(t *testing.T) {
	store := &outboundDispatcherTestStore{
		callContext: &callcontext.CallContext{
			ContextID: "ctx-provider-failure",
			Status:    callcontext.StatusPending,
		},
	}
	dispatcher := &OutboundDispatcher{
		store:  store,
		logger: newOutboundDispatcherTestLogger(t),
	}
	statusReporter := dispatcher.newProviderCallStatusReporter(store.callContext.ContextID)

	statusReporter(internal_type.ProviderCallStatusUpdate{
		ChannelUUID:        "call-486",
		CallStatus:         internal_telephony_base.OutboundCallStatusFailed,
		ErrorMessage:       "486 Busy Here",
		FailureClass:       "busy",
		FailureReason:      "Busy Here",
		DisconnectReason:   "outbound_rejected",
		ProviderStatusCode: 486,
	})

	if store.callContext.ChannelUUID != "call-486" {
		t.Fatalf("expected channel UUID persisted, got %q", store.callContext.ChannelUUID)
	}
	if store.callContext.Status != callcontext.StatusFailed {
		t.Fatalf("expected failed context status, got %q", store.callContext.Status)
	}
	if store.callContext.FailureClass != "busy" {
		t.Fatalf("expected busy failure class, got %q", store.callContext.FailureClass)
	}
	if store.callContext.ProviderStatusCode != 486 {
		t.Fatalf("expected provider status 486, got %d", store.callContext.ProviderStatusCode)
	}
}

func TestOutboundDispatcher_PersistConnectTimeoutUsesDurableFailureStatus(t *testing.T) {
	store := &outboundDispatcherTestStore{
		callContext: &callcontext.CallContext{
			ContextID: "ctx-timeout",
			Status:    callcontext.StatusPending,
		},
	}
	dispatcher := &OutboundDispatcher{
		store:  store,
		logger: newOutboundDispatcherTestLogger(t),
	}

	dispatcher.persistOutboundConnectTimeout(context.Background(), store.callContext)

	if store.callContext.Status != callcontext.StatusFailed {
		t.Fatalf("expected context failed, got %q", store.callContext.Status)
	}
	if store.lastStatus.FailureClass != internal_telephony_base.OutboundFailureClassNoAnswer {
		t.Fatalf("expected no_answer failure class, got %q", store.lastStatus.FailureClass)
	}
	if store.lastStatus.DisconnectReason != internal_telephony_base.OutboundDisconnectReasonNoAnswer {
		t.Fatalf("expected connect timeout disconnect reason, got %q", store.lastStatus.DisconnectReason)
	}
	if !store.lastStatus.Retryable {
		t.Fatal("expected connect timeout to be retryable")
	}
}

func TestOutboundDispatcher_MonitorConnectTimeoutSurvivesRequestCancellation(t *testing.T) {
	store := &outboundDispatcherTestStore{
		callContext: &callcontext.CallContext{
			ContextID: "ctx-monitor",
			Provider:  SIP.String(),
			Status:    callcontext.StatusPending,
		},
	}
	dispatcher := &OutboundDispatcher{
		store:                  store,
		logger:                 newOutboundDispatcherTestLogger(t),
		outboundConnectTimeout: 10 * time.Millisecond,
	}
	requestContext, cancelRequest := context.WithCancel(context.Background())
	callMonitorContext := context.WithoutCancel(requestContext)
	cancelRequest()

	go dispatcher.monitorCallConnect(callMonitorContext, store.callContext.ContextID, store.callContext)

	deadline := time.After(250 * time.Millisecond)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("expected answer monitor to persist no-answer after request context cancellation")
		case <-ticker.C:
			if store.updateCount == 0 {
				continue
			}
			if store.callContext.FailureClass != internal_telephony_base.OutboundFailureClassNoAnswer {
				t.Fatalf("expected no_answer failure class, got %q", store.callContext.FailureClass)
			}
			if store.callContext.DisconnectReason != internal_telephony_base.OutboundDisconnectReasonNoAnswer {
				t.Fatalf("expected no_answer disconnect reason, got %q", store.callContext.DisconnectReason)
			}
			return
		}
	}
}

func TestOutboundDispatcher_SIPConnectTimeoutDerivesFromInviteTimeout(t *testing.T) {
	dispatcher := &OutboundDispatcher{
		cfg: &config.AssistantConfig{
			SIPConfig: &config.SIPConfig{InviteTimeout: 30 * time.Second},
		},
		outboundConnectTimeout: defaultOutboundConnectTimeout,
	}

	got := dispatcher.providerOutboundConnectTimeout(SIP.String())

	if got != 45*time.Second {
		t.Fatalf("expected SIP connect timeout 45s, got %s", got)
	}
}

type assertOutboundError struct{}

func (assertOutboundError) Error() string {
	return "outbound setup failed"
}
