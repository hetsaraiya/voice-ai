// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_llm_agentkit

import (
	"context"
	"errors"
	"fmt"
	"io"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (e *agentkitExecutor) clearTransportLocked() agentkitTransport {
	transport := e.transport
	e.transport = agentkitTransport{}
	return transport
}

func (e *agentkitExecutor) send(req *protos.TalkInput) error {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	if e.transport.stream == nil {
		return fmt.Errorf("not connected")
	}
	return e.transport.stream.Send(req)
}

func (e *agentkitExecutor) listen(ctx context.Context, comm internal_type.Communication) {
	for {
		if ctx.Err() != nil {
			return
		}

		e.stateMu.RLock()
		talkStream := e.transport.stream
		e.stateMu.RUnlock()
		if talkStream == nil {
			return
		}

		resp, err := talkStream.Recv()
		if err != nil {
			e.stateMu.RLock()
			closing := e.closing
			e.stateMu.RUnlock()
			if closing {
				return
			}
			switch {
			case errors.Is(err, io.EOF):
				comm.OnPacket(ctx, internal_type.LLMToolCallPacket{
					ContextID: e.getActiveContextID(),
					Name:      "end_conversation",
					Action:    protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION,
					Arguments: map[string]string{"reason": "server closed connection"},
				})
			case status.Code(err) == codes.Canceled:
				comm.OnPacket(ctx, internal_type.LLMToolCallPacket{
					ContextID: e.getActiveContextID(),
					Name:      "end_conversation",
					Action:    protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION,
					Arguments: map[string]string{"reason": "connection canceled"},
				})
			case status.Code(err) == codes.Unavailable:
				comm.OnPacket(ctx, internal_type.LLMToolCallPacket{
					ContextID: e.getActiveContextID(),
					Name:      "end_conversation",
					Action:    protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION,
					Arguments: map[string]string{"reason": "server unavailable"},
				})
			default:
				if !validator.NotBlank(err.Error()) {
					comm.OnPacket(ctx, internal_type.LLMToolCallPacket{
						ContextID: e.getActiveContextID(),
						Name:      "end_conversation",
						Action:    protos.ToolCallAction_TOOL_CALL_ACTION_END_CONVERSATION,
						Arguments: map[string]string{"reason": err.Error()},
					})
				}
			}
			return
		}
		_ = e.Run(ctx, comm, ResponsePipeline{Response: resp})
	}
}
