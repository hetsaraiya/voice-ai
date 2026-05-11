package internal_exotel

import (
	"testing"

	internal_ambient "github.com/rapidaai/api/assistant-api/internal/audio/ambient"
)

func TestAudioProcessor_AmbientConfigureAndIdleFrame(t *testing.T) {
	p, err := NewAudioProcessor(nil)
	if err != nil {
		t.Fatalf("NewAudioProcessor error: %v", err)
	}
	if p.ambientMixer == nil {
		t.Fatal("expected ambient mixer to be initialized")
	}

	err = p.ConfigureAmbient(internal_ambient.NewConfig(internal_ambient.ProfileCafe, 18))
	if err != nil {
		t.Fatalf("ConfigureAmbient error: %v", err)
	}

	frame := p.IdleFrame()
	if len(frame) != OutputChunkSize {
		t.Fatalf("unexpected idle frame length: got=%d want=%d", len(frame), OutputChunkSize)
	}
}
