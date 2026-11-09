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
	s.Add("a.mp4", "Intro", 9000)
	if got := s.List("a.mp4"); len(got) != 2 || got[0].Name != "Later" || got[0].Position != 5000 ||
		got[1].Name != "Intro" || got[1].Position != 9000 {
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
	if got := again.List("a.mp4"); len(got) != 1 || got[0].Position != 9000 {
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

func TestAddKeepsSortedAfterReplace(t *testing.T) {
	s := &Store{Files: map[string][]Bookmark{}}
	s.Add("a.mp4", "one", 1000)
	s.Add("a.mp4", "two", 5000)
	s.Add("a.mp4", "one", 9000)
	got := s.List("a.mp4")
	if len(got) != 2 || got[0].Name != "two" || got[0].Position != 5000 ||
		got[1].Name != "one" || got[1].Position != 9000 {
		t.Fatalf("replace later: %+v", got)
	}
	s.Add("a.mp4", "one", 500)
	got = s.List("a.mp4")
	if len(got) != 2 || got[0].Name != "one" || got[0].Position != 500 ||
		got[1].Name != "two" || got[1].Position != 5000 {
		t.Fatalf("replace earlier: %+v", got)
	}
}

func TestRenameHasClear(t *testing.T) {
	s := &Store{Files: map[string][]Bookmark{}}
	s.Add("a.mp4", "Intro", 1000)
	s.Add("a.mp4", "Later", 5000)
	if !s.Has("a.mp4", "Intro") || s.Has("a.mp4", "Missing") {
		t.Fatalf("Has: %+v", s.List("a.mp4"))
	}
	if err := s.Rename("a.mp4", "Intro", "Opening"); err != nil {
		t.Fatal(err)
	}
	if s.Has("a.mp4", "Intro") || !s.Has("a.mp4", "Opening") {
		t.Fatalf("rename: %+v", s.List("a.mp4"))
	}
	if err := s.Rename("a.mp4", "Opening", "Later"); err == nil {
		t.Fatal("duplicate rename accepted")
	}
	if err := s.Rename("a.mp4", "Opening", "  "); err == nil {
		t.Fatal("empty rename accepted")
	}
	if err := s.Rename("a.mp4", "Missing", "X"); err == nil {
		t.Fatal("missing rename accepted")
	}
	s.Clear("a.mp4")
	if len(s.List("a.mp4")) != 0 || s.Has("a.mp4", "Later") {
		t.Fatalf("clear: %+v", s.List("a.mp4"))
	}
}
