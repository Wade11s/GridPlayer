package main

import (
	"testing"

	"gridplayer-go/internal/dummy"
	"gridplayer-go/internal/engine"
)

func TestChapterSpan(t *testing.T) {
	chapters := []engine.Chapter{{Index: 0, Time: 0}, {Index: 1, Time: 10000}, {Index: 2, Time: 30000}}
	for _, tt := range []struct {
		name       string
		at, length int64
		wantStart  int64
		wantEnd    int64
		wantOK     bool
	}{
		{name: "first", at: 0, length: 60000, wantStart: 0, wantEnd: 10000, wantOK: true},
		{name: "middle", at: 15000, length: 60000, wantStart: 10000, wantEnd: 30000, wantOK: true},
		{name: "last runs to EOF", at: 45000, length: 60000, wantStart: 30000, wantEnd: 0, wantOK: true},
		{name: "at boundary", at: 10000, length: 60000, wantStart: 10000, wantEnd: 30000, wantOK: true},
		{name: "before first", at: -1, length: 60000, wantStart: 0, wantEnd: 10000, wantOK: true},
		{name: "within tolerance", at: 9900, length: 60000, wantStart: 10000, wantEnd: 30000, wantOK: true},
	} {
		start, end, ok := chapterSpan(chapters, tt.at, tt.length)
		if ok != tt.wantOK || start != tt.wantStart || end != tt.wantEnd {
			t.Fatalf("%s: span=%d-%d ok=%v, want %d-%d %v", tt.name, start, end, ok, tt.wantStart, tt.wantEnd, tt.wantOK)
		}
	}
	if _, _, ok := chapterSpan(nil, 0, 1000); ok {
		t.Fatal("empty chapters produced a span")
	}
	if _, _, ok := chapterSpan([]engine.Chapter{{Time: 10000}}, 10000, 10000); ok {
		t.Fatal("zero-length last chapter produced a span")
	}
	start, end, ok := chapterSpan([]engine.Chapter{{Time: 10000}}, 12000, 0)
	if !ok || start != 10000 || end != 0 {
		t.Fatalf("unknown length span = %d-%d ok=%v", start, end, ok)
	}
	for _, tt := range []struct {
		start, end int64
		want       bool
	}{
		{start: 0, end: 20000, want: true},
		{start: 30000, end: 0, want: true},
		{start: 0, end: 0},
		{start: -5, end: 0},
	} {
		if got := segmentLoopActive(tt.start, tt.end); got != tt.want {
			t.Fatalf("segmentLoopActive(%d,%d) = %v, want %v", tt.start, tt.end, got, tt.want)
		}
	}
}

type chapterBackend struct {
	*dummy.Backend
	chapters []engine.Chapter
}

func (b *chapterBackend) Chapters() []engine.Chapter { return b.chapters }

func TestLoopChapterSetsBoundsAndReportsMissingChapters(t *testing.T) {
	newEngine := func(chapters []engine.Chapter) *engine.Engine {
		e := engine.New(func(string, bool, string) (engine.Backend, error) {
			return &chapterBackend{Backend: dummy.New(), chapters: chapters}, nil
		})
		if err := e.Load("movie.mkv"); err != nil {
			t.Fatal(err)
		}
		e.RefreshTracks()
		return e
	}
	withChapters := newEngine([]engine.Chapter{{Time: 0, Name: "one"}, {Time: 10000, Name: "two"}})
	defer withChapters.Close()
	a := &app{cells: []*cell{{eng: withChapters}}}
	a.loopChapter()
	start, end := a.cells[0].loopBounds()
	if start != 0 || end != 10000 || !a.dirty {
		t.Fatalf("chapter loop = %d-%d dirty=%v", start, end, a.dirty)
	}

	without := newEngine(nil)
	defer without.Close()
	a = &app{cells: []*cell{{eng: without}}}
	a.loopChapter()
	if a.dirty || a.status != "no chapter to loop" {
		t.Fatalf("missing chapters: dirty=%v status=%q", a.dirty, a.status)
	}
}

func TestCleanChapters(t *testing.T) {
	cleaned := cleanChapters([]engine.Chapter{
		{Index: 0, Time: 30000, Name: "three"},
		{Index: 1, Time: -5, Name: "negative"},
		{Index: 2, Time: 10000, Name: "two"},
		{Index: 3, Time: 10000, Name: "duplicate"},
		{Index: 4, Time: 90000, Name: "past end"},
		{Index: 5, Time: 120, Name: "snapped"},
	}, 60000)
	if len(cleaned) != 3 {
		t.Fatalf("cleaned = %+v", cleaned)
	}
	if cleaned[0].Time != 0 || cleaned[1].Time != 10000 || cleaned[2].Time != 30000 {
		t.Fatalf("cleaned times = %+v", cleaned)
	}
	if got := cleanChapters([]engine.Chapter{{Time: 0}, {Time: 100}}, 60000); got != nil {
		t.Fatalf("all-zero chapters = %+v", got)
	}
	if got := cleanChapters(nil, 0); got != nil {
		t.Fatalf("nil chapters = %+v", got)
	}
}

func TestLoopSeekTarget(t *testing.T) {
	for _, tt := range []struct {
		name       string
		at, start  int64
		end        int64
		wantTarget int64
		wantSeek   bool
	}{
		{name: "inside", at: 15000, start: 10000, end: 20000},
		{name: "at start", at: 10000, start: 10000, end: 20000},
		{name: "just before start", at: 9901, start: 10000, end: 20000},
		{name: "past slack", at: 9800, start: 10000, end: 20000, wantTarget: 10000, wantSeek: true},
		{name: "at end", at: 20000, start: 10000, end: 20000},
		{name: "past end", at: 20001, start: 10000, end: 20000, wantTarget: 10000, wantSeek: true},
		{name: "wrapped", at: 0, start: 10000, end: 20000, wantTarget: 10000, wantSeek: true},
		{name: "empty segment", at: 10000, start: 10000, end: 10000},
		{name: "negative start", at: 0, start: -5, end: 1000},
	} {
		target, seek := loopSeekTarget(tt.at, tt.start, tt.end)
		if seek != tt.wantSeek || target != tt.wantTarget {
			t.Fatalf("%s: target=%d seek=%v, want %d %v", tt.name, target, seek, tt.wantTarget, tt.wantSeek)
		}
	}
}
