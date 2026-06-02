// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package channel_telephony

import (
	"bufio"
	"context"
	"net"
	"testing"
)

func TestStreamerOption_AppliesSIPStreamerOptions(t *testing.T) {
	ctx := context.Background()
	var resolvedOptions streamerOptions

	WithSIPStreamer(ctx, nil, nil).apply(&resolvedOptions)

	if resolvedOptions.ctx != ctx {
		t.Fatal("expected streamer options to preserve context")
	}
}

func TestStreamerOption_AppliesAudioSocketStreamerOptions(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	reader := bufio.NewReader(clientConn)
	writer := bufio.NewWriter(clientConn)
	var resolvedOptions streamerOptions

	WithAudioSocketStreamer(serverConn, reader, writer).apply(&resolvedOptions)

	if resolvedOptions.audioSocketConn != serverConn {
		t.Fatal("expected streamer options to preserve AudioSocket connection")
	}
	if resolvedOptions.audioSocketReader != reader {
		t.Fatal("expected streamer options to preserve AudioSocket reader")
	}
	if resolvedOptions.audioSocketWriter != writer {
		t.Fatal("expected streamer options to preserve AudioSocket writer")
	}
}
