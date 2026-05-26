package internal_callcontext

import (
	"context"
	"testing"
	"time"

	"github.com/rapidaai/pkg/commons"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testPostgresConnector struct {
	db *gorm.DB
}

func (t *testPostgresConnector) Connect(ctx context.Context) error    { return nil }
func (t *testPostgresConnector) Name() string                         { return "test-postgres" }
func (t *testPostgresConnector) IsConnected(ctx context.Context) bool { return t.db != nil }
func (t *testPostgresConnector) Disconnect(ctx context.Context) error { return nil }
func (t *testPostgresConnector) Query(ctx context.Context, qry string, dest interface{}) error {
	return t.db.WithContext(ctx).Raw(qry).Scan(dest).Error
}
func (t *testPostgresConnector) DB(ctx context.Context) *gorm.DB { return t.db.WithContext(ctx) }

func newTestStore(t *testing.T) (Store, context.Context) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create sqlite db: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE call_contexts (
			id INTEGER PRIMARY KEY,
			context_id TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'pending',
			assistant_id BIGINT NOT NULL,
			conversation_id BIGINT NOT NULL,
			project_id BIGINT NOT NULL DEFAULT 0,
			organization_id BIGINT NOT NULL DEFAULT 0,
			auth_token TEXT NOT NULL DEFAULT '',
			auth_type TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			direction TEXT NOT NULL DEFAULT '',
			caller_number TEXT NOT NULL DEFAULT '',
			from_number TEXT NOT NULL DEFAULT '',
			created_date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_date DATETIME DEFAULT NULL,
			assistant_provider_id BIGINT NOT NULL DEFAULT 0,
			channel_uuid TEXT NOT NULL DEFAULT ''
		)
	`).Error; err != nil {
		t.Fatalf("failed to initialize call contexts schema: %v", err)
	}

	logger, err := commons.NewApplicationLogger(
		commons.EnableConsole(true),
		commons.EnableFile(false),
		commons.Level("error"),
	)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	return NewStore(&testPostgresConnector{db: db}, logger), context.Background()
}

func TestStoreGetByChannelUUID(t *testing.T) {
	store, ctx := newTestStore(t)

	older := &CallContext{
		ContextID:      "older-context",
		AssistantID:    77,
		ConversationID: 1001,
		Provider:       "vonage",
		ChannelUUID:    "call-uuid-1",
		Direction:      "outbound",
		CreatedDate:    time.Now().Add(-time.Minute),
	}
	newer := &CallContext{
		ContextID:      "newer-context",
		AssistantID:    77,
		ConversationID: 1002,
		Provider:       "vonage",
		ChannelUUID:    "call-uuid-1",
		Direction:      "outbound",
		CreatedDate:    time.Now(),
	}
	otherProvider := &CallContext{
		ContextID:      "twilio-context",
		AssistantID:    77,
		ConversationID: 1003,
		Provider:       "twilio",
		ChannelUUID:    "call-uuid-1",
		CreatedDate:    time.Now(),
	}
	otherAssistant := &CallContext{
		ContextID:      "other-assistant-context",
		AssistantID:    88,
		ConversationID: 1004,
		Provider:       "vonage",
		ChannelUUID:    "call-uuid-1",
		CreatedDate:    time.Now(),
	}

	for _, cc := range []*CallContext{older, newer, otherProvider, otherAssistant} {
		if _, err := store.Save(ctx, cc); err != nil {
			t.Fatalf("failed to save call context %s: %v", cc.ContextID, err)
		}
	}

	got, err := store.GetByChannelUUID(ctx, "vonage", 77, "call-uuid-1")
	if err != nil {
		t.Fatalf("GetByChannelUUID returned error: %v", err)
	}
	if got.ContextID != "newer-context" {
		t.Fatalf("expected newest vonage context, got %s", got.ContextID)
	}

	if _, err := store.GetByChannelUUID(ctx, "vonage", 99, "call-uuid-1"); err == nil {
		t.Fatal("expected error for unknown assistant")
	}
	if _, err := store.GetByChannelUUID(ctx, "exotel", 77, "call-uuid-1"); err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if _, err := store.GetByChannelUUID(ctx, " ", 77, "call-uuid-1"); err == nil {
		t.Fatal("expected error for blank provider")
	}
	if _, err := store.GetByChannelUUID(ctx, "vonage", 0, "call-uuid-1"); err == nil {
		t.Fatal("expected error for zero assistant id")
	}
	if _, err := store.GetByChannelUUID(ctx, "vonage", 77, ""); err == nil {
		t.Fatal("expected error for empty channel uuid")
	}
	if _, err := store.GetByChannelUUID(ctx, "vonage", 77, " "); err == nil {
		t.Fatal("expected error for blank channel uuid")
	}
}
