// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package adapter_internal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rapidaai/api/assistant-api/config"
	adapter_channel "github.com/rapidaai/api/assistant-api/internal/adapters/channel"
	adapter_lifecycle "github.com/rapidaai/api/assistant-api/internal/adapters/lifecycle"
	"github.com/rapidaai/protos"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"

	internal_agent_embeddings "github.com/rapidaai/api/assistant-api/internal/agent/embedding"
	internal_agent_rerankers "github.com/rapidaai/api/assistant-api/internal/agent/reranker"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_conversation_entity "github.com/rapidaai/api/assistant-api/internal/entity/conversations"
	internal_knowledge_gorm "github.com/rapidaai/api/assistant-api/internal/entity/knowledges"
	internal_llm "github.com/rapidaai/api/assistant-api/internal/llm"
	internal_input_normalizer "github.com/rapidaai/api/assistant-api/internal/normalizer/input"
	internal_output_normalizer "github.com/rapidaai/api/assistant-api/internal/normalizer/output"
	observe "github.com/rapidaai/api/assistant-api/internal/observe"
	observe_exporters "github.com/rapidaai/api/assistant-api/internal/observe/exporters"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	internal_assistant_service "github.com/rapidaai/api/assistant-api/internal/services/assistant"
	internal_knowledge_service "github.com/rapidaai/api/assistant-api/internal/services/knowledge"
	endpoint_client "github.com/rapidaai/pkg/clients/endpoint"
	integration_client "github.com/rapidaai/pkg/clients/integration"
	web_client "github.com/rapidaai/pkg/clients/web"

	//
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	"github.com/rapidaai/pkg/storages"
	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
)

// =============================================================================
// InteractionState — conversation turn state machine
// =============================================================================

const (
	Unknown               = adapter_lifecycle.Unknown
	Interrupt             = adapter_lifecycle.Interrupt
	Interrupted           = adapter_lifecycle.Interrupted
	LLMGenerating         = adapter_lifecycle.LLMGenerating
	LLMGenerated          = adapter_lifecycle.LLMGenerated
	dbWriteTimeout        = 5 * time.Second
	collectorWriteTimeout = 10 * time.Second
	connectDeadline       = 30 * time.Second
	disconnectDeadline    = 30 * time.Second
)

var (
	errDeploymentNotEnabled = errors.New("deployment is not enabled for source")
)

type genericRequestor struct {
	logger   commons.Logger
	config   *config.AssistantConfig
	source   utils.RapidaSource
	auth     types.SimplePrinciple
	streamer internal_type.Streamer

	// service
	assistantService     internal_services.AssistantService
	conversationService  internal_services.AssistantConversationService
	webhookService       internal_services.AssistantWebhookService
	knowledgeService     internal_services.KnowledgeService
	assistantToolService internal_services.AssistantToolService

	//
	postgres      connectors.PostgresConnector
	opensearch    connectors.OpenSearchConnector
	vectordb      connectors.VectorConnector
	queryEmbedder internal_agent_embeddings.QueryEmbedding
	textReranker  internal_agent_rerankers.TextReranking

	// observe — shared observability infrastructure (DB + exporters)
	observer *observe.ConversationObserver

	// integration client
	vaultClient       web_client.VaultClient
	integrationClient integration_client.IntegrationServiceClient
	deploymentClient  endpoint_client.DeploymentServiceClient

	// interaction/session lifecycle owners.
	messageLifecycle adapter_lifecycle.MessageLifecycle
	sessionLifecycle adapter_lifecycle.SessionLifecycle
	lowStart         sync.Once
	inputStart       sync.Once

	// listening
	speechToTextTransformer internal_type.SpeechToTextTransformer
	textToSpeechTransformer internal_type.TextToSpeechTransformer

	// audio intelligence
	endOfSpeech internal_type.EndOfSpeech
	vad         internal_type.Vad
	denoiser    internal_type.Denoiser

	// output preprocessor + TTS
	inputNormalizer  internal_type.PacketNormalizer
	outputNormalizer internal_type.PacketNormalizer

	recorder internal_type.Recorder

	// executor
	assistantExecutor      internal_llm.AssistantExecutor
	analysisExecutor       internal_type.AnalysisExecutor
	webhookExecutor        internal_type.WebhookExecutor
	authenticationExecutor internal_type.AuthenticationExecutor

	// states
	assistant             *internal_assistant_entity.Assistant
	assistantConversation *internal_conversation_entity.AssistantConversation
	histories             []internal_type.MessagePacket

	args     map[string]interface{}
	metadata map[string]interface{}
	options  map[string]interface{}

	// experience
	idleTimeoutTimer    *time.Timer
	idleTimeoutDeadline time.Time // when the current idle timer is set to fire
	idleTimeoutCount    uint64
	maxSessionTimer     *time.Timer

	// sessionCtx is the adapter-owned lifecycle context. Outlives the gRPC stream.
	// cancelSession is invoked exactly once, by HandleFinalizationCompleted, after
	// the disconnect chain has fully drained. No other call site should invoke it.
	sessionCtx    context.Context
	cancelSession context.CancelFunc

	// channel registry with semantic names.
	channels adapter_channel.RequestorChannelBus
}

func NewGenericRequestor(
	ctx context.Context,
	config *config.AssistantConfig,
	logger commons.Logger, source utils.RapidaSource,
	postgres connectors.PostgresConnector, opensearch connectors.OpenSearchConnector,
	redis connectors.RedisConnector, storage storages.Storage, streamer internal_type.Streamer,
) *genericRequestor {
	sessionCtx, cancelSession := context.WithCancel(context.Background())

	gr := &genericRequestor{
		logger:   logger,
		config:   config,
		source:   source,
		streamer: streamer,
		// services
		assistantService:     internal_assistant_service.NewAssistantService(config, logger, postgres, opensearch),
		knowledgeService:     internal_knowledge_service.NewKnowledgeService(config, logger, postgres, storage),
		conversationService:  internal_assistant_service.NewAssistantConversationService(logger, postgres, storage),
		webhookService:       internal_assistant_service.NewAssistantWebhookService(logger, postgres, storage),
		assistantToolService: internal_assistant_service.NewAssistantToolService(logger, postgres, storage),
		//

		postgres:      postgres,
		opensearch:    opensearch,
		vectordb:      opensearch,
		queryEmbedder: internal_agent_embeddings.NewQueryEmbedding(logger, config, redis),
		textReranker:  internal_agent_rerankers.NewTextReranker(logger, config, redis),

		// clients
		integrationClient: integration_client.NewIntegrationServiceClientGRPC(&config.AppConfig, logger, redis),
		deploymentClient:  endpoint_client.NewDeploymentServiceClientGRPC(&config.AppConfig, logger, redis),
		vaultClient:       web_client.NewVaultClientGRPC(&config.AppConfig, logger, redis),

		// observer and hook executors are initialized after session creation in initializeCollectors

		messageLifecycle: adapter_lifecycle.NewMessageLifecycle(),
		sessionLifecycle: adapter_lifecycle.NewSessionLifecycle(),

		inputNormalizer:  internal_input_normalizer.NewInputNormalizer(logger),
		outputNormalizer: internal_output_normalizer.NewOutputNormalizer(logger),

		//
		histories:     make([]internal_type.MessagePacket, 0),
		metadata:      make(map[string]interface{}),
		args:          make(map[string]interface{}),
		options:       make(map[string]interface{}),
		sessionCtx:    sessionCtx,
		cancelSession: cancelSession,
		channels:      adapter_channel.NewRequestorChannels(),
	}

	go gr.runBootstrapDispatcher(sessionCtx)
	go gr.runCriticalDispatcher(sessionCtx)
	go gr.runOutputDispatcher(sessionCtx)
	go gr.runDataDispatcher(sessionCtx)
	return gr
}

// GetSource implements internal_adapter_requests.Messaging.
func (dm *genericRequestor) GetSource() utils.RapidaSource {
	return dm.source
}

func (gr *genericRequestor) GetAssistantConversation(ctx context.Context, auth types.SimplePrinciple, assistantId uint64, assistantConversationId uint64) (*internal_conversation_entity.AssistantConversation, error) {
	return gr.conversationService.GetConversation(ctx, auth, assistantId, assistantConversationId, &internal_services.GetConversationOption{
		InjectContext:  true,
		InjectArgument: true,
		InjectMetadata: true,
		InjectOption:   true,
		InjectMetric:   false},
	)
}

func (talking *genericRequestor) BeginConversation(ctx context.Context, assistant *internal_assistant_entity.Assistant, direction type_enums.ConversationDirection, config *protos.ConversationInitialization) error {
	talking.assistant = assistant
	conversation, err := talking.conversationService.CreateConversation(ctx, talking.Auth(), talking.identifier(config), assistant.Id, assistant.AssistantProviderId, direction, talking.GetSource())
	if err != nil {
		return err
	}
	talking.assistantConversation = conversation
	if arguments, err := utils.AnyMapToInterfaceMap(config.GetArgs()); err == nil {
		talking.applyArguments(arguments)
	}
	if options, err := utils.AnyMapToInterfaceMap(config.GetOptions()); err == nil {
		talking.applyOptions(options)
	}
	if metadata, err := utils.AnyMapToInterfaceMap(config.GetMetadata()); err == nil {
		talking.applyMetadata(metadata)
	}
	return err
}

func (talking *genericRequestor) ResumeConversation(ctx context.Context, assistant *internal_assistant_entity.Assistant, config *protos.ConversationInitialization) error {
	talking.assistant = assistant
	conversation, err := talking.GetAssistantConversation(ctx, talking.Auth(), assistant.Id, config.GetAssistantConversationId())
	if err != nil {
		talking.logger.Errorf("failed to get assistant conversation: %+v", err)
		return err
	}
	if conversation == nil {
		talking.logger.Errorf("conversation not found: %d", config.GetAssistantConversationId())
		return fmt.Errorf("conversation not found: %d", config.GetAssistantConversationId())
	}
	talking.assistantConversation = conversation
	talking.args = conversation.GetArguments()
	talking.options = conversation.GetOptions()
	talking.metadata = conversation.GetMetadatas()
	if extra, err := utils.AnyMapToInterfaceMap(config.GetMetadata()); err == nil {
		talking.applyMetadata(extra)
	}
	return nil
}

func (talking *genericRequestor) IntegrationCaller() integration_client.IntegrationServiceClient {
	return talking.integrationClient

}

func (talking *genericRequestor) VaultCaller() web_client.VaultClient {
	return talking.vaultClient
}

func (talking *genericRequestor) DeploymentCaller() endpoint_client.DeploymentServiceClient {
	return talking.deploymentClient
}

func (talking *genericRequestor) GetKnowledge(ctx context.Context, knowledgeId uint64) (*internal_knowledge_gorm.Knowledge, error) {
	return talking.knowledgeService.Get(ctx, talking.auth, knowledgeId)
}

func (gr *genericRequestor) GetArgs() map[string]interface{} {
	return gr.args
}

func (gr *genericRequestor) GetOptions() utils.Option {
	return gr.options
}

func (dm *genericRequestor) GetHistories() []internal_type.MessagePacket {
	return dm.histories
}

func (gr *genericRequestor) CreateConversationRecording(_ context.Context, user, assistant []byte) error {
	dbCtx, cancel := context.WithTimeout(context.Background(), dbWriteTimeout)
	defer cancel()
	if _, err := gr.conversationService.CreateConversationRecording(dbCtx, gr.auth, gr.assistant.Id, gr.assistantConversation.Id, user, assistant); err != nil {
		gr.logger.Errorf("unable to create recording for the conversation id %d with error : %v", err)
		return err
	}
	return nil
}

// =============================================================================
// Interaction state methods — inline replacement for the former Messaging wrapper
// =============================================================================

// GetID returns the current interaction context UUID.
// Rotates to a new UUID each time an Interrupted transition fires.
func (r *genericRequestor) GetID() string {
	return r.messageLifecycle.ContextID()
}

// GetMode returns the current stream mode (text or audio).
func (r *genericRequestor) GetMode() type_enums.MessageMode {
	return r.messageLifecycle.Mode()
}

// SwitchMode sets the stream mode.
func (r *genericRequestor) SwitchMode(mm type_enums.MessageMode) {
	r.messageLifecycle.SetMode(mm)
}

// Transition advances the interaction state machine.
//
// Valid transitions:
//
//	LLMGenerating | LLMGenerated | Interrupt → Interrupt    (VAD soft-interrupt)
//	LLMGenerating | LLMGenerated | Interrupt → Interrupted  (word-interrupt, rotates contextID)
//	Unknown | Interrupted                    → LLMGenerating (new turn starts)
//	LLMGenerating                            → LLMGenerated  (LLM finished, TTS may still play)
//	Any except Unknown                       → LLMGenerated  (also used for error recovery)
//
// Blocked:
//
//   - → Unknown                          (no explicit reset)
//     Unknown     → Interrupt | Interrupted (nothing active — no LLM, no TTS)
//     Interrupted → Interrupted             (already interrupted)
//     Interrupt   → Interrupt               (already soft-interrupted)
func (r *genericRequestor) Transition(newState adapter_lifecycle.MessageState) error {
	oldCtxID := r.GetID()
	if err := r.messageLifecycle.Transition(newState); err != nil {
		return err
	}

	if newState == Interrupted {
		nCtxID := r.GetID()
		if oldCtxID == nCtxID {
			return nil
		}

		utils.Go(context.Background(), func() {
			r.OnPacket(context.Background(), internal_type.TurnChangePacket{
				ContextID:         nCtxID,
				PreviousContextID: oldCtxID,
				Reason:            "interrupted",
				Source:            "state_machine",
				Time:              time.Now(),
			})
		})
	}
	return nil
}

func (r *genericRequestor) getSessionState() adapter_lifecycle.SessionState {
	return r.sessionLifecycle.Current()
}

func (r *genericRequestor) canAcceptInput() bool {
	return r.getSessionState() == adapter_lifecycle.StateReady
}

func (r *genericRequestor) canSwitchSession() bool {
	return r.sessionLifecycle.CanBe(adapter_lifecycle.EventSwitchRequested)
}

func (r *genericRequestor) getInteractionState() adapter_lifecycle.MessageState {
	return r.messageLifecycle.Current()
}

func (r *genericRequestor) setInteractionStateForTest(state adapter_lifecycle.MessageState) {
	r.messageLifecycle = adapter_lifecycle.NewMessageLifecycleWithState(state, r.GetID(), r.GetMode(), nil)
}

func (r *genericRequestor) setContextIDForTest(contextID string) {
	r.messageLifecycle.SetContextID(contextID)
}

func (r *genericRequestor) setModeForTest(mode type_enums.MessageMode) {
	r.messageLifecycle.SetMode(mode)
}

// initializeCollectors builds EventCollector and MetricCollector from the
// assistant's telemetry provider configuration stored in the database.
// Connection details come from the provider's Options key-value pairs.
// Collectors default to no-op when no providers are configured.
func (r *genericRequestor) initializeCollectors(ctx context.Context) {
	providers, err := r.GetTelemetryProvider(ctx)
	if err != nil {
		r.logger.Errorf("observe: failed to load telemetry providers: %v", err)
	}

	var projectID, orgID uint64
	if pid := r.auth.GetCurrentProjectId(); pid != nil {
		projectID = *pid
	}
	if oid := r.auth.GetCurrentOrganizationId(); oid != nil {
		orgID = *oid
	}
	meta := observe.SessionMeta{
		AssistantID:             r.assistant.Id,
		AssistantConversationID: r.assistantConversation.Id,
		ProjectID:               projectID,
		OrganizationID:          orgID,
	}

	var eventExporters []observe.EventExporter
	var metricExporters []observe.MetricExporter
	// Register one default telemetry exporter from env config (asset-store style).
	if r.config != nil && r.config.TelemetryConfig != nil {
		envProviderType := r.config.TelemetryConfig.Type()
		if envProviderType != "" {
			envOpts := r.config.TelemetryConfig.ToMap()
			evtExp, metExp, err := observe_exporters.GetExporter(
				ctx, r.logger, &r.config.AppConfig, r.opensearch, string(envProviderType), envOpts,
			)
			if err != nil {
				r.logger.Errorf("observe: env telemetry exporter creation failed for type %s: %v", envProviderType, err)
			} else if evtExp == nil || metExp == nil {
				r.logger.Warnf("observe: env telemetry exporter returned nil for type %s", envProviderType)
			} else {
				eventExporters = append(eventExporters, evtExp)
				metricExporters = append(metricExporters, metExp)
			}
		}
	}

	for _, p := range providers {
		opts := p.GetOptions()
		credID, parseErr := opts.GetUint64("rapida.credential_id")
		if parseErr != nil {
			r.logger.Errorf("observe: invalid credential_id for provider %d (%s): %v", p.Id, p.ProviderType, parseErr)
		} else {
			credential, credErr := r.VaultCaller().GetCredential(ctx, r.Auth(), credID)
			if credErr != nil {
				r.logger.Errorf("observe: vault credential lookup failed for provider %d (%s): %v", p.Id, p.ProviderType, credErr)
			} else if credential != nil && credential.GetValue() != nil {
				for k, v := range credential.GetValue().AsMap() {
					if s, ok := v.(string); ok {
						opts[k] = s
					}
				}
			}
		}

		evtExp, metExp, err := observe_exporters.GetExporter(ctx, r.logger, &r.config.AppConfig, r.opensearch, p.ProviderType, opts)
		if err != nil {
			r.logger.Errorf("observe: exporter creation failed for provider %d (%s): %v", p.Id, p.ProviderType, err)
			continue
		}
		if evtExp == nil || metExp == nil {
			_, err := opts.GetString("endpoint")
			if (p.ProviderType == string(observe.OTLP_HTTP) || p.ProviderType == string(observe.OTLP_GRPC)) && err != nil {
				r.logger.Warnf("observe: skipping provider %d (%s): missing endpoint", p.Id, p.ProviderType)
				continue
			}
			r.logger.Warnf("observe: exporter returned nil for provider %d (%s)", p.Id, p.ProviderType)
			continue
		}
		eventExporters = append(eventExporters, evtExp)
		metricExporters = append(metricExporters, metExp)
	}

	r.observer = observe.NewConversationObserver(&observe.ConversationObserverConfig{
		Logger:         r.logger,
		Auth:           r.auth,
		AssistantID:    r.assistant.Id,
		ConversationID: r.assistantConversation.Id,
		ProjectID:      projectID,
		OrganizationID: orgID,
		Persist: &observe.ServicePersister{
			ApplyMetrics: func(ctx context.Context, auth types.SimplePrinciple, assistantID, conversationID uint64, metrics []*types.Metric) (interface{}, error) {
				dbCtx, cancel := context.WithTimeout(context.Background(), dbWriteTimeout)
				defer cancel()
				return r.conversationService.ApplyConversationMetrics(dbCtx, auth, assistantID, conversationID, metrics)
			},
			ApplyMetadata: func(ctx context.Context, auth types.SimplePrinciple, assistantID, conversationID uint64, metadata []*types.Metadata) (interface{}, error) {
				dbCtx, cancel := context.WithTimeout(context.Background(), dbWriteTimeout)
				defer cancel()
				return r.conversationService.ApplyConversationMetadata(dbCtx, auth, assistantID, conversationID, metadata)
			},
		},
		Events:  observe.NewEventCollector(r.logger, meta, eventExporters...),
		Metrics: observe.NewMetricCollector(r.logger, meta, metricExporters...),
	})

}

// shutdownCollectors waits for in-flight exports and shuts down all exporters.
func (r *genericRequestor) shutdownCollectors(ctx context.Context) {
	if r.observer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), collectorWriteTimeout)
		defer cancel()
		r.observer.Shutdown(shutdownCtx)
	}
	if r.analysisExecutor != nil {
		if err := r.analysisExecutor.Close(ctx); err != nil {
			r.logger.Errorf("close analysis executor: %v", err)
		}
	}
	if r.webhookExecutor != nil {
		if err := r.webhookExecutor.Close(ctx); err != nil {
			r.logger.Errorf("close webhook executor: %v", err)
		}
	}
}
