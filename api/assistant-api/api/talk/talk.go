// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package assistant_talk_api

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rapidaai/api/assistant-api/config"
	internal_adapter "github.com/rapidaai/api/assistant-api/internal/adapters"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	internal_grpc "github.com/rapidaai/api/assistant-api/internal/channel/grpc"
	channel_pipeline "github.com/rapidaai/api/assistant-api/internal/channel/pipeline"
	channel_telephony "github.com/rapidaai/api/assistant-api/internal/channel/telephony"
	internal_webrtc "github.com/rapidaai/api/assistant-api/internal/channel/webrtc"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/api/assistant-api/internal/observability/collectors"
	observe "github.com/rapidaai/api/assistant-api/internal/observe"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	internal_assistant_service "github.com/rapidaai/api/assistant-api/internal/services/assistant"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	web_client "github.com/rapidaai/pkg/clients/web"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	"github.com/rapidaai/pkg/storages"
	storage_files "github.com/rapidaai/pkg/storages/file-storage"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
	assistant_api "github.com/rapidaai/protos"
)

type ConversationApi struct {
	cfg        *config.AssistantConfig
	logger     commons.Logger
	postgres   connectors.PostgresConnector
	redis      connectors.RedisConnector
	opensearch connectors.OpenSearchConnector
	storage    storages.Storage

	callContextStore             callcontext.Store
	outboundDispatcher           *channel_telephony.OutboundDispatcher
	inboundDispatcher            *channel_telephony.InboundDispatcher
	channelPipeline              *channel_pipeline.Dispatcher
	conversationObserver         *conversationObserver
	assistantConversationService internal_services.AssistantConversationService
	assistantService             internal_services.AssistantService
	vaultClient                  web_client.VaultClient
	authClient                   web_client.AuthClient
}

type ConversationGrpcApi struct {
	ConversationApi
}

// newConversationApiCore builds the shared ConversationApi. All three public
// constructors delegate to this so that deps are created exactly once.
func newConversationApiCore(cfg *config.AssistantConfig, logger commons.Logger,
	postgres connectors.PostgresConnector,
	redis connectors.RedisConnector,
	opensearch connectors.OpenSearchConnector,
	sipServer *sip_infra.Server,
) *ConversationApi {
	store := callcontext.NewStore(postgres, logger)
	vaultClient := web_client.NewVaultClientGRPC(&cfg.AppConfig, logger, redis)
	assistantService := internal_assistant_service.NewAssistantService(cfg, logger, postgres, opensearch)
	fileStorage := storage_files.NewStorage(cfg.AssetStoreConfig, logger)
	conversationService := internal_assistant_service.NewAssistantConversationService(logger, postgres, fileStorage)

	telephonyDeps := channel_telephony.TelephonyDispatcherDeps{
		Cfg:                 cfg,
		Logger:              logger,
		Store:               store,
		VaultClient:         vaultClient,
		AssistantService:    assistantService,
		ConversationService: conversationService,
		TelephonyOpt:        channel_telephony.TelephonyOption{SIPServer: sipServer},
	}

	inbound := channel_telephony.NewInboundDispatcher(telephonyDeps)
	outboundDisp := channel_telephony.NewOutboundDispatcher(telephonyDeps)
	conversationObserver := NewConversationObserver(cfg, logger, opensearch)
	cApi := &ConversationApi{
		cfg:                          cfg,
		logger:                       logger,
		postgres:                     postgres,
		redis:                        redis,
		opensearch:                   opensearch,
		callContextStore:             store,
		outboundDispatcher:           outboundDisp,
		inboundDispatcher:            inbound,
		conversationObserver:         conversationObserver,
		assistantConversationService: conversationService,
		assistantService:             assistantService,
		storage:                      fileStorage,
		vaultClient:                  vaultClient,
		authClient:                   web_client.NewAuthenticator(&cfg.AppConfig, logger, redis),
		channelPipeline: channel_pipeline.NewDispatcher(&channel_pipeline.DispatcherConfig{
			Logger: logger,
			OnReceiveCall: func(ctx context.Context, provider string, ginCtx *gin.Context) (*internal_type.CallInfo, error) {
				return inbound.ReceiveCall(ginCtx, provider)
			},
			OnLoadAssistant: func(ctx context.Context, auth types.SimplePrinciple, assistantID uint64) (*internal_assistant_entity.Assistant, error) {
				return inbound.LoadAssistant(ctx, auth, assistantID)
			},
			OnCreateConversation: func(ctx context.Context, auth types.SimplePrinciple, callerNumber string, assistantID, assistantProviderID uint64, direction string) (uint64, error) {
				return inbound.CreateConversation(ctx, auth, callerNumber, assistantID, assistantProviderID, direction)
			},
			OnSaveCallContext: func(ctx context.Context, auth types.SimplePrinciple, assistant *internal_assistant_entity.Assistant, conversationID uint64, callInfo *internal_type.CallInfo, provider string) (string, error) {
				return inbound.SaveCallContext(ctx, auth, assistant, conversationID, callInfo, provider)
			},
			OnAnswerProvider: func(ctx context.Context, ginCtx *gin.Context, auth types.SimplePrinciple, provider string, assistantID uint64, callerNumber string, conversationID uint64) error {
				return inbound.AnswerProvider(ginCtx, auth, provider, assistantID, callerNumber, conversationID)
			},
			OnDispatchOutbound: func(ctx context.Context, contextID string) error {
				return outboundDisp.Dispatch(ctx, contextID)
			},
			OnApplyConversationExtras: func(ctx context.Context, auth types.SimplePrinciple, assistantID, conversationID uint64, opts, args, metadata map[string]interface{}) error {
				if len(opts) > 0 {
					if _, err := conversationService.ApplyConversationOption(ctx, auth, assistantID, conversationID, opts); err != nil {
						return err
					}
				}
				if len(args) > 0 {
					if _, err := conversationService.ApplyConversationArgument(ctx, auth, assistantID, conversationID, args); err != nil {
						return err
					}
				}
				if len(metadata) > 0 {
					md := make([]*types.Metadata, 0, len(metadata))
					for k, v := range metadata {
						md = append(md, types.NewMetadata(k, fmt.Sprintf("%v", v)))
					}
					if _, err := conversationService.CreateOrUpdateConversationMetadata(ctx, auth, assistantID, conversationID, md); err != nil {
						return err
					}
				}
				return nil
			},
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
			OnCreateObserver: func(ctx context.Context, callID string, auth types.SimplePrinciple, assistantID, conversationID uint64) *observe.ConversationObserver {
				return conversationObserver.ConversationObserver(auth, assistantID, conversationID)
			},
			OnCompleteSession: func(ctx context.Context, contextID string) {
				// No state transition needed — CLAIMED is the terminal state.
				logger.Debugf("session completed: contextId=%s", contextID)
			},
		}),
	}
	cApi.channelPipeline.Start(context.Background())
	return cApi
}

func NewConversationGRPCApi(config *config.AssistantConfig, logger commons.Logger,
	postgres connectors.PostgresConnector,
	redis connectors.RedisConnector,
	opensearch connectors.OpenSearchConnector,
	vectordb connectors.VectorConnector,
	sipServer *sip_infra.Server,
) assistant_api.TalkServiceServer {
	return &ConversationGrpcApi{*newConversationApiCore(config, logger, postgres, redis, opensearch, sipServer)}
}

func NewWebRtcApi(config *config.AssistantConfig, logger commons.Logger,
	postgres connectors.PostgresConnector,
	redis connectors.RedisConnector,
	opensearch connectors.OpenSearchConnector,
	vectordb connectors.VectorConnector,
	sipServer *sip_infra.Server,
) assistant_api.WebRTCServer {
	return &ConversationGrpcApi{*newConversationApiCore(config, logger, postgres, redis, opensearch, sipServer)}
}

// Pipeline returns the channel pipeline dispatcher for use by external engines (e.g. AudioSocket).
func (cApi *ConversationApi) Pipeline() *channel_pipeline.Dispatcher {
	return cApi.channelPipeline
}

func NewConversationApi(config *config.AssistantConfig, logger commons.Logger,
	postgres connectors.PostgresConnector,
	redis connectors.RedisConnector,
	opensearch connectors.OpenSearchConnector,
	vectordb connectors.VectorConnector,
	sipServer *sip_infra.Server,
) *ConversationApi {
	return newConversationApiCore(config, logger, postgres, redis, opensearch, sipServer)
}

// AssistantTalk handles incoming assistant talk requests.
// It establishes a connection with the client and processes the incoming requests.
//
// Parameters:
// - stream: A server stream for handling bidirectional communication with the client.
//
// Returns:
// - An error if any error occurs during the processing of the request.
func (cApi *ConversationGrpcApi) AssistantTalk(stream assistant_api.TalkService_AssistantTalkServer) error {
	auth, isAuthenticated := types.GetSimplePrincipleGRPC(stream.Context())
	if !isAuthenticated {
		cApi.logger.Errorf("unable to resolve the authentication object, please check the parameter for authentication")
		return errors.New("unauthenticated request for messaging")
	}

	source, ok := utils.GetClientSource(stream.Context())
	if !ok {
		cApi.logger.Errorf("unable to resolve the source from the context")
		return errors.New("illegal source")
	}
	streamer, err := internal_grpc.NewGrpcStreamer(stream.Context(), cApi.logger, stream)
	if err != nil {
		cApi.logger.Errorf("failed to create grpc streamer: %v", err)
		return err
	}
	talker, err := internal_adapter.GetTalker(
		source,
		stream.Context(),
		cApi.cfg,
		cApi.logger,
		cApi.postgres,
		cApi.opensearch,
		cApi.redis,
		cApi.storage,
		streamer,
	)
	if err != nil {
		cApi.logger.Errorf("failed to setup talker: %v", err)
		return err
	}

	return talker.Talk(stream.Context(), auth)
}

func (cApi *ConversationGrpcApi) WebTalk(stream assistant_api.WebRTC_WebTalkServer) error {
	auth, isAuthenticated := types.GetSimplePrincipleGRPC(stream.Context())
	if !isAuthenticated {
		cApi.logger.Errorf("unable to resolve the authentication object, please check the parameter for authentication")
		return errors.New("unauthenticated request for messaging")
	}

	source, ok := utils.GetClientSource(stream.Context())
	if !ok {
		cApi.logger.Errorf("unable to resolve the source from the context")
		return errors.New("illegal source")
	}
	observabilityRecorder := observability.New(observability.WithLogger(cApi.logger),
		observability.WithAuth(auth),
		observability.WithCollectors(collectors.NewWithEnv(stream.Context(), cApi.logger, cApi.cfg)...))
	defer func() {
		if err := observabilityRecorder.Close(context.Background()); err != nil {
			cApi.logger.Errorf("failed to close webrtc observability recorder: %v", err)
		}
	}()

	streamer, err := internal_webrtc.New(internal_webrtc.Config{
		Context:      stream.Context(),
		Logger:       cApi.logger,
		GRPCStream:   stream,
		ServerConfig: cApi.cfg.WebRTCConfig,
		Observer:     observabilityRecorder,
	})
	if err != nil {
		cApi.logger.Errorf("failed to create grpc streamer: %v", err)
		return err
	}
	talker, err := internal_adapter.GetTalker(
		source,
		stream.Context(),
		cApi.cfg,
		cApi.logger,
		cApi.postgres,
		cApi.opensearch,
		cApi.redis,
		cApi.storage,
		streamer,
	)
	if err != nil {
		cApi.logger.Errorf("failed to setup talker: %v", err)
		return err
	}

	return talker.Talk(stream.Context(), auth)
}
