// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_denoiser_rnnoise

/*
#cgo CFLAGS: -I.
#cgo LDFLAGS: -L./models -lrnnoise
#include <rnnoise.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

var frameSize int

func init() {
	frameSize = int(C.rnnoise_get_frame_size())
}

// RNNoise wraps the RNNoise C library with thread-safe processing
type RNNoise struct {
	mu           sync.Mutex
	denoiseState *C.DenoiseState
	frameCount   int
}

// NewRNNoise creates a new RNNoise instance
func NewRNNoise() (*RNNoise, error) {
	state := C.rnnoise_create(nil)
	if state == nil {
		return nil, fmt.Errorf("failed to create rnnoise state")
	}

	return &RNNoise{
		denoiseState: state,
		frameCount:   0,
	}, nil
}

// SuppressNoise processes a single frame of audio and returns confidence score and cleaned audio
// Input must be exactly frameSize samples (typically 480 at 48kHz)
// Audio must be at 48kHz sample rate for proper noise suppression
func (st *RNNoise) SuppressNoise(input []float32) (float64, []float32, error) {
	if st.denoiseState == nil {
		return 0, nil, fmt.Errorf("rnnoise state is not initialized")
	}

	if len(input) != frameSize {
		return 0, nil, fmt.Errorf("input must be exactly %d samples, got %d", frameSize, len(input))
	}

	output := make([]float32, frameSize)
	copy(output, input) // Copy input to output for in-place processing

	st.mu.Lock()
	defer st.mu.Unlock()

	// Process frame - rnnoise_process_frame modifies the output buffer in-place
	// and returns the VAD probability (0.0 = noise, 1.0 = speech)
	inputPtr := (*C.float)(unsafe.Pointer(&input[0]))
	outputPtr := (*C.float)(unsafe.Pointer(&output[0]))

	vad := C.rnnoise_process_frame(st.denoiseState, outputPtr, inputPtr)

	st.frameCount++

	return float64(vad), output, nil
}

// ProcessAudio processes multiple frames, preserving the exact input length
// while averaging the per-frame speech confidence returned by RNNoise.
func (st *RNNoise) ProcessAudio(input []float32) (float64, []float32, error) {
	if st.denoiseState == nil {
		return 0, nil, fmt.Errorf("rnnoise state is not initialized")
	}

	if len(input) == 0 {
		return 0, nil, fmt.Errorf("input audio is empty")
	}

	frameCount := (len(input) + frameSize - 1) / frameSize
	cleanedAudio := make([]float32, len(input))
	var totalConfidence float64

	st.mu.Lock()
	defer st.mu.Unlock()

	var paddedInput []float32
	var paddedOutput []float32

	for i := 0; i < len(input); i += frameSize {
		end := i + frameSize
		if end > len(input) {
			end = len(input)
		}

		chunk := input[i:end]
		outputChunk := cleanedAudio[i:end]

		var (
			inputPtr  *C.float
			outputPtr *C.float
		)

		if len(chunk) == frameSize {
			inputPtr = (*C.float)(unsafe.Pointer(&chunk[0]))
			outputPtr = (*C.float)(unsafe.Pointer(&outputChunk[0]))
		} else {
			if paddedInput == nil {
				paddedInput = make([]float32, frameSize)
				paddedOutput = make([]float32, frameSize)
			}
			clear(paddedInput)
			copy(paddedInput, chunk)
			inputPtr = (*C.float)(unsafe.Pointer(&paddedInput[0]))
			outputPtr = (*C.float)(unsafe.Pointer(&paddedOutput[0]))
		}

		vad := C.rnnoise_process_frame(st.denoiseState, outputPtr, inputPtr)
		totalConfidence += float64(vad)

		if len(chunk) < frameSize {
			copy(outputChunk, paddedOutput[:len(chunk)])
		}

		st.frameCount++
	}

	return totalConfidence / float64(frameCount), cleanedAudio, nil
}

// Close cleans up resources
func (st *RNNoise) Close() error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.denoiseState == nil {
		return fmt.Errorf("double-free attempt")
	}

	C.rnnoise_destroy(st.denoiseState)
	st.denoiseState = nil

	return nil
}
