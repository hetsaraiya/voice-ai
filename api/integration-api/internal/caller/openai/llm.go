package internal_openai_callers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	internal_caller_metrics "github.com/rapidaai/api/integration-api/internal/caller/metrics"
	internal_callers "github.com/rapidaai/api/integration-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/pkg/utils"
	protos "github.com/rapidaai/protos"
)

type largeLanguageCaller struct {
	OpenAI
}

func NewLargeLanguageCaller(logger commons.Logger, credential *protos.Credential) internal_callers.LargeLanguageCaller {
	return &largeLanguageCaller{
		OpenAI: openAI(logger, credential),
	}
}

func (llc *largeLanguageCaller) getResponseOptions(
	opts *internal_callers.ChatCompletionOptions, streaming bool,
) responses.ResponseNewParams {
	_ = streaming
	options := responses.ResponseNewParams{
		Store: openai.Bool(false),
	}
	if len(opts.ToolDefinitions) > 0 {
		fns := make([]responses.ToolUnionParam, 0, len(opts.ToolDefinitions))
		for _, tl := range opts.ToolDefinitions {
			switch tl.Type {
			case "tool":
			case "function":
				fn := tl.Function
				if fn != nil {
					funcDef := responses.FunctionToolParam{
						Name:   fn.Name,
						Strict: openai.Bool(false),
					}
					if fn.Description != "" {
						funcDef.Description = openai.String(fn.Description)
					}
					// Always set parameters with valid JSON schema format
					if fn.Parameters != nil {
						funcDef.Parameters = fn.Parameters.ToMap()
					} else {
						// Default empty parameters with properties field for valid schema
						funcDef.Parameters = map[string]interface{}{
							"type":       "object",
							"properties": map[string]interface{}{},
						}
					}
					fns = append(fns, responses.ToolUnionParam{OfFunction: &funcDef})
				}
			}
		}
		options.Tools = fns
	}

	for key, value := range opts.ModelParameter {
		switch key {
		case "model.name":
			if modelName, err := utils.AnyToString(value); err == nil {
				options.Model = shared.ResponsesModel(modelName)
			}
		case "model.user":
			if user, err := utils.AnyToString(value); err == nil {
				options.User = openai.String(user)
			}
		case "model.reasoning_effort":
			if re, err := utils.AnyToString(value); err == nil {
				options.Reasoning = shared.ReasoningParam{
					Effort: shared.ReasoningEffort(re),
				}
			}
		case "model.service_tier":
			if st, err := utils.AnyToString(value); err == nil {
				options.ServiceTier = responses.ResponseNewParamsServiceTier(st)
			}
		case "model.top_logprobs":
			if tl, err := utils.AnyToInt64(value); err == nil {
				options.TopLogprobs = openai.Int(tl)
			}
		case "model.metadata":
			format, _ := utils.AnyToString(value)
			var mtd map[string]string
			if err := json.Unmarshal([]byte(format), &mtd); err == nil {
				options.Metadata = shared.Metadata(mtd)
			}
		case "model.frequency_penalty":
			// responses API does not support frequency_penalty
		case "model.temperature":
			if temp, err := utils.AnyToFloat64(value); err == nil {
				options.Temperature = openai.Float(temp)
			}
		case "model.top_p":
			if topP, err := utils.AnyToFloat64(value); err == nil {
				options.TopP = openai.Float(topP)
			}
		case "model.presence_penalty":
			// responses API does not support presence_penalty
		case "model.max_completion_tokens", "model.max_output_tokens":
			if maxTokens, err := utils.AnyToInt64(value); err == nil {
				options.MaxOutputTokens = openai.Int(maxTokens)
			}
		case "model.stop":
			// responses API does not support stop
		case "model.store":
			if store, err := utils.AnyToBool(value); err == nil {
				options.Store = openai.Bool(store)
			}
		case "model.tool_choice":
			if choice, err := utils.AnyToString(value); err == nil {
				switch choice {
				case "auto":
					options.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
						OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsAuto),
					}
				case "required":
					options.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
						OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsRequired),
					}
				case "none":
					options.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
						OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsNone),
					}
				default:
					options.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
						OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsNone),
					}
				}
			}
		case "model.response_format":
			if format, err := utils.AnyToJSON(value); err == nil {
				switch format["type"].(string) {
				case "json_object":
					options.Text.Format = responses.ResponseFormatTextConfigUnionParam{
						OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
					}
				case "text":
					options.Text.Format = responses.ResponseFormatTextConfigUnionParam{
						OfText: &shared.ResponseFormatTextParam{},
					}
				case "json_schema":
					if schemaData, ok := format["json_schema"].(map[string]interface{}); ok {
						cfg := responses.ResponseFormatTextJSONSchemaConfigParam{
							Name: "response",
						}
						if name, ok := schemaData["name"].(string); ok && strings.TrimSpace(name) != "" {
							cfg.Name = name
						}
						if description, ok := schemaData["description"].(string); ok && description != "" {
							cfg.Description = openai.String(description)
						}
						if strict, ok := schemaData["strict"].(bool); ok {
							cfg.Strict = openai.Bool(strict)
						}
						if schema, ok := schemaData["schema"].(map[string]interface{}); ok {
							cfg.Schema = schema
						} else {
							cfg.Schema = map[string]interface{}{
								"type":       "object",
								"properties": map[string]interface{}{},
							}
						}
						options.Text.Format = responses.ResponseFormatTextConfigUnionParam{
							OfJSONSchema: &cfg,
						}
					}
				}
			}
		}
	}
	return options
}

func (llc *largeLanguageCaller) GetChatCompletion(
	ctx context.Context,
	allMessages []*protos.Message,
	options *internal_callers.ChatCompletionOptions,
) (*protos.Message, []*protos.Metric, error) {
	metrics := internal_caller_metrics.NewMetricBuilder(options.RequestId)
	metrics.OnStart()

	client, err := llc.GetClient()
	if err != nil {
		llc.logger.Errorf("chat completion unable to get client for openai %v", err)
		return nil, metrics.OnFailure().Build(), err
	}

	// message and options
	llmRequest := llc.getResponseOptions(options, false)
	llmRequest.Input = responses.ResponseNewParamsInputUnion{
		OfInputItemList: llc.BuildHistory(allMessages),
	}

	// prehook
	options.PreHook(utils.ToJson(llmRequest))

	resp, err := client.Responses.New(ctx, llmRequest)
	if err != nil {
		llc.logger.Errorf("chat completion failed to get response from openai %v", err)
		options.PostHook(map[string]interface{}{
			"error":  err,
			"result": resp,
		}, metrics.OnFailure().Build())
		return nil, metrics.OnFailure().Build(), err
	}

	assistantMsg := &protos.AssistantMessage{
		Contents:  make([]string, 0),
		ToolCalls: make([]*protos.ToolCall, 0),
	}

	if outputText := resp.OutputText(); outputText != "" {
		assistantMsg.Contents = append(assistantMsg.Contents, outputText)
	}
	for _, item := range resp.Output {
		if item.Type == "function_call" {
			fnCall := item.AsFunctionCall()
			id := fnCall.CallID
			if id == "" {
				id = fnCall.ID
			}
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, &protos.ToolCall{
				Id:   id,
				Type: "function",
				Function: &protos.FunctionCall{
					Name:      fnCall.Name,
					Arguments: fnCall.Arguments,
				},
			})
		}
	}
	metrics.OnSuccess()

	// Add usage metrics from response
	metrics.OnAddMetrics(llc.GetResponseUsages(resp.Usage)...)

	options.PostHook(map[string]interface{}{
		"result": resp,
	}, metrics.Build())
	return &protos.Message{
		Role: "assistant",
		Message: &protos.Message_Assistant{
			Assistant: assistantMsg,
		},
	}, metrics.Build(), nil
}

func (llc *largeLanguageCaller) StreamChatCompletion(
	ctx context.Context,
	allMessages []*protos.Message,
	options *internal_callers.ChatCompletionOptions,
	onStream func(string, *protos.Message) error,
	onMetrics func(string, *protos.Message, []*protos.Metric) error,
	onError func(string, error),
) error {
	start := time.Now()
	metrics := internal_caller_metrics.NewMetricBuilder(options.RequestId)
	metrics.OnStart()
	var firstTokenTime *time.Time

	client, err := llc.GetClient()
	if err != nil {
		llc.logger.Errorf("chat completion unable to get client for openai: %v", err)
		onError(options.Request.GetRequestId(), err)
		options.PostHook(map[string]interface{}{
			"error": err,
		}, metrics.OnFailure().Build())
		return err
	}

	completionsOptions := llc.getResponseOptions(options, true)
	completionsOptions.Input = responses.ResponseNewParamsInputUnion{
		OfInputItemList: llc.BuildHistory(allMessages),
	}
	options.PreHook(utils.ToJson(completionsOptions))
	llc.logger.Benchmark("Openai.llm.GetChatCompletion.llmRequestPrepare", time.Since(start))

	// Get streaming response
	resp := client.Responses.NewStreaming(ctx, completionsOptions)
	if resp.Err() != nil {
		llc.logger.Errorf("Failed to get responses stream: %v", resp.Err())
		options.PostHook(map[string]interface{}{
			"result": utils.ToJson(resp),
			"error":  resp.Err(),
		}, metrics.Build())
		onError(options.Request.GetRequestId(), resp.Err())
		return resp.Err()
	}
	defer resp.Close()
	assistantMsg := &protos.AssistantMessage{
		Contents:  make([]string, 0),
		ToolCalls: make([]*protos.ToolCall, 0),
	}
	var contentBuffer strings.Builder
	hasToolCalls := false
	var finalResponse *responses.Response

	for resp.Next() {
		event := resp.Current()
		switch e := event.AsAny().(type) {
		case responses.ResponseTextDeltaEvent:
			if e.Delta == "" {
				continue
			}
			contentBuffer.WriteString(e.Delta)
			if !hasToolCalls {
				if firstTokenTime == nil {
					now := time.Now()
					firstTokenTime = &now
				}
				tokenMsg := &protos.Message{
					Role: "assistant",
					Message: &protos.Message_Assistant{
						Assistant: &protos.AssistantMessage{
							Contents: []string{e.Delta},
						},
					},
				}
				if err := onStream(options.Request.GetRequestId(), tokenMsg); err != nil {
					llc.logger.Warnf("error streaming token: %v", err)
				}
			}
		case responses.ResponseFunctionCallArgumentsDeltaEvent:
			hasToolCalls = true
		case responses.ResponseFunctionCallArgumentsDoneEvent:
			hasToolCalls = true
		case responses.ResponseOutputItemAddedEvent:
			if e.Item.Type == "function_call" {
				hasToolCalls = true
			}
		case responses.ResponseOutputItemDoneEvent:
			if e.Item.Type == "function_call" {
				hasToolCalls = true
			}
		case responses.ResponseCompletedEvent:
			finalResponse = &e.Response
			if llc.hasFunctionCall(e.Response.Output) {
				hasToolCalls = true
			}
		}
	}

	if resp.Err() != nil {
		llc.logger.Errorf("Failed while reading responses stream: %v", resp.Err())
		options.PostHook(map[string]interface{}{
			"result": utils.ToJson(resp),
			"error":  resp.Err(),
		}, metrics.OnFailure().Build())
		onError(options.Request.GetRequestId(), resp.Err())
		return resp.Err()
	}

	if finalResponse != nil {
		if outputText := finalResponse.OutputText(); outputText != "" {
			assistantMsg.Contents = append(assistantMsg.Contents, outputText)
		}
		for _, item := range finalResponse.Output {
			if item.Type == "function_call" {
				fnCall := item.AsFunctionCall()
				id := fnCall.CallID
				if id == "" {
					id = fnCall.ID
				}
				assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, &protos.ToolCall{
					Id:   id,
					Type: "function",
					Function: &protos.FunctionCall{
						Name:      fnCall.Name,
						Arguments: fnCall.Arguments,
					},
				})
			}
		}
		metrics.OnAddMetrics(llc.GetResponseUsages(finalResponse.Usage)...)
	} else if contentBuffer.Len() > 0 {
		assistantMsg.Contents = append(assistantMsg.Contents, contentBuffer.String())
	}

	protoMsg := &protos.Message{
		Role: "assistant",
		Message: &protos.Message_Assistant{
			Assistant: assistantMsg,
		},
	}

	if firstTokenTime != nil {
		metrics.OnAddMetrics(&protos.Metric{
			Name:        type_enums.TIME_TO_FIRST_TOKEN.String(),
			Value:       fmt.Sprintf("%d", firstTokenTime.Sub(start)),
			Description: "Time to receive first token from LLM",
		})
	}
	metrics.OnSuccess()

	onMetrics(options.Request.GetRequestId(), protoMsg, metrics.Build())
	result := utils.ToJson(resp)
	if finalResponse != nil {
		result = utils.ToJson(finalResponse)
	}
	options.PostHook(map[string]interface{}{
		"result": result,
	}, metrics.Build())

	return nil
}

func (llc *largeLanguageCaller) hasFunctionCall(items []responses.ResponseOutputItemUnion) bool {
	for _, item := range items {
		if item.Type == "function_call" {
			return true
		}
	}
	return false
}

func (llc *largeLanguageCaller) BuildHistory(allMessages []*protos.Message) []responses.ResponseInputItemUnionParam {
	msg := make([]responses.ResponseInputItemUnionParam, 0)
	for _, cntn := range allMessages {
		switch cntn.GetRole() {
		case ChatRoleUser:
			if user := cntn.GetUser(); user != nil {
				msg = append(msg, responses.ResponseInputItemParamOfMessage(user.GetContent(), responses.EasyInputMessageRoleUser))
			}
		case ChatRoleAssistant:
			if assistant := cntn.GetAssistant(); assistant != nil {
				txtContent := strings.Join(assistant.GetContents(), "")
				if txtContent != "" {
					msg = append(msg, responses.ResponseInputItemParamOfMessage(txtContent, responses.EasyInputMessageRoleAssistant))
				}

				for _, ttc := range assistant.GetToolCalls() {
					if ttc.GetFunction() == nil {
						continue
					}
					callID := ttc.GetId()
					if callID == "" {
						continue
					}
					msg = append(msg, responses.ResponseInputItemParamOfFunctionCall(
						ttc.GetFunction().GetArguments(),
						callID,
						ttc.GetFunction().GetName(),
					))
				}
			}

		case ChatRoleSystem:
			if system := cntn.GetSystem(); system != nil {
				txtContent := system.GetContent()
				if len(txtContent) > 0 {
					msg = append(msg, responses.ResponseInputItemParamOfMessage(txtContent, responses.EasyInputMessageRoleSystem))
				}
			}

		case ChatRoleTool:
			if tool := cntn.GetTool(); tool != nil {
				for _, t := range tool.GetTools() {
					if t.GetId() == "" {
						continue
					}
					msg = append(msg, responses.ResponseInputItemParamOfFunctionCallOutput(t.GetId(), t.GetContent()))
				}
			}
		}
	}
	return msg
}
