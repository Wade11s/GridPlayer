package bookmarks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddListRemoveSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bookmarks.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Add("a.mp4", "Intro", 1000)
	s.Add("a.mp4", "Later", 5000)
	s.Add("a.mp4", "Intro", 2000)
	if got := s.List("a.mp4"); len(got) != 2 || got[0].Name != "Intro" || got[0].Position != 2000 {
		t.Fatalf("list %+v", got)
	}
	s.Remove("a.mp4", "Later")
	if got := s.List("a.mp4"); len(got) != 1 {
		t.Fatalf("after remove %+v", got)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.List("a.mp4"); len(got) != 1 || got[0].Position != 2000 {
		t.Fatalf("reload %+v", got)
	}
}

func TestMissingAndInvalid(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || len(s.Files) != 0 {
		t.Fatalf("missing %v %+v", err, s)
	}
	p := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(p, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("want invalid json error")
	}
}
