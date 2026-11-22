package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRealVLCExternalFilesSurviveReloadAndClear(t *testing.T) {
	fixture, err := filepath.Abs("../../testdata/fixtures/feature.mkv")
	if err != nil {
		t.Fatal(err)
	}
	subtitle, err := filepath.Abs("../../testdata/fixtures/feature.de.srt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("feature fixture missing")
	}
	if _, err := os.Stat(subtitle); err != nil {
		t.Skip("external subtitle fixture missing")
	}
	e := New(NewVLCFactory(VLCOptions{Args: []string{"--vout=dummy", "--aout=dummy"}}))
	defer e.Close()
	e.SetExternalFilesForLoad(nil, []string{subtitle})
	if err := e.Load(fixture); err != nil {
		t.Skipf("libVLC unavailable: %v", err)
	}
	withExternal := waitForSubtitleCondition(t, e, func(s Snapshot) bool {
		return s.Phase == Playing && len(s.SubtitleTracks) >= 3
	})
	if err := e.Load(fixture); err != nil {
		t.Fatal(err)
	}
	reloaded := waitForSubtitleCondition(t, e, func(s Snapshot) bool {
		return s.Phase == Playing && len(s.SubtitleTracks) >= withExternal
	})
	if !e.SetExternalFilesForLoad(nil, nil) {
		t.Fatal("clearing configured external files reported no change")
	}
	if err := e.Load(fixture); err != nil {
		t.Fatal(err)
	}
	cleared := waitForSubtitleCondition(t, e, func(s Snapshot) bool {
		return s.Phase == Playing && len(s.SubtitleTracks) > 0 && len(s.SubtitleTracks) < reloaded
	})
	if cleared >= reloaded {
		t.Fatalf("external subtitle survived clear: before=%d after=%d", reloaded, cleared)
	}
}

func waitForSubtitleCondition(t *testing.T, e *Engine, condition func(Snapshot) bool) int {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		e.RefreshTracks()
		s := e.Snapshot()
		if condition(s) {
			return len(s.SubtitleTracks)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("subtitle condition not reached: %+v", e.Snapshot())
	return 0
}
