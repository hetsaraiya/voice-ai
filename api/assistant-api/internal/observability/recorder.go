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

type recorderOptions struct {
	logger           commons.Logger
	auth             types.SimplePrinciple
	globalScope      GlobalScope
	context          Context
	clock            func() time.Time
	buffer           int
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

func WithBuffer(buffer int) Option {
	return newOption(func(recorderOptions *recorderOptions) {
		recorderOptions.buffer = buffer
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
	queue            chan observation
	done             chan struct{}
	closeRequested   bool
	closed           bool
	mu               sync.RWMutex
	errMu            sync.Mutex
	errs             []error
}

type observation struct {
	scope   Scope
	context Context
	record  Record
}

const defaultBufferSize = 1024

const defaultCloseGracePeriod time.Duration = 0

const recorderCloseGracePeriod = 5 * time.Second

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
	buffer := resolvedOptions.buffer
	if buffer <= 0 {
		buffer = defaultBufferSize
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
		fanout:           NewCollectors(resolvedOptions.collectors...),
		queue:            make(chan observation, buffer),
		done:             make(chan struct{}),
	}
	go r.run()
	return r
}

func (r *recorder) Record(ctx context.Context, scope Scope, records ...Record) error {
	for _, record := range records {
		item, err := r.normalize(scope, record)
		if err != nil {
			return err
		}
		item.context = r.context
		item.context.Auth = r.auth
		if traceID, ok := ctx.Value(types.REQUEST_ID_KEY).(string); ok && traceID != "" {
			item.context.TraceID = traceID
		}
		if item.context.TraceID == "" {
			item.context.TraceID = uuid.New().String()
		}

		r.mu.RLock()
		if r.closed {
			r.mu.RUnlock()
			return ErrRecorderClosed
		}
		select {
		case r.queue <- item:
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
	if r.closed {
		return ErrRecorderClosed
	}
	r.fanout.AddCollectors(collectors...)
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
	defer close(r.done)
	for item := range r.queue {
		ctx := context.Background()
		err := r.fanout.Collect(ctx, item.scope, item.context, item.record)
		if err != nil {
			r.addError(err)
		}
	}
}

func (r *recorder) Close(ctx context.Context) error {
	ownsClose := false
	var closeGracePeriodContextError error
	r.mu.Lock()
	if !r.closeRequested && !r.closed {
		r.closeRequested = true
		ownsClose = true
	}
	r.mu.Unlock()

	if !ownsClose {
		select {
		case <-r.done:
			return r.errors()
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if r.closeGracePeriod > 0 {
		closeGracePeriodTimer := time.NewTimer(r.closeGracePeriod)
		select {
		case <-closeGracePeriodTimer.C:
		case <-ctx.Done():
			if !closeGracePeriodTimer.Stop() {
				select {
				case <-closeGracePeriodTimer.C:
				default:
				}
			}
			closeGracePeriodContextError = ctx.Err()
		}
	}

	r.mu.Lock()
	if !r.closed {
		close(r.queue)
		r.closed = true
	}
	r.mu.Unlock()

	select {
	case <-r.done:
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := r.fanout.Close(ctx); err != nil {
		r.addError(err)
	}
	return errors.Join(closeGracePeriodContextError, r.errors())
}

func (r *recorder) addError(err error) {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	r.errs = append(r.errs, err)
}

func (r *recorder) errors() error {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	return errors.Join(r.errs...)
}
