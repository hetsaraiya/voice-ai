// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_llm_model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_agent_tool "github.com/rapidaai/api/assistant-api/internal/tool"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/api/assistant-api/internal/variable"
	internal_namespace "github.com/rapidaai/api/assistant-api/internal/variable/namespace"
	integration_client_builders "github.com/rapidaai/pkg/clients/integration/builders"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/parsers"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/protos"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

type modelAssistantExecutor struct {
	logger             commons.Logger
	toolExecutor       internal_agent_tool.ToolExecutor
	providerCredential *protos.VaultCredential
	inputBuilder       integration_client_builders.InputChatBuilder
	history            *ConversationHistory
	stream             grpc.BidiStreamingClient[protos.StreamChatRequest, protos.StreamChatResponse]

	currentPacket *internal_type.UserInputPacket
	mu            sync.RWMutex
	ctx           context.Context
	ctxCancel     context.CancelFunc
}

func NewModelAssistantExecutor(logger commons.Logger) *modelAssistantExecutor {
	return &modelAssistantExecutor{
		logger:       logger,
		inputBuilder: integration_client_builders.NewChatInputBuilder(logger),
		toolExecutor: internal_agent_tool.NewToolExecutor(logger),
		history:      NewConversationHistory(),
	}
}

func (e *modelAssistantExecutor) Name() string { return "model" }

// =============================================================================
// Initialize / Close
// =============================================================================

func (e *modelAssistantExecutor) Initialize(ctx context.Context, communication internal_type.Communication, cfg *protos.ConversationInitialization) error {
	start := time.Now()
	g, gCtx := errgroup.WithContext(ctx)
	var providerCredential *protos.VaultCredential

	g.Go(func() error {
		credentialID, err := communication.Assistant().AssistantProviderModel.GetOptions().GetUint64("rapida.credential_id")
		if err != nil {
			return fmt.Errorf("failed to get credential ID: %w", err)
		}
		cred, err := communication.VaultCaller().GetCredential(gCtx, communication.Auth(), credentialID)
		if err != nil {
			return fmt.Errorf("failed to get provider credential: %w", err)
		}
		providerCredential = cred
		return nil
	})
	g.Go(func() error {
		return e.toolExecutor.Initialize(gCtx, communication)
	})
	if err := g.Wait(); err != nil {
		communication.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: fmt.Sprintf("%s: error while initialization %s", e.Name(), err.Error()),
				Attributes: observability.Attributes{
					"component":  observability.ComponentLLM.String(),
					"provider":   communication.Assistant().AssistantProviderModel.ModelProviderName,
					"options":    observability.AttributeValue(communication.Assistant().AssistantProviderModel.GetOptions()),
					"error":      err.Error(),
					"error_type": fmt.Sprintf("%T", err),
				},
				OccurredAt: time.Now(),
			},
		})
		return err
	}

	e.providerCredential = providerCredential
	// Keep integration stream lifecycle independent from the short init deadline.
	// Initialization still respects ctx via open/send gating below.
	runtimeCtx, runtimeCancel := context.WithCancel(context.Background())
	stream, err := e.openStream(ctx, runtimeCtx, communication)
	if err != nil {
		runtimeCancel()
		communication.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: fmt.Sprintf("%s: error while initialization %s", e.Name(), err.Error()),
				Attributes: observability.Attributes{
					"component":  observability.ComponentLLM.String(),
					"provider":   communication.Assistant().AssistantProviderModel.ModelProviderName,
					"options":    observability.AttributeValue(communication.Assistant().AssistantProviderModel.GetOptions()),
					"error":      err.Error(),
					"error_type": fmt.Sprintf("%T", err),
				},
				OccurredAt: time.Now(),
			},
		})
		return fmt.Errorf("failed to open stream: %w", err)
	}
	if err := e.sendStreamConfiguration(ctx, stream, communication); err != nil {
		runtimeCancel()
		_ = stream.CloseSend()
		communication.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelError,
				Message: fmt.Sprintf("%s: error while initialization %s", e.Name(), err.Error()),
				Attributes: observability.Attributes{
					"component":  observability.ComponentLLM.String(),
					"provider":   communication.Assistant().AssistantProviderModel.ModelProviderName,
					"options":    observability.AttributeValue(communication.Assistant().AssistantProviderModel.GetOptions()),
					"error":      err.Error(),
					"error_type": fmt.Sprintf("%T", err),
				},
				OccurredAt: time.Now(),
			},
		})
		return err
	}
	e.stream = stream
	e.ctx = runtimeCtx
	e.ctxCancel = runtimeCancel
	utils.Go(e.ctx, func() { e.listen(e.ctx, communication) })

	provider := communication.Assistant().AssistantProviderModel.ModelProviderName
	communication.OnPacket(ctx,
		internal_type.ObservabilityMetricRecordPacket{
			Scope:  internal_type.ObservabilityRecordScopeConversation,
			Record: observability.NewMetricLLMInitLatencyMs(time.Since(start), observability.Attributes{"provider": provider}),
		},
		internal_type.ObservabilityLogRecordPacket{
			Scope: internal_type.ObservabilityRecordScopeConversation,
			Record: observability.RecordLog{
				Level:   observability.LevelInfo,
				Message: fmt.Sprintf("%s: initialization completed", e.Name()),
				Attributes: observability.Attributes{
					"component": observability.ComponentLLM.String(),
					"provider":  provider,
					"options":   observability.AttributeValue(communication.Assistant().AssistantProviderModel.GetOptions()),
				},
				OccurredAt: time.Now(),
			},
		},
	)
	return nil
}

func (e *modelAssistantExecutor) openStream(
	initCtx context.Context,
	runtimeCtx context.Context,
	communication internal_type.Communication,
) (grpc.BidiStreamingClient[protos.StreamChatRequest, protos.StreamChatResponse], error) {
	type result struct {
		stream grpc.BidiStreamingClient[protos.StreamChatRequest, protos.StreamChatResponse]
		err    error
	}
	done := make(chan result, 1)
	go func() {
		stream, err := communication.IntegrationCaller().StreamChat(
			runtimeCtx, communication.Auth(),
			communication.Assistant().AssistantProviderModel.ModelProviderName,
		)
		done <- result{stream: stream, err: err}
	}()
	select {
	case <-initCtx.Done():
		return nil, initCtx.Err()
	case res := <-done:
		return res.stream, res.err
	}
}

func (e *modelAssistantExecutor) sendStreamConfiguration(
	initCtx context.Context,
	stream grpc.BidiStreamingClient[protos.StreamChatRequest, protos.StreamChatResponse],
	communication internal_type.Communication,
) error {
	mergedOptions := utils.MergeMaps(
		communication.Assistant().AssistantProviderModel.GetOptions(),
		communication.GetOptions(),
	)
	connectionOptions := make(map[string]string)
	for key, value := range mergedOptions {
		if !strings.HasPrefix(key, "connection.") || value == nil {
			continue
		}
		connectionOptions[key] = fmt.Sprintf("%v", value)
	}

	done := make(chan error, 1)
	go func() {
		done <- stream.Send(&protos.StreamChatRequest{
			Request: &protos.StreamChatRequest_Configuration{
				Configuration: &protos.StreamChatConfiguration{
					Credential:        &protos.Credential{Id: e.providerCredential.GetId(), Value: e.providerCredential.GetValue()},
					ProviderName:      strings.ToLower(communication.Assistant().AssistantProviderModel.ModelProviderName),
					ConnectionOptions: connectionOptions,
				},
			},
		})
	}()
	select {
	case <-initCtx.Done():
		return initCtx.Err()
	case err := <-done:
		return err
	}
}

func (e *modelAssistantExecutor) Close(ctx context.Context) error {
	if e.ctxCancel != nil {
		e.ctxCancel()
	}
	e.mu.Lock()
	e.currentPacket = nil
	stream := e.stream
	e.stream = nil
	e.mu.Unlock()
	e.history.Reset()
	if stream != nil {
		_ = stream.Send(&protos.StreamChatRequest{
			Request: &protos.StreamChatRequest_Close{
				Close: &protos.StreamChatClose{Reason: "session ended"},
			},
		})
		_ = stream.CloseSend()
	}
	if e.toolExecutor != nil {
		if err := e.toolExecutor.Close(ctx); err != nil {
			e.logger.Errorf("error closing tool executor: %v", err)
		}
	}
	return nil
}

// =============================================================================
// Execute — maps incoming packets to pipeline types
// =============================================================================

func (e *modelAssistantExecutor) Execute(ctx context.Context, communication internal_type.Communication, pctk internal_type.Packet) error {
	switch p := pctk.(type) {
	case internal_type.UserInputPacket:
		if supersededCtx := e.history.SupersedePending(); supersededCtx != "" {
			communication.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
				ContextID: supersededCtx,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.RecordLog{
					Level:   observability.LevelInfo,
					Message: "tool block superseded",
					Attributes: observability.Attributes{
						"component":             observability.ComponentTool.String(),
						"operation":             "supersede_tool_block",
						"provider":              communication.Assistant().AssistantProviderModel.ModelProviderName,
						"context_id":            supersededCtx,
						"reason":                "user_interrupted",
						"superseded_context_id": supersededCtx,
					},
					OccurredAt: time.Now(),
				},
			})
		}
		e.mu.Lock()
		e.currentPacket = &p
		e.mu.Unlock()
		e.Run(ctx, communication, UserTurnPipeline{Packet: p})

	case internal_type.InjectMessagePacket:
		e.Run(ctx, communication, InjectMessagePipeline{Packet: p})

	case internal_type.LLMToolCallPacket:
		// no-op: dispatch handles logging/notification

	case internal_type.LLMToolResultPacket:
		e.Run(ctx, communication, ToolResultPipeline{Packet: p})

	case internal_type.LLMInterruptPacket:
		e.Run(ctx, communication, InterruptionPipeline{Packet: p})

	default:
		e.logger.Errorf("unsupported packet type: %T", pctk)
	}
	return nil
}

// =============================================================================
// Run — central pipeline dispatch
// =============================================================================

func (e *modelAssistantExecutor) Run(ctx context.Context, communication internal_type.Communication, p AgentPipeline) {
	switch v := p.(type) {
	case UserTurnPipeline:
		e.handleUserTurn(ctx, communication, v.Packet)
	case InjectMessagePipeline:
		e.history.AppendInjected(v.Packet.Text)
	case ToolResultPipeline:
		e.handleToolResult(ctx, communication, v.Packet)
	case InterruptionPipeline:
		e.handleInterruption()
	case ResponsePipeline:
		e.handleResponse(ctx, communication, v.Response)
	case ToolFollowUpPipeline:
		e.handleToolFollowUp(ctx, communication, v.ContextID)
	default:
		e.logger.Errorf("unknown pipeline type: %T", p)
	}
}

// =============================================================================
// Pipeline handlers
// =============================================================================

func (e *modelAssistantExecutor) handleUserTurn(ctx context.Context, communication internal_type.Communication, p internal_type.UserInputPacket) {
	snapshot := e.history.Snapshot()
	promptArgs := e.buildPromptArgs(communication, p)
	providerName := communication.Assistant().AssistantProviderModel.ModelProviderName

	if err := e.validateHistorySequence(snapshot); err != nil {
		err = fmt.Errorf("history integrity: %w", err)
		communication.OnPacket(ctx,
			internal_type.LLMErrorPacket{ContextID: p.ContextID, Error: err},
			internal_type.ObservabilityEventRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.NewMessageRecord(p.ContextID, observability.ComponentLLM, observability.LLMError, observability.MessageRoleAssistant, observability.Attributes{
					"provider":   providerName,
					"context_id": p.ContextID,
					"error":      err.Error(),
				}),
			},
			internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "llm request failed",
					Attributes: observability.Attributes{
						"component":  observability.ComponentLLM.String(),
						"operation":  "execute",
						"provider":   providerName,
						"context_id": p.ContextID,
						"error":      err.Error(),
					},
					OccurredAt: time.Now(),
				},
			},
		)
		return
	}

	communication.OnPacket(ctx,
		internal_type.ObservabilityEventRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.NewMessageRecord(p.ContextID, observability.ComponentLLM, observability.LLMStarted, observability.MessageRoleAssistant, observability.Attributes{
				"provider":         providerName,
				"context_id":       p.ContextID,
				"input_char_count": fmt.Sprintf("%d", len(p.Text)),
				"history_count":    fmt.Sprintf("%d", len(snapshot)),
			}),
		},
		internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelDebug,
				Message: "llm request started",
				Attributes: observability.Attributes{
					"component":        observability.ComponentLLM.String(),
					"operation":        "execute",
					"provider":         providerName,
					"context_id":       p.ContextID,
					"input_char_count": fmt.Sprintf("%d", len(p.Text)),
					"history_count":    fmt.Sprintf("%d", len(snapshot)),
				},
				OccurredAt: time.Now(),
			},
		},
		internal_type.ObservabilityMetricRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordMetric{
				Attributes: observability.Attributes{"provider": providerName},
				Metrics: []*protos.Metric{
					{Name: "llm_input_char_count", Value: fmt.Sprintf("%d", len(p.Text)), Description: "Input character count sent to LLM"},
					{Name: "llm_history_count", Value: fmt.Sprintf("%d", len(snapshot)), Description: "History message count sent to LLM"},
				},
			},
		},
	)

	userMsg := &protos.Message{
		Role:    "user",
		Message: &protos.Message_User{User: &protos.UserMessage{Content: p.Text}},
	}
	if err := e.sendChat(communication, p.ContextID, promptArgs, append(snapshot, userMsg)...); err != nil {
		communication.OnPacket(ctx,
			internal_type.LLMErrorPacket{ContextID: p.ContextID, Error: err},
			internal_type.ObservabilityEventRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.NewMessageRecord(p.ContextID, observability.ComponentLLM, observability.LLMError, observability.MessageRoleAssistant, observability.Attributes{
					"provider":   providerName,
					"context_id": p.ContextID,
					"error":      err.Error(),
				}),
			},
			internal_type.ObservabilityLogRecordPacket{
				ContextID: p.ContextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "llm request failed",
					Attributes: observability.Attributes{
						"component":  observability.ComponentLLM.String(),
						"operation":  "execute",
						"provider":   providerName,
						"context_id": p.ContextID,
						"error":      err.Error(),
					},
					OccurredAt: time.Now(),
				},
			},
		)
		return
	}
	e.history.AppendUser(p.Text)
}

func (e *modelAssistantExecutor) handleToolResult(ctx context.Context, communication internal_type.Communication, p internal_type.LLMToolResultPacket) {
	providerName := communication.Assistant().AssistantProviderModel.ModelProviderName
	resultJSON, _ := json.Marshal(p.Result)
	accepted, resolved := e.history.AcceptToolResult(p.ContextID, p.ToolID, p.Name, string(resultJSON))
	if !accepted {
		pendingCtx := e.history.PendingContextID()
		reason := "no_pending_block"
		data := map[string]string{"type": "tool_result_ignored", "reason": reason, "tool_id": p.ToolID}
		if pendingCtx != "" {
			reason = "context_or_id_mismatch"
			data["reason"] = reason
			data["pending_context"] = pendingCtx
		}
		communication.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: p.ContextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelInfo,
				Message: "tool result ignored",
				Attributes: observability.Attributes{
					"component":       observability.ComponentTool.String(),
					"operation":       "ignore_tool_result",
					"provider":        providerName,
					"context_id":      p.ContextID,
					"reason":          data["reason"],
					"tool_id":         p.ToolID,
					"name":            p.Name,
					"pending_context": data["pending_context"],
				},
				OccurredAt: time.Now(),
			},
		})
		return
	}
	if !resolved {
		return
	}

	contextID, followUp := e.history.FlushToolBlock()
	if !followUp {
		communication.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: contextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelInfo,
				Message: "tool block discarded",
				Attributes: observability.Attributes{
					"component":  observability.ComponentTool.String(),
					"operation":  "discard_tool_block",
					"provider":   providerName,
					"context_id": contextID,
					"reason":     "superseded",
				},
				OccurredAt: time.Now(),
			},
		})
		return
	}
	e.Run(ctx, communication, ToolFollowUpPipeline{ContextID: contextID})
}

func (e *modelAssistantExecutor) handleInterruption() {
	e.history.SupersedePending()
}

func (e *modelAssistantExecutor) handleResponse(ctx context.Context, communication internal_type.Communication, resp *protos.StreamChatOutput) {
	if e.isStaleResponse(resp.GetRequestId()) {
		return
	}
	contextID := resp.GetRequestId()
	providerName := communication.Assistant().AssistantProviderModel.ModelProviderName

	if resp.GetError() != nil {
		errMsg := resp.GetError().GetErrorMessage()
		communication.OnPacket(ctx,
			internal_type.LLMErrorPacket{ContextID: contextID, Error: errors.New(errMsg)},
			internal_type.ObservabilityEventRecordPacket{
				ContextID: contextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.NewMessageRecord(contextID, observability.ComponentLLM, observability.LLMError, observability.MessageRoleAssistant, observability.Attributes{
					"provider":   providerName,
					"context_id": contextID,
					"error":      errMsg,
				}),
			},
			internal_type.ObservabilityLogRecordPacket{
				ContextID: contextID,
				Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
				Record: observability.RecordLog{
					Level:   observability.LevelError,
					Message: "llm response failed",
					Attributes: observability.Attributes{
						"component":  observability.ComponentLLM.String(),
						"operation":  "response",
						"provider":   providerName,
						"context_id": contextID,
						"error":      errMsg,
					},
					OccurredAt: time.Now(),
				},
			},
		)
		return
	}

	output := resp.GetData()
	if output == nil || output.GetAssistant() == nil {
		return
	}

	if len(resp.GetMetrics()) == 0 {
		e.onStreamingChunk(ctx, communication, contextID, output)
		return
	}
	e.onCompletion(ctx, communication, contextID, resp.GetFinishReason(), output, resp.GetMetrics())
}

func (e *modelAssistantExecutor) handleToolFollowUp(ctx context.Context, communication internal_type.Communication, contextID string) {
	snapshot := e.history.Snapshot()

	e.mu.RLock()
	stream := e.stream
	e.mu.RUnlock()
	if stream == nil {
		e.logger.Errorf("stream not connected for tool follow-up")
		return
	}
	if err := e.validateHistorySequence(snapshot); err != nil {
		e.logger.Errorf("history integrity failed, blocking tool follow-up: %v", err)
		return
	}
	promptArgs := e.buildBasePromptArgs(communication)
	if err := stream.Send(&protos.StreamChatRequest{Request: &protos.StreamChatRequest_Chat{Chat: e.chatStreamRequest(communication, contextID, promptArgs, snapshot...)}}); err != nil {
		e.logger.Errorf("tool follow-up send failed: %v", err)
	}
}

// =============================================================================
// Stream I/O
// =============================================================================

func (e *modelAssistantExecutor) sendChat(
	communication internal_type.Communication,
	contextID string,
	promptArgs map[string]interface{},
	messages ...*protos.Message,
) error {
	e.mu.RLock()
	stream := e.stream
	e.mu.RUnlock()
	if stream == nil {
		return fmt.Errorf("stream not connected")
	}
	return stream.Send(&protos.StreamChatRequest{
		Request: &protos.StreamChatRequest_Chat{Chat: e.chatStreamRequest(communication, contextID, promptArgs, messages...)},
	})
}

func (e *modelAssistantExecutor) listen(ctx context.Context, communication internal_type.Communication) {
	for {
		e.mu.RLock()
		stream := e.stream
		e.mu.RUnlock()
		if stream == nil {
			return
		}
		resp, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			contextID := e.currentContextID()
			providerName := communication.Assistant().AssistantProviderModel.ModelProviderName
			communication.OnPacket(ctx,
				internal_type.LLMErrorPacket{
					ContextID: contextID,
					Error:     err,
					Type:      internal_type.LLMSystemPanic,
				},
				internal_type.ObservabilityEventRecordPacket{
					ContextID: contextID,
					Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
					Record: observability.NewMessageRecord(contextID, observability.ComponentLLM, observability.LLMError, observability.MessageRoleAssistant, observability.Attributes{
						"provider":   providerName,
						"context_id": contextID,
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					}),
				},
				internal_type.ObservabilityLogRecordPacket{
					ContextID: contextID,
					Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
					Record: observability.RecordLog{
						Level:   observability.LevelError,
						Message: "llm stream receive failed",
						Attributes: observability.Attributes{
							"component":  observability.ComponentLLM.String(),
							"operation":  "listen",
							"provider":   providerName,
							"context_id": contextID,
							"error":      err.Error(),
							"error_type": fmt.Sprintf("%T", err),
						},
						OccurredAt: time.Now(),
					},
				},
			)
			return
		}
		switch v := resp.GetResponse().(type) {
		case *protos.StreamChatResponse_Chat:
			e.Run(ctx, communication, ResponsePipeline{Response: v.Chat})
		case *protos.StreamChatResponse_Close:
			communication.OnPacket(ctx, internal_type.LLMToolCallPacket{
				ContextID: e.currentContextID(),
				Name:      "end_conversation",
				Action:    protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION,
				Arguments: map[string]string{"reason": v.Close.GetReason()},
			})
			return
		case *protos.StreamChatResponse_Configuration:
			// Late configuration response (we already handled it in Connect). Ignore.
		default:
			e.logger.Warnf("unknown stream response variant: %T", v)
		}
	}
}

// =============================================================================
// Response sub-handlers
// =============================================================================

func (e *modelAssistantExecutor) onStreamingChunk(ctx context.Context, communication internal_type.Communication, contextID string, output *protos.Message) {
	text := strings.Join(output.GetAssistant().GetContents(), "")
	communication.OnPacket(ctx, internal_type.LLMResponseDeltaPacket{ContextID: contextID, Text: text})
}

func (e *modelAssistantExecutor) onCompletion(ctx context.Context, communication internal_type.Communication, contextID, finishReason string, output *protos.Message, metrics []*protos.Metric) {
	assistant := output.GetAssistant()
	responseText := strings.Join(assistant.GetContents(), "")
	toolCalls := assistant.GetToolCalls()
	providerName := communication.Assistant().AssistantProviderModel.ModelProviderName

	supersededCtx := e.history.AppendAssistant(contextID, output)
	if supersededCtx != "" {
		e.logger.Errorf("new tool block while previous unresolved (context=%s), superseding", supersededCtx)
		communication.OnPacket(ctx, internal_type.ObservabilityLogRecordPacket{
			ContextID: supersededCtx,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordLog{
				Level:   observability.LevelInfo,
				Message: "tool block superseded",
				Attributes: observability.Attributes{
					"component":             observability.ComponentTool.String(),
					"operation":             "supersede_tool_block",
					"provider":              providerName,
					"context_id":            supersededCtx,
					"reason":                "new_tool_block",
					"superseded_context_id": supersededCtx,
				},
				OccurredAt: time.Now(),
			},
		})
	}
	if len(toolCalls) > 0 {
		e.toolExecutor.ExecuteAll(ctx, contextID, toolCalls, communication)
	}
	packets := []internal_type.Packet{
		internal_type.LLMResponseDonePacket{ContextID: contextID, Text: responseText},
		internal_type.ObservabilityEventRecordPacket{
			ContextID: contextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.NewMessageRecord(contextID, observability.ComponentLLM, observability.LLMCompleted, observability.MessageRoleAssistant, observability.Attributes{
				"provider":            providerName,
				"context_id":          contextID,
				"response_char_count": fmt.Sprintf("%d", len(responseText)),
				"finish_reason":       finishReason,
				"tool_call_count":     fmt.Sprintf("%d", len(toolCalls)),
			}),
		},
		internal_type.ObservabilityMetricRecordPacket{
			ContextID: contextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.RecordMetric{
				Attributes: observability.Attributes{"provider": providerName},
				Metrics:    e.buildCompletionMetrics(metrics),
			},
		},
	}
	var usageDuration time.Duration
	for _, metric := range metrics {
		switch metric.GetName() {
		case observability.MetricTimeTaken, observability.MetricProviderTotalTime:
			if ns, err := strconv.ParseInt(metric.GetValue(), 10, 64); err == nil && ns > 0 {
				usageDuration = time.Duration(ns)
			}
		}
	}
	if usageDuration > 0 {
		packets = append(packets, internal_type.ObservabilityUsageRecordPacket{
			ContextID: contextID,
			Scope:     internal_type.ObservabilityRecordScopeAssistantMessage,
			Record: observability.NewLLMDurationUsageRecord(
				providerName,
				usageDuration,
				observability.Attributes{
					"context_id":          contextID,
					"finish_reason":       finishReason,
					"response_char_count": fmt.Sprintf("%d", len(responseText)),
					"tool_call_count":     fmt.Sprintf("%d", len(toolCalls)),
				},
			),
		})
	}
	communication.OnPacket(ctx, packets...)
}

// =============================================================================
// Context state
// =============================================================================

func (e *modelAssistantExecutor) currentContextID() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.currentPacket == nil {
		return ""
	}
	return e.currentPacket.ContextID
}

func (e *modelAssistantExecutor) isStaleResponse(requestID string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.currentPacket == nil {
		return true
	}
	return requestID != e.currentPacket.ContextId()
}

// =============================================================================
// Metrics
// =============================================================================

func (e *modelAssistantExecutor) buildCompletionMetrics(providerMetrics []*protos.Metric) []*protos.Metric {
	out := make([]*protos.Metric, 0, len(providerMetrics)+1)
	for _, m := range providerMetrics {
		out = append(out, &protos.Metric{
			Name: "agent_" + m.GetName(), Value: m.GetValue(), Description: m.GetDescription(),
		})
		if m.GetName() == "time_to_first_token" {
			if ns, err := strconv.ParseInt(m.GetValue(), 10, 64); err == nil {
				out = append(out, &protos.Metric{
					Name: "llm_latency_ms", Value: fmt.Sprintf("%d", ns/int64(time.Millisecond)),
				})
			}
		}
	}
	return out
}

// =============================================================================
// Prompt argumentation
// =============================================================================

func (e *modelAssistantExecutor) buildPromptArgs(communication internal_type.Communication, p internal_type.UserInputPacket) map[string]interface{} {
	return utils.MergeMaps(e.buildBasePromptArgs(communication), map[string]interface{}{"message": map[string]interface{}{
		"text": p.Text, "language_code": p.Language.ISO639_1, "language": p.Language.Name,
	}})
}

// buildBasePromptArgs builds the nested prompt-argument map consumed by the
// LLM template engine. Resolution is delegated to the shared variable
// resolver — see api/assistant-api/internal/variable. The message.* sub-tree
// is per-message and stays here; buildPromptArgs overlays it on top.
func (e *modelAssistantExecutor) buildBasePromptArgs(communication internal_type.Communication) map[string]interface{} {
	registry := internal_namespace.NewDefaultRegistry()
	src := variable.NewCommunicationSource(communication)
	out := registry.Expand(src, variable.ResolveContext{})
	out["message"] = map[string]interface{}{"language": "English"}
	return out
}

// =============================================================================
// Chat request builder
// =============================================================================

func (e *modelAssistantExecutor) chatStreamRequest(communication internal_type.Communication, contextID string, promptArgs map[string]interface{}, messages ...*protos.Message) *protos.StreamChatInput {
	assistant := communication.Assistant()
	template := assistant.AssistantProviderModel.Template.GetTextChatCompleteTemplate()
	defaultArgs := parsers.CanonicalizePromptArguments(e.inputBuilder.PromptArguments(template.Variables))
	runtimeArgs := parsers.CanonicalizePromptArguments(promptArgs)
	systemMessages := e.inputBuilder.Message(template.Prompt, utils.MergeMaps(defaultArgs, runtimeArgs))
	src := e.buildStreamChatInput(communication, contextID, append(systemMessages, messages...)...)
	return &protos.StreamChatInput{
		RequestId:       src.GetRequestId(),
		ProviderName:    strings.ToLower(assistant.AssistantProviderModel.ModelProviderName),
		Conversations:   src.GetConversations(),
		AdditionalData:  src.GetAdditionalData(),
		ModelParameters: src.GetModelParameters(),
		ToolDefinitions: src.GetToolDefinitions(),
	}
}

func (e *modelAssistantExecutor) buildStreamChatInput(
	communication internal_type.Communication,
	contextID string,
	conversations ...*protos.Message,
) *protos.StreamChatInput {
	assistant := communication.Assistant()
	mergedOptions := utils.MergeMaps(
		assistant.AssistantProviderModel.GetOptions(),
		communication.GetOptions(),
	)
	modelOptions := make(map[string]interface{}, len(mergedOptions))
	for key, value := range mergedOptions {
		if key == "rapida.credential_id" || strings.HasPrefix(key, "connection.") {
			continue
		}
		modelOptions[key] = value
	}

	functionDefinitions := e.toolExecutor.GetFunctionDefinitions()
	toolDefinitions := make([]*protos.ToolDefinition, 0, len(functionDefinitions))
	for _, definition := range functionDefinitions {
		toolDefinitions = append(toolDefinitions, &protos.ToolDefinition{
			Type:               "function",
			FunctionDefinition: definition,
		})
	}

	return &protos.StreamChatInput{
		RequestId:     contextID,
		ProviderName:  strings.ToLower(assistant.AssistantProviderModel.ModelProviderName),
		Conversations: conversations,
		AdditionalData: map[string]string{
			"assistant_id":                fmt.Sprintf("%d", communication.Conversation().AssistantId),
			"conversation_id":             fmt.Sprintf("%d", communication.Conversation().Id),
			"user_identifier":             fmt.Sprintf("%s", communication.Conversation().Identifier),
			"message_id":                  contextID,
			"assistant_provider_model_id": fmt.Sprintf("%d", assistant.AssistantProviderModel.Id),
		},
		ModelParameters: e.inputBuilder.Options(modelOptions, nil),
		ToolDefinitions: toolDefinitions,
	}
}

// =============================================================================
// History validation
// =============================================================================

func (e *modelAssistantExecutor) validateHistorySequence(messages []*protos.Message) error {
	for i, msg := range messages {
		if ast := msg.GetAssistant(); ast != nil && len(ast.GetToolCalls()) > 0 {
			if i+1 >= len(messages) || messages[i+1].GetTool() == nil {
				return fmt.Errorf("history: assistant tool_call at %d not followed by tool response", i)
			}
			if err := e.validateToolIDMatch(ast.GetToolCalls(), messages[i+1].GetTool().GetTools(), i); err != nil {
				return err
			}
		}
		if tool := msg.GetTool(); tool != nil {
			if i == 0 {
				return fmt.Errorf("history: orphan tool response at %d", i)
			}
			prev := messages[i-1].GetAssistant()
			if prev == nil || len(prev.GetToolCalls()) == 0 {
				return fmt.Errorf("history: orphan tool response at %d", i)
			}
		}
	}
	return nil
}

func (e *modelAssistantExecutor) validateToolIDMatch(calls []*protos.ToolCall, tools []*protos.ToolMessage_Tool, idx int) error {
	expected := make(map[string]struct{}, len(calls))
	for _, c := range calls {
		if id := strings.TrimSpace(c.GetId()); id != "" {
			expected[id] = struct{}{}
		}
	}
	for _, t := range tools {
		id := strings.TrimSpace(t.GetId())
		if _, ok := expected[id]; !ok {
			return fmt.Errorf("history: orphan tool result %q at assistant %d", id, idx)
		}
		delete(expected, id)
	}
	for id := range expected {
		return fmt.Errorf("history: missing tool result for %q at assistant %d", id, idx)
	}
	return nil
}
