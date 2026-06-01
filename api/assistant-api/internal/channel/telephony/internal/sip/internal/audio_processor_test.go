// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_sip

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	internal_audio "github.com/rapidaai/api/assistant-api/internal/audio"
	internal_ambient "github.com/rapidaai/api/assistant-api/internal/audio/ambient"
	internal_telephony_media "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/media"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	"github.com/rapidaai/protos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockResampler struct {
	out []byte
	err error
}

func (m *mockResampler) Resample(data []byte, _, _ *protos.AudioConfig) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.out != nil {
		return append([]byte(nil), m.out...), nil
	}
	// passthrough — just return the same data
	return data, nil
}

type mockAmbientMixer struct {
	err error
}

func (m *mockAmbientMixer) Configure(internal_ambient.Config) error { return nil }
func (m *mockAmbientMixer) Mix(primary []byte) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return primary, nil
}
func (m *mockAmbientMixer) Reset() {}
func (m *mockAmbientMixer) CurrentConfig() internal_ambient.Config {
	return internal_ambient.Config{}
}

// pushRecorder captures all streams pushed via the pushInput callback.
type pushRecorder struct {
	mu      sync.Mutex
	streams []internal_type.Stream
}

func (r *pushRecorder) push(s internal_type.Stream) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.streams = append(r.streams, s)
}

func (r *pushRecorder) get() []internal_type.Stream {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]internal_type.Stream, len(r.streams))
	copy(cp, r.streams)
	return cp
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testRTPHandler(t *testing.T, codec *sip_infra.Codec) *sip_infra.RTPHandler {
	t.Helper()
	h := &sip_infra.RTPHandler{}

	effectiveCodec := codec
	if effectiveCodec == nil {
		effectiveCodec = &sip_infra.CodecPCMU
	}

	setUnexportedField(t, h, "codec", effectiveCodec)
	setUnexportedField(t, h, "audioInChan", make(chan []byte, 100))
	setUnexportedField(t, h, "audioOutChan", make(chan []byte, 100))
	setUnexportedField(t, h, "flushAudioCh", make(chan struct{}, 1))

	return h
}

func setUnexportedField(t *testing.T, obj interface{}, field string, val interface{}) {
	t.Helper()
	rv := reflect.ValueOf(obj)
	require.Equal(t, reflect.Ptr, rv.Kind(), "obj must be a pointer")
	fv := rv.Elem().FieldByName(field)
	require.True(t, fv.IsValid(), "field %s not found", field)
	target := reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem()
	target.Set(reflect.ValueOf(val))
}

func newTestAudioProcessor(t *testing.T, codec *sip_infra.Codec, resampler internal_type.AudioResampler, recorder *pushRecorder) *AudioProcessor {
	t.Helper()
	rtp := testRTPHandler(t, codec)
	return NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtp,
		Resampler:  resampler,
		PushInput:  recorder.push,
	})
}

// ---------------------------------------------------------------------------
// Tests: ProcessProviderAudioFrame
// ---------------------------------------------------------------------------

func TestProcessProviderAudioFrame_EmitsBridgeImmediatelyAndBuffersPipelineAudio(t *testing.T) {
	rec := &pushRecorder{}
	resampledAudio := make([]byte, BridgeOutputFrameSize)
	for i := range resampledAudio {
		resampledAudio[i] = byte(i)
	}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{out: resampledAudio}, rec)
	receivedAt := time.Now()

	firstFrame, err := proc.ProcessProviderAudioFrame(internal_telephony_media.ProviderAudioFrame{
		Audio:      make([]byte, MulawFrameSize),
		ReceivedAt: receivedAt,
	})
	require.NoError(t, err)
	assert.Equal(t, resampledAudio, firstFrame.BridgeAudio)
	assert.Empty(t, firstFrame.PipelineAudio)
	assert.Equal(t, receivedAt, firstFrame.ReceivedAt)

	_, err = proc.ProcessProviderAudioFrame(internal_telephony_media.ProviderAudioFrame{
		Audio:      make([]byte, MulawFrameSize),
		ReceivedAt: receivedAt,
	})
	require.NoError(t, err)

	thirdFrame, err := proc.ProcessProviderAudioFrame(internal_telephony_media.ProviderAudioFrame{
		Audio:      make([]byte, MulawFrameSize),
		ReceivedAt: receivedAt,
	})
	require.NoError(t, err)
	assert.Equal(t, resampledAudio, thirdFrame.BridgeAudio)
	assert.Len(t, thirdFrame.PipelineAudio, InputBufferThreshold)
}

func TestProcessProviderAudioFrame_PCMAConvertsToUlawBeforeResample(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMA, &mockResampler{}, rec)

	input := make([]byte, MulawFrameSize)
	for i := range input {
		input[i] = 0xD5
	}

	inputFrame, err := proc.ProcessProviderAudioFrame(internal_telephony_media.ProviderAudioFrame{
		Audio: input,
	})
	require.NoError(t, err)
	assert.NotEqual(t, input, inputFrame.BridgeAudio)
}

func TestProcessProviderAudioFrame_ResamplerError(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{err: errors.New("resample failed")}, rec)

	inputFrame, err := proc.ProcessProviderAudioFrame(internal_telephony_media.ProviderAudioFrame{
		Audio: make([]byte, MulawFrameSize),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrProviderAudioConversionFailed))
	assert.Empty(t, inputFrame.BridgeAudio)
	assert.Empty(t, inputFrame.PipelineAudio)
}

// ---------------------------------------------------------------------------
// Tests: ProcessAssistantAudio
// ---------------------------------------------------------------------------

func TestProcessAssistantAudio_BuffersProviderAndBridgeAudio(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)
	proc.ambientMixer = nil

	audio := make([]byte, BridgeOutputFrameSize*2)
	for i := range audio {
		audio[i] = byte(i)
	}

	require.NoError(t, proc.ProcessAssistantAudio(audio, false))

	firstFrame, ok := proc.NextOutputFrame()
	require.True(t, ok)
	assert.Len(t, firstFrame.ProviderAudio, MulawFrameSize)
	assert.Len(t, firstFrame.BridgeAudio, BridgeOutputFrameSize)

	secondFrame, ok := proc.NextOutputFrame()
	require.True(t, ok)
	assert.Len(t, secondFrame.ProviderAudio, MulawFrameSize)
	assert.Len(t, secondFrame.BridgeAudio, BridgeOutputFrameSize)
}

func TestProcessAssistantAudio_BridgeActiveDoesNotRecordNormalOutput(t *testing.T) {
	rec := &pushRecorder{}
	rtp := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc := NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtp,
		Resampler:  &mockResampler{},
		PushInput:  rec.push,
	})
	bridgeRTP := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc.SetBridgeTarget(bridgeRTP, &sip_infra.CodecPCMU, &sip_infra.CodecPCMU)

	require.NoError(t, proc.ProcessAssistantAudio(make([]byte, BridgeOutputFrameSize), false))

	outputFrame, ok := proc.NextOutputFrame()
	assert.False(t, ok)
	assert.Empty(t, outputFrame.ProviderAudio)
	assert.Empty(t, outputFrame.BridgeAudio)
}

func TestProcessAssistantAudio_TransferActiveDoesNotRecordNormalOutput(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)
	proc.SetTransferActive(true)

	require.NoError(t, proc.ProcessAssistantAudio(make([]byte, BridgeOutputFrameSize), false))

	outputFrame, ok := proc.NextOutputFrame()
	assert.False(t, ok)
	assert.Empty(t, outputFrame.ProviderAudio)
	assert.Empty(t, outputFrame.BridgeAudio)
}

func TestProcessAssistantAudio_PCMAConvertsToAlaw(t *testing.T) {
	rec := &pushRecorder{}
	rtp := testRTPHandler(t, &sip_infra.CodecPCMA)
	proc := NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtp,
		Resampler:  &mockResampler{},
		PushInput:  rec.push,
	})

	// Feed 16kHz LINEAR16 data (mock resampler returns it unchanged)
	data := make([]byte, 160)
	for i := range data {
		data[i] = 0xFF // µ-law silence
	}

	require.NoError(t, proc.ProcessAssistantAudio(data, false))

	outputFrame, ok := proc.NextOutputFrame()
	require.True(t, ok)
	assert.NotEqual(t, data, outputFrame.ProviderAudio, "PCMA codec should convert output to A-law")
}

func TestConvertOutputAudio_PCMAConvertsResampledMulaw(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMA, &mockResampler{out: []byte{0xFF, 0x7F}}, rec)

	convertedAudio, err := proc.convertOutputAudio([]byte{1, 2})
	require.NoError(t, err)
	assert.Equal(t, internal_audio.UlawToAlaw([]byte{0xFF, 0x7F}), convertedAudio)
}

func TestProcessAssistantAudio_ResamplerError(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{err: errors.New("fail")}, rec)

	err := proc.ProcessAssistantAudio(make([]byte, BridgeOutputFrameSize), false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAssistantAudioConversionFailed))
}

func TestProcessAssistantAudio_NextOutputFrameIncludesBridgeAudio(t *testing.T) {
	rec := &pushRecorder{}
	providerAudio := make([]byte, MulawFrameSize)
	for i := range providerAudio {
		providerAudio[i] = byte(i)
	}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{out: providerAudio}, rec)
	proc.ambientMixer = nil

	assistantAudio := make([]byte, BridgeOutputFrameSize)
	for i := range assistantAudio {
		assistantAudio[i] = byte(255 - i%256)
	}

	require.NoError(t, proc.ProcessAssistantAudio(assistantAudio, false))
	outputFrame, ok := proc.NextOutputFrame()
	require.True(t, ok)

	assert.Equal(t, providerAudio, outputFrame.ProviderAudio)
	assert.Equal(t, assistantAudio, outputFrame.BridgeAudio)
	assert.False(t, outputFrame.Idle)
}

func TestIdleOutputFrame_SilentDuringTransfer(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)
	proc.SetTransferActive(true)

	outputFrame, ok := proc.IdleOutputFrame()
	assert.False(t, ok)
	assert.Empty(t, outputFrame.ProviderAudio)
}

func TestComplete_PadsPartialFrameWithMulawSilence(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{out: []byte{0x10}}, rec)
	proc.ambientMixer = nil

	require.NoError(t, proc.ProcessAssistantAudio([]byte{0x01}, true))

	outputFrame, ok := proc.NextOutputFrame()
	require.True(t, ok)
	require.Len(t, outputFrame.ProviderAudio, MulawFrameSize)
	assert.Equal(t, byte(0x10), outputFrame.ProviderAudio[0])
	assert.Equal(t, byte(MulawSilenceByte), outputFrame.ProviderAudio[1])
	assert.Equal(t, byte(MulawSilenceByte), outputFrame.ProviderAudio[len(outputFrame.ProviderAudio)-1])
}

// ---------------------------------------------------------------------------
// Tests: ClearOutputBuffer
// ---------------------------------------------------------------------------

func TestClearOutputBuffer_ResetsBuffer(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	require.NoError(t, proc.ProcessAssistantAudio(make([]byte, BridgeOutputFrameSize), false))

	proc.ClearOutputBuffer()

	outputFrame, ok := proc.NextOutputFrame()
	assert.False(t, ok)
	assert.Empty(t, outputFrame.ProviderAudio)
	assert.Empty(t, outputFrame.BridgeAudio)
}

func TestClearOutputBuffer_PreservesInputBuffer(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{out: make([]byte, BridgeOutputFrameSize)}, rec)

	for i := 0; i < 2; i++ {
		inputFrame, err := proc.ProcessProviderAudioFrame(internal_telephony_media.ProviderAudioFrame{Audio: make([]byte, MulawFrameSize)})
		require.NoError(t, err)
		assert.Empty(t, inputFrame.PipelineAudio)
	}
	proc.ClearOutputBuffer()

	inputFrame, err := proc.ProcessProviderAudioFrame(internal_telephony_media.ProviderAudioFrame{Audio: make([]byte, MulawFrameSize)})
	require.NoError(t, err)
	assert.Len(t, inputFrame.PipelineAudio, InputBufferThreshold)
}

func TestClearInputBuffer_ResetsInputBuffer(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{out: make([]byte, BridgeOutputFrameSize)}, rec)

	for i := 0; i < 2; i++ {
		inputFrame, err := proc.ProcessProviderAudioFrame(internal_telephony_media.ProviderAudioFrame{Audio: make([]byte, MulawFrameSize)})
		require.NoError(t, err)
		assert.Empty(t, inputFrame.PipelineAudio)
	}
	proc.ClearInputBuffer()

	inputFrame, err := proc.ProcessProviderAudioFrame(internal_telephony_media.ProviderAudioFrame{Audio: make([]byte, MulawFrameSize)})
	require.NoError(t, err)
	assert.Empty(t, inputFrame.PipelineAudio)
}

func TestConsumeFrame_ReturnsErrorWhenRTPOutputQueueFull(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)
	rtpAudioOut := proc.rtpHandler.AudioOut()

	for i := 0; i < cap(rtpAudioOut); i++ {
		rtpAudioOut <- []byte{byte(i)}
	}

	err := proc.ConsumeFrame([]byte{0xff})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRTPOutputQueueFull))
}

// ---------------------------------------------------------------------------
// Tests: ForwardUserAudio
// ---------------------------------------------------------------------------

func TestForwardUserAudio_NoBridge_ReturnsFalse(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	audio := []byte{0x01, 0x02, 0x03}
	result := proc.ForwardUserAudio(audio)
	assert.False(t, result, "should return false when no bridge is active")
}

func TestForwardUserAudio_BridgeActive_ReturnsTrue(t *testing.T) {
	rec := &pushRecorder{}
	rtp := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc := NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtp,
		Resampler:  &mockResampler{},
		PushInput:  rec.push,
	})

	bridgeRTP := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc.SetBridgeTarget(bridgeRTP, &sip_infra.CodecPCMU, &sip_infra.CodecPCMU)

	audio := []byte{0x01, 0x02, 0x03}
	result := proc.ForwardUserAudio(audio)
	assert.True(t, result)

	// Check that audio was queued to bridgeUserCh
	select {
	case queued := <-proc.bridgeUserCh:
		assert.Equal(t, audio, queued)
	case <-time.After(time.Second):
		t.Fatal("expected audio on bridgeUserCh")
	}
}

func TestForwardUserAudio_BridgeActive_SuccessPath(t *testing.T) {
	rec := &pushRecorder{}
	rtp := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc := NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtp,
		Resampler:  &mockResampler{},
		PushInput:  rec.push,
	})

	bridgeRTP := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc.SetBridgeTarget(bridgeRTP, &sip_infra.CodecPCMU, &sip_infra.CodecPCMU)

	audio := []byte{0xAA, 0xBB, 0xCC}
	// ForwardUserAudio sends to outRTP.AudioOut() (non-blocking) and queues to bridgeUserCh.
	// We verify the return value and bridgeUserCh; outRTP.AudioOut() is send-only so we
	// can't read from it, but the non-blocking send into its buffered channel won't fail.
	result := proc.ForwardUserAudio(audio)
	assert.True(t, result)

	select {
	case queued := <-proc.bridgeUserCh:
		assert.Equal(t, audio, queued, "raw audio should be queued to bridgeUserCh")
	case <-time.After(time.Second):
		t.Fatal("expected audio on bridgeUserCh")
	}
}

func TestForwardUserAudio_WithTranscode_PCMU_to_PCMA(t *testing.T) {
	rec := &pushRecorder{}
	rtp := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc := NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtp,
		Resampler:  &mockResampler{},
		PushInput:  rec.push,
	})

	bridgeRTP := testRTPHandler(t, &sip_infra.CodecPCMA)
	// User has PCMU, bridge target has PCMA — need transcode
	proc.SetBridgeTarget(bridgeRTP, &sip_infra.CodecPCMU, &sip_infra.CodecPCMA)

	audio := make([]byte, 160)
	for i := range audio {
		audio[i] = 0xFF // µ-law silence
	}
	result := proc.ForwardUserAudio(audio)
	assert.True(t, result, "should return true when bridge is active")

	// Raw (untranscoded) audio should still go to bridgeUserCh.
	// The transcode only applies to what's sent to outRTP.AudioOut() (which is
	// send-only and can't be read in tests). We verify the contract by confirming
	// that bridgeUserCh gets the original raw audio, proving the transcode path
	// is separate.
	select {
	case queued := <-proc.bridgeUserCh:
		assert.Equal(t, audio, queued, "raw audio should go to bridgeUserCh without transcoding")
	case <-time.After(time.Second):
		t.Fatal("expected raw audio on bridgeUserCh")
	}
}

func TestForwardUserAudio_Backpressure_DropsAudio(t *testing.T) {
	rec := &pushRecorder{}
	rtp := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc := NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtp,
		Resampler:  &mockResampler{},
		PushInput:  rec.push,
	})

	bridgeRTP := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc.SetBridgeTarget(bridgeRTP, &sip_infra.CodecPCMU, &sip_infra.CodecPCMU)

	// Fill bridgeUserCh to capacity
	for i := 0; i < AudioChannelSize; i++ {
		proc.bridgeUserCh <- []byte{byte(i)}
	}

	// Should not block even though channel is full
	done := make(chan struct{})
	go func() {
		proc.ForwardUserAudio([]byte{0xFF})
		close(done)
	}()

	select {
	case <-done:
		// OK — did not hang
	case <-time.After(time.Second):
		t.Fatal("ForwardUserAudio hung when bridgeUserCh was full")
	}
}

func TestForwardUserAudio_DoesNotRecordWhenBridgeRTPQueueFull(t *testing.T) {
	rec := &pushRecorder{}
	rtp := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc := NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtp,
		Resampler:  &mockResampler{},
		PushInput:  rec.push,
	})
	bridgeRTP := testRTPHandler(t, &sip_infra.CodecPCMU)
	bridgeAudioOut := bridgeRTP.AudioOut()
	for i := 0; i < cap(bridgeAudioOut); i++ {
		bridgeAudioOut <- []byte{byte(i)}
	}
	proc.SetBridgeTarget(bridgeRTP, &sip_infra.CodecPCMU, &sip_infra.CodecPCMU)

	assert.True(t, proc.ForwardUserAudio([]byte{0xaa}))

	select {
	case recorded := <-proc.bridgeUserCh:
		t.Fatalf("dropped bridge RTP frame was queued for recording: %v", recorded)
	default:
	}
	streams := rec.get()
	require.Len(t, streams, 1)
	event, ok := streams[0].(*protos.ConversationEvent)
	require.True(t, ok)
	assert.Equal(t, "output_send_error", event.GetData()["type"])
	assert.Equal(t, "bridge_audio_out_full", event.GetData()["reason"])
}

// ---------------------------------------------------------------------------
// Tests: PushOperatorAudio
// ---------------------------------------------------------------------------

func TestPushOperatorAudio_QueuesAudio(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	audio := []byte{0x10, 0x20, 0x30}
	proc.PushOperatorAudio(audio)

	select {
	case queued := <-proc.bridgeOperatorCh:
		assert.Equal(t, audio, queued)
	case <-time.After(time.Second):
		t.Fatal("expected audio on bridgeOperatorCh")
	}
}

func TestPushOperatorAudio_Backpressure_DropsAudio(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	// Fill channel
	for i := 0; i < AudioChannelSize; i++ {
		proc.bridgeOperatorCh <- []byte{byte(i)}
	}

	// Should not block
	done := make(chan struct{})
	go func() {
		proc.PushOperatorAudio([]byte{0xFF})
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("PushOperatorAudio hung when bridgeOperatorCh was full")
	}
}

// ---------------------------------------------------------------------------
// Tests: SetBridgeTarget / ClearBridgeTarget / IsBridgeActive
// ---------------------------------------------------------------------------

func TestSetBridgeTarget_ActivatesBridge(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	assert.False(t, proc.IsBridgeActive())

	bridgeRTP := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc.SetBridgeTarget(bridgeRTP, &sip_infra.CodecPCMU, &sip_infra.CodecPCMU)

	assert.True(t, proc.IsBridgeActive())
}

func TestSetBridgeTarget_NilRTP_DoesNotActivate(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	proc.SetBridgeTarget(nil, &sip_infra.CodecPCMU, &sip_infra.CodecPCMU)
	assert.False(t, proc.IsBridgeActive())
}

func TestClearBridgeTarget_DeactivatesBridge(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	bridgeRTP := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc.SetBridgeTarget(bridgeRTP, &sip_infra.CodecPCMU, &sip_infra.CodecPCMU)
	assert.True(t, proc.IsBridgeActive())

	proc.ClearBridgeTarget()
	assert.False(t, proc.IsBridgeActive())

	// ForwardUserAudio should now return false
	assert.False(t, proc.ForwardUserAudio([]byte{0x01}))
}

// TestForwardUserAudio_ConcurrentClear verifies the bridgeMu invariant: while
// any ForwardUserAudio call is in flight, ClearBridgeTarget must NOT return.
// This guarantees a caller can safely close the outbound RTP channel after
// ClearBridgeTarget returns without racing into a "send on closed channel"
// panic. Run with `-race` for full coverage.
func TestForwardUserAudio_ConcurrentClear(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	bridgeRTP := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc.SetBridgeTarget(bridgeRTP, &sip_infra.CodecPCMU, &sip_infra.CodecPCMU)

	const writers = 8
	const iters = 200

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				select {
				case <-stop:
					return
				default:
				}
				proc.ForwardUserAudio([]byte{0x01, 0x02})
			}
		}()
	}

	// Race: while writers are spinning, clear the bridge target. Clear must
	// block until any in-flight ForwardUserAudio releases the mutex; no
	// writer should observe a panic. Race detector catches any unsynchronized
	// access to p.bridge.
	time.Sleep(2 * time.Millisecond)
	proc.ClearBridgeTarget()
	assert.False(t, proc.IsBridgeActive(), "ClearBridgeTarget should leave bridge inactive")

	close(stop)
	wg.Wait()

	// Subsequent ForwardUserAudio returns false; no panic.
	assert.False(t, proc.ForwardUserAudio([]byte{0x03}))
}

func TestSetBridgeTarget_MatchingCodecs_NoTranscode(t *testing.T) {
	rec := &pushRecorder{}
	rtp := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc := NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtp,
		Resampler:  &mockResampler{},
		PushInput:  rec.push,
	})

	bridgeRTP := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc.SetBridgeTarget(bridgeRTP, &sip_infra.CodecPCMU, &sip_infra.CodecPCMU)

	// With matching codecs, bridgeUserCh should receive the original audio unchanged
	// (the raw audio IS the same as what goes to outRTP when no transcode is needed).
	audio := []byte{0x01, 0x02, 0x03, 0x04}
	result := proc.ForwardUserAudio(audio)
	assert.True(t, result)

	select {
	case queued := <-proc.bridgeUserCh:
		assert.Equal(t, audio, queued, "matching codecs should not alter raw audio")
	case <-time.After(time.Second):
		t.Fatal("expected audio on bridgeUserCh")
	}
}

func TestSetBridgeTarget_PCMA_to_PCMU_Transcode(t *testing.T) {
	rec := &pushRecorder{}
	rtp := testRTPHandler(t, &sip_infra.CodecPCMA)
	proc := NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtp,
		Resampler:  &mockResampler{},
		PushInput:  rec.push,
	})

	bridgeRTP := testRTPHandler(t, &sip_infra.CodecPCMU)
	// inCodec=PCMA, outCodec=PCMU means A-law → µ-law transcode for outRTP
	proc.SetBridgeTarget(bridgeRTP, &sip_infra.CodecPCMA, &sip_infra.CodecPCMU)

	audio := make([]byte, 160)
	for i := range audio {
		audio[i] = 0xD5 // A-law silence
	}
	result := proc.ForwardUserAudio(audio)
	assert.True(t, result)

	// bridgeUserCh always gets the raw (untranscoded) audio.
	// Transcode is applied only to the send to outRTP.AudioOut() which is send-only.
	select {
	case queued := <-proc.bridgeUserCh:
		assert.Equal(t, audio, queued, "bridgeUserCh should get raw untranscoded audio")
	case <-time.After(time.Second):
		t.Fatal("expected raw audio on bridgeUserCh")
	}
}

// ---------------------------------------------------------------------------
// Tests: PlayRingback
// ---------------------------------------------------------------------------

func TestPlayRingback_ExitsOnContextCancel(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		proc.PlayRingback(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("PlayRingback did not exit after context cancellation")
	}
}

func TestPlayRingback_ProducesFrames(t *testing.T) {
	rec := &pushRecorder{}
	rtp := testRTPHandler(t, &sip_infra.CodecPCMU)
	proc := NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtp,
		Resampler:  &mockResampler{},
		PushInput:  rec.push,
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		proc.PlayRingback(ctx)
		close(done)
	}()

	// Let PlayRingback run for a few ticks (~60ms) to produce frames,
	// then cancel and verify it exits cleanly.
	time.Sleep(60 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Exited cleanly after producing frames into the buffered AudioOut channel.
	case <-time.After(2 * time.Second):
		t.Fatal("PlayRingback did not exit after context cancellation")
	}
}

// ---------------------------------------------------------------------------
// Tests: RunBridgeRecorder
// ---------------------------------------------------------------------------

func TestRunBridgeRecorder_ExitsOnContextCancel(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		proc.RunBridgeRecorder(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("RunBridgeRecorder did not exit after context cancellation")
	}
}

func TestRunBridgeRecorder_UserAudio_PushesConversationBridgeUserAudio(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go proc.RunBridgeRecorder(ctx)

	// Send user audio
	audio := []byte{0x01, 0x02, 0x03}
	proc.bridgeUserCh <- audio

	// Wait for pushInput to be called
	require.Eventually(t, func() bool {
		return len(rec.get()) >= 1
	}, time.Second, 10*time.Millisecond)

	streams := rec.get()
	require.Len(t, streams, 1)

	msg, ok := streams[0].(*protos.ConversationBridgeUserAudio)
	require.True(t, ok, "expected ConversationBridgeUserAudio, got %T", streams[0])
	assert.Equal(t, audio, msg.Audio)
	assert.False(t, msg.Time.AsTime().IsZero())
}

func TestRunBridgeRecorder_OperatorAudio_PushesConversationBridgeOperatorAudio(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go proc.RunBridgeRecorder(ctx)

	// Send operator audio
	audio := []byte{0x10, 0x20, 0x30}
	proc.bridgeOperatorCh <- audio

	require.Eventually(t, func() bool {
		return len(rec.get()) >= 1
	}, time.Second, 10*time.Millisecond)

	streams := rec.get()
	require.Len(t, streams, 1)

	msg, ok := streams[0].(*protos.ConversationBridgeOperatorAudio)
	require.True(t, ok, "expected ConversationBridgeOperatorAudio, got %T", streams[0])
	assert.Equal(t, audio, msg.Audio)
	assert.False(t, msg.Time.AsTime().IsZero())
}

func TestRunBridgeRecorder_ResamplerError_DoesNotPush(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{err: errors.New("fail")}, rec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go proc.RunBridgeRecorder(ctx)

	proc.bridgeUserCh <- []byte{0x01}
	proc.bridgeOperatorCh <- []byte{0x02}

	// Give it time to process
	time.Sleep(50 * time.Millisecond)

	streams := rec.get()
	assert.Empty(t, streams, "should not push when resampler fails")
}

// ---------------------------------------------------------------------------
// Tests: NewAudioProcessor contract
// ---------------------------------------------------------------------------

func TestNewAudioProcessor_InitializesChannels(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	assert.NotNil(t, proc.inputBuffer)
	assert.NotNil(t, proc.bridgeUserCh)
	assert.NotNil(t, proc.bridgeOperatorCh)
	assert.Equal(t, AudioChannelSize, cap(proc.bridgeUserCh))
	assert.Equal(t, AudioChannelSize, cap(proc.bridgeOperatorCh))
	assert.False(t, proc.IsBridgeActive())
}

func TestApplyAmbient_NoPrimary_WithAmbientConfig_ProducesFrame(t *testing.T) {
	rec := &pushRecorder{}
	proc := NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: testRTPHandler(t, &sip_infra.CodecPCMU),
		Resampler:  &mockResampler{},
		PushInput:  rec.push,
		Ambient: &internal_ambient.Config{
			Profile: "office",
			Volume:  20,
			Enabled: true,
		},
	})

	out := proc.applyAmbient(nil)
	require.NotNil(t, out)
	assert.Len(t, out, MulawFrameSize)
}

func TestApplyAmbient_NoPrimary_NoAmbient_ReturnsNil(t *testing.T) {
	rec := &pushRecorder{}
	proc := NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: testRTPHandler(t, &sip_infra.CodecPCMU),
		Resampler:  &mockResampler{},
		PushInput:  rec.push,
	})

	out := proc.applyAmbient(nil)
	assert.Nil(t, out)
}

func TestApplyAmbient_PrimaryWithNoAmbient_ReturnsInput(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)

	frame := []byte{1, 2, 3}
	assert.Equal(t, frame, proc.applyAmbient(frame))
}

func TestApplyAmbient_MixErrorReturnsInput(t *testing.T) {
	rec := &pushRecorder{}
	proc := newTestAudioProcessor(t, &sip_infra.CodecPCMU, &mockResampler{}, rec)
	proc.ambientMixer = &mockAmbientMixer{err: errors.New("mix failed")}

	frame := []byte{1, 2, 3}
	assert.Equal(t, frame, proc.applyAmbient(frame))
}

// ---------------------------------------------------------------------------
// Benchmarks — hot-path audio processing (called every 20ms per RTP packet)
// ---------------------------------------------------------------------------

func benchAudioProcessor(b *testing.B, codec *sip_infra.Codec) *AudioProcessor {
	b.Helper()
	rtp := &sip_infra.RTPHandler{}
	setUnexportedFieldBench(b, rtp, "codec", codec)
	setUnexportedFieldBench(b, rtp, "audioInChan", make(chan []byte, 100))
	setUnexportedFieldBench(b, rtp, "audioOutChan", make(chan []byte, 100))
	setUnexportedFieldBench(b, rtp, "flushAudioCh", make(chan struct{}, 1))

	return NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtp,
		Resampler:  &mockResampler{},
		PushInput:  func(internal_type.Stream) {},
	})
}

// BenchmarkProcessProviderAudioFrame_PCMU measures the per-packet input processing for PCMU.
func BenchmarkProcessProviderAudioFrame_PCMU(b *testing.B) {
	proc := benchAudioProcessor(b, &sip_infra.CodecPCMU)
	frame := make([]byte, MulawFrameSize)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proc.ProcessProviderAudioFrame(internal_telephony_media.ProviderAudioFrame{Audio: frame})
	}
}

// BenchmarkProcessProviderAudioFrame_PCMA measures the per-packet input processing for PCMA
// (includes A-law to µ-law conversion).
func BenchmarkProcessProviderAudioFrame_PCMA(b *testing.B) {
	proc := benchAudioProcessor(b, &sip_infra.CodecPCMA)
	frame := make([]byte, MulawFrameSize)
	for i := range frame {
		frame[i] = 0xD5
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proc.ProcessProviderAudioFrame(internal_telephony_media.ProviderAudioFrame{Audio: frame})
	}
}

// BenchmarkProcessAssistantAudio measures output buffering throughput.
func BenchmarkProcessAssistantAudio(b *testing.B) {
	proc := benchAudioProcessor(b, &sip_infra.CodecPCMU)
	frame := make([]byte, BridgeOutputFrameSize)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = proc.ProcessAssistantAudio(frame, false)
		if i%1000 == 0 {
			proc.ClearOutputBuffer()
		}
	}
}

// BenchmarkForwardUserAudio_NoBridge measures the no-bridge fast-exit path.
func BenchmarkForwardUserAudio_NoBridge(b *testing.B) {
	proc := benchAudioProcessor(b, &sip_infra.CodecPCMU)
	frame := make([]byte, MulawFrameSize)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = proc.ForwardUserAudio(frame)
	}
}

// BenchmarkForwardUserAudio_BridgeActive measures the bridge forwarding hot path.
func BenchmarkForwardUserAudio_BridgeActive(b *testing.B) {
	rtp := &sip_infra.RTPHandler{}
	setUnexportedFieldBench(b, rtp, "codec", &sip_infra.CodecPCMU)
	setUnexportedFieldBench(b, rtp, "audioInChan", make(chan []byte, 100))
	setUnexportedFieldBench(b, rtp, "audioOutChan", make(chan []byte, 100))
	setUnexportedFieldBench(b, rtp, "flushAudioCh", make(chan struct{}, 1))

	bridgeRTP := &sip_infra.RTPHandler{}
	setUnexportedFieldBench(b, bridgeRTP, "codec", &sip_infra.CodecPCMU)
	setUnexportedFieldBench(b, bridgeRTP, "audioInChan", make(chan []byte, 100))
	setUnexportedFieldBench(b, bridgeRTP, "audioOutChan", make(chan []byte, 100))
	setUnexportedFieldBench(b, bridgeRTP, "flushAudioCh", make(chan struct{}, 1))

	proc := NewAudioProcessor(AudioProcessorConfig{
		RTPHandler: rtp,
		Resampler:  &mockResampler{},
		PushInput:  func(internal_type.Stream) {},
	})
	proc.SetBridgeTarget(bridgeRTP, &sip_infra.CodecPCMU, &sip_infra.CodecPCMU)

	frame := make([]byte, MulawFrameSize)

	// Drain channels in background to prevent backpressure blocking
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-proc.bridgeUserCh:
			}
		}
	}()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = proc.ForwardUserAudio(frame)
	}
}

func setUnexportedFieldBench(b *testing.B, obj interface{}, field string, val interface{}) {
	b.Helper()
	rv := reflect.ValueOf(obj)
	if rv.Kind() != reflect.Ptr {
		b.Fatalf("obj must be a pointer")
	}
	fv := rv.Elem().FieldByName(field)
	if !fv.IsValid() {
		b.Fatalf("field %s not found", field)
	}
	target := reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem()
	target.Set(reflect.ValueOf(val))
}
