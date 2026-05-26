// Copyright (c) 2023-2026 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_recorder

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	internal_audio "github.com/rapidaai/api/assistant-api/internal/audio"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
)

// Audio format constants for LINEAR16 PCM encoding.
const (
	AudioBytesPerSample = 2  // LINEAR16 → 2 bytes per sample
	AudioBitsPerSample  = 16 // LINEAR16 → 16 bits per sample
	AudioPCMFormat      = 1  // WAV PCM format tag
	wavHeaderSize       = 44 // Standard WAV header: RIFF(12) + fmt(24) + data-header(8)
)

// trackCount is the number of independent audio tracks (user + system).
const trackCount = 2

// Track indices for the dual-track recording model.
const (
	trackUser   = 0 // Microphone / user-side audio
	trackSystem = 1 // TTS / system-side audio
)

var audioConfig = internal_audio.RAPIDA_INTERNAL_AUDIO_CONFIG

// chunk is a recorded audio fragment placed at a specific byte position on
// the timeline. ByteOffset is measured in bytes from the recording start.
type chunk struct {
	ByteOffset int    // Byte position relative to Start()
	Data       []byte // Raw PCM audio data
	Track      int    // trackUser or trackSystem
}

// audioRecorder implements the Recorder interface by capturing user and
// system audio onto two independent timeline-aligned tracks. Chunks are
// positioned based on wall-clock arrival time, and the final output is
// rendered as a pair of equal-length WAV files (one per track).
type audioRecorder struct {
	logger    commons.Logger
	mu        sync.Mutex
	startTime time.Time
	started   bool
	chunks    []chunk

	// cursor tracks the byte position just past the last written byte on
	// each track. For the user track, wall-clock placement is authoritative.
	// For the system (TTS) track, the cursor paces burst audio at the
	// playback rate — only the first chunk after a silence gap uses
	// wall-clock to anchor its position.
	cursor [trackCount]int

	// clock is injectable for deterministic testing; defaults to time.Now.
	clock func() time.Time
}

// NewDefaultAudioRecorder creates a timeline-based dual-track audio recorder.
// The returned Recorder is safe for concurrent use.
func NewDefaultAudioRecorder(logger commons.Logger) (internal_type.Recorder, error) {
	return &audioRecorder{
		logger: logger,
		clock:  time.Now,
	}, nil
}

// Start begins the recording session. Both tracks share this start time.
// All subsequent Record calls are placed on the timeline relative to this
// moment.
func (r *audioRecorder) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startTime = r.clock()
	r.started = true
}

// bytesPerSecond returns the PCM byte rate for the internal audio format.
func bytesPerSecond() int {
	return internal_audio.BytesPerSecond(audioConfig)
}

// frameSize returns the number of bytes in a single audio frame (all channels).
func frameSize() int {
	return internal_audio.FrameSize(audioConfig)
}

// durationBytes converts a wall-clock duration to a frame-aligned byte count.
func durationBytes(d time.Duration) int {
	raw := int(d.Seconds() * float64(bytesPerSecond()))
	fs := frameSize()
	return (raw / fs) * fs
}

// Record places audio on the appropriate track at the current wall-clock
// position. Each chunk is positioned based on WHEN it arrives, not just
// appended. Both tracks share the same timeline (Start → Persist).
//
// Supported packet types:
//   - UserAudioReceivedPacket:          placed on the user track at wall-clock offset
//   - TextToSpeechAudioPacket:  placed on the system track with burst pacing
//   - InterruptionDetectedPacket:       truncates system track at current wall-clock,
//     mirroring the streamer's ClearOutputBuffer behaviour
//
// Unrecognised packet types are silently ignored.
func (r *audioRecorder) Record(_ context.Context, p internal_type.Packet) error {
	switch pkt := p.(type) {
	case internal_type.RecordUserAudioPacket:
		return r.push(pkt.Audio, trackUser)
	case internal_type.RecordAssistantAudioPacket:
		if pkt.Truncate {
			r.truncateSystemTrack()
			return nil
		}
		return r.push(pkt.Audio, trackSystem)
	// Legacy packet types kept for backward compatibility
	case internal_type.UserAudioReceivedPacket:
		return r.push(pkt.Audio, trackUser)
	case internal_type.TextToSpeechAudioPacket:
		return r.push(pkt.AudioChunk, trackSystem)
	case internal_type.InterruptionDetectedPacket:
		r.truncateSystemTrack()
		return nil
	}
	return nil
}

// push appends a PCM chunk to the specified track at the appropriate timeline
// position. Empty data is silently ignored. The caller's slice is copied to
// prevent external mutation.
func (r *audioRecorder) push(data []byte, track int) error {
	if len(data) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	offset := r.resolveOffset(track, r.wallClockOffsetBytes())

	// Copy to decouple from the caller's buffer.
	buf := make([]byte, len(data))
	copy(buf, data)

	r.chunks = append(r.chunks, chunk{
		ByteOffset: offset,
		Data:       buf,
		Track:      track,
	})

	// Advance cursor past this chunk.
	r.cursor[track] = offset + len(buf)
	return nil
}

// resolveOffset determines the byte-offset at which a new chunk should be
// placed on the given track, considering both the wall-clock position and
// the track cursor.
func (r *audioRecorder) resolveOffset(track, wallOffset int) int {
	switch track {
	case trackUser:
		// User (mic) audio: wall-clock placement. Microphone delivers at
		// real-time rate, so wall-clock offset is the correct position.
		// If the cursor is ahead (back-to-back packets), use the cursor.
		if r.cursor[track] > wallOffset {
			return r.cursor[track]
		}
		return wallOffset

	case trackSystem:
		// Burst continuation: TTS chunks arrive faster than real-time
		// playback; pace them contiguously from the cursor.
		if r.cursor[track] > wallOffset {
			return r.cursor[track]
		}
		// New TTS segment after a gap: anchor at wall-clock.
		return wallOffset

	default:
		return wallOffset
	}
}

// truncateSystemTrack removes any system (TTS) audio that extends past the
// current wall-clock position. When the user interrupts, the streamer clears
// its output buffer (ClearOutputBuffer) so queued TTS audio is never played.
// The recorder must mirror this: any system audio recorded beyond "now" on
// the timeline is audio the listener never heard.
func (r *audioRecorder) truncateSystemTrack() {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := r.wallClockOffsetBytes()

	// Rebuild the chunk list in-place, trimming or removing system chunks
	// that extend past the cutoff.
	kept := r.chunks[:0] // reuse backing array
	for _, c := range r.chunks {
		if c.Track != trackSystem {
			kept = append(kept, c)
			continue
		}

		chunkEnd := c.ByteOffset + len(c.Data)

		switch {
		case chunkEnd <= cutoff:
			// Entirely before the cut — keep as-is.
			kept = append(kept, c)
		case c.ByteOffset >= cutoff:
			// Entirely after the cut — discard.
		default:
			// Partially overlaps — trim to cutoff.
			trimmed := c
			trimmed.Data = c.Data[:cutoff-c.ByteOffset]
			kept = append(kept, trimmed)
		}
	}
	r.chunks = kept

	// Reset system cursor to cutoff so the next TTS segment anchors
	// correctly.
	r.cursor[trackSystem] = cutoff
}

// wallClockOffsetBytes returns the current wall-clock position as a
// frame-aligned byte offset from the recording start. Returns 0 if the
// recorder has not been started.
//
// Caller must hold r.mu.
func (r *audioRecorder) wallClockOffsetBytes() int {
	if !r.started {
		return 0
	}
	return durationBytes(r.clock().Sub(r.startTime))
}

// Persist renders user, assistant, and conversation WAV files. All outputs span
// the full session duration (Start → Persist). Audio chunks are painted at
// their recorded timeline positions; gaps are zero-filled (silence).
func (r *audioRecorder) Persist() (userWAV, systemWAV, conversationWAV []byte, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.chunks) == 0 {
		return nil, nil, nil, fmt.Errorf("no audio chunks to persist")
	}

	totalLen := r.computeBufferLength()

	// Allocate zero-filled (silence) buffers for each track.
	trackPCM := [trackCount][]byte{
		make([]byte, totalLen), // user
		make([]byte, totalLen), // system
	}

	// Paint each chunk onto its track buffer.
	audioBytes := [trackCount]int{}
	for _, c := range r.chunks {
		copy(trackPCM[c.Track][c.ByteOffset:], c.Data)
		audioBytes[c.Track] += len(c.Data)
	}

	userInfo := internal_audio.GetAudioInfo(trackPCM[trackUser][:audioBytes[trackUser]], audioConfig)
	systemInfo := internal_audio.GetAudioInfo(trackPCM[trackSystem][:audioBytes[trackSystem]], audioConfig)
	totalInfo := internal_audio.GetAudioInfo(trackPCM[trackUser], audioConfig)
	r.logger.Infow(fmt.Sprintf(
		"Audio persist: userAudio=%d (%.2fms), systemAudio=%d (%.2fms), totalLen=%d (%.2fms), chunks=%d",
		audioBytes[trackUser], userInfo.DurationMs,
		audioBytes[trackSystem], systemInfo.DurationMs,
		totalLen, totalInfo.DurationMs,
		len(r.chunks),
	))

	userWAV, err = encodeWAV(trackPCM[trackUser], 1)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encoding user WAV: %w", err)
	}
	systemWAV, err = encodeWAV(trackPCM[trackSystem], 1)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encoding system WAV: %w", err)
	}
	conversationWAV, err = encodeWAV(interleaveStereo(trackPCM[trackUser], trackPCM[trackSystem]), 2)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encoding conversation WAV: %w", err)
	}
	return userWAV, systemWAV, conversationWAV, nil
}

// computeBufferLength returns the PCM buffer size needed to hold the entire
// recording session, accounting for both the session duration and the
// furthest chunk endpoint.
//
// Caller must hold r.mu.
func (r *audioRecorder) computeBufferLength() int {
	sessionBytes := 0
	if r.started {
		sessionBytes = durationBytes(r.clock().Sub(r.startTime))
	}

	totalLen := sessionBytes
	for _, c := range r.chunks {
		if end := c.ByteOffset + len(c.Data); end > totalLen {
			totalLen = end
		}
	}
	return totalLen
}

func interleaveStereo(leftPCM, rightPCM []byte) []byte {
	samples := min(len(leftPCM), len(rightPCM)) / AudioBytesPerSample
	stereo := make([]byte, samples*AudioBytesPerSample*2)
	for i := 0; i < samples; i++ {
		monoOffset := i * AudioBytesPerSample
		stereoOffset := i * AudioBytesPerSample * 2
		copy(stereo[stereoOffset:stereoOffset+AudioBytesPerSample], leftPCM[monoOffset:monoOffset+AudioBytesPerSample])
		copy(stereo[stereoOffset+AudioBytesPerSample:stereoOffset+AudioBytesPerSample*2], rightPCM[monoOffset:monoOffset+AudioBytesPerSample])
	}
	return stereo
}

// encodeWAV wraps raw PCM data in a canonical WAV (RIFF) container.
// Format: 16-bit LINEAR PCM at the configured sample rate and channel count.
func encodeWAV(pcmData []byte, channels int) ([]byte, error) {
	sampleRate := audioConfig.SampleRate
	blockAlign := uint16(channels) * uint16(AudioBytesPerSample)
	byteRate := uint32(sampleRate) * uint32(blockAlign)

	// Pre-allocate: header (44 bytes) + PCM payload.
	buf := bytes.NewBuffer(make([]byte, 0, wavHeaderSize+len(pcmData)))

	// --- RIFF header ---
	buf.Write([]byte("RIFF"))
	if err := binary.Write(buf, binary.LittleEndian, uint32(36+len(pcmData))); err != nil {
		return nil, fmt.Errorf("writing RIFF size: %w", err)
	}
	buf.Write([]byte("WAVE"))

	// --- fmt sub-chunk ---
	buf.Write([]byte("fmt "))
	if err := binary.Write(buf, binary.LittleEndian, uint32(16)); err != nil {
		return nil, fmt.Errorf("writing fmt chunk size: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, uint16(AudioPCMFormat)); err != nil {
		return nil, fmt.Errorf("writing audio format: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, uint16(channels)); err != nil {
		return nil, fmt.Errorf("writing channels: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return nil, fmt.Errorf("writing sample rate: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, byteRate); err != nil {
		return nil, fmt.Errorf("writing byte rate: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, blockAlign); err != nil {
		return nil, fmt.Errorf("writing block align: %w", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, uint16(AudioBitsPerSample)); err != nil {
		return nil, fmt.Errorf("writing bits per sample: %w", err)
	}

	// --- data sub-chunk ---
	buf.Write([]byte("data"))
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(pcmData))); err != nil {
		return nil, fmt.Errorf("writing data chunk size: %w", err)
	}
	buf.Write(pcmData)

	return buf.Bytes(), nil
}
