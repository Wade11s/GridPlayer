package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"gridplayer-go/internal/dummy"
	"gridplayer-go/internal/engine"
	"gridplayer-go/internal/keymap"
)

func TestToggleAllPlaybackPausesIfAnyCellIsPlaying(t *testing.T) {
	newEngine := func() *engine.Engine {
		e := engine.New(func(string, bool, string) (engine.Backend, error) {
			return dummy.New(), nil
		})
		if err := e.Load("movie.mp4"); err != nil {
			t.Fatal(err)
		}
		return e
	}
	playing := newEngine()
	paused := newEngine()
	defer playing.Close()
	defer paused.Close()
	paused.Pause()
	a := &app{cells: []*cell{{eng: playing}, {eng: paused}, {eng: nil}}}
	a.toggleAllPlayback()
	for i, c := range a.cells[:2] {
		if got := c.eng.Snapshot().Phase; got != engine.Paused {
			t.Fatalf("cell %d phase %s, want paused", i, got)
		}
	}
	a.toggleAllPlayback()
	for i, c := range a.cells[:2] {
		if got := c.eng.Snapshot().Phase; got != engine.Playing {
			t.Fatalf("second toggle cell %d phase %s, want playing", i, got)
		}
	}
}

func TestSpaceAllWorksWithFocusedCellButton(t *testing.T) {
	newEngine := func() *engine.Engine {
		e := engine.New(func(string, bool, string) (engine.Backend, error) {
			return dummy.New(), nil
		})
		if err := e.Load("movie.mp4"); err != nil {
			t.Fatal(err)
		}
		e.Play()
		return e
	}
	first := newEngine()
	second := newEngine()
	defer first.Close()
	defer second.Close()
	a := &app{keys: keymap.Default(), cells: []*cell{{eng: first}, {eng: second}}}
	tt := ui.NewTester(func(c *ui.Context) {
		a.shortcuts(c)
		el := a.allPlaybackCellShortcut(c, ui.Box(c))
		el.Children(func() {
			if ui.Button(c, "cell play/pause").Clicked() {
				first.Toggle()
			}
		})
	}, 200, 100)
	tt.Key(0, ui.KeyTab) // Focus the cell button.
	tt.Key(0, ui.KeySpace)
	for i, eng := range []*engine.Engine{first, second} {
		if got := eng.Snapshot().Phase; got != engine.Paused {
			t.Fatalf("focused-button Space cell %d phase %s, want all paused", i, got)
		}
	}
}

func TestAllListStepsUseManualWrapSemantics(t *testing.T) {
	first := &cell{list: []string{"a.mp4", "b.mp4"}, listIndex: 1, listEnd: listEndStop, path: "b.mp4"}
	second := &cell{list: []string{"c.mp4", "d.mp4"}, listIndex: 0, path: "c.mp4"}
	single := &cell{path: "single.mp4"}
	a := &app{cells: []*cell{first, second, single}}
	a.nextAllLists()
	if first.listIndex != 0 || first.path != "a.mp4" || second.listIndex != 1 || second.path != "d.mp4" {
		t.Fatalf("next all: first=%+v second=%+v", first, second)
	}
	a.previousAllLists()
	if first.listIndex != 1 || second.listIndex != 0 || single.path != "single.mp4" {
		t.Fatalf("previous all: first=%+v second=%+v", first, second)
	}
}

func TestSeekPercentOffsetTarget(t *testing.T) {
	for _, tt := range []struct {
		now, length int64
		percent     float64
		want        int64
	}{
		{now: 5000, length: 20000, percent: 1, want: 5200},
		{now: 5000, length: 20000, percent: 5, want: 6000},
		{now: 5000, length: 20000, percent: -10, want: 3000},
		{now: 5000, length: 0, percent: 5, want: 5000},
	} {
		if got := seekPercentOffsetTarget(tt.now, tt.length, tt.percent); got != tt.want {
			t.Errorf("%+v -> %d, want %d", tt, got, tt.want)
		}
	}
}
