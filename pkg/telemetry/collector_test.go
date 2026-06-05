package telemetry_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/telemetry"
)

type fakeExporter struct {
	mu          sync.Mutex
	eventCalls  int
	metricCalls int
	CloseCalls  int
	eventErr    error
	metricErr   error
	CloseErr    error
	blockUntil  <-chan struct{}
}

func (f *fakeExporter) ExportEvent(_ context.Context, _ telemetry.SessionMeta, _ telemetry.EventRecord) error {
	if f.blockUntil != nil {
		<-f.blockUntil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.eventCalls++
	return f.eventErr
}

func (f *fakeExporter) ExportMetric(_ context.Context, _ telemetry.SessionMeta, _ telemetry.MetricRecord) error {
	if f.blockUntil != nil {
		<-f.blockUntil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.metricCalls++
	return f.metricErr
}

func (f *fakeExporter) Close(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CloseCalls++
	return f.CloseErr
}

func testLogger(t *testing.T) commons.Logger {
	t.Helper()
	logger, err := commons.NewApplicationLogger(
		commons.Name("telemetry-collector-test"),
		commons.Level("error"),
		commons.EnableFile(false),
	)
	require.NoError(t, err)
	return logger
}

func TestCollectors_FanoutAndClose(t *testing.T) {
	logger := testLogger(t)
	meta := telemetry.SessionMeta{AssistantID: 1}

	evt1 := &fakeExporter{}
	evt2 := &fakeExporter{}
	eventCollector := telemetry.NewEventCollector(logger, meta, evt1, evt2)
	eventCollector.Collect(context.Background(), telemetry.EventRecord{Name: "session"})
	eventCollector.Close(context.Background())

	assert.Equal(t, 1, evt1.eventCalls)
	assert.Equal(t, 1, evt2.eventCalls)
	assert.Equal(t, 1, evt1.CloseCalls)
	assert.Equal(t, 1, evt2.CloseCalls)

	met1 := &fakeExporter{}
	met2 := &fakeExporter{}
	metricCollector := telemetry.NewMetricCollector(logger, meta, met1, met2)
	metricCollector.Collect(context.Background(), telemetry.ConversationMetricRecord{})
	metricCollector.Close(context.Background())

	assert.Equal(t, 1, met1.metricCalls)
	assert.Equal(t, 1, met2.metricCalls)
	assert.Equal(t, 1, met1.CloseCalls)
	assert.Equal(t, 1, met2.CloseCalls)
}

func TestCollectors_Noop(t *testing.T) {
	logger := testLogger(t)
	meta := telemetry.SessionMeta{}

	eventCollector := telemetry.NewEventCollector(logger, meta)
	metricCollector := telemetry.NewMetricCollector(logger, meta)

	assert.NotPanics(t, func() {
		eventCollector.Collect(context.Background(), telemetry.EventRecord{Name: "x"})
		metricCollector.Collect(context.Background(), telemetry.ConversationMetricRecord{})
		eventCollector.Close(context.Background())
		metricCollector.Close(context.Background())
	})
}

func TestCollectors_ExporterErrorsDoNotPanic(t *testing.T) {
	logger := testLogger(t)
	meta := telemetry.SessionMeta{AssistantID: 1}

	exp := &fakeExporter{
		eventErr:  errors.New("event export failed"),
		metricErr: errors.New("metric export failed"),
		CloseErr:  errors.New("Close failed"),
	}

	eventCollector := telemetry.NewEventCollector(logger, meta, exp)
	metricCollector := telemetry.NewMetricCollector(logger, meta, exp)

	assert.NotPanics(t, func() {
		eventCollector.Collect(context.Background(), telemetry.EventRecord{Name: "session"})
		metricCollector.Collect(context.Background(), telemetry.ConversationMetricRecord{})
		eventCollector.Close(context.Background())
		metricCollector.Close(context.Background())
	})
}

func TestCollectors_CloseWaitsForInflightExports(t *testing.T) {
	logger := testLogger(t)
	meta := telemetry.SessionMeta{AssistantID: 1}

	blocker := make(chan struct{})
	exp := &fakeExporter{blockUntil: blocker}
	eventCollector := telemetry.NewEventCollector(logger, meta, exp)
	metricCollector := telemetry.NewMetricCollector(logger, meta, exp)

	eventCollector.Collect(context.Background(), telemetry.EventRecord{Name: "session"})
	metricCollector.Collect(context.Background(), telemetry.ConversationMetricRecord{})

	done := make(chan struct{})
	go func() {
		defer close(done)
		eventCollector.Close(context.Background())
		metricCollector.Close(context.Background())
	}()

	select {
	case <-done:
		t.Fatal("Close returned before in-flight exports were unblocked")
	case <-time.After(50 * time.Millisecond):
	}

	close(blocker)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not complete after unblocking in-flight exports")
	}
}
