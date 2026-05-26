// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_type

import "context"

type Recorder interface {
	// Start begins the recording timeline. All subsequent Record calls are
	// placed on a wall-clock timeline relative to this moment.
	Start()

	// recording is done by calling Record with audio data. The implementation is
	Record(context.Context, Packet) error

	// Persist saves the recorded audio and returns user, assistant, and
	// conversation audio data.
	Persist() ([]byte, []byte, []byte, error)
}
