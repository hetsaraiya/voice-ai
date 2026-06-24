// Copyright (c) 2023-2026 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_artifact_storage

import (
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/utils"
)

const (
	filesToPushOptionKey = "files_to_push"

	fileRecordingConversation = "recording.conversation"
	fileRecordingUser         = "recording.user"
	fileRecordingAssistant    = "recording.assistant"
)

func filterArtifactsToPush(artifacts []internal_type.ArtifactPushArtifact, options utils.Option) []internal_type.ArtifactPushArtifact {
	selectedFiles := options.GetStringSlice(filesToPushOptionKey)
	if len(selectedFiles) == 0 {
		return artifacts
	}

	selectedSet := make(map[string]struct{}, len(selectedFiles))
	for _, file := range selectedFiles {
		selectedSet[file] = struct{}{}
	}

	out := make([]internal_type.ArtifactPushArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		fileID, ok := artifactFileID(artifact)
		if !ok {
			out = append(out, artifact)
			continue
		}
		if _, selected := selectedSet[fileID]; selected {
			out = append(out, artifact)
		}
	}
	if len(out) == 0 {
		return artifacts
	}
	return out
}

func artifactFileID(artifact internal_type.ArtifactPushArtifact) (string, bool) {
	if artifact.Type != "recording" {
		return "", false
	}
	switch artifact.Name {
	case "conversation":
		return fileRecordingConversation, true
	case "user":
		return fileRecordingUser, true
	case "assistant":
		return fileRecordingAssistant, true
	default:
		return "", false
	}
}
