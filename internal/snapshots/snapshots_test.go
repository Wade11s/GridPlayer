package snapshots

import (
	"os"
	"path/filepath"
	"testing"

	"gridplayer-go/internal/playlist"
)

func TestSaveListGetRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshots.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	one := &playlist.Document{
		Format: playlist.FormatID, Version: 1,
		Videos: []playlist.Video{{URI: "/a.mp4", CellViewMode: "stretch", ListEnd: "next"}},
	}
	if err := s.Save("one", one); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("two", &playlist.Document{Format: playlist.FormatID, Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("one", one); err != nil {
		t.Fatal(err)
	}
	if got := s.List(); len(got) != 2 || got[0].Name != "one" || got[1].Name != "two" {
		t.Fatalf("list %+v", got)
	}
	d, ok := s.Get("one")
	if !ok || len(d.Videos) != 1 || d.Videos[0].CellViewMode != "stretch" || d.Videos[0].ListEnd != "next" {
		t.Fatalf("get %+v %v", d, ok)
	}
	if err := s.Remove("one"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("one"); ok {
		t.Fatal("removed snapshot still present")
	}

	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.List(); len(got) != 1 || got[0].Name != "two" {
		t.Fatalf("reload %+v", got)
	}
}

func TestEmptyNameAndInvalidFile(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "snapshots.json"))
	if err := s.Save("  ", &playlist.Document{}); err == nil {
		t.Fatal("empty name accepted")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil {
		t.Fatal("invalid json accepted")
	}
}

func TestSaveFailureKeepsMemory(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(filepath.Join(blocker, "snapshots.json"))
	if err := s.Save("one", &playlist.Document{Format: playlist.FormatID, Version: 1}); err == nil {
		t.Fatal("want write error")
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("memory changed: %+v", got)
	}
}

func TestRemoveFailureKeepsMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshots.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save("one", &playlist.Document{Format: playlist.FormatID, Version: 1}); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.path = filepath.Join(blocker, "snapshots.json")
	if err := s.Remove("one"); err == nil {
		t.Fatal("want write error")
	}
	if got := s.List(); len(got) != 1 || got[0].Name != "one" {
		t.Fatalf("memory changed: %+v", got)
	}
}
