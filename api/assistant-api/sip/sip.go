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

	"github.com/rapidaai/api/assistant-api/config"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	internal_assistant_service "github.com/rapidaai/api/assistant-api/internal/services/assistant"
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
	assistantToolService         internal_services.AssistantToolService
	assistantService             internal_services.AssistantService
	deploymentService            internal_services.AssistantDeploymentService
	webhookService               internal_services.AssistantWebhookService
	httpLogService               internal_services.AssistantHTTPLogService
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
	fileStorage := storage_files.NewStorage(config.AssetStoreConfig, logger)
	return &SIPEngine{
		cfg:                          config,
		logger:                       logger,
		postgres:                     postgres,
		redis:                        redis,
		opensearch:                   opensearch,
		assistantConversationService: internal_assistant_service.NewAssistantConversationService(logger, postgres, fileStorage),
		assistantToolService:         internal_assistant_service.NewAssistantToolService(logger, postgres, fileStorage),
		assistantService:             internal_assistant_service.NewAssistantService(config, logger, postgres, opensearch),
		deploymentService:            internal_assistant_service.NewAssistantDeploymentService(config, logger, postgres),
		webhookService:               internal_assistant_service.NewAssistantWebhookService(logger, postgres, fileStorage),
		httpLogService:               internal_assistant_service.NewAssistantHTTPLogService(logger, postgres, fileStorage),
		storage:                      fileStorage,
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

	m.dispatcher = sip_pipeline.New(
		sip_pipeline.WithLogger(m.logger),
		sip_pipeline.WithServer(server),
		sip_pipeline.WithAssistantConfig(m.cfg),
		sip_pipeline.WithAssistantService(m.assistantService),
		sip_pipeline.WithAssistantConversationService(m.assistantConversationService),
		sip_pipeline.WithAssistantToolService(m.assistantToolService),
		sip_pipeline.WithWebhookService(m.webhookService),
		sip_pipeline.WithHTTPLogService(m.httpLogService),
		sip_pipeline.WithCallContextStore(m.callContextStore),
		sip_pipeline.WithPostgres(m.postgres),
		sip_pipeline.WithOpenSearch(m.opensearch),
		sip_pipeline.WithRedis(m.redis),
		sip_pipeline.WithStorage(m.storage),
	)
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
		CallStatus:         callcontext.CallStatusCompleted,
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
