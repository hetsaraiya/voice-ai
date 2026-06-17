// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package notification

import (
	"context"
	"time"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/pkg/validator"
)

type Notification struct {
	ID         string
	Event      observability.EventName
	Component  observability.ComponentName
	Scope      observability.Scope
	Attributes observability.Attributes
	OccurredAt time.Time
}

type Notifier interface {
	Notify(ctx context.Context, notification Notification) error
}

type Selector func(record observability.RecordEvent) bool

type Config struct {
	Notifier Notifier
	Selector Selector
}

type Collector struct {
	notifier Notifier
	selector Selector
}

func New(cfg Config) observability.Collector {
	if !validator.NonNil(cfg.Notifier) {
		return observability.NoopCollector{}
	}
	selector := cfg.Selector
	if !validator.NonNil(selector) {
		selector = DefaultSelector
	}
	return &Collector{notifier: cfg.Notifier, selector: selector}
}

func (c *Collector) Key() string {
	return "notification"
}

func (c *Collector) Collect(ctx context.Context, scope observability.Scope, _ observability.Context, record observability.Record) error {
	event, ok := record.(observability.RecordEvent)
	if !ok || !c.selector(event) {
		return nil
	}
	return c.notifier.Notify(ctx, Notification{
		ID:         event.ID,
		Event:      event.Event,
		Component:  event.Component,
		Scope:      scope,
		Attributes: event.Attributes.Clone(),
		OccurredAt: event.OccurredAt,
	})
}

func (c *Collector) Close(context.Context) error {
	return nil
}

func DefaultSelector(record observability.RecordEvent) bool {
	switch record.Event {
	case observability.CallFailed,
		observability.ConversationError:
		return true
	default:
		return false
	}
}
