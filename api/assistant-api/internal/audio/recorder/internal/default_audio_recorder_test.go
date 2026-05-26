// Copyright (c) 2023-2026 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_recorder

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/commons"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestLogger creates a logger that writes to a temporary directory scoped
// to the running test.
func newTestLogger(t *testing.T) commons.Logger {
	t.Helper()
	logger, err := commons.NewApplicationLogger(
		commons.Name("test-recorder"),
		commons.Path(t.TempDir()),
		commons.Level("debug"),
	)
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}
	return logger
}

// fakeClock is a controllable clock for deterministic timeline tests.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

// newTestRecorderWithClock returns an audioRecorder wired to a fakeClock
// starting at 2025-01-01T00:00:00Z.
func newTestRecorderWithClock(t *testing.T) (*audioRecorder, *fakeClock) {
	t.Helper()
	rec, err := NewDefaultAudioRecorder(newTestLogger(t))
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}
	ar := rec.(*audioRecorder)
	fc := &fakeClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	ar.clock = fc.Now
	return ar, fc
}

// pcm generates a byte slice of the given length filled with val, simulating
// constant-amplitude PCM audio.
func pcm(val byte, length int) []byte {
	buf := make([]byte, length)
	for i := range buf {
		buf[i] = val
	}
	return buf
}

// wavPCMData strips the 44-byte WAV header and returns the raw PCM payload.
func wavPCMData(wav []byte) []byte { return wav[wavHeaderSize:] }

func wavChannels(wav []byte) uint16 { return binary.LittleEndian.Uint16(wav[22:24]) }

// ---------------------------------------------------------------------------
// Basic recording
// ---------------------------------------------------------------------------

func TestRecordUserAudio(t *testing.T) {
	rec, _ := newTestRecorderWithClock(t)
	rec.Start()
	data := pcm(0x01, 320)
	rec.Record(context.Background(), internal_type.UserAudioReceivedPacket{Audio: data})

	if len(rec.chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(rec.chunks))
	}
	c := rec.chunks[0]
	if c.Track != trackUser {
		t.Errorf("expected trackUser")
	}
	if c.ByteOffset != 0 {
		t.Errorf("expected offset 0, got %d", c.ByteOffset)
	}
	if len(c.Data) != 320 {
		t.Errorf("expected 320 bytes, got %d", len(c.Data))
	}
}

func TestRecordSystemAudio(t *testing.T) {
	rec, _ := newTestRecorderWithClock(t)
	rec.Start()
	rec.Record(context.Background(), internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0x02, 640)})

	if len(rec.chunks) != 1 || rec.chunks[0].Track != trackSystem {
		t.Errorf("expected 1 system chunk")
	}
}

func TestRecordEmptyDataIsIgnored(t *testing.T) {
	rec, _ := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()
	rec.Record(ctx, internal_type.UserAudioReceivedPacket{Audio: nil})
	rec.Record(ctx, internal_type.UserAudioReceivedPacket{Audio: []byte{}})
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c", AudioChunk: nil})

	if len(rec.chunks) != 0 {
		t.Fatalf("expected 0 chunks, got %d", len(rec.chunks))
	}
}

// ---------------------------------------------------------------------------
// Wall-clock placement
// ---------------------------------------------------------------------------

func TestTimelineBasedPlacement(t *testing.T) {
	// 16kHz mono 16-bit = 32000 bytes/sec
	//   t=0ms   : user speaks 100 bytes  → offset 0
	//   t=100ms : system speaks 200 bytes → offset 3200 (100ms * 32000)
	rec, fc := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	rec.Record(ctx, internal_type.UserAudioReceivedPacket{Audio: pcm(0x11, 100)})

	fc.Advance(100 * time.Millisecond)
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0x22, 200)})

	userWAV, systemWAV, _, err := rec.Persist()
	if err != nil {
		t.Fatalf("Persist error: %v", err)
	}
	userPCM := wavPCMData(userWAV)
	systemPCM := wavPCMData(systemWAV)

	// Total = max(sessionBytes=3200+200=3400, ...) = 3400
	expectedLen := 3200 + 200 // 100ms offset + 200 bytes data
	if len(userPCM) != expectedLen || len(systemPCM) != expectedLen {
		t.Fatalf("expected %d bytes each, got user=%d system=%d", expectedLen, len(userPCM), len(systemPCM))
	}

	// User audio at offset 0..100
	for i := 0; i < 100; i++ {
		if userPCM[i] != 0x11 {
			t.Errorf("user byte %d: expected 0x11, got 0x%02x", i, userPCM[i])
			break
		}
	}
	// User silence from 100 onward
	for i := 100; i < expectedLen; i++ {
		if userPCM[i] != 0x00 {
			t.Errorf("user byte %d: expected silence, got 0x%02x", i, userPCM[i])
			break
		}
	}

	// System silence from 0..3200
	for i := 0; i < 3200; i++ {
		if systemPCM[i] != 0x00 {
			t.Errorf("system byte %d: expected silence, got 0x%02x", i, systemPCM[i])
			break
		}
	}
	// System audio at 3200..3400
	for i := 3200; i < 3400; i++ {
		if systemPCM[i] != 0x22 {
			t.Errorf("system byte %d: expected 0x22, got 0x%02x", i, systemPCM[i])
			break
		}
	}
}

func TestTracksAreIndependent(t *testing.T) {
	// User and system audio at the same wall-clock time land on their own
	// tracks at offset 0 — they don't interfere.
	rec, _ := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	rec.Record(ctx, internal_type.UserAudioReceivedPacket{Audio: pcm(0x11, 100)})
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0x22, 150)})

	userWAV, systemWAV, _, err := rec.Persist()
	if err != nil {
		t.Fatalf("Persist error: %v", err)
	}
	userPCM := wavPCMData(userWAV)
	systemPCM := wavPCMData(systemWAV)

	// Both tracks must be the same total length = max(100, 150) = 150
	if len(userPCM) != 150 || len(systemPCM) != 150 {
		t.Fatalf("expected 150 bytes each, got user=%d system=%d", len(userPCM), len(systemPCM))
	}
	// User: 100 audio + 50 silence
	if userPCM[0] != 0x11 || userPCM[99] != 0x11 || userPCM[100] != 0x00 {
		t.Error("user track layout wrong")
	}
	// System: 150 audio
	if systemPCM[0] != 0x22 || systemPCM[149] != 0x22 {
		t.Error("system track layout wrong")
	}
}

func TestTTSBurstDoesNotOverlap(t *testing.T) {
	// Two TTS chunks arrive at the same wall-clock instant.
	// Cursor ensures they are placed back-to-back, not overlapping.
	rec, _ := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0xAA, 100)})
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0xBB, 100)})

	if len(rec.chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(rec.chunks))
	}
	// First at offset 0, second at offset 100 (cursor advanced).
	if rec.chunks[0].ByteOffset != 0 {
		t.Errorf("chunk 0: expected offset 0, got %d", rec.chunks[0].ByteOffset)
	}
	if rec.chunks[1].ByteOffset != 100 {
		t.Errorf("chunk 1: expected offset 100, got %d", rec.chunks[1].ByteOffset)
	}

	_, systemWAV, _, err := rec.Persist()
	if err != nil {
		t.Fatalf("Persist error: %v", err)
	}
	sysPCM := wavPCMData(systemWAV)

	if len(sysPCM) < 200 {
		t.Fatalf("expected at least 200 bytes, got %d", len(sysPCM))
	}
	for i := 0; i < 100; i++ {
		if sysPCM[i] != 0xAA {
			t.Errorf("byte %d: expected 0xAA, got 0x%02x", i, sysPCM[i])
			break
		}
	}
	for i := 100; i < 200; i++ {
		if sysPCM[i] != 0xBB {
			t.Errorf("byte %d: expected 0xBB, got 0x%02x", i, sysPCM[i])
			break
		}
	}
}

func TestTTSBurstFiveChunksContiguous(t *testing.T) {
	rec, _ := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		rec.Record(ctx, internal_type.TextToSpeechAudioPacket{
			ContextID:  "c1",
			AudioChunk: pcm(byte(i+1), 320),
		})
	}
	if len(rec.chunks) != 5 {
		t.Fatalf("expected 5 chunks, got %d", len(rec.chunks))
	}
	for i, c := range rec.chunks {
		expectedOffset := i * 320
		if c.ByteOffset != expectedOffset {
			t.Errorf("chunk %d: expected offset %d, got %d", i, expectedOffset, c.ByteOffset)
		}
	}
}

func TestTTSPacingNoGaps(t *testing.T) {
	// TTS chunks arrive with small wall-clock gaps between them (e.g. 5ms
	// between chunks), but each chunk represents 10ms of audio (320 bytes).
	// Without pacing the 5ms gaps would create silence between chunks,
	// breaking the audio. Pacing places them contiguously.
	rec, fc := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	// First TTS chunk at t=100ms → anchored at 100ms = 3200 bytes offset.
	fc.Advance(100 * time.Millisecond)
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0xA1, 320)}) // 10ms audio

	// Second TTS chunk at t=105ms (only 5ms later), but audio rate demands
	// it at offset 3520 (3200+320). With pacing cursor is at 3520, wall
	// offset is 3360 (105ms), so cursor wins → 3520.
	fc.Advance(5 * time.Millisecond)
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0xA2, 320)})

	// Third TTS chunk at t=110ms. Wall offset = 3520, cursor = 3840.
	// Cursor wins → 3840.
	fc.Advance(5 * time.Millisecond)
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0xA3, 320)})

	// Verify all 3 chunks are contiguous starting at 3200.
	if len(rec.chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(rec.chunks))
	}
	expectedOffsets := []int{3200, 3520, 3840}
	for i, c := range rec.chunks {
		if c.ByteOffset != expectedOffsets[i] {
			t.Errorf("chunk %d: expected offset %d, got %d", i, expectedOffsets[i], c.ByteOffset)
		}
	}

	fc.Advance(890 * time.Millisecond) // session = 1s
	_, systemWAV, _, err := rec.Persist()
	if err != nil {
		t.Fatalf("Persist error: %v", err)
	}
	sysPCM := wavPCMData(systemWAV)

	// Verify contiguous audio bytes — no silence gaps between chunks.
	for i := 3200; i < 3200+960; i++ {
		if sysPCM[i] == 0x00 {
			t.Errorf("byte %d: unexpected silence gap in paced TTS audio", i)
			break
		}
	}
}

func TestTTSNewSegmentAfterGap(t *testing.T) {
	// TTS audio, then a real gap (user speaks, TTS resumes later).
	// The second TTS segment should anchor at its wall-clock offset,
	// not continue from the old cursor.
	rec, fc := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	// First TTS at t=0: 320 bytes (10ms audio). Cursor → 320.
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0xA1, 320)})

	// Gap: 500ms pass. Wall offset = 16000, cursor = 320.
	// 16000 > 320, so this is a new segment → anchored at 16000.
	fc.Advance(500 * time.Millisecond)
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c2", AudioChunk: pcm(0xA2, 320)})

	if len(rec.chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(rec.chunks))
	}
	if rec.chunks[0].ByteOffset != 0 {
		t.Errorf("first chunk: expected offset 0, got %d", rec.chunks[0].ByteOffset)
	}
	expectedSecond := durationBytes(500 * time.Millisecond) // 16000
	if rec.chunks[1].ByteOffset != expectedSecond {
		t.Errorf("second chunk: expected offset %d, got %d", expectedSecond, rec.chunks[1].ByteOffset)
	}

	fc.Advance(500 * time.Millisecond)
	_, systemWAV, _, err := rec.Persist()
	if err != nil {
		t.Fatalf("Persist error: %v", err)
	}
	sysPCM := wavPCMData(systemWAV)

	// Silence between first and second TTS segment.
	for i := 320; i < expectedSecond; i++ {
		if sysPCM[i] != 0x00 {
			t.Errorf("byte %d: expected silence between segments, got 0x%02x", i, sysPCM[i])
			break
		}
	}
	// Second segment present.
	for i := expectedSecond; i < expectedSecond+320; i++ {
		if sysPCM[i] != 0xA2 {
			t.Errorf("byte %d: expected 0xA2, got 0x%02x", i, sysPCM[i])
			break
		}
	}
}

// ---------------------------------------------------------------------------
// Interruption — truncate system track
// ---------------------------------------------------------------------------

func TestInterruptionTruncatesSystemTrack(t *testing.T) {
	// TTS sends 3 chunks of 320 bytes each (30ms total audio) at t=0.
	// User interrupts at t=10ms (= 320 bytes into the session).
	// The streamer would ClearOutputBuffer, so only the first 320 bytes
	// of TTS audio were actually heard.  The recorder must mirror that.
	rec, fc := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	// 3 TTS chunks at t=0 — paced contiguously at offsets 0, 320, 640.
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0xA1, 320)})
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0xA2, 320)})
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0xA3, 320)})

	if len(rec.chunks) != 3 {
		t.Fatalf("expected 3 chunks before interruption, got %d", len(rec.chunks))
	}

	// User interrupts at t=10ms (320 bytes).
	fc.Advance(10 * time.Millisecond)
	rec.Record(ctx, internal_type.InterruptionDetectedPacket{ContextID: "c1", Source: internal_type.InterruptionSourceVad})

	// Only the first chunk (offset 0, 320 bytes) should survive — it ends
	// exactly at cutoff=320, so chunkEnd(320) <= cutoff(320) → kept.
	// Chunk 2 starts at 320 → offset >= cutoff → discarded.
	// Chunk 3 starts at 640 → discarded.
	if len(rec.chunks) != 1 {
		t.Fatalf("expected 1 chunk after interruption, got %d", len(rec.chunks))
	}
	if rec.chunks[0].ByteOffset != 0 || len(rec.chunks[0].Data) != 320 {
		t.Errorf("surviving chunk: offset=%d len=%d", rec.chunks[0].ByteOffset, len(rec.chunks[0].Data))
	}

	// System cursor should be reset to cutoff.
	if rec.cursor[trackSystem] != 320 {
		t.Errorf("system cursor: expected 320, got %d", rec.cursor[trackSystem])
	}
}

func TestInterruptionPartialTrim(t *testing.T) {
	// One large TTS chunk (640 bytes = 20ms audio) placed at offset 0.
	// Interruption at t=10ms (cutoff=320). The chunk should be trimmed
	// to 320 bytes, not fully removed.
	rec, fc := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0xBB, 640)})

	fc.Advance(10 * time.Millisecond) // cutoff = 320
	rec.Record(ctx, internal_type.InterruptionDetectedPacket{ContextID: "c1", Source: internal_type.InterruptionSourceWord})

	if len(rec.chunks) != 1 {
		t.Fatalf("expected 1 trimmed chunk, got %d", len(rec.chunks))
	}
	if len(rec.chunks[0].Data) != 320 {
		t.Errorf("trimmed chunk: expected 320 bytes, got %d", len(rec.chunks[0].Data))
	}

	fc.Advance(490 * time.Millisecond) // session 500ms
	_, systemWAV, _, err := rec.Persist()
	if err != nil {
		t.Fatalf("Persist error: %v", err)
	}
	sysPCM := wavPCMData(systemWAV)

	// First 320 bytes = 0xBB, rest = silence.
	for i := 0; i < 320; i++ {
		if sysPCM[i] != 0xBB {
			t.Errorf("byte %d: expected 0xBB, got 0x%02x", i, sysPCM[i])
			break
		}
	}
	for i := 320; i < len(sysPCM); i++ {
		if sysPCM[i] != 0x00 {
			t.Errorf("byte %d: expected silence, got 0x%02x", i, sysPCM[i])
			break
		}
	}
}

func TestInterruptionPreservesUserTrack(t *testing.T) {
	// User audio is NOT affected by interruption.
	rec, fc := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	rec.Record(ctx, internal_type.UserAudioReceivedPacket{Audio: pcm(0x11, 640)})
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0x22, 640)})

	fc.Advance(10 * time.Millisecond) // cutoff = 320
	rec.Record(ctx, internal_type.InterruptionDetectedPacket{ContextID: "c1", Source: internal_type.InterruptionSourceVad})

	// User chunk (640 bytes) untouched.  System chunk trimmed to 320.
	userChunks := 0
	sysChunks := 0
	for _, c := range rec.chunks {
		if c.Track == trackUser {
			userChunks++
			if len(c.Data) != 640 {
				t.Errorf("user chunk size: expected 640, got %d", len(c.Data))
			}
		} else {
			sysChunks++
			if len(c.Data) != 320 {
				t.Errorf("system chunk size after trim: expected 320, got %d", len(c.Data))
			}
		}
	}
	if userChunks != 1 {
		t.Errorf("expected 1 user chunk, got %d", userChunks)
	}
	if sysChunks != 1 {
		t.Errorf("expected 1 system chunk, got %d", sysChunks)
	}
}

func TestInterruptionThenNewTTS(t *testing.T) {
	// After an interruption, new TTS audio should anchor at wall-clock.
	rec, fc := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	// TTS at t=0: 960 bytes (30ms audio) paced at offset 0..960.
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0xA1, 960)})

	// Interrupt at t=10ms (cutoff = 320).
	fc.Advance(10 * time.Millisecond)
	rec.Record(ctx, internal_type.InterruptionDetectedPacket{ContextID: "c1", Source: internal_type.InterruptionSourceVad})

	// After interrupt, cursor is at 320, so first chunk is trimmed.
	if rec.cursor[trackSystem] != 320 {
		t.Errorf("cursor after interrupt: expected 320, got %d", rec.cursor[trackSystem])
	}

	// New TTS at t=500ms (offset 16000). Cursor = 320, wall = 16000.
	// wall > cursor → new segment anchored at 16000.
	fc.Advance(490 * time.Millisecond)
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c2", AudioChunk: pcm(0xB1, 320)})

	// Find the new chunk.
	var newChunk *chunk
	for i := range rec.chunks {
		if rec.chunks[i].Data[0] == 0xB1 {
			newChunk = &rec.chunks[i]
		}
	}
	if newChunk == nil {
		t.Fatal("new TTS chunk not found")
	}
	expectedOffset := durationBytes(500 * time.Millisecond)
	if newChunk.ByteOffset != expectedOffset {
		t.Errorf("new TTS offset: expected %d, got %d", expectedOffset, newChunk.ByteOffset)
	}
}

// ---------------------------------------------------------------------------
// Data isolation
// ---------------------------------------------------------------------------

func TestPushCopiesData(t *testing.T) {
	rec, _ := newTestRecorderWithClock(t)
	rec.Start()
	data := pcm(0xFF, 100)
	rec.Record(context.Background(), internal_type.UserAudioReceivedPacket{Audio: data})
	data[0] = 0x00
	if rec.chunks[0].Data[0] != 0xFF {
		t.Error("push must copy data")
	}
}

// ---------------------------------------------------------------------------
// Persist
// ---------------------------------------------------------------------------

func TestPersistEmptyReturnsError(t *testing.T) {
	rec, _ := newTestRecorderWithClock(t)
	rec.Start()
	if _, _, _, err := rec.Persist(); err == nil {
		t.Fatal("expected error for empty recorder")
	}
}

func TestPersistProducesValidWAV(t *testing.T) {
	rec, fc := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	rec.Record(ctx, internal_type.UserAudioReceivedPacket{Audio: pcm(0x01, 3200)})
	fc.Advance(200 * time.Millisecond)
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0x02, 6400)})

	// Advance clock so session = 500ms
	fc.Advance(300 * time.Millisecond)

	userWAV, systemWAV, _, err := rec.Persist()
	if err != nil {
		t.Fatalf("Persist error: %v", err)
	}
	for name, wav := range map[string][]byte{"user": userWAV, "system": systemWAV} {
		if len(wav) < 44 {
			t.Fatalf("%s WAV too short", name)
		}
		if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
			t.Errorf("%s WAV missing RIFF/WAVE header", name)
		}
		if sr := binary.LittleEndian.Uint32(wav[24:28]); sr != audioConfig.SampleRate {
			t.Errorf("%s sample rate: got %d", name, sr)
		}
	}
	// Both WAVs must have the same PCM length.
	if len(wavPCMData(userWAV)) != len(wavPCMData(systemWAV)) {
		t.Error("user and system WAV PCM lengths differ")
	}
}

func TestPersistProducesConversationStereoWAV(t *testing.T) {
	rec, fc := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	rec.Record(ctx, internal_type.UserAudioReceivedPacket{Audio: pcm(0x11, 4)})
	fc.Advance(10 * time.Millisecond)
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0x22, 4)})

	_, _, conversationWAV, err := rec.Persist()
	if err != nil {
		t.Fatalf("Persist error: %v", err)
	}
	if got := wavChannels(conversationWAV); got != 2 {
		t.Fatalf("conversation WAV channels = %d, want 2", got)
	}

	conversationPCM := wavPCMData(conversationWAV)
	if len(conversationPCM) < 648 {
		t.Fatalf("conversation PCM too short: %d", len(conversationPCM))
	}

	// First stereo frame: user on left, assistant silence on right.
	if conversationPCM[0] != 0x11 || conversationPCM[1] != 0x11 {
		t.Fatalf("left channel at start = % x, want user audio", conversationPCM[0:2])
	}
	if conversationPCM[2] != 0x00 || conversationPCM[3] != 0x00 {
		t.Fatalf("right channel at start = % x, want silence", conversationPCM[2:4])
	}

	// Assistant begins at 10ms: left silence, assistant on right.
	assistantFrameOffset := durationBytes(10*time.Millisecond) / AudioBytesPerSample * AudioBytesPerSample * 2
	if conversationPCM[assistantFrameOffset] != 0x00 || conversationPCM[assistantFrameOffset+1] != 0x00 {
		t.Fatalf("left channel at assistant offset = % x, want silence", conversationPCM[assistantFrameOffset:assistantFrameOffset+2])
	}
	if conversationPCM[assistantFrameOffset+2] != 0x22 || conversationPCM[assistantFrameOffset+3] != 0x22 {
		t.Fatalf("right channel at assistant offset = % x, want assistant audio", conversationPCM[assistantFrameOffset+2:assistantFrameOffset+4])
	}
}

func TestPersistPadsToSessionDuration(t *testing.T) {
	// Session lasts 500ms = 16000 bytes at 32000 B/s.
	// User: 100 bytes at t=0. System: 200 bytes at t=0.
	// Both padded to 16000 bytes.
	rec, fc := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	rec.Record(ctx, internal_type.UserAudioReceivedPacket{Audio: pcm(0x11, 100)})
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0x22, 200)})

	fc.Advance(500 * time.Millisecond)

	userWAV, systemWAV, _, err := rec.Persist()
	if err != nil {
		t.Fatalf("Persist error: %v", err)
	}
	userPCM := wavPCMData(userWAV)
	systemPCM := wavPCMData(systemWAV)

	sessionBytes := durationBytes(500 * time.Millisecond)
	if len(userPCM) != sessionBytes {
		t.Fatalf("user PCM: expected %d, got %d", sessionBytes, len(userPCM))
	}
	if len(systemPCM) != sessionBytes {
		t.Fatalf("system PCM: expected %d, got %d", sessionBytes, len(systemPCM))
	}

	// User: 100 bytes audio at offset 0, rest silence
	if userPCM[0] != 0x11 || userPCM[99] != 0x11 || userPCM[100] != 0x00 {
		t.Error("user layout wrong")
	}
	// System: 200 bytes audio at offset 0, rest silence
	if systemPCM[0] != 0x22 || systemPCM[199] != 0x22 || systemPCM[200] != 0x00 {
		t.Error("system layout wrong")
	}
}

func TestPersistSystemAudioPlacedAtCorrectTime(t *testing.T) {
	// User speaks from t=0 (100 bytes), system replies at t=500ms (200 bytes).
	// Session = 1s → 32000 bytes.
	rec, fc := newTestRecorderWithClock(t)
	rec.Start()
	ctx := context.Background()

	rec.Record(ctx, internal_type.UserAudioReceivedPacket{Audio: pcm(0x11, 100)})

	fc.Advance(500 * time.Millisecond)
	rec.Record(ctx, internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0x22, 200)})

	fc.Advance(500 * time.Millisecond) // session = 1s

	userWAV, systemWAV, _, err := rec.Persist()
	if err != nil {
		t.Fatalf("Persist error: %v", err)
	}
	userPCM := wavPCMData(userWAV)
	systemPCM := wavPCMData(systemWAV)

	sessionBytes := durationBytes(1 * time.Second)
	if len(userPCM) != sessionBytes || len(systemPCM) != sessionBytes {
		t.Fatalf("expected %d each, got user=%d system=%d", sessionBytes, len(userPCM), len(systemPCM))
	}

	// System: silence from 0..16000, audio from 16000..16200, silence after.
	sysOffset := durationBytes(500 * time.Millisecond)
	for i := 0; i < sysOffset; i++ {
		if systemPCM[i] != 0x00 {
			t.Errorf("system byte %d: expected silence, got 0x%02x", i, systemPCM[i])
			break
		}
	}
	for i := sysOffset; i < sysOffset+200; i++ {
		if systemPCM[i] != 0x22 {
			t.Errorf("system byte %d: expected 0x22, got 0x%02x", i, systemPCM[i])
			break
		}
	}
	for i := sysOffset + 200; i < sessionBytes; i++ {
		if systemPCM[i] != 0x00 {
			t.Errorf("system byte %d: expected silence, got 0x%02x", i, systemPCM[i])
			break
		}
	}

	// User: audio from 0..100, silence after.
	if userPCM[0] != 0x11 || userPCM[99] != 0x11 || userPCM[100] != 0x00 {
		t.Error("user layout wrong")
	}
}

func TestPersistOnlyUserAudio(t *testing.T) {
	rec, fc := newTestRecorderWithClock(t)
	rec.Start()
	rec.Record(context.Background(), internal_type.UserAudioReceivedPacket{Audio: pcm(0xAA, 500)})
	fc.Advance(100 * time.Millisecond) // 3200 session bytes

	userWAV, systemWAV, _, err := rec.Persist()
	if err != nil {
		t.Fatalf("Persist error: %v", err)
	}
	userPCM := wavPCMData(userWAV)
	systemPCM := wavPCMData(systemWAV)

	sessionBytes := durationBytes(100 * time.Millisecond)
	if len(userPCM) != sessionBytes || len(systemPCM) != sessionBytes {
		t.Fatalf("expected %d each, got user=%d system=%d", sessionBytes, len(userPCM), len(systemPCM))
	}
	if userPCM[0] != 0xAA || userPCM[499] != 0xAA || userPCM[500] != 0x00 {
		t.Error("user track layout wrong")
	}
	for i := 0; i < sessionBytes; i++ {
		if systemPCM[i] != 0x00 {
			t.Errorf("system byte %d: expected silence", i)
			break
		}
	}
}

func TestPersistOnlySystemAudio(t *testing.T) {
	rec, fc := newTestRecorderWithClock(t)
	rec.Start()
	rec.Record(context.Background(), internal_type.TextToSpeechAudioPacket{ContextID: "c1", AudioChunk: pcm(0xBB, 300)})
	fc.Advance(100 * time.Millisecond)

	userWAV, systemWAV, _, err := rec.Persist()
	if err != nil {
		t.Fatalf("Persist error: %v", err)
	}
	userPCM := wavPCMData(userWAV)
	systemPCM := wavPCMData(systemWAV)

	sessionBytes := durationBytes(100 * time.Millisecond)
	if len(userPCM) != sessionBytes || len(systemPCM) != sessionBytes {
		t.Fatalf("expected %d each, got user=%d system=%d", sessionBytes, len(userPCM), len(systemPCM))
	}
	for i := 0; i < sessionBytes; i++ {
		if userPCM[i] != 0x00 {
			t.Errorf("user byte %d: expected silence", i)
			break
		}
	}
	if systemPCM[0] != 0xBB || systemPCM[299] != 0xBB || systemPCM[300] != 0x00 {
		t.Error("system track layout wrong")
	}
}
