// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

// TEMP — instrumentation to find slow packet handlers.
// Delete this file and the one `defer r.benchmarkDispatch(p)()` line in
// dispatch.go when no longer needed.
package adapter_internal

import (
	"fmt"
	"time"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
)

const slowDispatchThreshold = 5 * time.Millisecond

func (r *genericRequestor) benchmarkDispatch(p internal_type.Packet) func() {
	start := time.Now()
	return func() {
		if elapsed := time.Since(start); elapsed > slowDispatchThreshold {
			r.logger.Benchmark(fmt.Sprintf("Dispatch %T", p), time.Since(start))
		}
	}
}
