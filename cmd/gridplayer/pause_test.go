package main

import (
	"testing"

	"gridplayer-go/internal/dummy"
	"gridplayer-go/internal/engine"
)

func playingEngine(t *testing.T) *engine.Engine {
	t.Helper()
	e := engine.New(func(string, bool, string) (engine.Backend, error) {
		return dummy.New(), nil
	})
	if err := e.Load("movie.mp4"); err != nil {
		t.Fatal(err)
	}
	e.Play()
	return e
}

func TestMinimizePausesThenRestoresPlayingCells(t *testing.T) {
	first := playingEngine(t)
	second := playingEngine(t)
	alreadyPaused := playingEngine(t)
	alreadyPaused.Pause()
	defer first.Close()
	defer second.Close()
	defer alreadyPaused.Close()

	a := &app{
		pauseMinimized: true,
		cells:          []*cell{{eng: first}, {eng: second}, {eng: alreadyPaused}},
	}
	a.applyMinimized(true)
	for i, c := range a.cells {
		if got := c.eng.Snapshot().Phase; got != engine.Paused {
			t.Fatalf("minimized cell %d phase = %s, want paused", i, got)
		}
	}
	if len(a.minimizePaused) != 2 {
		t.Fatalf("recorded %d paused cells, want 2", len(a.minimizePaused))
	}
	a.applyMinimized(false)
	if got := first.Snapshot().Phase; got != engine.Playing {
		t.Fatalf("restored first phase = %s, want playing", got)
	}
	if got := second.Snapshot().Phase; got != engine.Playing {
		t.Fatalf("restored second phase = %s, want playing", got)
	}
	if got := alreadyPaused.Snapshot().Phase; got != engine.Paused {
		t.Fatalf("pre-paused cell phase = %s, want paused", got)
	}
	if a.minimizePaused != nil {
		t.Fatal("minimize record not cleared")
	}
}

func TestMinimizePauseCanBeDisabled(t *testing.T) {
	playing := playingEngine(t)
	defer playing.Close()
	a := &app{
		pauseMinimized: false,
		cells:          []*cell{{eng: playing}},
	}
	a.applyMinimized(true)
	if got := playing.Snapshot().Phase; got != engine.Playing {
		t.Fatalf("phase = %s, want playing", got)
	}
	if a.minimizePaused != nil {
		t.Fatal("disabled minimize recorded a pause")
	}
	a.applyMinimized(false)
	if got := playing.Snapshot().Phase; got != engine.Playing {
		t.Fatalf("restored phase = %s, want playing", got)
	}
}
