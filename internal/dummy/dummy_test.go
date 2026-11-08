package dummy

import (
	"testing"
	"time"

	"gridplayer-go/internal/engine"
)

func TestClockRunsAndLoop(t *testing.T) {
	b := New()
	defer b.Close()
	if b.Length() != LengthMs || b.State() != engine.BackendPlaying {
		t.Fatalf("length/state %d %v", b.Length(), b.State())
	}
	before := b.Time()
	time.Sleep(50 * time.Millisecond)
	if got := b.Time(); got <= before {
		t.Fatalf("clock did not advance %d -> %d", before, got)
	}
	b.SeekTo(LengthMs - 1000)
	if got := b.Time(); got != LengthMs-1000 {
		t.Fatalf("seek %d", got)
	}
	b.Stop()
	stopped := b.Time()
	time.Sleep(30 * time.Millisecond)
	if got := b.Time(); got != stopped || b.State() != engine.BackendIdle {
		t.Fatalf("stopped clock moved %d -> %d (state %v)", stopped, got, b.State())
	}
	b.Pause()
	paused := b.Time()
	time.Sleep(30 * time.Millisecond)
	if got := b.Time(); got != paused {
		t.Fatalf("paused clock moved %d -> %d", paused, got)
	}
}

func TestDummyEngineLoad(t *testing.T) {
	f := New()
	e := engine.New(func(path string, loop bool) (engine.Backend, error) {
		return f, nil
	})
	defer e.Close()
	if err := e.Load("dummy"); err != nil {
		t.Fatal(err)
	}
	f.Tick()
	deadline := time.Now().Add(2 * time.Second)
	for e.Snapshot().Length == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := e.Snapshot().Length; got != LengthMs {
		t.Fatalf("length %d", got)
	}
}
