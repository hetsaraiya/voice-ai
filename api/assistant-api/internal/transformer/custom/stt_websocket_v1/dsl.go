// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_transformer_custom_stt_websocket_v1

import (
	"encoding/base64"

	internal_transformer_custom_websocketdsl "github.com/rapidaai/api/assistant-api/internal/transformer/custom/internal/websocketdsl"
)

type queryScope struct {
	Model      string
	Language   string
	Encoding   string
	SampleRate int
}

type outboundRequest struct {
	Frame string
	Body  any
}

type responseFrame = internal_transformer_custom_websocketdsl.Frame

type responseOutcome struct {
	Matched bool

	Script     string
	Interim    bool
	Confidence float64
	Language   string
	ErrorText  string
}

type dslEngine struct {
	config *Config
	core   *internal_transformer_custom_websocketdsl.Core
}

func (config *Config) newEngine() *dslEngine {
	return &dslEngine{
		config: config,
		core:   internal_transformer_custom_websocketdsl.NewCore("custom-stt websocket_v1"),
	}
}

func (config *Config) newQueryScope() queryScope {
	return queryScope{
		Model:      config.Model,
		Language:   config.Language,
		Encoding:   config.Encoding,
		SampleRate: config.SampleRate,
	}
}

func (config *Config) newRequestScope(packet string, contextID string, audio []byte) map[string]any {
	scope := map[string]any{
		"config": map[string]any{
			"model":    config.Model,
			"language": config.Language,
			"audio": map[string]any{
				"encoding":    config.Encoding,
				"sample_rate": config.SampleRate,
			},
		},
		"packet": map[string]any{
			"kind":       packet,
			"context_id": contextID,
		},
	}

	if len(audio) > 0 {
		scope["packet"].(map[string]any)["audio"] = map[string]any{
			"bytes":  append([]byte(nil), audio...),
			"base64": base64.StdEncoding.EncodeToString(audio),
		}
	}

	return scope
}

func (engine *dslEngine) BuildConnectionURL(scope queryScope) (string, error) {
	return engine.core.BuildConnectionURL(engine.config.BaseURL, engine.config.QueryParams, func(name string) (any, error) {
		return engine.resolveQueryVariable(name, scope)
	})
}

func (engine *dslEngine) EvaluateRequestRules(packet string, scope map[string]any) ([]outboundRequest, error) {
	requests := make([]outboundRequest, 0, len(engine.config.RequestRules))
	for _, rule := range engine.config.RequestRules {
		if !engine.core.MatchRequestWhen(rule.When, packet) {
			continue
		}

		body, err := engine.core.EvalRequestRuleBody(rule.Send.Body, scope)
		if err != nil {
			return nil, err
		}

		switch rule.Send.Frame {
		case frameTypeBinary:
			payload, err := engine.core.ToBytes(body)
			if err != nil {
				return nil, err
			}
			requests = append(requests, outboundRequest{
				Frame: frameTypeBinary,
				Body:  payload,
			})
		case frameTypeText:
			payload, err := engine.core.ToString(body)
			if err != nil {
				return nil, err
			}
			requests = append(requests, outboundRequest{
				Frame: frameTypeText,
				Body:  payload,
			})
		case frameTypeJSON:
			requests = append(requests, outboundRequest{
				Frame: frameTypeJSON,
				Body:  body,
			})
		default:
			return nil, engine.core.Errorf("unsupported request frame %q", rule.Send.Frame)
		}
	}

	return requests, nil
}

func (engine *dslEngine) HasRequestRules(packet string) bool {
	for _, rule := range engine.config.RequestRules {
		if engine.core.MatchRequestWhen(rule.When, packet) {
			return true
		}
	}
	return false
}

func (engine *dslEngine) resolveQueryVariable(name string, scope queryScope) (any, error) {
	switch name {
	case "model":
		return scope.Model, nil
	case "language":
		return scope.Language, nil
	case "encoding":
		return scope.Encoding, nil
	case "sample_rate":
		return scope.SampleRate, nil
	default:
		return nil, engine.core.Errorf("unknown variable %q", name)
	}
}

func (engine *dslEngine) ParseFrame(messageType int, payload []byte) (responseFrame, error) {
	frame, err := engine.core.ParseFrame(messageType, payload, func(currentType int) bool {
		return currentType == 2
	})
	if err != nil {
		return responseFrame{}, err
	}

	// STT response rules use JSON-path extraction for structured payloads and
	// text-frame extraction for raw transcript chunks. Upstreams that send quoted
	// text chunks (JSON string primitives) should map to text-frame behavior.
	if frame.Kind != frameTypeJSON {
		return frame, nil
	}

	if _, isJSONObject := frame.JSON.(map[string]any); isJSONObject {
		return frame, nil
	}

	if textValue, isJSONString := frame.JSON.(string); isJSONString {
		frame.Kind = frameTypeText
		frame.JSON = nil
		frame.Text = textValue
		return frame, nil
	}

	frame.Kind = frameTypeText
	frame.JSON = nil
	return frame, nil
}

func (engine *dslEngine) EvaluateResponse(frame responseFrame) (responseOutcome, error) {
	for _, rule := range engine.config.ResponseRules {
		matched, err := engine.core.MatchWhen(
			internal_transformer_custom_websocketdsl.When{
				Frame:  rule.When.Frame,
				Path:   rule.When.Path,
				Equals: rule.When.Equals,
			},
			frame,
		)
		if err != nil {
			return responseOutcome{}, err
		}
		if !matched {
			continue
		}

		outcome, err := engine.emitOutcome(rule.Emit, frame)
		if err != nil {
			return responseOutcome{}, err
		}
		outcome.Matched = true
		return outcome, nil
	}
	return responseOutcome{}, nil
}

func (engine *dslEngine) emitOutcome(emit map[string]any, frame responseFrame) (responseOutcome, error) {
	outcome := responseOutcome{}

	for key, expr := range emit {
		value, err := engine.core.EvalResponseExpr(expr, frame, responseContract)
		if err != nil {
			return responseOutcome{}, err
		}

		switch key {
		case "script":
			script, err := engine.core.ToString(value)
			if err != nil {
				return responseOutcome{}, err
			}
			outcome.Script = script
		case "confidence":
			confidence, err := engine.core.ToFloat64(value)
			if err != nil {
				return responseOutcome{}, err
			}
			outcome.Confidence = confidence
		case "language":
			language, err := engine.core.ToString(value)
			if err != nil {
				return responseOutcome{}, err
			}
			outcome.Language = language
		case "interim":
			interim, err := engine.core.ToBool(value)
			if err != nil {
				return responseOutcome{}, err
			}
			outcome.Interim = interim
		case "error":
			errorText, err := engine.core.ToString(value)
			if err != nil {
				return responseOutcome{}, err
			}
			outcome.ErrorText = errorText
		}
	}

	return outcome, nil
}
