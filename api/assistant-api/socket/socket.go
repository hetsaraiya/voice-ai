// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package assistant_socket

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/rapidaai/api/assistant-api/config"
	internal_adapter "github.com/rapidaai/api/assistant-api/internal/adapters"
	internal_callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	channel_pipeline "github.com/rapidaai/api/assistant-api/internal/channel/pipeline"
	channel_telephony "github.com/rapidaai/api/assistant-api/internal/channel/telephony"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/api/assistant-api/internal/observability/collectors"
	observability_collector_conversationmetadata "github.com/rapidaai/api/assistant-api/internal/observability/collectors/conversationmetadata"
	observability_collector_conversationmetric "github.com/rapidaai/api/assistant-api/internal/observability/collectors/conversationmetric"
	observability_collector_requestlog "github.com/rapidaai/api/assistant-api/internal/observability/collectors/requestlog"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	internal_assistant_service "github.com/rapidaai/api/assistant-api/internal/services/assistant"
	web_client "github.com/rapidaai/pkg/clients/web"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	"github.com/rapidaai/pkg/storages"
	storage_files "github.com/rapidaai/pkg/storages/file-storage"
	"github.com/rapidaai/pkg/utils"
)

type audioSocketEngine struct {
	logger     commons.Logger
	cfg        *config.AssistantConfig
	postgres   connectors.PostgresConnector
	redis      connectors.RedisConnector
	opensearch connectors.OpenSearchConnector
	storage    storages.Storage
	listener   net.Listener
	pipeline   *channel_pipeline.Dispatcher
	inbound    *channel_telephony.InboundDispatcher
	mu         sync.RWMutex

	vaultClient                  web_client.VaultClient
	callcontext                  internal_callcontext.Store
	assistantConversationService internal_services.AssistantConversationService
	webhookService               internal_services.AssistantWebhookService
	httpLogService               internal_services.AssistantHTTPLogService
}

func NewAudioSocketEngine(cfg *config.AssistantConfig, logger commons.Logger,
	postgres connectors.PostgresConnector,
	redis connectors.RedisConnector,
	opensearch connectors.OpenSearchConnector,
) *audioSocketEngine {
	fileStorage := storage_files.NewStorage(cfg.AssetStoreConfig, logger)
	return &audioSocketEngine{
		cfg:                          cfg,
		logger:                       logger,
		postgres:                     postgres,
		redis:                        redis,
		opensearch:                   opensearch,
		storage:                      fileStorage,
		callcontext:                  internal_callcontext.NewStore(postgres, logger),
		vaultClient:                  web_client.NewVaultClientGRPC(&cfg.AppConfig, logger, redis),
		assistantConversationService: internal_assistant_service.NewAssistantConversationService(logger, postgres, fileStorage),
		webhookService:               internal_assistant_service.NewAssistantWebhookService(logger, postgres, fileStorage),
		httpLogService:               internal_assistant_service.NewAssistantHTTPLogService(logger, postgres, fileStorage),
	}
}

func (m *audioSocketEngine) Connect(ctx context.Context) error {
	m.inbound = channel_telephony.NewInboundDispatcher(
		channel_telephony.WithConfig(m.cfg),
		channel_telephony.WithLogger(m.logger),
		channel_telephony.WithStore(m.callcontext),
		channel_telephony.WithVaultClient(m.vaultClient),
		channel_telephony.WithAssistantService(internal_assistant_service.NewAssistantService(m.cfg, m.logger, m.postgres, m.opensearch)),
		channel_telephony.WithConversationService(m.assistantConversationService),
		channel_telephony.WithTelephonyOption(channel_telephony.TelephonyOption{}),
	)

	m.pipeline = channel_pipeline.NewDispatcher(
		channel_pipeline.WithLogger(m.logger),
		channel_pipeline.WithConversationService(m.assistantConversationService),
		channel_pipeline.WithAssistantService(internal_assistant_service.NewAssistantService(m.cfg, m.logger, m.postgres, m.opensearch)),
		channel_pipeline.WithWebhookService(m.webhookService),
		channel_pipeline.WithHTTPLogService(m.httpLogService),
	)

	addr := fmt.Sprintf("%s:%d", m.cfg.AudioSocketConfig.Host, m.cfg.AudioSocketConfig.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("audiosocket listen failed: %w", err)
	}
	m.listener = listener
	m.logger.Infow("AudioSocket server started", "addr", addr)
	go m.acceptLoop(ctx)
	return nil
}

func (m *audioSocketEngine) Disconnect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listener == nil {
		return nil
	}
	_ = m.listener.Close()
	m.listener = nil
	return nil
}

func (m *audioSocketEngine) acceptLoop(ctx context.Context) {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			m.logger.Warnw("AudioSocket accept error", "error", err)
			continue
		}
		go m.handleConnection(ctx, conn)
	}
}

func (m *audioSocketEngine) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	contextID, err := m.readContextID(reader)
	if err != nil {
		if errors.Is(err, io.EOF) {
			m.logger.Debugw("AudioSocket connection closed before UUID frame", "remote", conn.RemoteAddr())
			return
		}
		m.logger.Warnw("AudioSocket failed to read UUID frame", "error", err)
		return
	}

	m.logger.Infof("AudioSocket connection contextId=%s", contextID)

	callContext, vaultCredential, err := m.inbound.ResolveCallSessionByContext(ctx, contextID)
	if err != nil {
		m.logger.Warnw("AudioSocket failed to resolve call context", "contextId", contextID, "error", err)
		return
	}
	observabilityCollectors := make([]observability.Collector, 0)
	observabilityCollectors = append(observabilityCollectors,
		observability_collector_conversationmetric.New(observability_collector_conversationmetric.Config{
			Logger:              m.logger,
			ConversationService: m.assistantConversationService,
		}),
		observability_collector_conversationmetadata.New(observability_collector_conversationmetadata.Config{
			Logger:              m.logger,
			ConversationService: m.assistantConversationService,
		}),
	)
	observabilityCollectors = append(observabilityCollectors, collectors.NewWithEnv(ctx, m.logger, m.cfg)...)
	observer := observability.New(
		observability.WithLogger(m.logger),
		observability.WithAuth(callContext.ToAuth()),
		observability.WithContext(ctx),
		observability.WithCollectors(observabilityCollectors...),
		observability.WithGracePeriod(),
	)
	defer observer.Close(context.Background())
	assistantScopedCollectors := make([]observability.Collector, 0)
	assistantScopedCollectors = append(assistantScopedCollectors,
		observability_collector_requestlog.New(observability_collector_requestlog.Config{
			Logger:         m.logger,
			HTTPLogService: m.httpLogService,
		}),
	)
	assistantScopedCollectors = append(assistantScopedCollectors, collectors.NewWithAssistantWebhook(ctx, m.logger, callContext.ToAuth(), callContext.AssistantID, m.webhookService, observer)...)
	if err := observer.AddCollectors(assistantScopedCollectors...); err != nil {
		m.logger.Warnw("observability collector registration failed",
			"component", "call",
			"operation", "add_assistant_collectors",
			"assistant_id", callContext.AssistantID,
			"context_id", contextID,
			"error", err,
		)
	}

	streamer, err := channel_telephony.Telephony(callContext.Provider).NewStreamer(
		m.logger,
		callContext,
		vaultCredential,
		channel_telephony.WithAudioSocketStreamer(conn, reader, writer),
		channel_telephony.WithObserver(observer),
	)
	if err != nil {
		m.logger.Warnw("AudioSocket failed to create streamer", "contextId", contextID, "error", err)
		return
	}
	talker, err := internal_adapter.New(
		internal_adapter.WithSource(utils.PhoneCall),
		internal_adapter.WithContext(ctx),
		internal_adapter.WithConfig(m.cfg),
		internal_adapter.WithLogger(m.logger),
		internal_adapter.WithPostgres(m.postgres),
		internal_adapter.WithOpenSearch(m.opensearch),
		internal_adapter.WithRedis(m.redis),
		internal_adapter.WithStorage(m.storage),
		internal_adapter.WithStreamer(streamer),
		internal_adapter.WithObserver(observer),
	)
	if err != nil {
		m.logger.Warnw("AudioSocket failed to create talker", "contextId", contextID, "error", err)
		return
	}

	result := m.pipeline.Run(ctx, channel_pipeline.SessionConnectedPipeline{
		ID:          contextID,
		ContextID:   contextID,
		CallContext: callContext,
		Talker:      talker,
		Observer:    observer,
	})

	if result.Error != nil {
		m.logger.Warnw("AudioSocket call failed", "contextId", contextID, "error", result.Error)
	}
}

func (m *audioSocketEngine) readContextID(reader *bufio.Reader) (string, error) {
	const frameTypeUUID byte = 0x01

	frameType, err := reader.ReadByte()
	if err != nil {
		return "", fmt.Errorf("failed to read frame type: %w", err)
	}

	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(reader, lenBuf); err != nil {
		return "", fmt.Errorf("failed to read frame length: %w", err)
	}
	payloadLen := int(binary.BigEndian.Uint16(lenBuf))

	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(reader, payload); err != nil {
			return "", fmt.Errorf("failed to read frame payload: %w", err)
		}
	}

	if frameType != frameTypeUUID {
		return "", fmt.Errorf("expected UUID frame (0x01), got frame type 0x%02x", frameType)
	}

	if len(payload) != 16 {
		return "", fmt.Errorf("invalid UUID payload length: %d (expected 16)", len(payload))
	}

	h := hex.EncodeToString(payload)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32], nil
}
