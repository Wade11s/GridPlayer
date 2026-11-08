package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadQSettingsSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.ini")
	src := "; comment\n[playlist]\ngrid_mode=fixed\ngrid_rows=2\n\n[player]\nvideo_driver=vlc_hw_sp\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get("playlist/grid_mode", ""); got != "fixed" {
		t.Fatalf("grid mode %q", got)
	}
	if got := s.Get("player/video_driver", ""); got != "vlc_hw_sp" {
		t.Fatalf("driver %q", got)
	}
	if got := s.Get("missing/key", "def"); got != "def" {
		t.Fatalf("default %q", got)
	}
}

func TestRoundTripSorted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.ini")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Set("playlist/grid_cols", "3")
	s.Set("playlist/grid_rows", "2")
	s.Set("app/name", "GridPlayer")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "[app]\nname=GridPlayer\n") {
		t.Fatalf("app section:\n%s", text)
	}
	if strings.Index(text, "grid_cols=3") > strings.Index(text, "grid_rows=2") {
		t.Fatalf("keys not sorted:\n%s", text)
	}
	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if again.Get("playlist/grid_cols", "") != "3" {
		t.Fatalf("roundtrip: %+v", again)
	}
}

func TestMissingFileIsEmpty(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "nope.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Get("x", "y") != "y" {
		t.Fatal("not empty")
	}
}
