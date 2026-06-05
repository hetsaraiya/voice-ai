// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/configs"
	"github.com/rapidaai/pkg/connectors"
	"github.com/rapidaai/pkg/telemetry"
)

// OpenSearchExporter indexes events and metrics to dedicated OpenSearch indices.
type OpenSearchExporter struct {
	logger    commons.Logger
	config    OpenSearchConfig
	connector connectors.OpenSearchConnector
}

func NewOpenSearchExporter(
	logger commons.Logger,
	config OpenSearchConfig,
	connector connectors.OpenSearchConnector,
) *OpenSearchExporter {
	return &OpenSearchExporter{logger: logger, config: config, connector: connector}
}

func NewOpenSearchExporterFromOptions(
	ctx context.Context,
	logger commons.Logger,
	opts map[string]interface{},
) (telemetry.Exporter, error) {
	connectorConfig := &configs.OpenSearchConfig{}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           connectorConfig,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(opts); err != nil {
		return nil, err
	}
	if !connectorConfig.IsValid() {
		return nil, fmt.Errorf("telemetry/opensearch: opensearch config is required")
	}
	exporterConfig, err := OpenSearchConfigFromOptions(opts)
	if err != nil {
		return nil, err
	}
	connector := connectors.NewOpenSearchConnector(connectorConfig, logger)
	if err := connector.Connect(ctx); err != nil {
		return nil, err
	}
	return &OpenSearchExporter{
		logger:    logger,
		config:    exporterConfig,
		connector: connector,
	}, nil
}

func (e *OpenSearchExporter) eventIndex() string {
	prefix := "rapida"
	if strings.TrimSpace(e.config.IndexPrefix) != "" {
		prefix = strings.TrimSpace(e.config.IndexPrefix)
	}
	return prefix + "-events-" + time.Now().UTC().Format("20060102")
}

func (e *OpenSearchExporter) metricIndex() string {
	prefix := "rapida"
	if strings.TrimSpace(e.config.IndexPrefix) != "" {
		prefix = strings.TrimSpace(e.config.IndexPrefix)
	}
	return prefix + "-metrics-" + time.Now().UTC().Format("20060102")
}

type opensearchEventDoc struct {
	ProjectID               uint64            `json:"projectId"`
	OrganizationID          uint64            `json:"organizationId"`
	AssistantID             uint64            `json:"assistantId"`
	AssistantConversationID uint64            `json:"assistantConversationId"`
	MessageID               string            `json:"messageId"`
	Name                    string            `json:"name"`
	Data                    map[string]string `json:"data"`
	Time                    time.Time         `json:"time"`
}

type opensearchMetricEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type opensearchMetricDoc struct {
	ProjectID               uint64                  `json:"projectId"`
	OrganizationID          uint64                  `json:"organizationId"`
	AssistantID             uint64                  `json:"assistantId"`
	AssistantConversationID uint64                  `json:"assistantConversationId"`
	Scope                   string                  `json:"scope"` // "conversation" or "message"
	ContextID               string                  `json:"contextId"`
	Metrics                 []opensearchMetricEntry `json:"metrics"`
	Time                    time.Time               `json:"time"`
}

func (e *OpenSearchExporter) ExportEvent(ctx context.Context, meta telemetry.SessionMeta, rec telemetry.EventRecord) error {
	conversationID := rec.ConversationID
	if conversationID == 0 {
		conversationID = meta.AssistantConversationID
	}
	doc := opensearchEventDoc{
		ProjectID:               meta.ProjectID,
		OrganizationID:          meta.OrganizationID,
		AssistantID:             meta.AssistantID,
		AssistantConversationID: conversationID,
		MessageID:               rec.MessageID,
		Name:                    rec.Name,
		Data:                    rec.Data,
		Time:                    rec.Time,
	}
	return e.bulk(ctx, e.eventIndex(), doc)
}

func (e *OpenSearchExporter) ExportMetric(ctx context.Context, meta telemetry.SessionMeta, rec telemetry.MetricRecord) error {
	doc := opensearchMetricDoc{
		ProjectID:               meta.ProjectID,
		OrganizationID:          meta.OrganizationID,
		AssistantID:             meta.AssistantID,
		AssistantConversationID: meta.AssistantConversationID,
	}
	switch m := rec.(type) {
	case telemetry.ConversationMetricRecord:
		doc.Scope = "conversation"
		doc.ContextID = m.ConversationID
		doc.Time = m.Time
		for _, metric := range m.Metrics {
			doc.Metrics = append(doc.Metrics, opensearchMetricEntry{
				Name:  metric.GetName(),
				Value: metric.GetValue(),
			})
		}
	case telemetry.MessageMetricRecord:
		doc.Scope = "message"
		doc.ContextID = m.MessageID
		doc.Time = m.Time
		for _, metric := range m.Metrics {
			doc.Metrics = append(doc.Metrics, opensearchMetricEntry{
				Name:  metric.GetName(),
				Value: metric.GetValue(),
			})
		}
	}
	return e.bulk(ctx, e.metricIndex(), doc)
}

func (e *OpenSearchExporter) Close(ctx context.Context) error {
	if e.connector != nil {
		return e.connector.Disconnect(ctx)
	}
	return nil
}

func (e *OpenSearchExporter) bulk(ctx context.Context, index string, doc interface{}) error {
	var sb strings.Builder
	meta := fmt.Sprintf(`{ "index": { "_index": "%s", "_id": "%s" } }`, index, uuid.NewString())
	sb.WriteString(meta + "\n")
	b, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	sb.WriteString(string(b) + "\n")
	if err := e.connector.Bulk(ctx, sb.String()); err != nil {
		e.logger.Errorf("telemetry/opensearch: bulk index error: %v", err)
		return err
	}
	return nil
}
