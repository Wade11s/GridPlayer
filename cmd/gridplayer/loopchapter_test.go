package main

import (
	"encoding/json"
	"testing"

	"gridplayer-go/internal/dummy"
	"gridplayer-go/internal/engine"
	"gridplayer-go/internal/playlist"
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
	var c cell
	c.setLoopBounds(0, 20000)
	if _, _, active := c.loopState(); !active {
		t.Fatal("zero-start segment is not active")
	}
	c.setLoopBounds(30000, 0)
	if _, _, active := c.loopState(); !active {
		t.Fatal("start-only segment is not active")
	}
	c.setLoopBounds(0, 0)
	if _, _, active := c.loopState(); active {
		t.Fatal("empty bounds are active")
	}
	c.setLoopSegment(0, 0, true)
	if _, _, active := c.loopState(); !active {
		t.Fatal("explicit whole-file segment is not active")
	}
	c.setLoopSegment(0, 0, false)
	if _, _, active := c.loopState(); active {
		t.Fatal("reset segment is active")
	}
}

type chapterBackend struct {
	*dummy.Backend
	chapters []engine.Chapter
}

func (b *chapterBackend) Chapters() []engine.Chapter { return b.chapters }

func TestLoopSeekBounds(t *testing.T) {
	for _, tt := range []struct {
		name           string
		start, end     int64
		length         int64
		active         bool
		wantLo, wantHi int64
		wantOK         bool
	}{
		{name: "inactive", start: 0, end: 20000, length: 60000},
		{name: "chapter", start: 10000, end: 20000, length: 60000, active: true, wantLo: 10000, wantHi: 20000, wantOK: true},
		{name: "start only", start: 40000, end: 0, length: 60000, active: true, wantLo: 40000, wantHi: 60000, wantOK: true},
		{name: "whole file", start: 0, end: 0, length: 60000, active: true, wantLo: 0, wantHi: 60000, wantOK: true},
		{name: "empty", start: 0, end: 0, length: 0, active: true},
		{name: "negative start", start: -5, end: 1000, length: 60000, active: true, wantLo: 0, wantHi: 1000, wantOK: true},
	} {
		lo, hi, ok := loopSeekBounds(tt.start, tt.end, tt.length, tt.active)
		if ok != tt.wantOK || lo != tt.wantLo || hi != tt.wantHi {
			t.Fatalf("%s: bounds=%d-%d ok=%v, want %d-%d %v", tt.name, lo, hi, ok, tt.wantLo, tt.wantHi, tt.wantOK)
		}
	}
}

func TestLoopActiveFromDocument(t *testing.T) {
	active := true
	for _, tt := range []struct {
		name string
		v    playlist.Video
		want bool
	}{
		{name: "whole file marker", v: playlist.Video{LoopActive: &active}, want: true},
		{name: "start", v: playlist.Video{LoopStart: 1000}, want: true},
		{name: "end", v: playlist.Video{LoopEnd: 2000}, want: true},
		{name: "none", v: playlist.Video{}},
	} {
		if got := loopActiveFromVideo(tt.v); got != tt.want {
			t.Fatalf("%s: active=%v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSeekCellToClampsSegment(t *testing.T) {
	e := engine.New(func(string, bool, string) (engine.Backend, error) {
		return dummy.New(), nil
	})
	if err := e.Load("movie.mp4"); err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	c := &cell{eng: e}
	a := &app{cells: []*cell{c}}
	c.setLoopSegment(40000, 0, true)
	a.seekCellTo(c, 1000)
	if got := e.Snapshot().Time; got != 40000 {
		t.Fatalf("before start seek = %d, want 40000", got)
	}
	c.setLoopSegment(0, 20000, true)
	a.seekCellTo(c, 25000)
	if got := e.Snapshot().Time; got != 0 {
		t.Fatalf("past end seek = %d, want 0", got)
	}
	c.setLoopSegment(0, 0, false)
	a.seekCellTo(c, 25000)
	if got := e.Snapshot().Time; got != 25000 {
		t.Fatalf("no segment seek = %d, want 25000", got)
	}
	e.Pause()
	c.setLoopSegment(10000, 20000, true)
	a.seekCellTo(c, 25000)
	if got := e.Snapshot().Time; got != 10000 {
		t.Fatalf("paused past-end seek = %d, want 10000", got)
	}
}

func TestWholeFileSegmentSurvivesDocument(t *testing.T) {
	a := &app{cells: []*cell{{}}}
	a.cells[0].setLoopSegment(0, 0, true)
	raw, err := json.Marshal(a.document())
	if err != nil {
		t.Fatal(err)
	}
	doc, err := playlist.Parse(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if !loopActiveFromVideo(doc.Videos[0]) {
		t.Fatalf("whole-file segment lost: %s", raw)
	}
}

func TestChapterStopSelection(t *testing.T) {
	chapters := []engine.Chapter{{Time: 0}, {Time: 10000}, {Time: 20000}, {Time: 30000}}
	if got := chapterStops(chapters, 10000, 30000); len(got) != 2 || got[0] != 10000 || got[1] != 20000 {
		t.Fatalf("stops = %v", got)
	}
	if stop, ok := nextStop(chapters, 9000, 0, 60000); !ok || stop != 10000 {
		t.Fatalf("next from 9 s = %d %v", stop, ok)
	}
	if stop, ok := nextStop(chapters, 9800, 0, 60000); !ok || stop != 20000 {
		t.Fatalf("next inside tolerance = %d %v, want 20000", stop, ok)
	}
	if stop, ok := nextStop(chapters, 35000, 0, 60000); ok {
		t.Fatalf("next past last = %d, want none", stop)
	}
	if stop := previousStop(chapters, 25000, 0, 60000); stop != 20000 {
		t.Fatalf("previous deep in chapter = %d, want 20000", stop)
	}
	if stop := previousStop(chapters, 21000, 0, 60000); stop != 10000 {
		t.Fatalf("previous near chapter start = %d, want 10000", stop)
	}
	if stop := previousStop(chapters, 5000, 0, 60000); stop != 0 {
		t.Fatalf("previous in first chapter = %d, want 0", stop)
	}
}

func TestChapterJumpSeeksInsideSegment(t *testing.T) {
	e := engine.New(func(string, bool, string) (engine.Backend, error) {
		return &chapterBackend{Backend: dummy.New(), chapters: []engine.Chapter{
			{Time: 0}, {Time: 10000}, {Time: 20000},
		}}, nil
	})
	if err := e.Load("movie.mkv"); err != nil {
		t.Fatal(err)
	}
	e.RefreshTracks()
	defer e.Close()
	c := &cell{eng: e}
	a := &app{cells: []*cell{c}}
	c.setLoopSegment(10000, 0, true)
	a.chapter(1)
	if got := e.Snapshot().Time; got != 10000 {
		t.Fatalf("chapter seek = %d, want 10000", got)
	}
	c.setLoopSegment(20000, 0, true)
	a.chapter(1)
	if got := e.Snapshot().Time; got != 20000 {
		t.Fatalf("outside chapter seek = %d, want segment start 20000", got)
	}
}

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

func TestListStepClearsLoopSegment(t *testing.T) {
	e := engine.New(func(string, bool, string) (engine.Backend, error) {
		return dummy.New(), nil
	})
	if err := e.Load("a.mp4"); err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	c := &cell{eng: e, list: []string{"a.mp4", "b.mp4"}}
	c.setLoopBounds(40000, 0)
	a := &app{cells: []*cell{c}}
	a.loadListIndex(c, 1)
	if _, _, active := c.loopState(); active {
		t.Fatal("new list item kept the old loop segment")
	}
	if c.path != "b.mp4" {
		t.Fatalf("path = %q", c.path)
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
