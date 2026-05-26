package internal_telnyx

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunOutputSender_IsIdempotent(t *testing.T) {
	p, err := NewAudioProcessor(nil)
	if err != nil {
		t.Fatalf("NewAudioProcessor error: %v", err)
	}

	var inFlight atomic.Int32
	var maxInFlight atomic.Int32

	p.SetOutputChunkCallback(func(_ *AudioChunk) error {
		cur := inFlight.Add(1)
		for {
			prev := maxInFlight.Load()
			if cur <= prev || maxInFlight.CompareAndSwap(prev, cur) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		inFlight.Add(-1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done1 := make(chan struct{})
	done2 := make(chan struct{})
	go func() {
		p.RunOutputSender(ctx)
		close(done1)
	}()
	go func() {
		p.RunOutputSender(ctx)
		close(done2)
	}()

	time.Sleep(110 * time.Millisecond)
	cancel()

	select {
	case <-done1:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("first sender did not stop")
	}
	select {
	case <-done2:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("second sender did not stop")
	}

	if got := maxInFlight.Load(); got > 1 {
		t.Fatalf("expected at most one sender loop; max concurrent callbacks=%d", got)
	}
	if stats := p.OutputHealthSnapshot(); stats.Ticks == 0 {
		t.Fatal("expected output health ticks to be recorded")
	}
}
