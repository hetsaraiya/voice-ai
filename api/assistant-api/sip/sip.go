// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package assistant_sip

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rapidaai/api/assistant-api/config"
	internal_adapter "github.com/rapidaai/api/assistant-api/internal/adapters"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	internal_telephony "github.com/rapidaai/api/assistant-api/internal/channel/telephony"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/api/assistant-api/internal/observability/collectors"
	observability_collector_conversationdb "github.com/rapidaai/api/assistant-api/internal/observability/collectors/conversationdb"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	internal_assistant_service "github.com/rapidaai/api/assistant-api/internal/services/assistant"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	sip_pipeline "github.com/rapidaai/api/assistant-api/sip/pipeline"
	sip_registration "github.com/rapidaai/api/assistant-api/sip/registration"
	web_client "github.com/rapidaai/pkg/clients/web"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	"github.com/rapidaai/pkg/storages"
	storage_files "github.com/rapidaai/pkg/storages/file-storage"
	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SIPEngine manages a multi-tenant SIP server. Config is resolved per-call
// from each assistant's phone deployment and vault credentials.
type SIPEngine struct {
	cfg    *config.AssistantConfig
	logger commons.Logger
	mu     sync.RWMutex
	server *sip_infra.Server

	ctx    context.Context
	cancel context.CancelFunc

	postgres   connectors.PostgresConnector
	redis      connectors.RedisConnector
	opensearch connectors.OpenSearchConnector
	storage    storages.Storage

	assistantConversationService internal_services.AssistantConversationService
	assistantService             internal_services.AssistantService
	deploymentService            internal_services.AssistantDeploymentService
	vaultClient                  web_client.VaultClient
	callContextStore             callcontext.Store

	// Registration client for maintaining SIP REGISTER with external providers.
	registrationClient *sip_infra.RegistrationClient

	// Distributed registration manager — runs the GetRecord -> ClaimOwner ->
	// Register -> UpdateStatus pipeline, sharded across instances by externalIP.
	regManager sip_registration.Manager

	// Pipeline dispatcher — routes SIP call lifecycle through extensible stages.
	dispatcher *sip_pipeline.Dispatcher
}

func NewSIPEngine(config *config.AssistantConfig, logger commons.Logger,
	postgres connectors.PostgresConnector,
	redis connectors.RedisConnector,
	opensearch connectors.OpenSearchConnector,
	vectordb connectors.VectorConnector) *SIPEngine {
	return &SIPEngine{
		cfg:                          config,
		logger:                       logger,
		postgres:                     postgres,
		redis:                        redis,
		opensearch:                   opensearch,
		assistantConversationService: internal_assistant_service.NewAssistantConversationService(logger, postgres, storage_files.NewStorage(config.AssetStoreConfig, logger)),
		assistantService:             internal_assistant_service.NewAssistantService(config, logger, postgres, opensearch),
		deploymentService:            internal_assistant_service.NewAssistantDeploymentService(config, logger, postgres),
		storage:                      storage_files.NewStorage(config.AssetStoreConfig, logger),
		vaultClient:                  web_client.NewVaultClientGRPC(&config.AppConfig, logger, redis),
		callContextStore:             callcontext.NewStore(postgres, logger),
	}
}

func (m *SIPEngine) listenConfig() *sip_infra.ListenConfig {
	transportType := sip_infra.TransportUDP
	switch m.cfg.SIPConfig.Transport {
	case "tcp":
		transportType = sip_infra.TransportTCP
	case "tls":
		transportType = sip_infra.TransportTLS
	}
	lc := &sip_infra.ListenConfig{
		Address:                 m.cfg.SIPConfig.Server,
		ExternalIP:              m.cfg.SIPConfig.ExternalIP,
		AllowLoopbackExternalIP: m.cfg.SIPConfig.AllowLoopbackExternalIP,
		Port:                    m.cfg.SIPConfig.Port,
		Transport:               transportType,
	}
	m.logger.Infow("SIP ListenConfig from app config",
		"address", lc.Address,
		"external_ip", lc.ExternalIP,
		"port", lc.Port,
		"transport", lc.Transport,
		"raw_sip_config_external_ip", m.cfg.SIPConfig.ExternalIP,
		"raw_sip_config_server", m.cfg.SIPConfig.Server)
	return lc
}

// Connect initializes the SIP server. The middleware chain resolves the
// assistant from the DID in the To-URI:
//
//	routingMiddleware (DID lookup) -> assistantMiddleware -> vaultConfigResolver
func (m *SIPEngine) Connect(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)
	server, err := sip_infra.NewServer(m.ctx, &sip_infra.ServerConfig{
		ListenConfig:      m.listenConfig(),
		Logger:            m.logger,
		RedisClient:       m.redis.GetConnection(),
		RTPPortRangeStart: m.cfg.SIPConfig.RTPPortRangeStart,
		RTPPortRangeEnd:   m.cfg.SIPConfig.RTPPortRangeEnd,
	})
	if err != nil {
		return fmt.Errorf("failed to create SIP server: %w", err)
	}
	server.SetMiddlewares(
		[]sip_infra.Middleware{
			m.routingMiddleware,   // Resolve assistant by DID
			m.assistantMiddleware, // Load assistant entity
		},
		m.vaultConfigResolver, // Fetch SIP config from vault
	)
	server.SetOnApplicationReady(m.onApplicationReady)
	server.SetOnApplicationCleanup(m.onApplicationCleanup)
	server.SetOnInvite(m.onInvite)
	server.SetOnBye(m.onBye)
	server.SetOnCancel(m.onCancel)
	server.SetOnError(m.onError)
	m.registrationClient = sip_infra.NewRegistrationClient(server.Client(), server.GetListenConfig(), m.logger)
	m.regManager = sip_registration.New(
		sip_registration.WithLogger(m.logger),
		sip_registration.WithPostgres(m.postgres),
		sip_registration.WithRedis(m.redis),
		sip_registration.WithVault(m.vaultClient),
		sip_registration.WithRegistrationClient(m.registrationClient),
		sip_registration.WithAssistantConfig(m.cfg),
		sip_registration.WithSIPConfig(m.cfg.SIPConfig),
		sip_registration.WithApplyOpDefaults(m.applySIPOperationalDefaults),
	)

	m.dispatcher = sip_pipeline.NewDispatcher(&sip_pipeline.DispatcherConfig{
		Logger:               m.logger,
		Server:               server,
		RegistrationClient:   m.registrationClient,
		DIDResolver:          m.resolveAssistantByDID,
		OnCreateConversation: m.pipelineCreateConversation,
		OnEnsureCallContext:  m.pipelineEnsureCallContext,
		OnCallSetup:          m.pipelineCallSetup,
		OnPrepareCallRuntime: m.prepareInboundCallRuntime,
		OnCallStart:          m.pipelineCallStart,
		OnCallEnd:            m.pipelineCallEnd,
		OnCreateObserver:     m.createObserver,
	})
	m.dispatcher.Start(m.ctx)

	// Start server AFTER dispatcher is ready — incoming INVITEs call m.dispatcher.OnPipeline
	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start SIP server: %w", err)
	}
	m.server = server

	// Initial registration sync — runs before returning so DIDs are active before calls arrive.
	m.regManager.Reconcile(m.ctx)

	// Background watcher — polls DB every 5 minutes for new/removed/changed deployments.
	go m.regManager.Start(m.ctx)

	return nil
}

// applySIPOperationalDefaults overlays the engine-level SIP defaults (port,
// transport, RTP range, timeouts, inbound answer policy) onto a per-DID vault
// config. Passed to the registration manager as an injection point so the
// registration package stays decoupled from the assistant-api config types.
func (m *SIPEngine) applySIPOperationalDefaults(c *sip_infra.Config) {
	if m.cfg == nil || m.cfg.SIPConfig == nil {
		return
	}
	m.applySIPConfigDefaults(c)
}

func (m *SIPEngine) applySIPConfigDefaults(c *sip_infra.Config) {
	if c == nil || m.cfg == nil || m.cfg.SIPConfig == nil {
		return
	}
	c.ApplyOperationalDefaults(
		m.cfg.SIPConfig.Port,
		sip_infra.Transport(m.cfg.SIPConfig.Transport),
		m.cfg.SIPConfig.RTPPortRangeStart,
		m.cfg.SIPConfig.RTPPortRangeEnd,
	)
	c.ApplyTimeoutDefaults(
		m.cfg.SIPConfig.RegisterTimeout,
		m.cfg.SIPConfig.InviteTimeout,
		m.cfg.SIPConfig.SessionTimeout,
	)
	inboundConfig := m.cfg.SIPConfig.Inbound
	c.ApplyInboundAnswerDefaults(
		sip_infra.InboundAnswerMode(inboundConfig.AnswerMode),
		inboundConfig.MinRingDuration,
		inboundConfig.MaxRingDuration,
		inboundConfig.ACKTimeout,
	)
}

func (m *SIPEngine) GetServer() *sip_infra.Server {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.server
}

// assistantMiddleware loads the assistant entity and verifies project-level access.
func (m *SIPEngine) assistantMiddleware(ctx *sip_infra.SIPRequestContext, next func() (*sip_infra.InviteResult, error)) (*sip_infra.InviteResult, error) {
	authVal, _ := ctx.Get("auth")
	auth, _ := authVal.(types.SimplePrinciple)
	if auth == nil {
		return sip_infra.Reject(401, "Authentication required"), nil
	}

	if ctx.AssistantID == "" {
		return sip_infra.Reject(404, "Invalid SIP URI format, expected: sip:{assistantID}:{apiKey}@host"), nil
	}
	assistantID, err := strconv.ParseUint(ctx.AssistantID, 10, 64)
	if err != nil {
		m.logger.Warnw("SIP: invalid assistant ID", "call_id", ctx.CallID, "method", ctx.Method, "assistant_id", ctx.AssistantID)
		return sip_infra.Reject(404, "Invalid assistant ID format"), nil
	}

	assistant, err := m.assistantService.Get(m.ctx, auth, assistantID, utils.GetVersionDefinition("latest"),
		&internal_services.GetAssistantOption{InjectPhoneDeployment: true})
	if err != nil {
		m.logger.Error("SIP: assistant not found", "call_id", ctx.CallID, "method", ctx.Method, "assistant_id", assistantID, "error", err)
		return sip_infra.Reject(404, "Assistant not found"), nil
	}

	if !m.hasAccessToAssistant(auth, assistant) {
		return sip_infra.Reject(403, "API key does not have access to this assistant"), nil
	}

	ctx.Set("assistant", assistant)
	return next()
}

// vaultConfigResolver is the terminal middleware handler. It fetches provider
// config from vault and returns the InviteResult with resolved metadata.
func (m *SIPEngine) vaultConfigResolver(ctx *sip_infra.SIPRequestContext) (*sip_infra.InviteResult, error) {
	authVal, _ := ctx.Get("auth")
	auth, _ := authVal.(types.SimplePrinciple)
	assistantVal, _ := ctx.Get("assistant")
	assistant, _ := assistantVal.(*internal_assistant_entity.Assistant)

	if auth == nil || assistant == nil {
		return sip_infra.Reject(500, "Middleware chain incomplete"), nil
	}

	sipConfig, vaultCred, err := m.fetchSIPConfigAndVaultCredential(auth, assistant)
	if err != nil {
		m.logger.Error("SIP: failed to resolve config", "call_id", ctx.CallID, "method", ctx.Method, "error", err)
		return sip_infra.Reject(500, "Failed to resolve SIP configuration"), nil
	}

	var orgID uint64
	if auth.GetCurrentOrganizationId() != nil {
		orgID = *auth.GetCurrentOrganizationId()
	}
	m.logger.Infow("SIP request authenticated",
		"call_id", ctx.CallID,
		"method", ctx.Method,
		"assistant_id", assistant.Id,
		"org_id", orgID)

	return sip_infra.AllowWithExtra(sipConfig, map[string]interface{}{
		"auth":             auth,
		"assistant":        assistant,
		"sip_config":       sipConfig,
		"vault_credential": vaultCred,
	}), nil
}

func (m *SIPEngine) hasAccessToAssistant(auth types.SimplePrinciple, assistant *internal_assistant_entity.Assistant) bool {
	if auth.GetCurrentProjectId() == nil || assistant.ProjectId == 0 {
		return false
	}
	return *auth.GetCurrentProjectId() == assistant.ProjectId
}

func (m *SIPEngine) onApplicationReady(session *sip_infra.Session, fromURI, toURI string) error {
	stage, err := m.sessionEstablishedStage(session, fromURI, toURI)
	if err != nil {
		return err
	}
	if m.dispatcher == nil {
		return fmt.Errorf("SIP dispatcher not initialized")
	}
	return m.dispatcher.PrepareSession(m.ctx, stage)
}

func (m *SIPEngine) onApplicationCleanup(session *sip_infra.Session) {
	if m.dispatcher == nil || session == nil {
		return
	}
	m.dispatcher.DiscardPreparedSession(m.ctx, session.GetCallID())
}

func (m *SIPEngine) onInvite(session *sip_infra.Session, fromURI, toURI string) error {
	stage, err := m.sessionEstablishedStage(session, fromURI, toURI)
	if err != nil {
		return err
	}
	if m.dispatcher == nil {
		return fmt.Errorf("SIP dispatcher not initialized")
	}
	if stage.Direction == sip_infra.CallDirectionInbound {
		return m.dispatcher.StartPreparedSession(m.ctx, stage)
	}
	m.dispatcher.OnPipeline(m.ctx, stage)
	return nil
}

func (m *SIPEngine) sessionEstablishedStage(session *sip_infra.Session, fromURI, toURI string) (sip_infra.SessionEstablishedPipeline, error) {
	if session == nil {
		return sip_infra.SessionEstablishedPipeline{}, fmt.Errorf("session is nil")
	}
	info := session.GetInfo()
	callID := info.CallID

	if session.IsEnded() {
		return sip_infra.SessionEstablishedPipeline{}, fmt.Errorf("session already ended")
	}

	auth := session.GetAuth()
	if auth == nil {
		return sip_infra.SessionEstablishedPipeline{}, fmt.Errorf("missing auth on session %s", callID)
	}

	assistant := session.GetAssistant()
	if assistant == nil {
		return sip_infra.SessionEstablishedPipeline{}, fmt.Errorf("missing assistant context on session %s", callID)
	}

	return sip_infra.SessionEstablishedPipeline{
		ID:              callID,
		Session:         session,
		Config:          session.GetConfig(),
		VaultCredential: session.GetVaultCredential(),
		Direction:       info.Direction,
		AssistantID:     assistant.Id,
		Auth:            auth,
		FromURI:         fromURI,
		ToURI:           toURI,
		ConversationID:  session.GetConversationID(),
	}, nil
}

func (m *SIPEngine) onBye(session *sip_infra.Session) error {
	disconnectMetadata := session.GetDisconnectMetadata()
	m.persistRemoteByeCallStatus(session, disconnectMetadata)
	m.dispatcher.OnPipeline(m.ctx, sip_infra.ByeReceivedPipeline{
		ID:      session.GetInfo().CallID,
		Session: session,
		Reason:  disconnectMetadata.Reason,
	})
	return nil
}

func (m *SIPEngine) persistRemoteByeCallStatus(session *sip_infra.Session, metadata sip_infra.DisconnectMetadata) {
	if m.callContextStore == nil || session == nil {
		return
	}
	contextID := session.GetContextID()
	if contextID == "" {
		assistant := session.GetAssistant()
		if assistant == nil {
			m.logger.Debugw("SIP BYE status persistence skipped: assistant missing",
				"call_id", session.GetCallID())
			return
		}
		callContext, err := m.callContextStore.GetByChannelUUID(context.Background(), "sip", assistant.Id, session.GetCallID())
		if err != nil {
			m.logger.Debugw("SIP BYE status persistence skipped: call context missing",
				"call_id", session.GetCallID(),
				"assistant_id", assistant.Id,
				"error", err)
			return
		}
		contextID = callContext.ContextID
	}
	if contextID == "" {
		return
	}

	current, err := m.callContextStore.Get(context.Background(), contextID)
	if err == nil && callContextHasTerminalFailure(current) {
		return
	}
	if metadata.Reason == "" {
		metadata.Reason = sip_infra.DisconnectReasonRemoteHangup
	}
	if err := m.callContextStore.UpdateCallStatus(context.Background(), contextID, callcontext.CallStatusUpdate{
		CallStatus:         callcontext.StatusCompleted,
		DisconnectReason:   metadata.Reason,
		ProviderStatusCode: metadata.ProviderStatusCode,
	}); err != nil {
		m.logger.Warnw("SIP BYE status persistence failed",
			"call_id", session.GetCallID(),
			"context_id", contextID,
			"disconnect_reason", metadata.Reason,
			"error", err)
	}
}

func callContextHasTerminalFailure(callContext *callcontext.CallContext) bool {
	if callContext == nil {
		return false
	}
	return callContext.Status == callcontext.StatusFailed ||
		callContext.CallStatus == callcontext.StatusFailed ||
		callContext.CallStatus == "cancelled"
}

func (m *SIPEngine) onCancel(session *sip_infra.Session) error {
	m.dispatcher.OnPipeline(m.ctx, sip_infra.CancelReceivedPipeline{
		ID:      session.GetInfo().CallID,
		Session: session,
	})
	return nil
}

// onError handles SIP-level errors by emitting a CallFailedPipeline event.
// The pipeline handler (signal.go) creates the observer and persists metrics.
func (m *SIPEngine) onError(session *sip_infra.Session, callErr error) {
	m.logger.Warnw("SIP error", "call_id", session.GetCallID(), "error", callErr)
	m.dispatcher.OnPipeline(m.ctx, sip_infra.CallFailedPipeline{
		ID:      session.GetCallID(),
		Session: session,
		Error:   callErr,
	})
}

func (m *SIPEngine) EndCall(callID string) error {
	m.mu.RLock()
	srv := m.server
	m.mu.RUnlock()
	if srv == nil {
		return fmt.Errorf("SIP server not running")
	}
	session, ok := srv.GetSession(callID)
	if !ok {
		return fmt.Errorf("session %s not found", callID)
	}
	return srv.EndCall(session)
}

func (m *SIPEngine) GetActiveCalls() int {
	m.mu.RLock()
	srv := m.server
	m.mu.RUnlock()
	if srv != nil {
		return srv.SessionCount()
	}
	return 0
}

func (m *SIPEngine) Stop() {
	// Release Redis ownership keys BEFORE UnregisterAll — UnregisterAll drains
	// the active-DID set, after which ReleaseAll would have nothing to walk.
	if m.regManager != nil {
		m.regManager.ReleaseAll(context.Background())
	}
	if m.registrationClient != nil {
		m.registrationClient.UnregisterAll(context.Background())
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Lock()
	srv := m.server
	m.server = nil
	m.mu.Unlock()
	if srv != nil {
		srv.Stop()
	}
	m.logger.Infow("SIP Manager stopped")
}

func (m *SIPEngine) Disconnect(ctx context.Context) error {
	m.Stop()
	return nil
}

func (m *SIPEngine) fetchSIPConfigAndVaultCredential(auth types.SimplePrinciple, assistant *internal_assistant_entity.Assistant) (*sip_infra.Config, *protos.VaultCredential, error) {
	if assistant.AssistantPhoneDeployment == nil {
		return nil, nil, fmt.Errorf("assistant has no phone deployment configured")
	}

	opts := assistant.AssistantPhoneDeployment.GetOptions()
	credentialID, err := opts.GetUint64("rapida.credential_id")
	if err != nil {
		return nil, nil, fmt.Errorf("no credential_id in phone deployment: %w", err)
	}

	vaultCred, err := m.vaultClient.GetCredential(m.ctx, auth, credentialID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch vault credential %d: %w", credentialID, err)
	}

	sipConfig, err := sip_infra.ParseConfigFromVault(vaultCred)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse SIP config from vault: %w", err)
	}

	// Set CallerID to the assistant's DID from the phone deployment.
	// This is used as the From URI user in outbound INVITEs.
	if did, err := opts.GetString("phone"); err == nil && did != "" {
		sipConfig.CallerID = strings.TrimPrefix(did, "+")
	}

	m.applySIPConfigDefaults(sipConfig)

	return sipConfig, vaultCred, nil
}

// routingMiddleware resolves the assistant for an inbound INVITE by looking up
// the DID from the To-URI (or From-URI fallback) against phone deployments.
func (m *SIPEngine) routingMiddleware(ctx *sip_infra.SIPRequestContext, next func() (*sip_infra.InviteResult, error)) (*sip_infra.InviteResult, error) {
	did := sip_infra.ExtractDIDFromURI(ctx.ToURI)
	if did == "" {
		did = sip_infra.ExtractDIDFromURI(ctx.FromURI)
	}
	if did == "" {
		return sip_infra.Reject(404, "No DID found in SIP URI"), nil
	}

	assistantID, auth, err := m.resolveAssistantByDID(did)
	if err != nil {
		m.logger.Warnw("SIP: DID lookup failed",
			"call_id", ctx.CallID,
			"did", did,
			"error", err)
		return sip_infra.Reject(404, "No assistant found for this number"), nil
	}

	ctx.AssistantID = strconv.FormatUint(assistantID, 10)
	ctx.Set("auth", auth)

	m.logger.Infow("SIP: Routed by DID",
		"call_id", ctx.CallID,
		"did", did,
		"assistant_id", assistantID)

	return next()
}

// resolveAssistantByDID looks up which assistant owns the given DID (phone number)
// using a single joined query across assistants, phone deployments, and telephony options.
func (m *SIPEngine) resolveAssistantByDID(did string) (uint64, types.SimplePrinciple, error) {
	db := m.postgres.DB(m.ctx)
	type didLookupResult struct {
		AssistantID    uint64
		ProjectID      uint64
		OrganizationID uint64
	}
	var result didLookupResult
	tx := db.Model(&internal_assistant_entity.Assistant{}).
		Select("assistants.id AS assistant_id, assistants.project_id, assistants.organization_id").
		Joins("JOIN assistant_phone_deployments apd ON apd.assistant_id = assistants.id").
		Joins("JOIN assistant_deployment_telephony_options o ON o.assistant_deployment_telephony_id = apd.id").
		Where("apd.telephony_provider = ? AND apd.status = ?", "sip", type_enums.RECORD_ACTIVE).
		Where("o.key = ?", "phone").
		Where("o.value IN ?", []string{did, strings.TrimPrefix(did, "+")}).
		First(&result)
	if tx.Error != nil {
		return 0, nil, fmt.Errorf("no SIP phone deployment found for DID %s: %w", did, tx.Error)
	}

	projectScope := &types.ProjectScope{
		ProjectId:      &result.ProjectID,
		OrganizationId: &result.OrganizationID,
	}
	return result.AssistantID, projectScope, nil
}

// pipelineCreateConversation creates a conversation for inbound calls.
// For outbound calls, the conversation is already created by the channel pipeline.
func (m *SIPEngine) pipelineCreateConversation(ctx context.Context, auth types.SimplePrinciple, assistantID uint64, fromURI string, direction string) (uint64, error) {
	dirEnum := type_enums.DIRECTION_INBOUND

	if direction == "outbound" {
		dirEnum = type_enums.DIRECTION_OUTBOUND
	}

	// Normalize SIP URI to phone number for caller identity
	callerNumber := sip_infra.ExtractDIDFromURI(fromURI)
	if callerNumber == "" {
		callerNumber = fromURI
	}

	assistant, err := m.assistantService.Get(ctx, auth, assistantID, utils.GetVersionDefinition("latest"),
		&internal_services.GetAssistantOption{InjectPhoneDeployment: true})
	if err != nil {
		return 0, fmt.Errorf("failed to load assistant %d: %w", assistantID, err)
	}

	conversation, err := m.assistantConversationService.CreateConversation(
		ctx, auth, callerNumber, assistant.Id, assistant.AssistantProviderId, dirEnum, utils.PhoneCall,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create conversation: %w", err)
	}
	return conversation.Id, nil
}

// pipelineEnsureCallContext resolves the durable CallContext for a SIP session.
// Outbound: load + claim the record persisted by channel/pipeline/outbound.go.
// Inbound: build from the INVITE URIs and persist. Returns an in-memory cc
// when the DB is unreachable so the call can still proceed.
func (m *SIPEngine) pipelineEnsureCallContext(
	ctx context.Context,
	session *sip_infra.Session,
	auth types.SimplePrinciple,
	assistantID uint64,
	conversationID uint64,
	direction sip_infra.CallDirection,
	fromURI string,
	toURI string,
) (*callcontext.CallContext, error) {
	callID := session.GetCallID()
	dirStr := string(direction)

	if direction == sip_infra.CallDirectionOutbound {
		ctxID := session.GetContextID()
		if ctxID == "" {
			return m.reconstructCallContext(auth, assistantID, conversationID, dirStr, callID, "", fromURI, toURI), nil
		}
		if claimed, err := m.callContextStore.Claim(ctx, ctxID); err == nil {
			return claimed, nil
		}
		if loaded, err := m.callContextStore.Get(ctx, ctxID); err == nil {
			return loaded, nil
		}
		return m.reconstructCallContext(auth, assistantID, conversationID, dirStr, callID, ctxID, fromURI, toURI), nil
	}

	// Inbound. ContextID stays empty so store.Save generates a UUID that fits
	// the varchar(36) column; the raw SIP Call-ID lives in ChannelUUID instead.
	cc := &callcontext.CallContext{
		AssistantID:    assistantID,
		ConversationID: conversationID,
		AuthToken:      auth.GetCurrentToken(),
		AuthType:       auth.Type().String(),
		Direction:      dirStr,
		Provider:       "sip",
		CallerNumber:   extractDIDOrRaw(fromURI),
		FromNumber:     extractDIDOrRaw(toURI),
		ChannelUUID:    callID,
	}
	if pid := auth.GetCurrentProjectId(); pid != nil {
		cc.ProjectID = *pid
	}
	if oid := auth.GetCurrentOrganizationId(); oid != nil {
		cc.OrganizationID = *oid
	}
	if assistant := session.GetAssistant(); assistant != nil {
		cc.AssistantProviderId = assistant.AssistantProviderId
	}
	if _, err := m.callContextStore.Save(ctx, cc); err != nil {
		m.logger.Warnw("failed to persist inbound call context — continuing in-memory",
			"call_id", callID, "error", err)
		return cc, nil
	}
	if _, err := m.callContextStore.Claim(ctx, cc.ContextID); err != nil {
		m.logger.Debugw("inbound claim non-fatal", "call_id", callID, "error", err)
	}
	return cc, nil
}

func (m *SIPEngine) reconstructCallContext(
	auth types.SimplePrinciple,
	assistantID uint64,
	conversationID uint64,
	direction string,
	callID string,
	contextID string,
	fromURI string,
	toURI string,
) *callcontext.CallContext {
	cc := &callcontext.CallContext{
		AssistantID:    assistantID,
		ConversationID: conversationID,
		AuthToken:      auth.GetCurrentToken(),
		AuthType:       auth.Type().String(),
		Direction:      direction,
		Provider:       "sip",
		ChannelUUID:    callID,
		ContextID:      contextID,
	}
	if direction == string(sip_infra.CallDirectionOutbound) {
		cc.CallerNumber = extractDIDOrRaw(toURI)
		cc.FromNumber = extractDIDOrRaw(fromURI)
	} else {
		cc.CallerNumber = extractDIDOrRaw(fromURI)
		cc.FromNumber = extractDIDOrRaw(toURI)
	}
	if pid := auth.GetCurrentProjectId(); pid != nil {
		cc.ProjectID = *pid
	}
	if oid := auth.GetCurrentOrganizationId(); oid != nil {
		cc.OrganizationID = *oid
	}
	return cc
}

func extractDIDOrRaw(uri string) string {
	if uri == "" {
		return ""
	}
	if did := sip_infra.ExtractDIDFromURI(uri); did != "" {
		return did
	}
	return uri
}

// pipelineCallSetup builds the CallSetupResult from auth, IDs, and the
// CallContext resolved by pipelineEnsureCallContext.
func (m *SIPEngine) pipelineCallSetup(ctx context.Context, session *sip_infra.Session, auth types.SimplePrinciple, assistantID uint64, conversationID uint64, cc *callcontext.CallContext) (*sip_pipeline.CallSetupResult, error) {
	assistant := session.GetAssistant()
	if assistant == nil {
		var err error
		assistant, err = m.assistantService.Get(ctx, auth, assistantID, utils.GetVersionDefinition("latest"),
			&internal_services.GetAssistantOption{InjectPhoneDeployment: true})
		if err != nil {
			return nil, fmt.Errorf("failed to load assistant %d: %w", assistantID, err)
		}
	}

	result := &sip_pipeline.CallSetupResult{
		AssistantID:         assistant.Id,
		ConversationID:      conversationID,
		AssistantProviderId: assistant.AssistantProviderId,
		AuthToken:           auth.GetCurrentToken(),
		AuthType:            auth.Type().String(),
		CallContext:         cc,
	}
	if auth.GetCurrentProjectId() != nil {
		result.ProjectID = *auth.GetCurrentProjectId()
	}
	if auth.GetCurrentOrganizationId() != nil {
		result.OrganizationID = *auth.GetCurrentOrganizationId()
	}

	return result, nil
}

type sipPreparedCallRuntime struct {
	logger      commons.Logger
	session     *sip_infra.Session
	auth        types.SimplePrinciple
	callContext *callcontext.CallContext
	talkContext context.Context
	cancelTalk  context.CancelFunc
	streamer    internal_type.SIPStreamer
	talker      internal_type.Talking
	direction   string
	talkDone    chan error
}

func (runtime *sipPreparedCallRuntime) Start(_ context.Context) error {
	if runtime.session.IsEnded() {
		runtime.logger.Warnw("Session already ended before runtime start", "call_id", runtime.session.GetCallID())
		return fmt.Errorf("session_ended_before_start")
	}
	runtime.streamer.StartAssistantOutput()
	if runtime.talkDone != nil {
		return <-runtime.talkDone
	}
	return runtime.runTalker()
}

func (runtime *sipPreparedCallRuntime) runTalker() error {
	runtime.logger.Infow("SIP call started",
		"call_id", runtime.session.GetCallID(),
		"assistant_id", runtime.callContext.AssistantID,
		"conversation_id", runtime.callContext.ConversationID,
		"direction", runtime.direction)
	if err := runtime.talker.Talk(runtime.talkContext, runtime.auth); err != nil {
		runtime.logger.Warnw("SIP talker exited", "error", err, "call_id", runtime.session.GetCallID())
	}
	runtime.logger.Infow("SIP call ended", "call_id", runtime.session.GetCallID())
	return nil
}

func (runtime *sipPreparedCallRuntime) Close(_ context.Context) {
	if runtime == nil {
		return
	}
	if runtime.cancelTalk != nil {
		runtime.cancelTalk()
	}
	_ = runtime.streamer.Close()
}

func (m *SIPEngine) prepareInboundCallRuntime(ctx context.Context, stage sip_infra.SessionEstablishedPipeline, setup *sip_pipeline.CallSetupResult, observer observability.Recorder) (sip_pipeline.PreparedCallRuntime, error) {
	runtime, err := m.prepareSIPCallRuntime(ctx, stage.Session, setup, observer, stage.VaultCredential, stage.Config, string(stage.Direction))
	if err != nil {
		return nil, err
	}
	if err := runtime.StartBeforeAnswer(ctx, inboundRuntimeReadyTimeout(stage.Config)); err != nil {
		runtime.Close(ctx)
		return nil, err
	}
	return runtime, nil
}

// pipelineCallStart starts a prepared SIP runtime and blocks until Talk returns.
func (m *SIPEngine) pipelineCallStart(ctx context.Context, session *sip_infra.Session, setup *sip_pipeline.CallSetupResult, observer observability.Recorder, vaultCred interface{}, sipConfig *sip_infra.Config, direction string) error {
	isOutbound := session.GetInfo().Direction == sip_infra.CallDirectionOutbound
	if isOutbound {
		defer func() {
			if !session.IsEnded() {
				state := session.GetState()
				if state == sip_infra.CallStateTransferring || state == sip_infra.CallStateBridgeConnected {
					return
				}
				m.endSession(session, sip_infra.LifecycleReasonPipelineTalkCompleted)
			}
		}()
	}
	runtime, err := m.prepareSIPCallRuntime(ctx, session, setup, observer, vaultCred, sipConfig, direction)
	if err != nil {
		return err
	}
	return runtime.Start(ctx)
}

func (m *SIPEngine) prepareSIPCallRuntime(ctx context.Context, session *sip_infra.Session, setup *sip_pipeline.CallSetupResult, observer observability.Recorder, vaultCred interface{}, sipConfig *sip_infra.Config, direction string) (*sipPreparedCallRuntime, error) {
	callID := session.GetCallID()
	if session.IsEnded() {
		m.logger.Warnw("Session already ended before call runtime preparation", "call_id", callID)
		return nil, fmt.Errorf("session_ended_before_start")
	}
	auth := session.GetAuth()
	resolvedCallContext := m.resolveSIPCallContext(session, setup, direction)
	talkContext, cancelTalk := context.WithCancel(session.Context())

	go func() {
		select {
		case <-session.ByeReceived():
			cancelTalk()
		case <-talkContext.Done():
		}
	}()

	select {
	case <-session.ByeReceived():
		cancelTalk()
		m.logger.Infow("BYE received before call runtime preparation", "call_id", callID)
		return nil, fmt.Errorf("bye_before_start")
	default:
	}

	var vaultCredential *protos.VaultCredential
	if v, ok := vaultCred.(*protos.VaultCredential); ok {
		vaultCredential = v
	} else {
		vaultCredential = session.GetVaultCredential()
	}
	streamer, err := internal_telephony.New(
		internal_telephony.WithSIPContext(talkContext),
		internal_telephony.WithSIPLogger(m.logger),
		internal_telephony.WithSIPSession(session),
		internal_telephony.WithSIPLifecycle(m.lifecycleController()),
		internal_telephony.WithSIPCallContext(resolvedCallContext),
		internal_telephony.WithSIPVaultCredential(vaultCredential),
		internal_telephony.WithSIPObserver(observer),
	)
	if err != nil {
		cancelTalk()
		m.logger.Error("Failed to create SIP streamer", "error", err, "call_id", callID)
		return nil, fmt.Errorf("streamer_failed: %w", err)
	}
	if session.IsEnded() {
		cancelTalk()
		_ = streamer.Close()
		return nil, fmt.Errorf("session_ended_after_streamer")
	}

	m.configureSIPTransfer(session, sipConfig, resolvedCallContext, streamer)
	talker, err := internal_adapter.GetTalker(
		utils.PhoneCall, talkContext, m.cfg, m.logger,
		m.postgres, m.opensearch, m.redis, m.storage, streamer,
	)
	if err != nil {
		cancelTalk()
		_ = streamer.Close()
		m.logger.Error("Failed to create SIP talker", "error", err, "call_id", callID)
		return nil, fmt.Errorf("talker_failed: %w", err)
	}
	return &sipPreparedCallRuntime{
		logger:      m.logger,
		session:     session,
		auth:        auth,
		callContext: resolvedCallContext,
		talkContext: talkContext,
		cancelTalk:  cancelTalk,
		streamer:    streamer,
		talker:      talker,
		direction:   direction,
	}, nil
}

func (runtime *sipPreparedCallRuntime) StartBeforeAnswer(ctx context.Context, timeout time.Duration) error {
	if runtime.talkDone != nil {
		return nil
	}
	runtime.talkDone = make(chan error, 1)
	go func() {
		runtime.talkDone <- runtime.runTalker()
	}()
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-runtime.talkDone:
			runtime.talkDone <- err
			if err != nil {
				return err
			}
			return fmt.Errorf("SIP runtime exited before answer")
		case <-ctx.Done():
			return ctx.Err()
		case <-runtime.session.Context().Done():
			return runtime.session.Context().Err()
		case <-timer.C:
			return fmt.Errorf("SIP runtime readiness timeout")
		case <-ticker.C:
			if runtime.talker.Assistant() != nil && runtime.talker.Conversation() != nil {
				return nil
			}
		}
	}
}

func inboundRuntimeReadyTimeout(config *sip_infra.Config) time.Duration {
	if config != nil && config.InboundMaxRingDuration > 0 {
		return config.InboundMaxRingDuration
	}
	return 30 * time.Second
}

func (m *SIPEngine) resolveSIPCallContext(session *sip_infra.Session, setup *sip_pipeline.CallSetupResult, direction string) *callcontext.CallContext {
	callID := session.GetCallID()
	if setup.CallContext != nil {
		call := setup.CallContext
		if call.AssistantProviderId == 0 {
			call.AssistantProviderId = setup.AssistantProviderId
		}
		if call.ProjectID == 0 {
			call.ProjectID = setup.ProjectID
		}
		if call.OrganizationID == 0 {
			call.OrganizationID = setup.OrganizationID
		}
		return call
	}

	m.logger.Warnw("setup.CallContext missing — reconstructing from session", "call_id", callID)
	info := session.GetInfo()
	clientPhone := sip_infra.ExtractDIDFromURI(info.RemoteURI)
	if clientPhone == "" {
		clientPhone = info.RemoteURI
	}
	return &callcontext.CallContext{
		AssistantID:         setup.AssistantID,
		ConversationID:      setup.ConversationID,
		AssistantProviderId: setup.AssistantProviderId,
		AuthToken:           setup.AuthToken,
		AuthType:            setup.AuthType,
		Direction:           direction,
		Provider:            "sip",
		CallerNumber:        clientPhone,
		FromNumber:          sip_infra.ExtractDIDFromURI(info.LocalURI),
		ChannelUUID:         callID,
		ContextID:           callID,
		ProjectID:           setup.ProjectID,
		OrganizationID:      setup.OrganizationID,
	}
}

func (m *SIPEngine) configureSIPTransfer(session *sip_infra.Session, sipConfig *sip_infra.Config, call *callcontext.CallContext, transferStreamer internal_type.SIPTransferStreamer) {
	callID := session.GetCallID()
	transferStreamer.SetTransferRequestHandler(func(targets []string, postTransferAction string) {
		toolID, _ := session.GetMetadata("tool_id")
		toolIDStr, _ := toolID.(string)
		toolCtxID, _ := session.GetMetadata("tool_context_id")
		toolCtxIDStr, _ := toolCtxID.(string)
		primaryTarget := targets[0]
		conversationID := strconv.FormatUint(call.ConversationID, 10)
		legID := string(session.GetInfo().Direction)
		emitTransferEvent := func(eventType string, data map[string]string) {
			payload := map[string]string{
				"event_type":  eventType,
				"call_id":     callID,
				"leg_id":      legID,
				"transfer_id": toolCtxIDStr,
				"attempt_id":  "",
				"from_state":  "",
				"to_state":    "",
				"reason":      "",
				"sip_method":  "",
				"sip_status":  "",
				"target":      "",
				"duration_ms": "",
				"error":       "",
			}
			for k, v := range data {
				payload[k] = v
			}
			transferStreamer.SendTransferEvent(&protos.ConversationEvent{
				Id:   conversationID,
				Name: observability.ComponentSIP.String(),
				Data: payload,
				Time: timestamppb.Now(),
			})
		}
		m.dispatcher.OnPipeline(m.ctx, sip_infra.TransferInitiatedPipeline{
			ID:                 callID,
			Session:            session,
			TargetURI:          primaryTarget,
			Targets:            targets,
			Config:             sipConfig,
			PostTransferAction: postTransferAction,
			OnAttempt: func(target string, attempt int, total int) {
				emitTransferEvent("transfer_attempt", map[string]string{
					"type":       observability.SIPTransferring.String(),
					"provider":   "sip",
					"target":     target,
					"attempt_id": strconv.Itoa(attempt),
					"reason":     "dial_attempt",
					"total":      strconv.Itoa(total),
				})
			},
			OnConnected: func(outboundRTP *sip_infra.RTPHandler) {
				outputCodecName := ""
				if outboundRTP != nil {
					if codec := outboundRTP.GetCodec(); codec != nil {
						outputCodecName = codec.Name
					}
				}
				transferStreamer.StopTransferRingback()
				transferStreamer.ConnectTransferMedia(outboundRTP, outputCodecName)
				emitTransferEvent("transfer_connected", map[string]string{
					"target":     primaryTarget,
					"from_state": "transferring",
					"to_state":   "bridge_connected",
					"reason":     "target_answered",
				})
			},
			OnFailed: func() {
				transferStreamer.ResumeAssistant()
				emitTransferEvent("transfer_failed", map[string]string{
					"target": primaryTarget,
					"reason": "dial_failed",
					"error":  fmt.Sprintf("transfer to %s failed", primaryTarget),
				})
				if toolIDStr != "" {
					transferStreamer.SendTransferToolResult(toolCtxIDStr, toolIDStr, "transfer_call", protos.ToolCallAction_TOOL_CALL_ACTION_TRANSFER_CONVERSATION, map[string]string{
						"status":      "failed",
						"reason":      fmt.Sprintf("Transfer to %s failed", primaryTarget),
						"next_action": postTransferAction,
					})
				}
			},
			OnTeardown: func() {
				transferStreamer.DisconnectTransferMedia()
				durationMs := ""
				if d, ok := session.GetMetadata(sip_infra.MetadataBridgeTransferDuration); ok {
					if duration, ok := d.(string); ok && duration != "" {
						if parsed, err := time.ParseDuration(duration); err == nil {
							durationMs = strconv.FormatInt(parsed.Milliseconds(), 10)
						}
					}
				}
				emitTransferEvent("transfer_teardown", map[string]string{
					"target":      primaryTarget,
					"from_state":  "bridge_connected",
					"to_state":    "connected",
					"reason":      "bridge_end",
					"duration_ms": durationMs,
				})
				if toolIDStr != "" {
					status, _ := session.GetMetadata(sip_infra.MetadataBridgeTransferStatus)
					statusStr, _ := status.(string)
					transferStreamer.SendTransferToolResult(toolCtxIDStr, toolIDStr, "transfer_call", protos.ToolCallAction_TOOL_CALL_ACTION_TRANSFER_CONVERSATION, map[string]string{
						"status":      statusStr,
						"reason":      fmt.Sprintf("Transfer to %s %s", primaryTarget, statusStr),
						"next_action": postTransferAction,
					})
				}
			},
			OnResumeAI: func() {
				transferStreamer.ResumeAssistant()
				emitTransferEvent("transfer_resume_ai", map[string]string{
					"target":     primaryTarget,
					"from_state": "bridge_connected",
					"to_state":   "connected",
					"reason":     "handoff_to_ai",
				})
			},
			OnOperatorAudio: func(audio []byte) { transferStreamer.RecordTransferOperatorAudio(audio) },
		})
	})
}

func (m *SIPEngine) pipelineCallEnd(callID string) {
	m.mu.RLock()
	srv := m.server
	m.mu.RUnlock()
	if srv == nil {
		return
	}
	session, ok := srv.GetSession(callID)
	if !ok {
		return
	}
	m.endSession(session, sip_infra.LifecycleReasonPipelineCallEnd)
}

func (m *SIPEngine) lifecycleController() sip_infra.LifecycleController {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.server
}

func (m *SIPEngine) endSession(session *sip_infra.Session, reason sip_infra.LifecycleReason) {
	server := m.lifecycleController()
	if server == nil {
		m.logger.Warnw("SIP lifecycle end skipped: server unavailable",
			"call_id", session.GetCallID(),
			"reason", reason)
		return
	}
	if err := server.EndCallWithReason(session, reason); err != nil {
		m.logger.Warnw("SIP lifecycle end failed",
			"call_id", session.GetCallID(),
			"reason", reason,
			"error", err)
	}
}

func (m *SIPEngine) createObserver(ctx context.Context, setup *sip_pipeline.CallSetupResult, auth types.SimplePrinciple) observability.Recorder {
	otelCollectors := make([]observability.Collector, 0)
	otelCollectors = append(otelCollectors, observability_collector_conversationdb.New(observability_collector_conversationdb.Config{
		Logger:              m.logger,
		ConversationService: m.assistantConversationService,
	}))
	otelCollectors = append(otelCollectors, collectors.NewWithEnv(ctx, m.logger, m.cfg)...)
	return observability.New(
		observability.WithLogger(m.logger),
		observability.WithAuth(auth),
		observability.WithGlobalScope(observability.GlobalScope{
			ProjectID:      setup.ProjectID,
			OrganizationID: setup.OrganizationID,
		}),
		observability.WithContext(ctx),
		observability.WithCollectors(otelCollectors...),
	)
}
