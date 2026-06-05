// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability

import (
	"context"
	"errors"
)

type Collector interface {
	Collect(ctx context.Context, record Record) error
	Close(ctx context.Context) error
}

type NoopCollector struct{}

func (NoopCollector) Collect(context.Context, Record) error {
	return nil
}

func (NoopCollector) Close(context.Context) error {
	return nil
}

type Collectors struct {
	collectors []Collector
}

func NewCollectors(collectors ...Collector) Collector {
	if len(collectors) == 0 {
		return NoopCollector{}
	}
	return &Collectors{collectors: append([]Collector(nil), collectors...)}
}

func (c *Collectors) Collect(ctx context.Context, record Record) error {
	var errs []error
	for _, collector := range c.collectors {
		if collector == nil {
			continue
		}
		if err := collector.Collect(ctx, record); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *Collectors) Close(ctx context.Context) error {
	var errs []error
	for _, collector := range c.collectors {
		if collector == nil {
			continue
		}
		if err := collector.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
