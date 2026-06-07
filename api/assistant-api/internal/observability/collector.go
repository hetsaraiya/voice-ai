// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability

import (
	"context"
	"errors"
	"sync"
)

type Collector interface {
	Key() string
	Collect(ctx context.Context, scope Scope, record Record) error
	Close(ctx context.Context) error
}

type NoopCollector struct{}

func (NoopCollector) Key() string {
	return "noop"
}

func (NoopCollector) Collect(context.Context, Scope, Record) error {
	return nil
}

func (NoopCollector) Close(context.Context) error {
	return nil
}

type Collectors struct {
	mu         sync.RWMutex
	collectors []Collector
	keys       map[string]struct{}
}

func NewCollectors(collectors ...Collector) *Collectors {
	fanout := &Collectors{keys: make(map[string]struct{})}
	fanout.AddCollectors(collectors...)
	return fanout
}

func (c *Collectors) Key() string {
	return "collectors"
}

func (c *Collectors) AddCollectors(collectors ...Collector) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.keys == nil {
		c.keys = make(map[string]struct{})
	}
	for _, collector := range collectors {
		if collector == nil {
			continue
		}
		key := collector.Key()
		if _, ok := c.keys[key]; ok {
			continue
		}
		c.keys[key] = struct{}{}
		c.collectors = append(c.collectors, collector)
	}
}

func (c *Collectors) Collect(ctx context.Context, scope Scope, record Record) error {
	c.mu.RLock()
	collectors := append([]Collector(nil), c.collectors...)
	c.mu.RUnlock()
	var errs []error
	for _, collector := range collectors {
		if collector == nil {
			continue
		}
		if err := collector.Collect(ctx, scope, record); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *Collectors) Close(ctx context.Context) error {
	c.mu.RLock()
	collectors := append([]Collector(nil), c.collectors...)
	c.mu.RUnlock()
	var errs []error
	for _, collector := range collectors {
		if collector == nil {
			continue
		}
		if err := collector.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
