// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package assistant_socket

import (
	"bufio"
	"context"
	"net"

	"github.com/gorilla/websocket"
	"github.com/rapidaai/api/assistant-api/config"
	internal_adapter "github.com/rapidaai/api/assistant-api/internal/adapters"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	channel_pipeline "github.com/rapidaai/api/assistant-api/internal/channel/pipeline"
	channel_telephony "github.com/rapidaai/api/assistant-api/internal/channel/telephony"
	observe "github.com/rapidaai/api/assistant-api/internal/observe"
	observe_exporters "github.com/rapidaai/api/assistant-api/internal/observe/exporters"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	internal_assistant_service "github.com/rapidaai/api/assistant-api/internal/services/assistant"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	web_client "github.com/rapidaai/pkg/clients/web"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	storage_files "github.com/rapidaai/pkg/storages/file-storage"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

// newSessionPipeline creates a pipeline dispatcher wired for session handling
// (resolve context, create streamer, create talker, run Talk, observe, complete).
func newSessionPipeline(ctx context.Context, cfg *config.AssistantConfig, logger commons.Logger,
	postgres connectors.PostgresConnector,
	redis connectors.RedisConnector,
	opensearch connectors.OpenSearchConnector,
) *channel_pipeline.Dispatcher {
	store := callcontext.NewStore(postgres, logger)
	vaultClient := web_client.NewVaultClientGRPC(&cfg.AppConfig, logger, redis)
	fileStorage := storage_files.NewStorage(cfg.AssetStoreConfig, logger)
	conversationService := internal_assistant_service.NewAssistantConversationService(logger, postgres, fileStorage)

	inbound := channel_telephony.NewInboundDispatcher(channel_telephony.TelephonyDispatcherDeps{
		Cfg:                 cfg,
		Logger:              logger,
		Store:               store,
		VaultClient:         vaultClient,
		AssistantService:    internal_assistant_service.NewAssistantService(cfg, logger, postgres, opensearch),
		ConversationService: conversationService,
		TelephonyOpt:        channel_telephony.TelephonyOption{},
	})

	dispatcher := channel_pipeline.NewDispatcher(&channel_pipeline.DispatcherConfig{
		Logger: logger,
		OnResolveSession: func(ctx context.Context, contextID string) (*callcontext.CallContext, *protos.VaultCredential, error) {
			return inbound.ResolveCallSessionByContext(ctx, contextID)
		},
		OnCreateStreamer: func(ctx context.Context, cc *callcontext.CallContext, vc *protos.VaultCredential, ws *websocket.Conn, conn net.Conn, reader *bufio.Reader, writer *bufio.Writer) (internal_type.Streamer, error) {
			return channel_telephony.Telephony(cc.Provider).NewStreamer(
				logger,
				cc,
				vc,
				channel_telephony.WithWebSocketStreamer(ws),
				channel_telephony.WithAudioSocketStreamer(conn, reader, writer),
			)
		},
		OnCreateTalker: func(ctx context.Context, streamer internal_type.Streamer) (internal_type.Talking, error) {
			return internal_adapter.GetTalker(utils.PhoneCall, ctx, cfg, logger, postgres, opensearch, redis, fileStorage, streamer)
		},
		OnRunTalk: func(ctx context.Context, talker internal_type.Talking, auth types.SimplePrinciple) error {
			return talker.Talk(ctx, auth)
		},
		OnCreateObserver: newObserverFactory(cfg, logger, opensearch, conversationService),
		OnCompleteSession: func(ctx context.Context, contextID string) {
			logger.Debugf("session completed: contextId=%s", contextID)
		},
	})

	dispatcher.Start(ctx)
	return dispatcher
}

func newObserverFactory(cfg *config.AssistantConfig, logger commons.Logger, opensearch connectors.OpenSearchConnector, conversationService internal_services.AssistantConversationService) channel_pipeline.OnCreateObserverFunc {
	return func(ctx context.Context, callID string, auth types.SimplePrinciple, assistantID, conversationID uint64) *observe.ConversationObserver {
		var projectID, orgID uint64
		if pid := auth.GetCurrentProjectId(); pid != nil {
			projectID = *pid
		}
		if oid := auth.GetCurrentOrganizationId(); oid != nil {
			orgID = *oid
		}
		meta := observe.SessionMeta{
			AssistantID:             assistantID,
			AssistantConversationID: conversationID,
			ProjectID:               projectID,
			OrganizationID:          orgID,
		}
		var eventExporters []observe.EventExporter
		var metricExporters []observe.MetricExporter
		if cfg.TelemetryConfig != nil {
			if envType := cfg.TelemetryConfig.Type(); envType != "" {
				evtExp, metExp, err := observe_exporters.GetExporter(
					ctx, logger, &cfg.AppConfig, opensearch, string(envType), cfg.TelemetryConfig.ToMap(),
				)
				if err != nil {
					logger.Warnf("pipeline observer: default exporter creation failed: %v", err)
				} else if evtExp != nil && metExp != nil {
					eventExporters = append(eventExporters, evtExp)
					metricExporters = append(metricExporters, metExp)
				}
			}
		}
		return observe.NewConversationObserver(&observe.ConversationObserverConfig{
			Logger:         logger,
			Auth:           auth,
			AssistantID:    assistantID,
			ConversationID: conversationID,
			ProjectID:      projectID,
			OrganizationID: orgID,
			Persist:        conversationService,
			Events:         observe.NewEventCollector(logger, meta, eventExporters...),
			Metrics:        observe.NewMetricCollector(logger, meta, metricExporters...),
		})
	}
}
