// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package assistant_talk_api

import (
	"context"

	"github.com/rapidaai/api/assistant-api/config"
	observe "github.com/rapidaai/api/assistant-api/internal/observe"
	observe_exporters "github.com/rapidaai/api/assistant-api/internal/observe/exporters"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	"github.com/rapidaai/pkg/types"
)

type conversationObserver struct {
	cfg        *config.AssistantConfig
	logger     commons.Logger
	opensearch connectors.OpenSearchConnector
	persist    observe.ConversationPersister
}

func NewConversationObserver(
	cfg *config.AssistantConfig,
	logger commons.Logger,
	opensearch connectors.OpenSearchConnector,
	persist observe.ConversationPersister,
) *conversationObserver {
	return &conversationObserver{
		cfg:        cfg,
		logger:     logger,
		opensearch: opensearch,
		persist:    persist,
	}
}

func (o *conversationObserver) ConversationObserver(auth types.SimplePrinciple, assistantID, conversationID uint64) *observe.ConversationObserver {
	var projectID, orgID uint64
	if pid := auth.GetCurrentProjectId(); pid != nil {
		projectID = *pid
	}
	if oid := auth.GetCurrentOrganizationId(); oid != nil {
		orgID = *oid
	}

	meta := observe.SessionMeta{
		AssistantID:             assistantID,
		AssistantConversationID: conversationID,
		ProjectID:               projectID,
		OrganizationID:          orgID,
	}

	var eventExporters []observe.EventExporter
	var metricExporters []observe.MetricExporter
	if o.cfg.TelemetryConfig != nil {
		if envType := o.cfg.TelemetryConfig.Type(); envType != "" {
			evtExp, metExp, err := observe_exporters.GetExporter(
				context.Background(), o.logger, &o.cfg.AppConfig, o.opensearch, string(envType), o.cfg.TelemetryConfig.ToMap(),
			)
			if err != nil {
				o.logger.Warnf("conversation observer: default exporter creation failed: %v", err)
			} else if evtExp != nil && metExp != nil {
				eventExporters = append(eventExporters, evtExp)
				metricExporters = append(metricExporters, metExp)
			}
		}
	}

	return observe.NewConversationObserver(&observe.ConversationObserverConfig{
		Logger:         o.logger,
		Auth:           auth,
		AssistantID:    assistantID,
		ConversationID: conversationID,
		ProjectID:      projectID,
		OrganizationID: orgID,
		Persist:        o.persist,
		Events:         observe.NewEventCollector(o.logger, meta, eventExporters...),
		Metrics:        observe.NewMetricCollector(o.logger, meta, metricExporters...),
	})
}
