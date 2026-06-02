// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package sip_pipeline

import (
	"context"
	"fmt"
	"sync"

	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	observe "github.com/rapidaai/api/assistant-api/internal/observe"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
)

const (
	signalChSize  = 64
	setupChSize   = 256
	mediaChSize   = 256
	controlChSize = 512
)

type callEnvelope struct {
	ctx context.Context
	p   sip_infra.Pipeline
}

// Dispatcher routes SIP pipeline stages to priority-based channel goroutines.
// Stateless — no per-call state stored on the Dispatcher.
type Dispatcher struct {
	logger commons.Logger

	signalCh  chan callEnvelope
	setupCh   chan callEnvelope
	mediaCh   chan callEnvelope
	controlCh chan callEnvelope

	preparedMu       sync.Mutex
	preparedSessions map[string]*preparedSession

	server             TransferServer
	registrationClient *sip_infra.RegistrationClient

	didResolver          DIDResolverFunc
	onCreateConversation OnCreateConversationFunc
	onEnsureCallContext  OnEnsureCallContextFunc
	onCallSetup          OnCallSetupFunc
	onPrepareCallRuntime OnPrepareCallRuntimeFunc
	onCallStart          OnCallStartFunc
	onCallEnd            OnCallEndFunc
	onCreateObserver     OnCreateObserverFunc
}

type DIDResolverFunc func(did string) (assistantID uint64, auth types.SimplePrinciple, err error)

type OnCreateConversationFunc func(ctx context.Context, auth types.SimplePrinciple, assistantID uint64, fromURI string, direction string) (conversationID uint64, err error)

// OnEnsureCallContextFunc resolves the durable CallContext for a SIP session.
// Outbound: claim/load the record persisted by the channel pipeline. Inbound:
// build from the INVITE URIs and persist. Should return an in-memory cc on DB
// failure so the call still proceeds.
type OnEnsureCallContextFunc func(
	ctx context.Context,
	session *sip_infra.Session,
	auth types.SimplePrinciple,
	assistantID uint64,
	conversationID uint64,
	direction sip_infra.CallDirection,
	fromURI string,
	toURI string,
) (*callcontext.CallContext, error)

type OnCallSetupFunc func(ctx context.Context, session *sip_infra.Session, auth types.SimplePrinciple, assistantID uint64, conversationID uint64, cc *callcontext.CallContext) (*CallSetupResult, error)

type CallSetupResult struct {
	AssistantID         uint64
	ConversationID      uint64
	AssistantProviderId uint64
	AuthToken           string
	AuthType            string
	ProjectID           uint64
	OrganizationID      uint64
	// CallContext is resolved by OnEnsureCallContext and carried in memory
	// into pipelineCallStart. May be nil if OnEnsureCallContext is unset.
	CallContext *callcontext.CallContext
}

type OnCallStartFunc func(ctx context.Context, session *sip_infra.Session, setup *CallSetupResult, vaultCred interface{}, sipConfig *sip_infra.Config, direction string) error
type OnCallEndFunc func(callID string)
type OnCreateObserverFunc func(ctx context.Context, setup *CallSetupResult, auth types.SimplePrinciple) *observe.ConversationObserver

type PreparedCallRuntime interface {
	Start(ctx context.Context) error
	Close(ctx context.Context)
}

type OnPrepareCallRuntimeFunc func(ctx context.Context, stage sip_infra.SessionEstablishedPipeline, setup *CallSetupResult, observer *observe.ConversationObserver) (PreparedCallRuntime, error)

type DispatcherConfig struct {
	Logger               commons.Logger
	Server               *sip_infra.Server
	TransferServer       TransferServer
	RegistrationClient   *sip_infra.RegistrationClient
	DIDResolver          DIDResolverFunc
	OnCreateConversation OnCreateConversationFunc
	OnEnsureCallContext  OnEnsureCallContextFunc
	OnCallSetup          OnCallSetupFunc
	OnPrepareCallRuntime OnPrepareCallRuntimeFunc
	OnCallStart          OnCallStartFunc
	OnCallEnd            OnCallEndFunc
	OnCreateObserver     OnCreateObserverFunc
}

// TransferServer is the minimal SIP infra surface required by transfer orchestration.
// It enables deterministic tests by allowing fake implementations.
type TransferServer interface {
	sip_infra.LifecycleController
	MakeTransferBridgeCall(ctx context.Context, cfg *sip_infra.Config, toURI, fromURI string, opts sip_infra.TransferBridgeCallOptions) (*sip_infra.Session, error)
	BridgeTransfer(ctx context.Context, inbound, outbound *sip_infra.Session, onOperatorAudio func([]byte)) (sip_infra.BridgeEndReason, error)
}

func NewDispatcher(cfg *DispatcherConfig) *Dispatcher {
	transferServer := cfg.TransferServer
	if transferServer == nil && cfg.Server != nil {
		transferServer = cfg.Server
	}
	return &Dispatcher{
		logger:               cfg.Logger,
		server:               transferServer,
		registrationClient:   cfg.RegistrationClient,
		didResolver:          cfg.DIDResolver,
		onCreateConversation: cfg.OnCreateConversation,
		onEnsureCallContext:  cfg.OnEnsureCallContext,
		onCallSetup:          cfg.OnCallSetup,
		onPrepareCallRuntime: cfg.OnPrepareCallRuntime,
		onCallStart:          cfg.OnCallStart,
		onCallEnd:            cfg.OnCallEnd,
		onCreateObserver:     cfg.OnCreateObserver,
		signalCh:             make(chan callEnvelope, signalChSize),
		setupCh:              make(chan callEnvelope, setupChSize),
		mediaCh:              make(chan callEnvelope, mediaChSize),
		controlCh:            make(chan callEnvelope, controlChSize),
		preparedSessions:     make(map[string]*preparedSession),
	}
}

func (d *Dispatcher) transitionCall(session *sip_infra.Session, next sip_infra.CallState, reason sip_infra.LifecycleReason) bool {
	if d.server == nil {
		d.logger.Warnw("SIP lifecycle transition skipped: server unavailable",
			"call_id", session.GetCallID(),
			"to", next,
			"reason", reason)
		return false
	}
	return d.server.TransitionCall(session, next, reason)
}

func (d *Dispatcher) endCall(session *sip_infra.Session, reason sip_infra.LifecycleReason) {
	if d.server == nil {
		d.logger.Warnw("SIP lifecycle end skipped: server unavailable",
			"call_id", session.GetCallID(),
			"reason", reason)
		return
	}
	if err := d.server.EndCallWithReason(session, reason); err != nil {
		d.logger.Warnw("SIP lifecycle end failed",
			"call_id", session.GetCallID(),
			"reason", reason,
			"error", err)
	}
}

func (d *Dispatcher) Start(ctx context.Context) {
	go d.runDispatcher(ctx, d.signalCh)
	go d.runDispatcher(ctx, d.setupCh)
	go d.runDispatcher(ctx, d.mediaCh)
	go d.runDispatcher(ctx, d.controlCh)
	d.logger.Infow("SIP pipeline dispatcher started")
}

func (d *Dispatcher) OnPipeline(ctx context.Context, stages ...sip_infra.Pipeline) {
	for _, s := range stages {
		e := callEnvelope{ctx: ctx, p: s}
		switch s.(type) {
		case sip_infra.ByeReceivedPipeline,
			sip_infra.CancelReceivedPipeline,
			sip_infra.TransferInitiatedPipeline,
			sip_infra.TransferConnectedPipeline,
			sip_infra.TransferFailedPipeline,
			sip_infra.CallEndedPipeline,
			sip_infra.CallFailedPipeline:
			d.signalCh <- e
		case sip_infra.SessionEstablishedPipeline:
			d.mediaCh <- e
		case sip_infra.EventEmittedPipeline,
			sip_infra.MetricEmittedPipeline,
			sip_infra.DTMFReceivedPipeline:
			d.controlCh <- e
		default:
			d.logger.Warnw("OnPipeline: unrouted type", "type", fmt.Sprintf("%T", s))
			d.controlCh <- e
		}
	}
}

func (d *Dispatcher) runDispatcher(ctx context.Context, ch chan callEnvelope) {
	for {
		select {
		case <-ctx.Done():
			d.drain(ch)
			return
		case e := <-ch:
			d.dispatch(e.ctx, e.p)
		}
	}
}

func (d *Dispatcher) drain(ch chan callEnvelope) {
	for {
		select {
		case e := <-ch:
			d.dispatch(e.ctx, e.p)
		default:
			return
		}
	}
}

func (d *Dispatcher) dispatch(ctx context.Context, p sip_infra.Pipeline) {
	switch v := p.(type) {
	case sip_infra.SessionEstablishedPipeline:
		d.handleSessionEstablished(ctx, v)
	case sip_infra.ByeReceivedPipeline:
		d.handleByeReceived(ctx, v)
	case sip_infra.CancelReceivedPipeline:
		d.handleCancelReceived(ctx, v)
	case sip_infra.TransferInitiatedPipeline:
		d.handleTransferInitiated(ctx, v)
	case sip_infra.TransferConnectedPipeline:
		d.handleTransferConnected(ctx, v)
	case sip_infra.TransferFailedPipeline:
		d.handleTransferFailed(ctx, v)
	case sip_infra.CallEndedPipeline:
		d.handleCallEnded(ctx, v)
	case sip_infra.CallFailedPipeline:
		d.handleCallFailed(ctx, v)
	case sip_infra.EventEmittedPipeline:
		d.handleEventEmitted(ctx, v)
	case sip_infra.MetricEmittedPipeline:
		d.handleMetricEmitted(ctx, v)
	case sip_infra.DTMFReceivedPipeline:
		d.handleDTMFReceived(ctx, v)
	default:
		d.logger.Warnw("dispatch: unknown pipeline type", "type", fmt.Sprintf("%T", p))
	}
}
