package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gridplayer-go/internal/dummy"
	"gridplayer-go/internal/engine"
	"gridplayer-go/internal/playlist"
)

func TestUpstreamListEndValuesApplyAndSave(t *testing.T) {
	dir := t.TempDir()
	list := filepath.Join(dir, "list.m3u")
	body := "#EXTM3U\n" + filepath.Join(dir, "a.mp4") + "\n" + filepath.Join(dir, "b.mp4") + "\n"
	if err := os.WriteFile(list, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		upstream string
		internal string
	}{
		{upstream: "start_over", internal: "loop"},
		{upstream: "pause", internal: "pause"},
		{upstream: "stop", internal: "stop"},
		{upstream: "close", internal: "close"},
	}
	for _, tt := range tests {
		t.Run(tt.upstream, func(t *testing.T) {
			a := &app{defaultVolume: 80, defaultRate: 1, defaultLoop: true}
			d := &playlist.Document{Format: playlist.FormatID, Version: 1, Videos: []playlist.Video{{URI: list, ListEnd: tt.upstream}}}
			a.applyDocument(d, "")
			if got := a.cells[0].listEnd; got != tt.internal {
				t.Fatalf("internal list end %q, want %q", got, tt.internal)
			}
			raw, err := json.Marshal(a.document())
			if err != nil {
				t.Fatal(err)
			}
			var out struct {
				Videos []map[string]json.RawMessage `json:"videos"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			var got string
			if err := json.Unmarshal(out.Videos[0]["list_end_action"], &got); err != nil {
				t.Fatal(err)
			}
			if got != tt.upstream {
				t.Fatalf("saved list end %q, want %q (%s)", got, tt.upstream, raw)
			}
		})
	}
}

func TestManualNextWrapsButAutomaticEndHonorsPolicy(t *testing.T) {
	manual := &cell{list: []string{"a.mp4", "b.mp4"}, listIndex: 1, listEnd: "stop", path: "b.mp4"}
	a := &app{cells: []*cell{manual}}
	a.nextList(manual)
	if manual.listIndex != 0 || manual.path != "a.mp4" {
		t.Fatalf("manual next did not wrap: index=%d path=%q", manual.listIndex, manual.path)
	}

	automatic := &cell{list: []string{"a.mp4", "b.mp4"}, listIndex: 1, listEnd: "stop", path: "b.mp4"}
	a = &app{cells: []*cell{automatic}}
	a.advanceList(automatic)
	if automatic.listIndex != 1 || automatic.path != "b.mp4" || !automatic.startPaused {
		t.Fatalf("automatic stop advanced or did not stop: %+v", automatic)
	}
}

func TestAutomaticPreviousKeepsLegacyNextCellDirection(t *testing.T) {
	first := &cell{path: "first.mp4"}
	current := &cell{list: []string{"a.mp4", "b.mp4"}, listIndex: 0, listEnd: listEndNext, path: "a.mp4"}
	third := &cell{path: "third.mp4"}
	a := &app{cells: []*cell{first, current, third}, active: 1}
	a.stepPrevious(current)
	if a.active != 0 {
		t.Fatalf("automatic Previous moved to cell %d, want previous cell 0", a.active)
	}
}

func TestListEndPauseReloadsLoopStartOnEngine(t *testing.T) {
	loads := 0
	eng := engine.New(func(string, bool, string) (engine.Backend, error) {
		loads++
		return dummy.New(), nil
	})
	defer eng.Close()
	if err := eng.Load("b.mp4"); err != nil {
		t.Fatal(err)
	}
	eng.SeekTo(9000)
	c := &cell{
		list: []string{"a.mp4", "b.mp4"}, listIndex: 1,
		listEnd: listEndPause, path: "b.mp4", loopStart: 2000, eng: eng,
	}
	a := &app{cells: []*cell{c}}
	a.advanceList(c)
	s := eng.Snapshot()
	if loads != 2 || s.Phase != engine.Paused || s.Time != 0 ||
		c.startAt != 2000 || !c.pauseOnPlay.Load() {
		t.Fatalf("pause reload snapshot/guard: loads=%d %+v start=%d guard=%v", loads, s, c.startAt, c.pauseOnPlay.Load())
	}
}

func TestListEndPauseAtStartAndClose(t *testing.T) {
	paused := &cell{list: []string{"a.mp4", "b.mp4"}, listIndex: 1, listEnd: "pause", path: "b.mp4", startAt: 9000}
	a := &app{cells: []*cell{paused}}
	a.advanceList(paused)
	if paused.listIndex != 1 || paused.path != "b.mp4" || paused.startAt != 0 || !paused.startPaused {
		t.Fatalf("pause-at-start state: %+v", paused)
	}

	closing := &cell{list: []string{"a.mp4"}, listIndex: 0, listEnd: "close", path: "a.mp4"}
	other := &cell{path: "other.mp4"}
	a = &app{cells: []*cell{closing, other}}
	a.advanceList(closing)
	if len(a.cells) != 1 || a.cells[0] != other || closing.eng != nil || !a.dirty {
		t.Fatalf("close did not remove only the ended cell: cells=%v dirty=%v", a.cells, a.dirty)
	}
}
