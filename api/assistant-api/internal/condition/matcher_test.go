// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package condition

import "testing"

func TestParse_ValidJSON_ReturnsTypedRules(t *testing.T) {
	raw := `[{"key":"source","condition":"=","value":"phone"},{"key":"mode","condition":"=","value":"voice"},{"key":"direction","condition":"=","value":"inbound"}]`
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	rules := parsed.Rules()
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
	if _, ok := rules[0].(SourceRule); !ok {
		t.Fatalf("expected first rule to be SourceRule, got %T", rules[0])
	}
	if _, ok := rules[1].(ModeRule); !ok {
		t.Fatalf("expected second rule to be ModeRule, got %T", rules[1])
	}
	if _, ok := rules[2].(DirectionRule); !ok {
		t.Fatalf("expected third rule to be DirectionRule, got %T", rules[2])
	}
}

func TestParse_EmptyRaw_ReturnsNoRules(t *testing.T) {
	parsed, err := Parse("")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	rules := parsed.Rules()
	if len(rules) != 0 {
		t.Fatalf("expected no rules for empty raw, got %d", len(rules))
	}
}

func TestParse_InvalidJSON_ReturnsError(t *testing.T) {
	_, err := Parse(`{"key":"source","condition":"=","value":"phone"}`)
	if err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestParse_EmptyArray_ReturnsError(t *testing.T) {
	_, err := Parse(`[]`)
	if err == nil {
		t.Fatalf("expected empty-array parse error")
	}
}

func TestParse_UnsupportedKey_ReturnsError(t *testing.T) {
	_, err := Parse(`[{"key":"region","condition":"=","value":"sg"}]`)
	if err == nil {
		t.Fatalf("expected unsupported key parse error")
	}
}

func TestParse_ConversationModeKey_ReturnsModeRule(t *testing.T) {
	parsed, err := Parse(`[{"key":"conversation_mode","condition":"=","value":"voice"}]`)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	rules := parsed.Rules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if _, ok := rules[0].(ModeRule); !ok {
		t.Fatalf("expected conversation_mode to parse as ModeRule, got %T", rules[0])
	}
}

func TestParseRun_EmptyCondition_Allows(t *testing.T) {
	parsed, err := Parse("")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	ok, err := parsed.Run(
		ConditionValue{RuleType: RuleTypeSource, Value: "phone-call"},
		ConditionValue{RuleType: RuleTypeMode, Value: "audio"},
		ConditionValue{RuleType: RuleTypeDirection, Value: "inbound"},
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !ok {
		t.Fatalf("expected empty condition to allow")
	}
}

func TestParseRun_SourceModeDirection_Match(t *testing.T) {
	raw := `[{"key":"source","condition":"=","value":"phone"},{"key":"mode","condition":"=","value":"voice"},{"key":"direction","condition":"=","value":"inbound"}]`
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("expected nil parse error, got %v", err)
	}
	ok, err := parsed.Run(
		ConditionValue{RuleType: RuleTypeSource, Value: "phone-call"},
		ConditionValue{RuleType: RuleTypeMode, Value: "audio"},
		ConditionValue{RuleType: RuleTypeDirection, Value: "inbound"},
	)
	if err != nil {
		t.Fatalf("expected nil run error, got %v", err)
	}
	if !ok {
		t.Fatalf("expected parse().run() to match")
	}
}

func TestParseRun_SourceConversationModeDirection_Match(t *testing.T) {
	raw := `[{"key":"source","condition":"=","value":"phone"},{"key":"conversation_mode","condition":"=","value":"voice"},{"key":"direction","condition":"=","value":"outbound"}]`
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("expected nil parse error, got %v", err)
	}
	ok, err := parsed.Run(
		ConditionValue{RuleType: RuleTypeSource, Value: "phone-call"},
		ConditionValue{RuleType: RuleTypeMode, Value: "audio"},
		ConditionValue{RuleType: RuleTypeDirection, Value: "outbound"},
	)
	if err != nil {
		t.Fatalf("expected nil run error, got %v", err)
	}
	if !ok {
		t.Fatalf("expected UI condition keys to match")
	}
}

func TestParseRun_SourceMismatch_Blocks(t *testing.T) {
	parsed, err := Parse(`[{"key":"source","condition":"=","value":"sdk"}]`)
	if err != nil {
		t.Fatalf("expected nil parse error, got %v", err)
	}
	ok, err := parsed.Run(ConditionValue{RuleType: RuleTypeSource, Value: "phone-call"})
	if err != nil {
		t.Fatalf("expected nil run error, got %v", err)
	}
	if ok {
		t.Fatalf("expected source mismatch to block")
	}
}

func TestParseRun_ModeMismatch_Blocks(t *testing.T) {
	parsed, err := Parse(`[{"key":"mode","condition":"=","value":"text"}]`)
	if err != nil {
		t.Fatalf("expected nil parse error, got %v", err)
	}
	ok, err := parsed.Run(ConditionValue{RuleType: RuleTypeMode, Value: "audio"})
	if err != nil {
		t.Fatalf("expected nil run error, got %v", err)
	}
	if ok {
		t.Fatalf("expected mode mismatch to block")
	}
}

func TestParseRun_DirectionMismatch_Blocks(t *testing.T) {
	parsed, err := Parse(`[{"key":"direction","condition":"=","value":"outbound"}]`)
	if err != nil {
		t.Fatalf("expected nil parse error, got %v", err)
	}
	ok, err := parsed.Run(ConditionValue{RuleType: RuleTypeDirection, Value: "inbound"})
	if err != nil {
		t.Fatalf("expected nil run error, got %v", err)
	}
	if ok {
		t.Fatalf("expected direction mismatch to block")
	}
}

func TestParseRun_AllAndBoth_Allow(t *testing.T) {
	raw := `[{"key":"source","condition":"=","value":"all"},{"key":"mode","condition":"=","value":"all"},{"key":"direction","condition":"=","value":"both"}]`
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("expected nil parse error, got %v", err)
	}
	ok, err := parsed.Run(
		ConditionValue{RuleType: RuleTypeSource, Value: "sdk"},
		ConditionValue{RuleType: RuleTypeMode, Value: "text"},
		ConditionValue{RuleType: RuleTypeDirection, Value: "outbound"},
	)
	if err != nil {
		t.Fatalf("expected nil run error, got %v", err)
	}
	if !ok {
		t.Fatalf("expected all/both to allow")
	}
}

func TestParseRun_DuplicateSourceConditions_AllThenWebPlugin(t *testing.T) {
	raw := `[{"key":"source","condition":"=","value":"all"},{"key":"source","condition":"=","value":"web_plugin"}]`
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("expected nil parse error, got %v", err)
	}

	ok, err := parsed.Run(ConditionValue{RuleType: RuleTypeSource, Value: "web-plugin"})
	if err != nil {
		t.Fatalf("expected nil error for web-plugin source, got %v", err)
	}
	if !ok {
		t.Fatalf("expected duplicate source conditions to allow web-plugin")
	}

	ok, err = parsed.Run(ConditionValue{RuleType: RuleTypeSource, Value: "sdk"})
	if err != nil {
		t.Fatalf("expected nil error for sdk source, got %v", err)
	}
	if ok {
		t.Fatalf("expected sdk source to be blocked by web_plugin condition")
	}
}

func TestParseRun_UnsupportedOperator_ReturnsError(t *testing.T) {
	parsed, err := Parse(`[{"key":"source","condition":"!=","value":"phone"}]`)
	if err != nil {
		t.Fatalf("expected nil parse error, got %v", err)
	}
	_, err = parsed.Run(ConditionValue{RuleType: RuleTypeSource, Value: "phone-call"})
	if err == nil {
		t.Fatalf("expected unsupported operator run error")
	}
}

func TestParseRun_UppercaseRuleType_Works(t *testing.T) {
	parsed, err := Parse(`[{"key":"source","condition":"=","value":"web_plugin"}]`)
	if err != nil {
		t.Fatalf("expected nil parse error, got %v", err)
	}
	ok, err := parsed.Run(ConditionValue{RuleType: "Source", Value: "web-plugin"})
	if err != nil {
		t.Fatalf("expected nil run error, got %v", err)
	}
	if !ok {
		t.Fatalf("expected uppercase RuleType to work")
	}
}

func TestRules_ReturnsCopy(t *testing.T) {
	parsed, err := Parse(`[{"key":"source","condition":"=","value":"web_plugin"}]`)
	if err != nil {
		t.Fatalf("expected nil parse error, got %v", err)
	}
	rules := parsed.Rules()
	rules[0] = ModeRule{Operator: "=", Mode: "text"}

	ok, err := parsed.Run(ConditionValue{RuleType: RuleTypeSource, Value: "web-plugin"})
	if err != nil {
		t.Fatalf("expected nil run error, got %v", err)
	}
	if !ok {
		t.Fatalf("expected matcher internals to remain unchanged after modifying returned rules")
	}
}
