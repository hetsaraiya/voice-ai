// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package channel_telephony

import (
	"bufio"
	"context"
	"fmt"
	"net"

	"github.com/gorilla/websocket"
	callcontext "github.com/rapidaai/api/assistant-api/internal/callcontext"
	internal_asterisk_audiosocket "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/asterisk/audiosocket"
	internal_asterisk_websocket "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/asterisk/websocket"
	internal_exotel_telephony "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/exotel"
	internal_sip_telephony "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/sip"
	internal_telnyx_telephony "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/telnyx"
	internal_twilio_telephony "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/twilio"
	internal_vonage_telephony "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/vonage"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/protos"
)

type streamerOptions struct {
	webSocketConn *websocket.Conn

	audioSocketConn   net.Conn
	audioSocketReader *bufio.Reader
	audioSocketWriter *bufio.Writer

	ctx          context.Context
	sipSession   *sip_infra.Session
	sipLifecycle sip_infra.LifecycleController
}

// StreamerOption configures transport-specific streamer construction.
type StreamerOption interface {
	apply(*streamerOptions)
}

type funcStreamerOption struct {
	applyFunc func(*streamerOptions)
}

func (option *funcStreamerOption) apply(streamerOptions *streamerOptions) {
	option.applyFunc(streamerOptions)
}

func newStreamerOption(applyFunc func(*streamerOptions)) StreamerOption {
	return &funcStreamerOption{
		applyFunc: applyFunc,
	}
}

// WithWebSocketStreamer configures websocket media transport.
func WithWebSocketStreamer(conn *websocket.Conn) StreamerOption {
	return newStreamerOption(func(streamerOptions *streamerOptions) {
		streamerOptions.webSocketConn = conn
	})
}

// WithAudioSocketStreamer configures Asterisk AudioSocket media transport.
func WithAudioSocketStreamer(conn net.Conn, reader *bufio.Reader, writer *bufio.Writer) StreamerOption {
	return newStreamerOption(func(streamerOptions *streamerOptions) {
		streamerOptions.audioSocketConn = conn
		streamerOptions.audioSocketReader = reader
		streamerOptions.audioSocketWriter = writer
	})
}

// WithSIPStreamer configures SIP session-owned media transport.
func WithSIPStreamer(ctx context.Context, session *sip_infra.Session, lifecycle sip_infra.LifecycleController) StreamerOption {
	return newStreamerOption(func(streamerOptions *streamerOptions) {
		streamerOptions.ctx = ctx
		streamerOptions.sipSession = session
		streamerOptions.sipLifecycle = lifecycle
	})
}

func (at Telephony) NewStreamer(
	logger commons.Logger,
	callContext *callcontext.CallContext,
	vaultCredential *protos.VaultCredential,
	options ...StreamerOption,
) (internal_type.Streamer, error) {
	var resolvedOptions streamerOptions
	for _, option := range options {
		option.apply(&resolvedOptions)
	}
	switch at {
	case Twilio:
		return internal_twilio_telephony.NewTwilioWebsocketStreamer(logger, resolvedOptions.webSocketConn, callContext, vaultCredential)
	case Exotel:
		return internal_exotel_telephony.NewExotelWebsocketStreamer(logger, resolvedOptions.webSocketConn, callContext, vaultCredential)
	case Vonage:
		return internal_vonage_telephony.NewVonageWebsocketStreamer(logger, resolvedOptions.webSocketConn, callContext, vaultCredential)
	case Asterisk:
		if resolvedOptions.audioSocketConn != nil {
			return internal_asterisk_audiosocket.NewStreamer(logger, resolvedOptions.audioSocketConn, resolvedOptions.audioSocketReader, resolvedOptions.audioSocketWriter, callContext, vaultCredential)
		}
		return internal_asterisk_websocket.NewAsteriskWebsocketStreamer(logger, resolvedOptions.webSocketConn, callContext, vaultCredential)
	case Telnyx:
		return internal_telnyx_telephony.NewTelnyxWebsocketStreamer(logger, resolvedOptions.webSocketConn, callContext, vaultCredential)
	case SIP:
		return NewSIPStreamer(resolvedOptions.ctx, logger, resolvedOptions.sipSession, resolvedOptions.sipLifecycle, callContext, vaultCredential)
	default:
		return nil, fmt.Errorf("streamer not supported for provider %q", at)
	}
}

func NewSIPStreamer(
	ctx context.Context,
	logger commons.Logger,
	session *sip_infra.Session,
	lifecycle sip_infra.LifecycleController,
	callContext *callcontext.CallContext,
	vaultCredential *protos.VaultCredential,
) (internal_type.SIPCallStreamer, error) {
	return internal_sip_telephony.NewStreamer(ctx, logger, session, lifecycle, callContext, vaultCredential)
}
