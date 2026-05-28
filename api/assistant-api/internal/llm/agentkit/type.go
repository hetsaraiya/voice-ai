// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_llm_agentkit

import (
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/protos"
	"google.golang.org/grpc"
	"sync"
)

type AgentPipeline interface {
	agentPipeline()
}

type UserTurnPipeline struct {
	Packet internal_type.UserInputPacket
}

type UserTextPipeline struct {
	Packet internal_type.UserTextReceivedPacket
}

type InjectMessagePipeline struct {
	Packet internal_type.InjectMessagePacket
}

type InterruptionPipeline struct {
	Packet internal_type.LLMInterruptPacket
}

type ResponsePipeline struct {
	Response *protos.TalkOutput
}

type agentkitExecutor struct {
	logger          commons.Logger
	transport       agentkitTransport
	stateMu         sync.RWMutex
	activeContextID string
	closing         bool
}

type agentkitTransport struct {
	conn         *grpc.ClientConn
	stream       grpc.BidiStreamingClient[protos.TalkInput, protos.TalkOutput]
	listenerDone chan struct{}
}

func (UserTurnPipeline) agentPipeline()      {}
func (UserTextPipeline) agentPipeline()      {}
func (InjectMessagePipeline) agentPipeline() {}
func (InterruptionPipeline) agentPipeline()  {}
func (ResponsePipeline) agentPipeline()      {}
