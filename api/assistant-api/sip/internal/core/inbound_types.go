// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package core

import (
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/protos"
)

type inboundInviteIdentity struct {
	callID  string
	fromURI string
	toURI   string
}

type inboundResolvedConfig struct {
	config          *Config
	auth            types.SimplePrinciple
	assistant       *internal_assistant_entity.Assistant
	vaultCredential *protos.VaultCredential
}

type inboundMediaOffer struct {
	sdpInfo         *SDPMediaInfo
	negotiatedCodec *Codec
}
