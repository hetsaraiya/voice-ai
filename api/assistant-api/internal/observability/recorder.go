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

	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/validator"
)

type Recorder interface {
	Record(ctx context.Context, scope Scope, record Record) error
	Close(ctx context.Context) error
}

type recorderOptions struct {
	logger      commons.Logger
	auth        types.SimplePrinciple
	globalScope GlobalScope
	clock       func() time.Time
	buffer      int
	collectors  []Collector
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

func WithCollector(collector Collector) Option {
	return WithCollectors(collector)
}

func WithCollectors(collectors ...Collector) Option {
	return newOption(func(recorderOptions *recorderOptions) {
		recorderOptions.collectors = append(recorderOptions.collectors, collectors...)
	})
}

type recorder struct {
	logger      commons.Logger
	auth        types.SimplePrinciple
	globalScope GlobalScope
	clock       func() time.Time
	fanout      Collector
	queue       chan observation
	done        chan struct{}
	closed      bool
	mu          sync.RWMutex
	errMu       sync.Mutex
	errs        []error
}

type observation struct {
	scope  Scope
	record Record
}

const defaultBufferSize = 1024

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
	r := &recorder{
		logger:      resolvedOptions.logger,
		auth:        resolvedOptions.auth,
		globalScope: globalScope,
		clock:       clock,
		fanout:      NewCollectors(resolvedOptions.collectors...),
		queue:       make(chan observation, buffer),
		done:        make(chan struct{}),
	}
	go r.run()
	return r
}

func (r *recorder) Record(ctx context.Context, scope Scope, record Record) error {
	normalized, err := r.normalize(scope, record)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return ErrRecorderClosed
	}
	select {
	case r.queue <- normalized:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrBufferFull
	}
}

func (r *recorder) normalize(scope Scope, record Record) (observation, error) {
	if !validator.NonNil(record) {
		return observation{}, errors.New("observability: record is nil")
	}
	now := r.clock()
	normalizedScope, err := normalizeScope(scope, r.globalScope)
	if err != nil {
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
		return observation{scope: normalizedScope, record: typed}, nil
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
		return observation{scope: normalizedScope, record: typed}, nil
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
		return observation{scope: normalizedScope, record: typed}, nil
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
		return observation{scope: normalizedScope, record: typed}, nil
	case RecordUsage:
		if typed.Duration <= 0 {
			return observation{}, errors.New("observability: usage duration must be greater than zero")
		}
		if typed.OccurredAt.IsZero() {
			typed.OccurredAt = now
		}
		typed.Attributes = typed.Attributes.Clone()
		return observation{scope: normalizedScope, record: typed}, nil
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
		return observation{scope: normalizedScope, record: typed}, nil
	default:
		return observation{}, fmt.Errorf("observability: unsupported record type %T", record)
	}
}

func normalizeScope(scope Scope, global GlobalScope) (Scope, error) {
	if !validator.NonNil(scope) {
		return nil, errors.New("observability: scope is required")
	}
	scope = scope.WithGlobal(global)
	if err := ValidateScope(scope); err != nil {
		return nil, err
	}
	return scope, nil
}

func (r *recorder) run() {
	defer close(r.done)
	for item := range r.queue {
		ctx := context.Background()
		if validator.NonNil(r.auth) {
			ctx = context.WithValue(ctx, types.CTX_, r.auth)
		}
		err := r.fanout.Collect(ctx, item.scope, item.record)
		if err != nil {
			r.addError(err)
		}
	}
}

func (r *recorder) Close(ctx context.Context) error {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.queue)
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
	return r.errors()
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
