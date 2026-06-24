// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_artifact

import (
	"context"
	"testing"

	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	gorm_models "github.com/rapidaai/pkg/models/gorm"
)

func testLogger(t *testing.T) commons.Logger {
	t.Helper()
	logger, err := commons.NewApplicationLogger()
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return logger
}

func testConfig(provider string, options map[string]interface{}) *internal_assistant_entity.AssistantConfiguration {
	cfg := &internal_assistant_entity.AssistantConfiguration{
		Provider:          provider,
		ConfigurationType: internal_assistant_entity.AssistantConfigurationTypeStorage,
		Enabled:           true,
	}
	for key, value := range options {
		metadata := gorm_models.NewMetadata(key, value)
		cfg.Options = append(cfg.Options, &internal_assistant_entity.AssistantConfigurationOption{
			Metadata: *metadata,
		})
	}
	return cfg
}

func testOnPacket(context.Context, ...internal_type.Packet) error {
	return nil
}

func TestNewExecutor_SupportsOnlyAWSAndAzureStorage(t *testing.T) {
	awsExec, err := New(
		WithContext(context.Background()),
		WithLogger(testLogger(t)),
		WithConfiguration(testConfig(providerAWS, nil)),
		WithOnPacket(testOnPacket),
	)
	if err != nil {
		t.Fatalf("new aws executor: %v", err)
	}
	if got, want := awsExec.Name(), "artifact-push-aws-0"; got != want {
		t.Fatalf("aws executor name = %q, want %q", got, want)
	}

	azureExec, err := New(
		WithContext(context.Background()),
		WithLogger(testLogger(t)),
		WithConfiguration(testConfig(providerAzure, nil)),
		WithOnPacket(testOnPacket),
	)
	if err != nil {
		t.Fatalf("new azure executor: %v", err)
	}
	if got, want := azureExec.Name(), "artifact-push-azure-0"; got != want {
		t.Fatalf("azure executor name = %q, want %q", got, want)
	}

	if _, err := New(
		WithContext(context.Background()),
		WithLogger(testLogger(t)),
		WithConfiguration(testConfig("s3", nil)),
		WithOnPacket(testOnPacket),
	); err == nil {
		t.Fatalf("new s3 executor error = nil, want unsupported provider error")
	}
}
