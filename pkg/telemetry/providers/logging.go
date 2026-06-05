// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package providers

import (
	"context"

	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/telemetry"
)

// LoggingExporter logs events and metrics at INFO level.
type LoggingExporter struct {
	logger commons.Logger
}

func NewLoggingExporter(logger commons.Logger, _ LoggingConfig) *LoggingExporter {
	return &LoggingExporter{logger: logger}
}

func (e *LoggingExporter) ExportEvent(_ context.Context, meta telemetry.SessionMeta, rec telemetry.EventRecord) error {
	e.logger.Infof("[telemetry/event] assistant=%d conversation=%d message=%s name=%s data=%v",
		meta.AssistantID, meta.AssistantConversationID, rec.MessageID, rec.Name, rec.Data)
	return nil
}

func (e *LoggingExporter) ExportMetric(_ context.Context, meta telemetry.SessionMeta, rec telemetry.MetricRecord) error {
	switch m := rec.(type) {
	case telemetry.ConversationMetricRecord:
		e.logger.Infof("[telemetry/metric/conversation] assistant=%d conversation=%s metrics=%v",
			meta.AssistantID, m.ConversationID, m.Metrics)
	case telemetry.MessageMetricRecord:
		e.logger.Infof("[telemetry/metric/message] assistant=%d message=%s conversation=%s metrics=%v",
			meta.AssistantID, m.MessageID, m.ConversationID, m.Metrics)
	}
	return nil
}

func (e *LoggingExporter) Close(_ context.Context) error { return nil }
