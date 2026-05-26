// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package condition

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rapidaai/pkg/utils"
)

type Rule interface {
	Match(values map[RuleType]string) (bool, error)
}

type Matcher interface {
	Run(values ...ConditionValue) (bool, error)
	Rules() []Rule
}

type RuleParser interface {
	Parse(raw string) (Matcher, error)
}

type SourceRule struct {
	Operator string
	Source   string
}

func (r SourceRule) Match(values map[RuleType]string) (bool, error) {
	op := strings.TrimSpace(r.Operator)
	if op != "=" {
		return false, fmt.Errorf("unsupported condition operator for source: %s", op)
	}
	source := strings.ToLower(strings.TrimSpace(r.Source))
	if source == "all" {
		return true, nil
	}

	var expected string
	switch source {
	case "sdk":
		expected = string(utils.SDK)
	case "web_plugin", "web-plugin":
		expected = string(utils.WebPlugin)
	case "debugger":
		expected = string(utils.Debugger)
	case "phone", "phone-call":
		expected = string(utils.PhoneCall)
	case "whatsapp":
		expected = string(utils.Whatsapp)
	default:
		return false, fmt.Errorf("unsupported source condition value: %s", r.Source)
	}

	actual := strings.ToLower(strings.TrimSpace(values[RuleTypeSource]))
	if actual != expected {
		return false, nil
	}
	return true, nil
}

type ModeRule struct {
	Operator string
	Mode     string
}

func (r ModeRule) Match(values map[RuleType]string) (bool, error) {
	op := strings.TrimSpace(r.Operator)
	if op != "=" {
		return false, fmt.Errorf("unsupported condition operator for mode: %s", op)
	}

	expected := strings.ToLower(strings.TrimSpace(r.Mode))
	if expected == "audio" {
		expected = "voice"
	}
	if expected != "all" && expected != "text" && expected != "voice" {
		return false, fmt.Errorf("unsupported mode condition value: %s", r.Mode)
	}

	actual := strings.ToLower(strings.TrimSpace(values[RuleTypeMode]))
	if actual == "audio" {
		actual = "voice"
	}

	if expected != "all" && expected != actual {
		return false, nil
	}
	return true, nil
}

type DirectionRule struct {
	Operator  string
	Direction string
}

func (r DirectionRule) Match(values map[RuleType]string) (bool, error) {
	op := strings.TrimSpace(r.Operator)
	if op != "=" {
		return false, fmt.Errorf("unsupported condition operator for direction: %s", op)
	}

	expected := strings.ToLower(strings.TrimSpace(r.Direction))
	if expected == "all" {
		expected = "both"
	}
	if expected != "both" && expected != "inbound" && expected != "outbound" {
		return false, fmt.Errorf("unsupported direction condition value: %s", r.Direction)
	}

	actual := strings.ToLower(strings.TrimSpace(values[RuleTypeDirection]))
	if actual == "all" {
		actual = "both"
	}

	if expected != "both" && expected != actual {
		return false, nil
	}
	return true, nil
}

type ConditionValue struct {
	RuleType RuleType
	Value    string
}

type rawRuleEntry struct {
	Key       string `json:"key"`
	Condition string `json:"condition"`
	Value     string `json:"value"`
}

type matcher struct {
	rules []Rule
}

type RuleType string

const (
	RuleTypeSource    RuleType = "source"
	RuleTypeMode      RuleType = "mode"
	RuleTypeDirection RuleType = "direction"
)

func (m matcher) Run(values ...ConditionValue) (bool, error) {
	inputValues := make(map[RuleType]string, len(values))
	for _, entry := range values {
		key := strings.ToLower(strings.TrimSpace(string(entry.RuleType)))
		key = strings.ReplaceAll(key, "-", "_")
		switch key {
		case "source":
			inputValues[RuleTypeSource] = entry.Value
		case "mode", "conversation_mode":
			inputValues[RuleTypeMode] = entry.Value
		case "direction":
			inputValues[RuleTypeDirection] = entry.Value
		default:
			return false, fmt.Errorf("unsupported condition value type: %s", entry.RuleType)
		}
	}

	for _, rule := range m.rules {
		allowed, err := rule.Match(inputValues)
		if err != nil {
			return false, err
		}
		if !allowed {
			return false, nil
		}
	}
	return true, nil
}

func (m matcher) Rules() []Rule {
	rules := make([]Rule, len(m.rules))
	copy(rules, m.rules)
	return rules
}

type parser struct{}

func (parser) Parse(raw string) (Matcher, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return matcher{rules: nil}, nil
	}

	var entries []rawRuleEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, fmt.Errorf("failed to parse condition JSON: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("condition must include at least one entry")
	}

	parsedRules := make([]Rule, 0, len(entries))
	for _, entry := range entries {
		key := strings.ToLower(strings.TrimSpace(entry.Key))
		key = strings.ReplaceAll(key, "-", "_")
		switch key {
		case "source":
			parsedRules = append(parsedRules, SourceRule{
				Operator: entry.Condition,
				Source:   entry.Value,
			})
		case "mode", "conversation_mode":
			parsedRules = append(parsedRules, ModeRule{
				Operator: entry.Condition,
				Mode:     entry.Value,
			})
		case "direction":
			parsedRules = append(parsedRules, DirectionRule{
				Operator:  entry.Condition,
				Direction: entry.Value,
			})
		default:
			return nil, fmt.Errorf("unsupported condition key: %s", entry.Key)
		}
	}

	return matcher{rules: parsedRules}, nil
}

var DefaultParser RuleParser = parser{}

func Parse(raw string) (Matcher, error) {
	return DefaultParser.Parse(raw)
}
