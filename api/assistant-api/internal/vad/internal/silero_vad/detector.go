// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_silero_vad

// #cgo CFLAGS: -Wall -Werror -std=c99
// #cgo LDFLAGS: -lonnxruntime
// #include "ort_bridge.h"
import "C"

import (
	"fmt"
	"unsafe"
)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const (
	// stateLen is the total number of float32 elements in the ONNX hidden state
	// tensor: shape [2, 1, 128] = 256 elements.
	stateLen = 2 * 1 * 128

	// contextLen is the number of trailing samples saved from each window
	// and prepended to the next inference call for temporal continuity.
	contextLen = 64
)

// -----------------------------------------------------------------------------
// Configuration
// -----------------------------------------------------------------------------

// DetectorConfig holds parameters for initializing a Silero VAD Detector.
type DetectorConfig struct {
	// ModelPath is the filesystem path to the Silero ONNX model file.
	ModelPath string
	// SampleRate of the input audio. Must be 8000 or 16000.
	SampleRate int
	// Threshold is the speech probability above which we detect voice.
	// Valid range: (0, 1). A good default is 0.5.
	Threshold float32
	// MinSilenceDurationMs is the minimum silence duration (ms) before
	// splitting a speech segment.
	MinSilenceDurationMs int
	// SpeechPadMs is padding (ms) added around speech segment boundaries.
	SpeechPadMs int
}

func (c DetectorConfig) validate() error {
	if c.ModelPath == "" {
		return fmt.Errorf("invalid ModelPath: should not be empty")
	}
	if c.SampleRate != 8000 && c.SampleRate != 16000 {
		return fmt.Errorf("invalid SampleRate: valid values are 8000 and 16000")
	}
	if c.Threshold <= 0 || c.Threshold >= 1 {
		return fmt.Errorf("invalid Threshold: should be in range (0, 1)")
	}
	if c.MinSilenceDurationMs < 0 {
		return fmt.Errorf("invalid MinSilenceDurationMs: should be a positive number")
	}
	if c.SpeechPadMs < 0 {
		return fmt.Errorf("invalid SpeechPadMs: should be a positive number")
	}
	return nil
}

// -----------------------------------------------------------------------------
// Segment
// -----------------------------------------------------------------------------

// Segment contains timing information of a detected speech segment.
type Segment struct {
	// SpeechStartAt is the relative timestamp in seconds where speech begins.
	// -1 means no speech-start event in this segment entry.
	SpeechStartAt float64
	// SpeechEndAt is the relative timestamp in seconds where speech ends.
	// -1 means no speech-end event in this segment entry.
	SpeechEndAt float64
}

// -----------------------------------------------------------------------------
// Detector
// -----------------------------------------------------------------------------

// Detector performs voice activity detection using the Silero ONNX model.
// It manages the ONNX Runtime session and maintains stateful hidden state
// across successive calls to Detect.
//
// NOT safe for concurrent use — the caller must serialize access.
type Detector struct {
	// ONNX Runtime handles (CGO-managed)
	api         *C.OrtApi
	env         *C.OrtEnv
	sessionOpts *C.OrtSessionOptions
	session     *C.OrtSession
	memoryInfo  *C.OrtMemoryInfo

	// Pre-allocated C strings for ONNX tensor names (freed on Destroy)
	cStrings map[string]*C.char

	cfg DetectorConfig

	// Model hidden state carried across inference calls
	state [stateLen]float32
	// Trailing samples from the previous window for temporal context
	ctx [contextLen]float32
	// Pending samples (< window size) carried across Detect calls
	pending []float32

	// Speech segmentation state
	currSample int
	triggered  bool
	tempEnd    int
}

// NewDetector creates a Detector by loading the ONNX model and
// initializing the inference session. Call Destroy when done.
func NewDetector(cfg DetectorConfig) (*Detector, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	sd := &Detector{
		cfg:      cfg,
		cStrings: map[string]*C.char{},
	}

	// Obtain the global ONNX Runtime API handle
	sd.api = C.OrtGetApi()
	if sd.api == nil {
		return nil, fmt.Errorf("failed to get ONNX Runtime API")
	}

	// Create ONNX environment
	sd.cStrings["loggerName"] = C.CString("silero_vad")
	status := C.OrtApiCreateEnv(sd.api, C.ORT_LOGGING_LEVEL_ERROR, sd.cStrings["loggerName"], &sd.env)
	defer C.OrtApiReleaseStatus(sd.api, status)
	if status != nil {
		sd.cleanup()
		return nil, fmt.Errorf("failed to create env: %s", C.GoString(C.OrtApiGetErrorMessage(sd.api, status)))
	}

	// Create session options: single-threaded, all optimizations
	status = C.OrtApiCreateSessionOptions(sd.api, &sd.sessionOpts)
	defer C.OrtApiReleaseStatus(sd.api, status)
	if status != nil {
		sd.cleanup()
		return nil, fmt.Errorf("failed to create session options: %s", C.GoString(C.OrtApiGetErrorMessage(sd.api, status)))
	}

	status = C.OrtApiSetIntraOpNumThreads(sd.api, sd.sessionOpts, 1)
	defer C.OrtApiReleaseStatus(sd.api, status)
	if status != nil {
		sd.cleanup()
		return nil, fmt.Errorf("failed to set intra op threads: %s", C.GoString(C.OrtApiGetErrorMessage(sd.api, status)))
	}

	status = C.OrtApiSetInterOpNumThreads(sd.api, sd.sessionOpts, 1)
	defer C.OrtApiReleaseStatus(sd.api, status)
	if status != nil {
		sd.cleanup()
		return nil, fmt.Errorf("failed to set inter op threads: %s", C.GoString(C.OrtApiGetErrorMessage(sd.api, status)))
	}

	status = C.OrtApiSetSessionGraphOptimizationLevel(sd.api, sd.sessionOpts, C.ORT_ENABLE_ALL)
	defer C.OrtApiReleaseStatus(sd.api, status)
	if status != nil {
		sd.cleanup()
		return nil, fmt.Errorf("failed to set optimization level: %s", C.GoString(C.OrtApiGetErrorMessage(sd.api, status)))
	}

	// Load model
	sd.cStrings["modelPath"] = C.CString(cfg.ModelPath)
	status = C.OrtApiCreateSession(sd.api, sd.env, sd.cStrings["modelPath"], sd.sessionOpts, &sd.session)
	defer C.OrtApiReleaseStatus(sd.api, status)
	if status != nil {
		sd.cleanup()
		return nil, fmt.Errorf("failed to create session: %s", C.GoString(C.OrtApiGetErrorMessage(sd.api, status)))
	}

	// CPU memory allocator for tensor creation
	status = C.OrtApiCreateCpuMemoryInfo(sd.api, C.OrtArenaAllocator, C.OrtMemTypeDefault, &sd.memoryInfo)
	defer C.OrtApiReleaseStatus(sd.api, status)
	if status != nil {
		sd.cleanup()
		return nil, fmt.Errorf("failed to create memory info: %s", C.GoString(C.OrtApiGetErrorMessage(sd.api, status)))
	}

	// Pre-allocate C strings for tensor I/O names
	sd.cStrings["input"] = C.CString("input")
	sd.cStrings["sr"] = C.CString("sr")
	sd.cStrings["state"] = C.CString("state")
	sd.cStrings["stateN"] = C.CString("stateN")
	sd.cStrings["output"] = C.CString("output")

	return sd, nil
}

// Detect runs speech detection on a buffer of float32 PCM samples.
// Returns detected speech segments with start/end times.
//
// If the buffer is smaller than the required window size (512 for 16kHz,
// 256 for 8kHz), the call returns nil with no error — this is normal
// for small network chunks at stream boundaries.
//
// The detector is stateful: call Detect with successive audio chunks
// from the same stream to maintain temporal context. Speech may start
// in one call and end in a later call.
func (sd *Detector) Detect(pcm []float32) ([]Segment, error) {
	if sd == nil {
		return nil, fmt.Errorf("invalid nil detector")
	}

	windowSize := 512
	if sd.cfg.SampleRate == 8000 {
		windowSize = 256
	}

	// Merge pending remainder from previous call with current audio.
	input := pcm
	if len(sd.pending) > 0 {
		merged := make([]float32, len(sd.pending)+len(pcm))
		copy(merged, sd.pending)
		copy(merged[len(sd.pending):], pcm)
		input = merged
	}

	// Small chunks are normal at stream boundaries — carry forward.
	if len(input) < windowSize {
		sd.pending = append(sd.pending[:0], input...)
		return nil, nil
	}

	minSilenceSamples := sd.cfg.MinSilenceDurationMs * sd.cfg.SampleRate / 1000
	speechPadSamples := sd.cfg.SpeechPadMs * sd.cfg.SampleRate / 1000

	var segments []Segment
	fullWindows := len(input) / windowSize
	for w := 0; w < fullWindows; w++ {
		i := w * windowSize
		speechProb, err := sd.infer(input[i : i+windowSize])
		if err != nil {
			return nil, fmt.Errorf("infer failed: %w", err)
		}

		sd.currSample += windowSize

		// Speech resumes during a silence measurement — cancel the silence timer
		if speechProb >= sd.cfg.Threshold && sd.tempEnd != 0 {
			sd.tempEnd = 0
		}

		// Speech onset
		if speechProb >= sd.cfg.Threshold && !sd.triggered {
			sd.triggered = true
			speechStartAt := float64(sd.currSample-windowSize-speechPadSamples) / float64(sd.cfg.SampleRate)
			if speechStartAt < 0 {
				speechStartAt = 0
			}
			segments = append(segments, Segment{
				SpeechStartAt: speechStartAt,
				SpeechEndAt:   -1,
			})
		}

		// Speech offset (with hysteresis)
		if speechProb < (sd.cfg.Threshold-0.15) && sd.triggered {
			if sd.tempEnd == 0 {
				sd.tempEnd = sd.currSample
			}

			// Not enough silence yet to split
			if sd.currSample-sd.tempEnd < minSilenceSamples {
				continue
			}

			speechEndAt := float64(sd.tempEnd+speechPadSamples) / float64(sd.cfg.SampleRate)
			sd.tempEnd = 0
			sd.triggered = false

			// Speech started in a previous Detect() call — no start event in
			// this call, but emit an end marker so callers can emit Event=end.
			if len(segments) == 0 {
				segments = append(segments, Segment{
					SpeechStartAt: -1,
					SpeechEndAt:   speechEndAt,
				})
				continue
			}
			segments[len(segments)-1].SpeechEndAt = speechEndAt
		}
	}

	// Preserve tail remainder for the next Detect() call.
	remainderStart := fullWindows * windowSize
	if remainderStart < len(input) {
		sd.pending = append(sd.pending[:0], input[remainderStart:]...)
	} else {
		sd.pending = sd.pending[:0]
	}

	return segments, nil
}

// Reset clears all stateful data: hidden state, context buffer, and
// speech segmentation counters. Use this to reuse the detector for
// a new audio stream without re-loading the model.
func (sd *Detector) Reset() {
	if sd == nil {
		return
	}
	sd.currSample = 0
	sd.triggered = false
	sd.tempEnd = 0
	for i := range sd.state {
		sd.state[i] = 0
	}
	for i := range sd.ctx {
		sd.ctx[i] = 0
	}
	sd.pending = sd.pending[:0]
}

// Destroy releases all ONNX Runtime resources. Safe to call on a nil
// or already-destroyed detector.
func (sd *Detector) Destroy() {
	if sd == nil {
		return
	}
	sd.cleanup()
}

// cleanup releases ONNX Runtime handles in reverse allocation order.
func (sd *Detector) cleanup() {
	if sd.memoryInfo != nil {
		C.OrtApiReleaseMemoryInfo(sd.api, sd.memoryInfo)
		sd.memoryInfo = nil
	}
	if sd.session != nil {
		C.OrtApiReleaseSession(sd.api, sd.session)
		sd.session = nil
	}
	if sd.sessionOpts != nil {
		C.OrtApiReleaseSessionOptions(sd.api, sd.sessionOpts)
		sd.sessionOpts = nil
	}
	if sd.env != nil {
		C.OrtApiReleaseEnv(sd.api, sd.env)
		sd.env = nil
	}
	for k, ptr := range sd.cStrings {
		C.free(unsafe.Pointer(ptr))
		delete(sd.cStrings, k)
	}
}
