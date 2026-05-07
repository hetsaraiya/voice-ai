// Rapida – Open Source Voice AI Orchestration Platform
// Copyright (C) 2023-2025 Prashant Srivastav <prashant@rapida.ai>
// Licensed under a modified GPL-2.0. See the LICENSE file for details.
package internal_openai_callers

import (
	"testing"

	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	internal_callers "github.com/rapidaai/api/integration-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/protos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

func newOpenAITestLogger() commons.Logger {
	lgr, _ := commons.NewApplicationLogger()
	return lgr
}

func newOpenAITestCaller() *largeLanguageCaller {
	return &largeLanguageCaller{
		OpenAI: OpenAI{logger: newOpenAITestLogger()},
	}
}

func mustAnyValue(t *testing.T, input interface{}) *anypb.Any {
	t.Helper()
	v, err := structpb.NewValue(input)
	require.NoError(t, err)
	a, err := anypb.New(v)
	require.NoError(t, err)
	return a
}

func TestBuildHistory_MixedMessagesWithToolFlow(t *testing.T) {
	caller := newOpenAITestCaller()
	msgs := []*protos.Message{
		{
			Role:    "system",
			Message: &protos.Message_System{System: &protos.SystemMessage{Content: "Be concise"}},
		},
		{
			Role:    "user",
			Message: &protos.Message_User{User: &protos.UserMessage{Content: "What's the weather?"}},
		},
		{
			Role: "assistant",
			Message: &protos.Message_Assistant{
				Assistant: &protos.AssistantMessage{
					Contents: []string{"Let me check."},
					ToolCalls: []*protos.ToolCall{
						{
							Id:   "call_weather_1",
							Type: "function",
							Function: &protos.FunctionCall{
								Name:      "get_weather",
								Arguments: `{"city":"NYC"}`,
							},
						},
					},
				},
			},
		},
		{
			Role: "tool",
			Message: &protos.Message_Tool{
				Tool: &protos.ToolMessage{
					Tools: []*protos.ToolMessage_Tool{
						{Id: "call_weather_1", Name: "get_weather", Content: `{"temp":72}`},
					},
				},
			},
		},
	}

	history := caller.BuildHistory(msgs)
	require.Len(t, history, 5)

	require.NotNil(t, history[0].OfMessage)
	assert.Equal(t, responses.EasyInputMessageRoleSystem, history[0].OfMessage.Role)

	require.NotNil(t, history[1].OfMessage)
	assert.Equal(t, responses.EasyInputMessageRoleUser, history[1].OfMessage.Role)

	require.NotNil(t, history[2].OfMessage)
	assert.Equal(t, responses.EasyInputMessageRoleAssistant, history[2].OfMessage.Role)
	assert.True(t, history[2].OfMessage.Content.OfString.Valid())
	assert.Equal(t, "Let me check.", history[2].OfMessage.Content.OfString.Value)

	require.NotNil(t, history[3].OfFunctionCall)
	assert.Equal(t, "call_weather_1", history[3].OfFunctionCall.CallID)
	assert.Equal(t, "get_weather", history[3].OfFunctionCall.Name)
	assert.Equal(t, `{"city":"NYC"}`, history[3].OfFunctionCall.Arguments)

	require.NotNil(t, history[4].OfFunctionCallOutput)
	assert.Equal(t, "call_weather_1", history[4].OfFunctionCallOutput.CallID)
	assert.True(t, history[4].OfFunctionCallOutput.Output.OfString.Valid())
	assert.Equal(t, `{"temp":72}`, history[4].OfFunctionCallOutput.Output.OfString.Value)
}

func TestBuildHistory_SkipsToolItemsWithoutCallID(t *testing.T) {
	caller := newOpenAITestCaller()
	msgs := []*protos.Message{
		{
			Role: "assistant",
			Message: &protos.Message_Assistant{
				Assistant: &protos.AssistantMessage{
					ToolCalls: []*protos.ToolCall{
						{
							Id: "",
							Function: &protos.FunctionCall{
								Name:      "get_weather",
								Arguments: `{}`,
							},
						},
					},
				},
			},
		},
		{
			Role: "tool",
			Message: &protos.Message_Tool{
				Tool: &protos.ToolMessage{
					Tools: []*protos.ToolMessage_Tool{
						{Id: "", Name: "get_weather", Content: `{"temp":72}`},
					},
				},
			},
		},
	}

	history := caller.BuildHistory(msgs)
	assert.Empty(t, history)
}

func TestGetResponseOptions_MapsModelParametersAndTools(t *testing.T) {
	caller := newOpenAITestCaller()
	opts := &internal_callers.ChatCompletionOptions{
		AIOptions: internal_callers.AIOptions{
			ModelParameter: map[string]*anypb.Any{
				"model.name":                  mustAnyValue(t, "gpt-5.4"),
				"model.user":                  mustAnyValue(t, "user-123"),
				"model.reasoning_effort":      mustAnyValue(t, "high"),
				"model.service_tier":          mustAnyValue(t, "auto"),
				"model.top_logprobs":          mustAnyValue(t, float64(2)),
				"model.metadata":              mustAnyValue(t, `{"env":"test"}`),
				"model.temperature":           mustAnyValue(t, 0.2),
				"model.top_p":                 mustAnyValue(t, 0.9),
				"model.max_completion_tokens": mustAnyValue(t, float64(512)),
				"model.tool_choice":           mustAnyValue(t, "required"),
				"model.response_format": mustAnyValue(t, map[string]interface{}{
					"type": "json_schema",
					"json_schema": map[string]interface{}{
						"name":   "weather_response",
						"strict": true,
						"schema": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"temp": map[string]interface{}{"type": "number"},
							},
						},
					},
				}),
			},
		},
		ToolDefinitions: []*internal_callers.ToolDefinition{
			{
				Type: "function",
				Function: &internal_callers.FunctionDefinition{
					Name:        "get_weather",
					Description: "Get weather by city",
					Parameters: &internal_callers.FunctionParameter{
						Type: "object",
						Properties: map[string]internal_callers.FunctionParameterProperty{
							"city": {Type: "string"},
						},
					},
				},
			},
		},
	}

	rst := caller.getResponseOptions(opts, false)

	assert.Equal(t, shared.ResponsesModel("gpt-5.4"), rst.Model)
	assert.True(t, rst.User.Valid())
	assert.Equal(t, "user-123", rst.User.Value)
	assert.Equal(t, shared.ReasoningEffort("high"), rst.Reasoning.Effort)
	assert.Equal(t, responses.ResponseNewParamsServiceTierAuto, rst.ServiceTier)
	assert.True(t, rst.TopLogprobs.Valid())
	assert.Equal(t, int64(2), rst.TopLogprobs.Value)
	assert.Equal(t, shared.Metadata{"env": "test"}, rst.Metadata)
	assert.True(t, rst.Temperature.Valid())
	assert.Equal(t, 0.2, rst.Temperature.Value)
	assert.True(t, rst.TopP.Valid())
	assert.Equal(t, 0.9, rst.TopP.Value)
	assert.True(t, rst.MaxOutputTokens.Valid())
	assert.Equal(t, int64(512), rst.MaxOutputTokens.Value)
	assert.True(t, rst.ToolChoice.OfToolChoiceMode.Valid())
	assert.Equal(t, responses.ToolChoiceOptionsRequired, rst.ToolChoice.OfToolChoiceMode.Value)

	require.Len(t, rst.Tools, 1)
	require.NotNil(t, rst.Tools[0].OfFunction)
	assert.Equal(t, "get_weather", rst.Tools[0].OfFunction.Name)
	assert.True(t, rst.Tools[0].OfFunction.Description.Valid())
	assert.Equal(t, "Get weather by city", rst.Tools[0].OfFunction.Description.Value)
	assert.Equal(t, "object", rst.Tools[0].OfFunction.Parameters["type"])

	require.NotNil(t, rst.Text.Format.OfJSONSchema)
	assert.Equal(t, "weather_response", rst.Text.Format.OfJSONSchema.Name)
	assert.True(t, rst.Text.Format.OfJSONSchema.Strict.Valid())
	assert.True(t, rst.Text.Format.OfJSONSchema.Strict.Value)
	assert.Equal(t, "object", rst.Text.Format.OfJSONSchema.Schema["type"])
}

func TestGetResponseUsages_MapsTokenMetrics(t *testing.T) {
	caller := newOpenAITestCaller()
	metrics := caller.GetResponseUsages(responses.ResponseUsage{
		InputTokens:  120,
		OutputTokens: 45,
		TotalTokens:  165,
	})

	require.Len(t, metrics, 3)
	assert.Equal(t, "45", metrics[0].GetValue())
	assert.Equal(t, "120", metrics[1].GetValue())
	assert.Equal(t, "165", metrics[2].GetValue())
}
