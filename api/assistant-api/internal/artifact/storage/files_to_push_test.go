// Copyright (c) 2023-2026 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_artifact_storage

import (
	"testing"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/utils"
)

func testArtifacts() []internal_type.ArtifactPushArtifact {
	return []internal_type.ArtifactPushArtifact{
		{Name: "user", Type: "recording", ContentType: "audio/wav", Content: []byte("user")},
		{Name: "assistant", Type: "recording", ContentType: "audio/wav", Content: []byte("assistant")},
		{Name: "conversation", Type: "recording", ContentType: "audio/wav", Content: []byte("conversation")},
	}
}

func artifactNames(artifacts []internal_type.ArtifactPushArtifact) []string {
	names := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		names = append(names, artifact.Name)
	}
	return names
}

func TestFilterArtifactsToPush_DefaultsToAllRecordings(t *testing.T) {
	artifacts := filterArtifactsToPush(testArtifacts(), utils.Option{})

	if got, want := artifactNames(artifacts), []string{"user", "assistant", "conversation"}; !equalStringSlices(got, want) {
		t.Fatalf("artifact names = %v, want %v", got, want)
	}
}

func TestFilterArtifactsToPush_FiltersSelectedRecordings(t *testing.T) {
	artifacts := filterArtifactsToPush(testArtifacts(), utils.Option{
		filesToPushOptionKey: `["recording.conversation","recording.user"]`,
	})

	if got, want := artifactNames(artifacts), []string{"user", "conversation"}; !equalStringSlices(got, want) {
		t.Fatalf("artifact names = %v, want %v", got, want)
	}
}

func TestFilterArtifactsToPush_KeepsNonRecordingArtifacts(t *testing.T) {
	input := append(testArtifacts(), internal_type.ArtifactPushArtifact{
		Name: "payload", Type: "analysis", ContentType: "application/json", Content: []byte(`{}`),
	})

	artifacts := filterArtifactsToPush(input, utils.Option{
		filesToPushOptionKey: `["recording.assistant"]`,
	})

	if got, want := artifactNames(artifacts), []string{"assistant", "payload"}; !equalStringSlices(got, want) {
		t.Fatalf("artifact names = %v, want %v", got, want)
	}
}

func TestFilterArtifactsToPush_UnknownSelectionFallsBackToAll(t *testing.T) {
	artifacts := filterArtifactsToPush(testArtifacts(), utils.Option{
		filesToPushOptionKey: `["transcript.text"]`,
	})

	if got, want := artifactNames(artifacts), []string{"user", "assistant", "conversation"}; !equalStringSlices(got, want) {
		t.Fatalf("artifact names = %v, want %v", got, want)
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
