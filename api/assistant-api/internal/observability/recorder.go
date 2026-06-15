// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/validator"
)

type Recorder interface {
	Record(ctx context.Context, scope Scope, record ...Record) error
	AddCollectors(collectors ...Collector) error
	Close(ctx context.Context) error
}

const (
	// Queue capacity is fixed by platform contract.
	recorderQueueSize                      = 2048
	defaultCloseGracePeriod  time.Duration = 0
	recorderCloseGracePeriod               = 5 * time.Second
)

type recorderOptions struct {
	logger           commons.Logger
	auth             types.SimplePrinciple
	globalScope      GlobalScope
	context          Context
	clock            func() time.Time
	closeGracePeriod *time.Duration
	collectors       []Collector
}

type Option interface {
	apply(*recorderOptions)
}

type funcOption struct {
	applyFunc func(*recorderOptions)
}

func (option *funcOption) apply(recorderOptions *recorderOptions) {
	option.applyFunc(recorderOptions)
}

func newOption(applyFunc func(*recorderOptions)) Option {
	return &funcOption{applyFunc: applyFunc}
}

func WithAuth(auth types.SimplePrinciple) Option {
	return newOption(func(recorderOptions *recorderOptions) {
		recorderOptions.auth = auth
	})
}

func WithLogger(logger commons.Logger) Option {
	return newOption(func(recorderOptions *recorderOptions) {
		recorderOptions.logger = logger
	})
}

func WithGlobalScope(scope GlobalScope) Option {
	return newOption(func(recorderOptions *recorderOptions) {
		recorderOptions.globalScope = scope
	})
}

func WithScope(scope GlobalScope) Option {
	return WithGlobalScope(scope)
}

func WithContext(ctx context.Context) Option {
	return newOption(func(recorderOptions *recorderOptions) {
		if traceID, ok := ctx.Value(types.REQUEST_ID_KEY).(string); ok {
			recorderOptions.context.TraceID = traceID
			return
		}
		recorderOptions.context.TraceID = uuid.New().String()
	})
}

func WithClock(clock func() time.Time) Option {
	return newOption(func(recorderOptions *recorderOptions) {
		recorderOptions.clock = clock
	})
}

func WithGracePeriod() Option {
	return newOption(func(recorderOptions *recorderOptions) {
		closeGracePeriod := recorderCloseGracePeriod
		recorderOptions.closeGracePeriod = &closeGracePeriod
	})
}

func WithCustomGracePeriod(closeGracePeriod time.Duration) Option {
	return newOption(func(recorderOptions *recorderOptions) {
		recorderOptions.closeGracePeriod = &closeGracePeriod
	})
}

func WithCollector(collector Collector) Option {
	return WithCollectors(collector)
}

func WithCollectors(collectors ...Collector) Option {
	return newOption(func(recorderOptions *recorderOptions) {
		recorderOptions.collectors = append(recorderOptions.collectors, collectors...)
	})
}

type recorder struct {
	logger           commons.Logger
	auth             types.SimplePrinciple
	globalScope      GlobalScope
	context          Context
	clock            func() time.Time
	closeGracePeriod time.Duration
	fanout           *Collectors
	operationQueue   chan interface{}
	done             chan struct{}
	closeRequested   bool
	closed           bool
	mu               sync.RWMutex
	errMu            sync.Mutex
	errs             []error
}

type addCollectorOperation struct {
	collector Collector
}

type recordOperation struct {
	observation observation
}

type closeOperation struct{}

type observation struct {
	scope   Scope
	context Context
	record  Record
}

var (
	ErrRecorderClosed = errors.New("observability: recorder is closed")
	ErrBufferFull     = errors.New("observability: recorder buffer is full")
)

func New(options ...Option) Recorder {
	var resolvedOptions recorderOptions
	for _, option := range options {
		if !validator.NonNil(option) {
			continue
		}
		option.apply(&resolvedOptions)
	}

	globalScope := resolvedOptions.globalScope
	if validator.NonNil(resolvedOptions.auth) {
		if pid := resolvedOptions.auth.GetCurrentProjectId(); pid != nil {
			globalScope.ProjectID = *pid
		}
		if oid := resolvedOptions.auth.GetCurrentOrganizationId(); oid != nil {
			globalScope.OrganizationID = *oid
		}
	}
	clock := resolvedOptions.clock
	if clock == nil {
		clock = time.Now
	}
	closeGracePeriod := defaultCloseGracePeriod
	if resolvedOptions.closeGracePeriod != nil {
		closeGracePeriod = *resolvedOptions.closeGracePeriod
	}
	r := &recorder{
		logger:           resolvedOptions.logger,
		auth:             resolvedOptions.auth,
		globalScope:      globalScope,
		context:          resolvedOptions.context,
		clock:            clock,
		closeGracePeriod: closeGracePeriod,
		fanout:           NewCollectors(),
		operationQueue:   make(chan interface{}, recorderQueueSize),
		done:             make(chan struct{}),
	}
	go r.run()
	for _, collector := range resolvedOptions.collectors {
		r.operationQueue <- addCollectorOperation{collector: collector}
	}
	return r
}

func (r *recorder) Record(ctx context.Context, scope Scope, records ...Record) error {
	for _, record := range records {
		normalizedObservation, err := r.normalize(scope, record)
		if err != nil {
			return err
		}
		normalizedObservation.context = r.context
		normalizedObservation.context.Auth = r.auth
		if traceID, ok := ctx.Value(types.REQUEST_ID_KEY).(string); ok && traceID != "" {
			normalizedObservation.context.TraceID = traceID
		}
		if normalizedObservation.context.TraceID == "" {
			normalizedObservation.context.TraceID = uuid.New().String()
		}

		r.mu.RLock()
		if r.closed || (r.closeRequested && r.closeGracePeriod <= 0) {
			r.mu.RUnlock()
			return ErrRecorderClosed
		}
		select {
		case r.operationQueue <- recordOperation{observation: normalizedObservation}:
			r.mu.RUnlock()
		default:
			r.mu.RUnlock()
			return ErrBufferFull
		}
	}
	return nil
}

func (r *recorder) AddCollectors(collectors ...Collector) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closeRequested || r.closed {
		return ErrRecorderClosed
	}
	for _, collector := range collectors {
		select {
		case r.operationQueue <- addCollectorOperation{collector: collector}:
		default:
			return ErrBufferFull
		}
	}
	return nil
}

func (r *recorder) normalize(scope Scope, record Record) (observation, error) {
	if !validator.NonNil(record) {
		return observation{}, errors.New("observability: record is nil")
	}
	now := r.clock()
	if !validator.NonNil(scope) {
		return observation{}, errors.New("observability: scope is required")
	}
	scope = scope.WithGlobal(r.globalScope)
	if err := ValidateScope(scope); err != nil {
		return observation{}, err
	}

	switch typed := record.(type) {
	case RecordLog:
		if !validator.NotBlank(typed.Message) {
			return observation{}, errors.New("observability: log message is required")
		}
		switch typed.Level {
		case "":
			typed.Level = LevelInfo
		case LevelInfo, LevelError, LevelDebug, LevelCritical:
		default:
			return observation{}, fmt.Errorf("observability: invalid log level %q", typed.Level)
		}
		if typed.OccurredAt.IsZero() {
			typed.OccurredAt = now
		}
		typed.Attributes = typed.Attributes.Clone()
		return observation{scope: scope, record: typed}, nil
	case RecordEvent:
		if !validator.NotBlank(typed.Event.String()) {
			return observation{}, errors.New("observability: event is required")
		}
		if typed.Component == "" {
			typed.Component = typed.Event.Component()
		}
		if typed.Component == ComponentUnknown {
			return observation{}, fmt.Errorf("observability: component is required for event %q", typed.Event)
		}
		if typed.OccurredAt.IsZero() {
			typed.OccurredAt = now
		}
		typed.Attributes = typed.Attributes.Clone()
		return observation{scope: scope, record: typed}, nil
	case RecordMetric:
		if len(typed.Metrics) == 0 {
			return observation{}, errors.New("observability: at least one metric is required")
		}
		for i, metric := range typed.Metrics {
			if !validator.NotBlank(metric.Name) {
				return observation{}, fmt.Errorf("observability: metric[%d] name is required", i)
			}
		}
		if typed.OccurredAt.IsZero() {
			typed.OccurredAt = now
		}
		typed.Attributes = typed.Attributes.Clone()
		return observation{scope: scope, record: typed}, nil
	case RecordMetadata:
		if len(typed.Metadata) == 0 {
			return observation{}, errors.New("observability: at least one metadata entry is required")
		}
		for i, metadata := range typed.Metadata {
			if !validator.NotBlank(metadata.Key) {
				return observation{}, fmt.Errorf("observability: metadata[%d] key is required", i)
			}
		}
		if typed.OccurredAt.IsZero() {
			typed.OccurredAt = now
		}
		return observation{scope: scope, record: typed}, nil
	case RecordUsage:
		if typed.Duration <= 0 {
			return observation{}, errors.New("observability: usage duration must be greater than zero")
		}
		if typed.OccurredAt.IsZero() {
			typed.OccurredAt = now
		}
		typed.Attributes = typed.Attributes.Clone()
		return observation{scope: scope, record: typed}, nil
	case RecordWebhook:
		if !validator.NotBlank(typed.Event.String()) {
			return observation{}, errors.New("observability: webhook event is required")
		}
		if typed.Payload == nil {
			typed.Payload = map[string]interface{}{}
		}
		if typed.OccurredAt.IsZero() {
			typed.OccurredAt = now
		}
		return observation{scope: scope, record: typed}, nil
	default:
		return observation{}, fmt.Errorf("observability: unsupported record type %T", record)
	}
}

func (r *recorder) run() {
	// The worker is the only place that touches collectors, preserving operation order.
	defer close(r.done)
	for queuedOperation := range r.operationQueue {
		ctx := context.Background()
		switch typedOperation := queuedOperation.(type) {
		case addCollectorOperation:
			r.fanout.AddCollectors(typedOperation.collector)
		case recordOperation:
			normalizedObservation := typedOperation.observation
			err := r.fanout.Collect(ctx, normalizedObservation.scope, normalizedObservation.context, normalizedObservation.record)
			if err != nil {
				r.addError(err)
			}
		case closeOperation:
			if err := r.fanout.Close(ctx); err != nil {
				r.addError(err)
			}
			return
		default:
			r.addError(fmt.Errorf("observability: unsupported recorder operation %T", queuedOperation))
		}
	}
}

func (r *recorder) Close(ctx context.Context) error {
	// Close is best-effort async; caller context must not shorten the grace period.
	ownsClose := false
	r.mu.Lock()
	if !r.closeRequested && !r.closed {
		r.closeRequested = true
		ownsClose = true
	}
	r.mu.Unlock()

	if !ownsClose {
		return nil
	}

	go func() {
		if r.closeGracePeriod > 0 {
			closeGracePeriodTimer := time.NewTimer(r.closeGracePeriod)
			<-closeGracePeriodTimer.C
		}

		r.mu.Lock()
		if !r.closed {
			r.closed = true
		}
		r.mu.Unlock()

		select {
		case r.operationQueue <- closeOperation{}:
		case <-r.done:
		}
	}()
	return nil
}

func (r *recorder) addError(err error) {
	// Observability failures are absorbed by contract and logged when possible.
	r.errMu.Lock()
	defer r.errMu.Unlock()
	r.errs = append(r.errs, err)
	if validator.NonNil(r.logger) {
		r.logger.Warnf("observability recorder absorbed failure: %v", err)
	}
}

func (r *recorder) errors() error {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	return errors.Join(r.errs...)
}
