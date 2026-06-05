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
	Record(ctx context.Context, record Record) error
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
	queue       chan Record
	done        chan struct{}
	closed      bool
	mu          sync.RWMutex
	errMu       sync.Mutex
	errs        []error
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
		queue:       make(chan Record, buffer),
		done:        make(chan struct{}),
	}
	go r.run()
	return r
}

func (r *recorder) Record(ctx context.Context, record Record) error {
	normalized, err := r.normalize(record)
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

func (r *recorder) normalize(record Record) (Record, error) {
	if !validator.NonNil(record) {
		return nil, errors.New("observability: record is nil")
	}
	now := r.clock()
	switch typed := record.(type) {
	case RecordLog:
		if err := normalizeCommonRecord(&typed.CommonRecord, now, r.auth, r.globalScope); err != nil {
			return nil, err
		}
		if !validator.NotBlank(typed.Message) {
			return nil, errors.New("observability: log message is required")
		}
		switch typed.Level {
		case "":
			typed.Level = LevelInfo
		case LevelInfo, LevelError, LevelDebug, LevelCritical:
		default:
			return nil, fmt.Errorf("observability: invalid log level %q", typed.Level)
		}
		typed.Attributes = typed.Attributes.Clone()
		return typed, nil
	case RecordEvent:
		if err := normalizeCommonRecord(&typed.CommonRecord, now, r.auth, r.globalScope); err != nil {
			return nil, err
		}
		if !validator.NotBlank(typed.Event.String()) {
			return nil, errors.New("observability: event is required")
		}
		if typed.Component == "" {
			typed.Component = typed.Event.Component()
		}
		if typed.Component == ComponentUnknown {
			return nil, fmt.Errorf("observability: component is required for event %q", typed.Event)
		}
		typed.Attributes = typed.Attributes.Clone()
		return typed, nil
	case RecordMetric:
		if err := normalizeCommonRecord(&typed.CommonRecord, now, r.auth, r.globalScope); err != nil {
			return nil, err
		}
		if len(typed.Metrics) == 0 {
			return nil, errors.New("observability: at least one metric is required")
		}
		for i, metric := range typed.Metrics {
			if !validator.NotBlank(metric.Name) {
				return nil, fmt.Errorf("observability: metric[%d] name is required", i)
			}
		}
		return typed, nil
	case RecordMetadata:
		if err := normalizeCommonRecord(&typed.CommonRecord, now, r.auth, r.globalScope); err != nil {
			return nil, err
		}
		if len(typed.Metadata) == 0 {
			return nil, errors.New("observability: at least one metadata entry is required")
		}
		for i, metadata := range typed.Metadata {
			if !validator.NotBlank(metadata.Key) {
				return nil, fmt.Errorf("observability: metadata[%d] key is required", i)
			}
		}
		return typed, nil
	case RecordUsage:
		if err := normalizeCommonRecord(&typed.CommonRecord, now, r.auth, r.globalScope); err != nil {
			return nil, err
		}
		if typed.Scope.ScopeType() == ScopeMessage {
			return nil, errors.New("observability: usage cannot be message-scoped")
		}
		if typed.Duration <= 0 {
			return nil, errors.New("observability: usage duration must be greater than zero")
		}
		typed.Attributes = typed.Attributes.Clone()
		return typed, nil
	case RecordWebhook:
		if err := normalizeCommonRecord(&typed.CommonRecord, now, r.auth, r.globalScope); err != nil {
			return nil, err
		}
		if !validator.NotBlank(typed.Event.String()) {
			return nil, errors.New("observability: webhook event is required")
		}
		if typed.Payload == nil {
			typed.Payload = map[string]interface{}{}
		}
		return typed, nil
	default:
		return nil, fmt.Errorf("observability: unsupported record type %T", record)
	}
}

func normalizeCommonRecord(record *CommonRecord, now time.Time, auth types.SimplePrinciple, global GlobalScope) error {
	if !validator.NonNil(record) {
		return errors.New("observability: record is nil")
	}
	record.Auth = auth
	if !validator.NonNil(record.Scope) {
		return errors.New("observability: scope is required")
	}
	record.Scope = record.Scope.WithGlobal(global)
	if err := ValidateScope(record.Scope); err != nil {
		return err
	}
	if record.OccurredAt.IsZero() {
		record.OccurredAt = now
	}
	return nil
}

func (r *recorder) run() {
	defer close(r.done)
	for record := range r.queue {
		err := r.fanout.Collect(context.Background(), record)
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
