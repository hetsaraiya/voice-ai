package observability_api

import (
	"reflect"
	"testing"

	"github.com/rapidaai/protos"
)

func telemetryCriteria(key string, value string) *protos.Criteria {
	return &protos.Criteria{Key: key, Value: value}
}

func TestTelemetryExactStringFilterUsesKeywordFallback(t *testing.T) {
	got := telemetryExactStringFilter("context.traceId", "trace-abc-123")
	want := map[string]interface{}{
		"bool": map[string]interface{}{
			"should": []interface{}{
				telemetryTermFilter("context.traceId.keyword", "trace-abc-123"),
				telemetryTermFilter("context.traceId", "trace-abc-123"),
			},
			"minimum_should_match": 1,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected exact string filter: got %#v want %#v", got, want)
	}
}

func TestTelemetryTraceIDFilterMatchesKnownTraceKeyVariants(t *testing.T) {
	got := telemetryTraceIDFilter("trace-abc-123")

	boolQuery, ok := got["bool"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected bool query, got %#v", got)
	}
	if boolQuery["minimum_should_match"] != 1 {
		t.Fatalf("minimum_should_match = %#v, want 1", boolQuery["minimum_should_match"])
	}

	should, ok := boolQuery["should"].([]interface{})
	if !ok {
		t.Fatalf("expected should list, got %#v", boolQuery["should"])
	}

	wantFields := []string{
		"context.traceId",
		"context.traceID",
		"context.trace_id",
	}
	if len(should) != len(wantFields) {
		t.Fatalf("trace filter count = %d, want %d", len(should), len(wantFields))
	}
	for index, field := range wantFields {
		if !reflect.DeepEqual(should[index], telemetryExactStringFilter(field, "trace-abc-123")) {
			t.Fatalf("trace filter[%d] = %#v, want exact filter for %s", index, should[index], field)
		}
	}
}

func TestTelemetrySearchFilterIncludesTraceIDMatching(t *testing.T) {
	got := telemetrySearchFilter("trace-abc-123")
	boolQuery, ok := got["bool"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected bool query, got %#v", got)
	}

	should, ok := boolQuery["should"].([]interface{})
	if !ok || len(should) != 2 {
		t.Fatalf("expected text and trace search branches, got %#v", boolQuery["should"])
	}
	if !reflect.DeepEqual(should[1], telemetryTraceIDFilter("trace-abc-123")) {
		t.Fatalf("search trace branch = %#v, want trace id filter", should[1])
	}
}

func TestTelemetryQueryPartsIgnoresEmptyCriteria(t *testing.T) {
	parts := newTelemetryQueryParts(1, 2)

	parts.applyCriteria([]*protos.Criteria{
		nil,
		telemetryCriteria("", "trace-abc-123"),
		telemetryCriteria("traceId", ""),
		telemetryCriteria("traceId", "   "),
		telemetryCriteria("   ", "trace-abc-123"),
		telemetryCriteria(" traceId ", " trace-abc-123 "),
	})

	if len(parts.filter) != 3 {
		t.Fatalf("filter count = %d, want 3", len(parts.filter))
	}
	if len(parts.must) != 0 {
		t.Fatalf("must count = %d, want 0", len(parts.must))
	}
	if len(parts.timeRange) != 0 {
		t.Fatalf("time range = %#v, want empty", parts.timeRange)
	}

	wantFilter := telemetryTraceIDFilter("trace-abc-123")
	if !reflect.DeepEqual(parts.filter[2], wantFilter) {
		t.Fatalf("trace filter = %#v, want %#v", parts.filter[2], wantFilter)
	}
}

func TestTelemetryQueryPartsSupportsQueryStyleCriteriaKeys(t *testing.T) {
	parts := newTelemetryQueryParts(1, 2)

	parts.applyCriteria([]*protos.Criteria{
		telemetryCriteria("scope", "message"),
		telemetryCriteria("message_id", "message-1"),
		telemetryCriteria("message_role", "user"),
		telemetryCriteria("conversation_id", "conversation-1"),
		telemetryCriteria("assistant_id", "assistant-1"),
		telemetryCriteria("trace_id", "trace-1"),
	})

	wantFilters := []interface{}{
		telemetryTermFilter("organizationId", uint64(1)),
		telemetryTermFilter("projectId", uint64(2)),
		telemetryExactStringFilter("scope", "message"),
		telemetryExactStringFilter("scopeAttributes.messageId", "message-1"),
		telemetryExactStringFilter("scopeAttributes.messageRole", "user"),
		telemetryExactStringFilter("scopeAttributes.assistantConversationId", "conversation-1"),
		telemetryExactStringFilter("scopeAttributes.assistantId", "assistant-1"),
		telemetryTraceIDFilter("trace-1"),
	}

	if !reflect.DeepEqual(parts.filter, wantFilters) {
		t.Fatalf("filters = %#v, want %#v", parts.filter, wantFilters)
	}
}
