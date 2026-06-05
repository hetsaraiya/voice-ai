// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package billing

import (
	"context"
	"time"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/pkg/validator"
)

type Usage struct {
	ID         string
	Scope      observability.Scope
	Component  observability.ComponentName
	Provider   string
	Duration   time.Duration
	Attributes observability.Attributes
	OccurredAt time.Time
}

type Publisher interface {
	PublishUsage(ctx context.Context, usage Usage) error
}

type Collector struct {
	publisher Publisher
}

func New(publisher Publisher) observability.Collector {
	if !validator.NonNil(publisher) {
		return observability.NoopCollector{}
	}
	return &Collector{publisher: publisher}
}

func (c *Collector) Collect(ctx context.Context, record observability.Record) error {
	usage, ok := record.(observability.RecordUsage)
	if !ok {
		return nil
	}
	return c.publisher.PublishUsage(ctx, Usage{
		ID:         usage.ID,
		Scope:      usage.Scope,
		Component:  usage.Component,
		Provider:   usage.Provider,
		Duration:   usage.Duration,
		Attributes: usage.Attributes.Clone(),
		OccurredAt: usage.OccurredAt,
	})
}

func (c *Collector) Close(context.Context) error {
	return nil
}
