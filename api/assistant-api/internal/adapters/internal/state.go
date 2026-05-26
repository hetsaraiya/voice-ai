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
	"time"

	"github.com/google/uuid"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_conversation_entity "github.com/rapidaai/api/assistant-api/internal/entity/conversations"
	internal_telemetry_entity "github.com/rapidaai/api/assistant-api/internal/entity/telemetry"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
)

func (dm *genericRequestor) Assistant() *internal_assistant_entity.Assistant {
	return dm.assistant
}

func (gr *genericRequestor) Conversation() *internal_conversation_entity.AssistantConversation {
	return gr.assistantConversation
}

func (gr *genericRequestor) Ready() bool {
	return gr.Conversation() != nil && gr.Assistant() != nil
}

func (gr *genericRequestor) GetSpeechToTextTransformer() (
	*internal_assistant_entity.AssistantDeploymentAudio,
	error,
) {
	switch gr.source {
	case utils.PhoneCall:
		if a := gr.assistant; a != nil && a.AssistantPhoneDeployment != nil && a.AssistantPhoneDeployment.InputAudio != nil {
			return a.AssistantPhoneDeployment.InputAudio, nil
		}

	case utils.SDK:
		if a := gr.assistant; a != nil && a.AssistantApiDeployment != nil && a.AssistantApiDeployment.InputAudio != nil {
			return a.AssistantApiDeployment.InputAudio, nil
		}

	case utils.WebPlugin:
		if a := gr.assistant; a != nil && a.AssistantWebPluginDeployment != nil && a.AssistantWebPluginDeployment.InputAudio != nil {
			return a.AssistantWebPluginDeployment.InputAudio, nil
		}

	case utils.Debugger:
		if a := gr.assistant; a != nil && a.AssistantDebuggerDeployment != nil && a.AssistantDebuggerDeployment.InputAudio != nil {
			return a.AssistantDebuggerDeployment.InputAudio, nil
		}
	}
	return nil, errors.New("audio is not enabled for the source")
}

func (gr *genericRequestor) GetTelemetryProvider(ctx context.Context) ([]*internal_telemetry_entity.AssistantTelemetryProvider, error) {
	if gr.assistant == nil {
		return nil, errors.New("assistant is not initialized")
	}
	if gr.assistant.AssistantTelemetryProviders == nil {
		return []*internal_telemetry_entity.AssistantTelemetryProvider{}, nil
	}
	return gr.assistant.AssistantTelemetryProviders, nil
}

func (gr *genericRequestor) GetTextToSpeechTransformer() (*internal_assistant_entity.AssistantDeploymentAudio, error) {
	switch gr.source {
	case utils.PhoneCall:
		if a := gr.assistant; a != nil && a.AssistantPhoneDeployment != nil && a.AssistantPhoneDeployment.OutputAudio != nil {
			return a.AssistantPhoneDeployment.OutputAudio, nil
		}

	case utils.SDK:
		if a := gr.assistant; a != nil && a.AssistantApiDeployment != nil && a.AssistantApiDeployment.OutputAudio != nil {
			return a.AssistantApiDeployment.OutputAudio, nil
		}

	case utils.WebPlugin:
		if a := gr.assistant; a != nil && a.AssistantWebPluginDeployment != nil && a.AssistantWebPluginDeployment.OutputAudio != nil {
			return a.AssistantWebPluginDeployment.OutputAudio, nil
		}

	case utils.Debugger:
		if a := gr.assistant; a != nil && a.AssistantDebuggerDeployment != nil && a.AssistantDebuggerDeployment.OutputAudio != nil {
			return a.AssistantDebuggerDeployment.OutputAudio, nil
		}
	}
	return nil, errors.New("audio is not enabled for the source")
}

func (gr *genericRequestor) GetAssistant(
	ctx context.Context,
	auth types.SimplePrinciple,
	assistantId uint64,
	version string) (*internal_assistant_entity.Assistant, error) {
	versionId := utils.GetVersionDefinition(version)
	assistantOpts := &internal_services.GetAssistantOption{
		InjectAssistantProvider:      true,
		InjectKnowledgeConfiguration: true,
		InjectTool:                   true,
		InjectAnalysis:               true,
		InjectWebhook:                true,
		InjectTelemetryProvider:      true,
		InjectAuthentication:         true,
		InjectConversations:          false,
		InjectTag:                    false,
	}
	switch gr.source {
	case utils.PhoneCall:
		assistantOpts.InjectPhoneDeployment = true
	case utils.Whatsapp:
		assistantOpts.InjectWhatsappDeployment = true
	case utils.SDK:
		assistantOpts.InjectApiDeployment = true
	case utils.WebPlugin:
		assistantOpts.InjectWebpluginDeployment = true
	case utils.Debugger:
		assistantOpts.InjectDebuggerDeployment = true
	}
	return gr.assistantService.Get(ctx, auth, assistantId, versionId, assistantOpts)
}

/*
 * Auth retrieves the authentication information associated with the debugger.
 *
 * This method returns the SimplePrinciple object that represents the current
 * authentication state of the debugger. The SimplePrinciple typically contains
 * information such as user ID, roles, or any other relevant authentication data.
 *
 * Returns:
 *   - types.SimplePrinciple: The authentication information for the debugger.
 */
func (dm *genericRequestor) Auth() types.SimplePrinciple {
	return dm.auth
}

/*
 * SetAuth sets the authentication information for the debugger.
 *
 * This method allows updating the authentication state of the debugger by
 * providing a new SimplePrinciple object. This is typically used when the
 * authentication state changes, such as after a successful login or when
 * switching users.
 *
 * Parameters:
 *   - auth: types.SimplePrinciple - The new authentication information to set.
 */
func (deb *genericRequestor) SetAuth(auth types.SimplePrinciple) {
	deb.auth = auth
}

/*
 * Metadata Management for Talking Conversations
 * ---------------------------------------------
 * These methods provide functionality to manage metadata associated with
 * a talking conversation. Metadata can be used to store additional
 * information about the conversation that may be useful for processing,
 * analysis, or integration with other systems.
 *
 * GetMetadata(): Retrieves the entire metadata map.
 * AddMetadata(): Adds a single key-value pair to the metadata.
 * SetMetadata(): Replaces the entire metadata map with a new one.
 *
 * Note: Proper use of these methods ensures consistent handling of
 * conversation metadata across the application.
 */
func (tc *genericRequestor) GetMetadata() map[string]interface{} {
	return tc.metadata
}

// applyMetadata merges metadata into in-memory state and persists asynchronously.
// Called from BeginConversation, ResumeConversation, and HandleSessionAuthenticationSucceeded.
func (tc *genericRequestor) applyMetadata(mt map[string]interface{}) {
	if len(mt) == 0 {
		return
	}
	modified := make(map[string]interface{})
	for k, v := range mt {
		vl, ok := tc.metadata[k]
		if ok && vl == v {
			continue
		}
		tc.metadata[k] = v
		modified[k] = v
	}
	if len(modified) == 0 {
		return
	}
	utils.Go(context.Background(), func() {
		dbCtx, cancel := context.WithTimeout(context.Background(), dbWriteTimeout)
		defer cancel()
		start := time.Now()
		tc.conversationService.ApplyConversationMetadata(
			dbCtx,
			tc.auth, tc.assistant.Id, tc.assistantConversation.Id, types.NewMetadataList(modified))
		tc.logger.Benchmark("genericRequestor.applyMetadata", time.Since(start))
	})
}

// applyArguments merges arguments into in-memory state and persists asynchronously.
func (tc *genericRequestor) applyArguments(args map[string]interface{}) {
	if len(args) == 0 {
		return
	}
	tc.args = utils.MergeMaps(tc.args, args)
	utils.Go(context.Background(), func() {
		dbCtx, cancel := context.WithTimeout(context.Background(), dbWriteTimeout)
		defer cancel()
		if _, err := tc.conversationService.ApplyConversationArgument(
			dbCtx, tc.auth, tc.assistant.Id, tc.assistantConversation.Id, args,
		); err != nil {
			tc.logger.Errorf("apply arguments: %v", err)
		}
	})
}

// applyOptions merges options into in-memory state and persists asynchronously.
func (tc *genericRequestor) applyOptions(opts map[string]interface{}) {
	if len(opts) == 0 {
		return
	}
	tc.options = utils.MergeMaps(tc.options, opts)
	utils.Go(context.Background(), func() {
		dbCtx, cancel := context.WithTimeout(context.Background(), dbWriteTimeout)
		defer cancel()
		if _, err := tc.conversationService.ApplyConversationOption(
			dbCtx, tc.auth, tc.assistant.Id, tc.assistantConversation.Id, opts,
		); err != nil {
			tc.logger.Errorf("apply options: %v", err)
		}
	})
}

func (tc *genericRequestor) onAddMetadata(_ context.Context, metadata ...*protos.Metadata) error {
	utils.Go(context.Background(), func() {
		dbCtx, cancel := context.WithTimeout(context.Background(), dbWriteTimeout)
		defer cancel()
		_, err := tc.conversationService.ApplyConversationMetadata(
			dbCtx,
			tc.auth,
			tc.assistant.Id,
			tc.assistantConversation.Id,
			types.ToMetadatas(metadata),
		)
		if err != nil {
			tc.logger.Errorf("unable to flush metadata for conversation %+v", err)
		}
	})
	return nil
}

func (deb *genericRequestor) onAddMessage(_ context.Context, msg internal_type.MessagePacket) error {
	deb.histories = append(deb.histories, msg)
	utils.Go(context.Background(), func() {
		dbCtx, cancel := context.WithTimeout(context.Background(), dbWriteTimeout)
		defer cancel()
		_, err := deb.conversationService.CreateConversationMessage(dbCtx, deb.Auth(), deb.GetSource(), deb.Assistant().Id, deb.Assistant().AssistantProviderId, deb.Conversation().Id,
			fmt.Sprintf("%s-%s", msg.Role(), msg.ContextId()), msg.Role(), msg.Content())
		if err != nil {
			deb.logger.Error("unable to create message for the user")
		}
	})
	return nil
}

func (deb *genericRequestor) onAddMessageMetric(_ context.Context, prefix string, messageId string, metrics []*protos.Metric) error {
	utils.Go(context.Background(), func() {
		dbCtx, cancel := context.WithTimeout(context.Background(), dbWriteTimeout)
		defer cancel()
		if _, err := deb.conversationService.ApplyMessageMetrics(dbCtx, deb.Auth(), deb.Conversation().Id, fmt.Sprintf("%s-%s", prefix, messageId), metrics); err != nil {
			deb.logger.Errorf("error updating metrics for message: %v", err)
		}
	})
	return nil
}

func (deb *genericRequestor) onAddMessageMetadata(_ context.Context, prefix string, messageId string, metadata []*protos.Metadata) error {
	utils.Go(context.Background(), func() {
		dbCtx, cancel := context.WithTimeout(context.Background(), dbWriteTimeout)
		defer cancel()
		if _, err := deb.conversationService.ApplyMessageMetadata(dbCtx, deb.Auth(), deb.Conversation().Id, fmt.Sprintf("%s-%s", prefix, messageId), metadata); err != nil {
			deb.logger.Errorf("Error in ApplyMessageMetadata: %v", err)
		}
	})
	return nil
}

func (gr *genericRequestor) CreateConversationRecording(_ context.Context, user, assistant, conversation []byte) error {
	utils.Go(context.Background(), func() {
		dbCtx, cancel := context.WithTimeout(context.Background(), dbWriteTimeout)
		defer cancel()
		if _, err := gr.conversationService.CreateConversationRecording(dbCtx, gr.auth, gr.assistant.Id, gr.assistantConversation.Id, user, assistant, conversation); err != nil {
			gr.logger.Errorf("unable to create recording for the conversation id %d with error : %v", gr.assistantConversation.Id, err)
		}
	})
	return nil
}

func (r *genericRequestor) identifier(config *protos.ConversationInitialization) string {
	switch identity := config.GetUserIdentity().(type) {
	case *protos.ConversationInitialization_Phone:
		return identity.Phone.GetPhoneNumber()
	case *protos.ConversationInitialization_Web:
		return identity.Web.GetUserId()
	default:
		return uuid.NewString()
	}
}
