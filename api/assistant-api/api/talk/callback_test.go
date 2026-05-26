package assistant_talk_api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rapidaai/api/assistant-api/config"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	channel_telephony "github.com/rapidaai/api/assistant-api/internal/channel/telephony"
	observe "github.com/rapidaai/api/assistant-api/internal/observe"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
)

type callbackTestStore struct {
	getByChannelUUIDCalled bool
	provider               string
	assistantID            uint64
	channelUUID            string
	cc                     *callcontext.CallContext
	err                    error
	updateFieldCalled      bool
	updatedContextID       string
	updatedField           string
	updatedValue           string
}

func (s *callbackTestStore) Save(ctx context.Context, cc *callcontext.CallContext) (string, error) {
	return "", errors.New("not implemented")
}

func (s *callbackTestStore) Get(ctx context.Context, contextID string) (*callcontext.CallContext, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.cc, nil
}

func (s *callbackTestStore) GetByChannelUUID(ctx context.Context, provider string, assistantID uint64, channelUUID string) (*callcontext.CallContext, error) {
	s.getByChannelUUIDCalled = true
	s.provider = provider
	s.assistantID = assistantID
	s.channelUUID = channelUUID
	if s.err != nil {
		return nil, s.err
	}
	return s.cc, nil
}

func (s *callbackTestStore) Claim(ctx context.Context, contextID string) (*callcontext.CallContext, error) {
	return nil, errors.New("not implemented")
}

func (s *callbackTestStore) UpdateField(ctx context.Context, contextID, field, value string) error {
	s.updateFieldCalled = true
	s.updatedContextID = contextID
	s.updatedField = field
	s.updatedValue = value
	return nil
}

type callbackTestPersister struct {
	metrics  []*types.Metric
	metadata []*types.Metadata
}

func (p *callbackTestPersister) PersistMetrics(ctx context.Context, auth types.SimplePrinciple, assistantID, conversationID uint64, metrics []*types.Metric) error {
	p.metrics = append(p.metrics, metrics...)
	return nil
}

func (p *callbackTestPersister) PersistMetadata(ctx context.Context, auth types.SimplePrinciple, assistantID, conversationID uint64, metadata []*types.Metadata) error {
	p.metadata = append(p.metadata, metadata...)
	return nil
}

func newCallbackTestAPI(t *testing.T, store *callbackTestStore) *ConversationApi {
	return newCallbackTestAPIWithPersister(t, store, nil)
}

func newCallbackTestAPIWithPersister(t *testing.T, store *callbackTestStore, persist *callbackTestPersister) *ConversationApi {
	t.Helper()

	logger, err := commons.NewApplicationLogger(
		commons.EnableConsole(true),
		commons.EnableFile(false),
		commons.Level("error"),
	)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	cfg := &config.AssistantConfig{}
	var persistArg observe.ConversationPersister
	if persist != nil {
		persistArg = persist
	}
	return &ConversationApi{
		cfg:              cfg,
		logger:           logger,
		callContextStore: store,
		inboundDispatcher: channel_telephony.NewInboundDispatcher(channel_telephony.TelephonyDispatcherDeps{
			Cfg:    cfg,
			Logger: logger,
			Store:  store,
		}),
		conversationObserver: NewConversationObserver(cfg, logger, nil, persistArg),
	}
}

func TestUnviersalCallback_VonageResolvesCallContextByUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &callbackTestStore{
		cc: &callcontext.CallContext{
			ContextID:      "ctx-1",
			AssistantID:    2276223893754609664,
			ConversationID: 123,
			Provider:       "vonage",
			Direction:      "outbound",
			ChannelUUID:    "6fbb257e-75e5-4c68-a5f2-bc560fa200d3",
		},
	}
	persist := &callbackTestPersister{}
	api := newCallbackTestAPIWithPersister(t, store, persist)

	req := httptest.NewRequest(http.MethodGet, "/vonage/event/2276223893754609664?status=completed&uuid=6fbb257e-75e5-4c68-a5f2-bc560fa200d3", nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{
		{Key: "telephony", Value: "vonage"},
		{Key: "assistantId", Value: "2276223893754609664"},
	}
	c.Request = req

	api.UnviersalCallback(c)

	if c.Writer.Status() != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", c.Writer.Status())
	}
	if !store.getByChannelUUIDCalled {
		t.Fatal("expected GetByChannelUUID to be called")
	}
	if store.provider != "vonage" {
		t.Fatalf("expected provider vonage, got %s", store.provider)
	}
	if store.assistantID != 2276223893754609664 {
		t.Fatalf("expected assistant id 2276223893754609664, got %d", store.assistantID)
	}
	if store.channelUUID != "6fbb257e-75e5-4c68-a5f2-bc560fa200d3" {
		t.Fatalf("expected vonage uuid, got %s", store.channelUUID)
	}
}

func TestUnviersalCallback_VonageRejectsMissingUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &callbackTestStore{}
	api := newCallbackTestAPI(t, store)

	req := httptest.NewRequest(http.MethodGet, "/vonage/event/2276223893754609664?status=completed", nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{
		{Key: "telephony", Value: "vonage"},
		{Key: "assistantId", Value: "2276223893754609664"},
	}
	c.Request = req

	api.UnviersalCallback(c)

	if c.Writer.Status() != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", c.Writer.Status())
	}
	if store.getByChannelUUIDCalled {
		t.Fatal("expected GetByChannelUUID not to be called")
	}
}

func TestUnviersalCallback_VonageFailedEventPersistsMetricsAndReason(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &callbackTestStore{
		cc: &callcontext.CallContext{
			ContextID:      "ctx-1",
			AssistantID:    2276223893754609664,
			ConversationID: 123,
			Provider:       "vonage",
			Direction:      "outbound",
			ChannelUUID:    "f9abbc8a-457a-40b2-a8bf-3717c0abc918",
		},
	}
	persist := &callbackTestPersister{}
	api := newCallbackTestAPIWithPersister(t, store, persist)

	req := httptest.NewRequest(http.MethodGet, "/vonage/event/2276223893754609664?status=completed&uuid=f9abbc8a-457a-40b2-a8bf-3717c0abc918&detail=remote_busy&duration=0&price=0.00000000", nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{
		{Key: "telephony", Value: "vonage"},
		{Key: "assistantId", Value: "2276223893754609664"},
	}
	c.Request = req

	api.UnviersalCallback(c)

	if c.Writer.Status() != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", c.Writer.Status())
	}
	if len(persist.metadata) != 1 || persist.metadata[0].Key != "disconnect_reason" || persist.metadata[0].Value != "remote_busy" {
		t.Fatalf("expected disconnect_reason metadata, got %+v", persist.metadata)
	}
	assertMetricValue(t, persist.metrics, "status", "FAILED")
	assertMetricValue(t, persist.metrics, observe.MetricTelephonyDuration, "0")
	assertMetricValue(t, persist.metrics, "telephony.price", "0.00000000")
	if store.updateFieldCalled {
		t.Fatal("expected failed completed event not to mark context completed")
	}
}

func TestCallbackByContext_VonageCompletedEventMarksContextComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &callbackTestStore{
		cc: &callcontext.CallContext{
			ContextID:      "ctx-1",
			AssistantID:    2276223893754609664,
			ConversationID: 123,
			Provider:       "vonage",
			Direction:      "outbound",
			ChannelUUID:    "f9abbc8a-457a-40b2-a8bf-3717c0abc918",
		},
	}
	persist := &callbackTestPersister{}
	api := newCallbackTestAPIWithPersister(t, store, persist)

	req := httptest.NewRequest(http.MethodGet, "/vonage/ctx/ctx-1/event?status=completed&uuid=f9abbc8a-457a-40b2-a8bf-3717c0abc918&duration=12&price=0.12000000", nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "contextId", Value: "ctx-1"}}
	c.Request = req

	api.CallbackByContext(c)

	if c.Writer.Status() != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", c.Writer.Status())
	}
	if !store.updateFieldCalled {
		t.Fatal("expected context status update")
	}
	if store.updatedContextID != "ctx-1" || store.updatedField != "status" || store.updatedValue != callcontext.StatusCompleted {
		t.Fatalf("unexpected update: context=%s field=%s value=%s", store.updatedContextID, store.updatedField, store.updatedValue)
	}
	assertMetricValue(t, persist.metrics, observe.MetricTelephonyDuration, "12000000000")
	assertMetricValue(t, persist.metrics, observe.MetricTelephonyPrice, "0.12000000")
}

func assertMetricValue(t *testing.T, metrics []*types.Metric, name, value string) {
	t.Helper()
	for _, metric := range metrics {
		if metric.Name == name && metric.Value == value {
			return
		}
	}
	t.Fatalf("expected metric %s=%s, got %+v", name, value, metrics)
}
