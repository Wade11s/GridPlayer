package engine

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeBackend struct {
	mu     sync.Mutex
	events chan Event
	closed bool

	plays, pauses int
	seek          int64
	rate          float64
	volume        int
	muted, loop   bool
	time, length  int64
	state         BackendState
	w, h          int
	audio         []Track
	video         []Track
	subs          []Track
	chapters      []Chapter
	audioID       int
	videoID       int
	subID         int
	chapter       int
}

func newFake() *fakeBackend {
	return &fakeBackend{events: make(chan Event, 32), volume: 100, rate: 1}
}

func (f *fakeBackend) emit(ev Event) { f.events <- ev }

func (f *fakeBackend) Play() {
	f.mu.Lock()
	f.plays++
	f.state = BackendPlaying
	f.mu.Unlock()
}
func (f *fakeBackend) Pause() {
	f.mu.Lock()
	f.pauses++
	f.state = BackendPaused
	f.mu.Unlock()
}
func (f *fakeBackend) Stop() {
	f.mu.Lock()
	f.state = BackendIdle
	f.mu.Unlock()
}
func (f *fakeBackend) SeekTo(ms int64) {
	f.mu.Lock()
	f.seek = ms
	f.time = ms
	f.mu.Unlock()
}
func (f *fakeBackend) SetRate(r float64) {
	f.mu.Lock()
	f.rate = r
	f.mu.Unlock()
}
func (f *fakeBackend) SetVolume(v int) {
	f.mu.Lock()
	f.volume = v
	f.mu.Unlock()
}
func (f *fakeBackend) SetMute(m bool) {
	f.mu.Lock()
	f.muted = m
	f.mu.Unlock()
}
func (f *fakeBackend) SetLoop(l bool) {
	f.mu.Lock()
	f.loop = l
	f.mu.Unlock()
}
func (f *fakeBackend) Time() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.time
}
func (f *fakeBackend) Length() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.length
}
func (f *fakeBackend) State() BackendState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}
func (f *fakeBackend) VideoSize() (int, int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.w, f.h, f.w > 0 && f.h > 0
}
func (f *fakeBackend) AudioTracks() []Track { return f.audio }
func (f *fakeBackend) VideoTracks() []Track { return f.video }
func (f *fakeBackend) SubtitleTracks() []Track {
	return f.subs
}
func (f *fakeBackend) AudioTrack() int    { return f.audioID }
func (f *fakeBackend) VideoTrack() int    { return f.videoID }
func (f *fakeBackend) SubtitleTrack() int { return f.subID }
func (f *fakeBackend) SetAudioTrack(id int) bool {
	f.audioID = id
	return true
}
func (f *fakeBackend) SetVideoTrack(id int) bool {
	f.videoID = id
	return true
}
func (f *fakeBackend) SetSubtitleTrack(id int) bool {
	f.subID = id
	return true
}
func (f *fakeBackend) Chapters() []Chapter { return f.chapters }
func (f *fakeBackend) Chapter() int        { return f.chapter }
func (f *fakeBackend) SetChapter(i int) bool {
	f.chapter = i
	return true
}
func (f *fakeBackend) Events() <-chan Event { return f.events }
func (f *fakeBackend) Close() {
	f.mu.Lock()
	f.closed = true
	f.state = BackendIdle
	f.mu.Unlock()
}

func (f *fakeBackend) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func waitFor(t *testing.T, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestLifecycleCommandsAndSnapshot(t *testing.T) {
	f := newFake()
	f.length = 10_000
	f.w, f.h = 640, 360
	f.audio = []Track{{ID: 1, Name: "English"}, {ID: 2, Name: "Spanish"}}
	f.video = []Track{{ID: 1, Name: "Main"}}
	f.subs = []Track{{ID: -1, Name: "Disable"}, {ID: 3, Name: "English"}}
	f.chapters = []Chapter{{Index: 0, Name: "Intro"}, {Index: 1, Name: "Part One"}}
	f.audioID, f.videoID, f.subID, f.chapter = 1, 1, -1, 0
	e := New(func(path string, loop bool) (Backend, error) { return f, nil })
	defer e.Close()

	if err := e.Load("a.mp4"); err != nil {
		t.Fatal(err)
	}
	f.emit(Event{Kind: EventPlaying})
	waitFor(t, "playing", func() bool { return e.Snapshot().Phase == Playing })
	s := e.Snapshot()
	if len(s.AudioTracks) != 2 || len(s.Chapters) != 2 || s.AudioTrack != 1 || s.Chapter != 0 {
		t.Fatalf("track snapshot %+v", s)
	}
	if !e.SetAudioTrack(2) || !e.SetSubtitleTrack(3) || !e.SetChapter(1) {
		t.Fatal("track/chapter command failed")
	}
	s = e.Snapshot()
	if s.AudioTrack != 2 || s.SubtitleTrack != 3 || s.Chapter != 1 {
		t.Fatalf("selection snapshot %+v", s)
	}

	e.SeekTo(3000)
	f.emit(Event{Kind: EventTime, Time: 3500})
	waitFor(t, "time 3500", func() bool { return e.Snapshot().Time == 3500 })

	e.Pause()
	f.emit(Event{Kind: EventPaused})
	waitFor(t, "paused", func() bool { return e.Snapshot().Phase == Paused })
	if f.pauses != 1 {
		t.Fatalf("pauses=%d", f.pauses)
	}

	e.SetRate(1.5)
	e.SetVolume(42)
	e.SetMute(true)
	s = e.Snapshot()
	if s.Rate != 1.5 || s.Volume != 42 || !s.Muted {
		t.Fatalf("snapshot %+v", s)
	}
	f.mu.Lock()
	gotRate, gotVol, gotMute := f.rate, f.volume, f.muted
	f.mu.Unlock()
	if gotRate != 1.5 || gotVol != 42 || !gotMute {
		t.Fatalf("backend rate=%v volume=%d muted=%v", gotRate, gotVol, gotMute)
	}
	if s.Width != 640 || s.Height != 360 || s.Length != 10_000 {
		t.Fatalf("size/length %+v", s)
	}
}

func TestReloadFencesStaleEvents(t *testing.T) {
	first := newFake()
	second := newFake()
	var mu sync.Mutex
	next := 0
	e := New(func(path string, loop bool) (Backend, error) {
		mu.Lock()
		defer mu.Unlock()
		next++
		if next == 1 {
			return first, nil
		}
		return second, nil
	})
	defer e.Close()

	if err := e.Load("one.mp4"); err != nil {
		t.Fatal(err)
	}
	first.emit(Event{Kind: EventPlaying})
	waitFor(t, "first playing", func() bool { return e.Snapshot().Phase == Playing })

	if err := e.Load("two.mp4"); err != nil {
		t.Fatal(err)
	}
	gen := e.Snapshot().Gen
	first.emit(Event{Kind: EventFailed})
	time.Sleep(20 * time.Millisecond)
	if s := e.Snapshot(); s.Phase == Failed || s.Gen != gen {
		t.Fatalf("stale event leaked: %+v", s)
	}
	if !first.isClosed() {
		t.Fatal("old backend not closed")
	}
	second.emit(Event{Kind: EventPlaying})
	waitFor(t, "second playing", func() bool { return e.Snapshot().Phase == Playing })
}

func TestLoadFailureAndCloseIdempotent(t *testing.T) {
	e := New(func(path string, loop bool) (Backend, error) {
		return nil, errors.New("boom")
	})
	if err := e.Load("bad"); err == nil {
		t.Fatal("want error")
	}
	if s := e.Snapshot(); s.Phase != Failed || s.Err != "boom" {
		t.Fatalf("snapshot %+v", s)
	}
	e.Close()
	e.Close()
}

func TestLoopCarriedIntoFactoryAndEnded(t *testing.T) {
	f := newFake()
	var gotLoop bool
	e := New(func(path string, loop bool) (Backend, error) {
		gotLoop = loop
		return f, nil
	})
	defer e.Close()
	e.SetLoop(true)
	if err := e.Load("loop.mp4"); err != nil {
		t.Fatal(err)
	}
	if !gotLoop {
		t.Fatal("factory did not receive loop=true")
	}
	f.emit(Event{Kind: EventEnded})
	time.Sleep(20 * time.Millisecond)
	if s := e.Snapshot(); s.Phase == Ended {
		t.Fatalf("loop should suppress ended: %+v", s)
	}
	e.SetLoop(false)
	f.emit(Event{Kind: EventEnded})
	waitFor(t, "ended", func() bool { return e.Snapshot().Phase == Ended })
}

func TestStartPositionSeeksOnFirstPlaying(t *testing.T) {
	f := newFake()
	e := New(func(path string, loop bool) (Backend, error) { return f, nil })
	defer e.Close()
	e.SetStartPosition(3000)
	if err := e.Load("resume.mp4"); err != nil {
		t.Fatal(err)
	}
	f.emit(Event{Kind: EventPlaying})
	waitFor(t, "resume seek", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.seek == 3000
	})
}
