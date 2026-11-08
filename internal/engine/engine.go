// Package engine is the playback boundary between the UI and libVLC.
//
// It owns the per-cell session: a backend (real libVLC or a fake in tests),
// generation fencing so late events from a replaced media never reach the
// UI, and a coalesced snapshot stream. It does not touch windows or AppKit;
// the caller supplies a backend factory and (macOS) binds the NSView before
// the first load.
package engine

import (
	"errors"
	"sync"
)

// Phase is the playback state shown to the UI.
type Phase uint8

const (
	Idle Phase = iota
	Loading
	Playing
	Paused
	Ended
	Failed
)

func (p Phase) String() string {
	switch p {
	case Idle:
		return "idle"
	case Loading:
		return "loading"
	case Playing:
		return "playing"
	case Paused:
		return "paused"
	case Ended:
		return "ended"
	case Failed:
		return "failed"
	default:
		return "unknown"
	}
}

// Snapshot is an immutable view of one cell.
type Snapshot struct {
	Gen           uint64
	Phase         Phase
	Time, Length  int64 // milliseconds
	Rate          float64
	Volume        int // 0..100 (libVLC scale)
	Muted         bool
	Loop          bool
	Width, Height int
	Err           string

	AudioTracks, VideoTracks, SubtitleTracks []Track
	Chapters                                 []Chapter
	AudioTrack, VideoTrack, SubtitleTrack    int
	Chapter                                  int
}

// Track is an audio, video or subtitle track.
type Track struct {
	ID   int
	Name string
}

// Chapter is a title chapter.
type Chapter struct {
	Index int
	Name  string
}

// BackendState is the subset of player states the engine distinguishes.
type BackendState uint8

const (
	BackendIdle BackendState = iota
	BackendLoading
	BackendPlaying
	BackendPaused
	BackendEnded
	BackendFailed
)

// EventKind is a backend event the engine consumes.
type EventKind uint8

const (
	EventPlaying EventKind = iota
	EventPaused
	EventStopped
	EventEnded
	EventFailed
	EventTime
)

// Event carries a backend event and, for EventTime, the new time in ms.
type Event struct {
	Kind EventKind
	Time int64
}

// Backend is one media player. All methods must be safe from any goroutine.
type Backend interface {
	Play()
	Pause()
	Stop()
	SeekTo(ms int64)
	SetRate(rate float64)
	SetVolume(volume int)
	SetMute(muted bool)
	SetLoop(loop bool)
	Time() int64
	Length() int64
	State() BackendState
	VideoSize() (width, height int, ok bool)
	AudioTracks() []Track
	VideoTracks() []Track
	SubtitleTracks() []Track
	AudioTrack() int
	VideoTrack() int
	SubtitleTrack() int
	SetAudioTrack(id int) bool
	SetVideoTrack(id int) bool
	SetSubtitleTrack(id int) bool
	Chapters() []Chapter
	Chapter() int
	SetChapter(i int) bool
	Events() <-chan Event
	Close()
}

// Factory builds a backend for one media path. The engine calls it once per
// load; the factory must bind any native surface before the first play.
// loop is fixed for the life of that media (libVLC's input-repeat).
type Factory func(path string, loop bool) (Backend, error)

// ErrClosed is returned by commands after Close.
var ErrClosed = errors.New("engine: closed")

// Engine is one cell's playback session. Create it once, reuse it across
// loads: every Load increments Gen and fences late events from the old one.
type Engine struct {
	factory Factory

	loadMu sync.Mutex
	mu     sync.Mutex
	gen    uint64
	be     Backend
	path   string
	start  int64
	quit   chan struct{}
	closed bool

	snap    Snapshot
	updates chan Snapshot

	// settings applied to each new backend and carried across loads
	rate   float64
	volume int
	muted  bool
	loop   bool
}

// New returns an idle engine with the given factory.
func New(factory Factory) *Engine {
	return &Engine{
		factory: factory,
		updates: make(chan Snapshot, 1),
		rate:    1,
		volume:  100,
		snap:    Snapshot{Phase: Idle, Rate: 1, Volume: 100},
	}
}

// Updates returns coalesced snapshots. It never blocks the engine.
func (e *Engine) Updates() <-chan Snapshot { return e.updates }

// Snapshot returns the current state.
func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snap
}

func (e *Engine) publishLocked() {
	select {
	case e.updates <- e.snap:
	default:
		// Keep only the newest state for the UI.
		select {
		case <-e.updates:
		default:
		}
		select {
		case e.updates <- e.snap:
		default:
		}
	}
}

// Load replaces the media. It is safe to call from any goroutine. The old
// backend is closed before the new one is created; a late event from it is
// ignored by generation.
func (e *Engine) Load(path string) error {
	e.loadMu.Lock()
	defer e.loadMu.Unlock()

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return ErrClosed
	}
	e.gen++
	gen := e.gen
	old := e.be
	oldQuit := e.quit
	e.be = nil
	e.path = path
	e.quit = make(chan struct{})
	quit := e.quit
	loop := e.loop
	e.snap = Snapshot{
		Gen: gen, Phase: Loading, Rate: e.rate, Volume: e.volume,
		Muted: e.muted, Loop: loop,
	}
	e.publishLocked()
	e.mu.Unlock()

	if oldQuit != nil {
		close(oldQuit)
	}
	if old != nil {
		old.Close()
	}

	be, err := e.factory(path, loop)
	if err != nil {
		e.mu.Lock()
		if gen == e.gen && !e.closed {
			e.snap.Phase = Failed
			e.snap.Err = err.Error()
			e.publishLocked()
		}
		e.mu.Unlock()
		return err
	}

	e.mu.Lock()
	if gen != e.gen || e.closed {
		e.mu.Unlock()
		be.Close()
		return ErrClosed
	}
	e.be = be
	rate, volume, muted, loop := e.rate, e.volume, e.muted, e.loop
	e.mu.Unlock()

	be.SetRate(rate)
	be.SetVolume(volume)
	be.SetMute(muted)
	be.SetLoop(loop)
	go e.pump(gen, be, quit)
	be.Play()
	return nil
}

// Reload re-opens the current media, applying settings that libVLC fixes
// when a media opens (loop). It is a no-op before the first Load.
func (e *Engine) Reload() error {
	e.mu.Lock()
	path := e.path
	e.mu.Unlock()
	if path == "" {
		return nil
	}
	return e.Load(path)
}

func (e *Engine) pump(gen uint64, be Backend, quit chan struct{}) {
	events := be.Events()
	for {
		select {
		case <-quit:
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			e.apply(gen, be, ev)
		}
	}
}

func (e *Engine) apply(gen uint64, be Backend, ev Event) {
	if ev.Kind == EventPlaying {
		e.mu.Lock()
		start := e.start
		if start != 0 {
			e.start = 0
		}
		e.mu.Unlock()
		if start != 0 {
			be.SeekTo(start)
		}
	}
	time := be.Time()
	length := be.Length()
	w, h, _ := be.VideoSize()
	state := be.State()
	var audioTracks, videoTracks, subtitleTracks []Track
	var chapters []Chapter
	var audioID, videoID, subtitleID, chapter int
	if ev.Kind == EventPlaying {
		audioTracks = be.AudioTracks()
		videoTracks = be.VideoTracks()
		subtitleTracks = be.SubtitleTracks()
		audioID = be.AudioTrack()
		videoID = be.VideoTrack()
		subtitleID = be.SubtitleTrack()
		chapters = be.Chapters()
		chapter = be.Chapter()
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if gen != e.gen || e.closed {
		return
	}
	e.snap.Time, e.snap.Length = time, length
	if w > 0 && h > 0 {
		e.snap.Width, e.snap.Height = w, h
	}
	switch ev.Kind {
	case EventPlaying:
		e.snap.Phase = Playing
		e.snap.Err = ""
		e.snap.AudioTracks = audioTracks
		e.snap.VideoTracks = videoTracks
		e.snap.SubtitleTracks = subtitleTracks
		e.snap.AudioTrack = audioID
		e.snap.VideoTrack = videoID
		e.snap.SubtitleTrack = subtitleID
		e.snap.Chapters = chapters
		e.snap.Chapter = chapter
	case EventPaused:
		e.snap.Phase = Paused
	case EventStopped:
		if e.snap.Phase != Ended {
			e.snap.Phase = Idle
		}
	case EventEnded:
		if e.loop {
			// libVLC loops the input in place; the next Playing resets it.
			break
		}
		e.snap.Phase = Ended
	case EventFailed:
		e.snap.Phase = Failed
		e.snap.Err = "libVLC reported an error"
	case EventTime:
		if ev.Time >= 0 {
			e.snap.Time = ev.Time
		}
		if e.snap.Phase == Loading && state == BackendPlaying {
			e.snap.Phase = Playing
		}
	}
	e.publishLocked()
}

// SetStartPosition seeks once, after the media starts playing.
func (e *Engine) SetStartPosition(ms int64) {
	if ms < 0 {
		ms = 0
	}
	e.mu.Lock()
	e.start = ms
	e.mu.Unlock()
}

func (e *Engine) backend() (Backend, uint64, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.be == nil {
		return nil, e.gen, false
	}
	return e.be, e.gen, true
}

// Play resumes playback.
func (e *Engine) Play() {
	be, gen, ok := e.backend()
	if !ok {
		return
	}
	be.Play()
	e.optimistic(gen, Playing)
}

// Pause pauses playback.
func (e *Engine) Pause() {
	be, gen, ok := e.backend()
	if !ok {
		return
	}
	be.Pause()
	e.optimistic(gen, Paused)
}

// Toggle plays if paused, pauses if playing.
func (e *Engine) Toggle() {
	if e.Snapshot().Phase == Playing {
		e.Pause()
		return
	}
	e.Play()
}

func (e *Engine) optimistic(gen uint64, phase Phase) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if gen != e.gen || e.closed {
		return
	}
	e.snap.Phase = phase
	e.publishLocked()
}

// Seek moves the playhead.
func (e *Engine) SeekTo(ms int64) {
	if ms < 0 {
		ms = 0
	}
	be, gen, ok := e.backend()
	if !ok {
		return
	}
	be.SeekTo(ms)
	e.mu.Lock()
	if gen == e.gen && !e.closed {
		e.snap.Time = ms
		e.publishLocked()
	}
	e.mu.Unlock()
}

// SetRate sets the playback rate.
func (e *Engine) SetRate(rate float64) {
	if rate <= 0 {
		return
	}
	e.mu.Lock()
	e.rate = rate
	e.snap.Rate = rate
	be := e.be
	e.publishLocked()
	e.mu.Unlock()
	if be != nil {
		be.SetRate(rate)
	}
}

// SetVolume sets the volume, 0..100.
func (e *Engine) SetVolume(volume int) {
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	e.mu.Lock()
	e.volume = volume
	e.snap.Volume = volume
	be := e.be
	e.publishLocked()
	e.mu.Unlock()
	if be != nil {
		be.SetVolume(volume)
	}
}

// SetMute mutes or unmutes.
func (e *Engine) SetMute(muted bool) {
	e.mu.Lock()
	e.muted = muted
	e.snap.Muted = muted
	be := e.be
	e.publishLocked()
	e.mu.Unlock()
	if be != nil {
		be.SetMute(muted)
	}
}

// SetLoop loops the current media. libVLC fixes input-repeat when the
// media opens, so a change takes effect at the next Load; the snapshot
// records the requested value immediately.
func (e *Engine) SetLoop(loop bool) {
	e.mu.Lock()
	e.loop = loop
	e.snap.Loop = loop
	be := e.be
	e.publishLocked()
	e.mu.Unlock()
	if be != nil {
		be.SetLoop(loop)
	}
}

// SetAudioTrack selects an audio track by libVLC ID.
func (e *Engine) SetAudioTrack(id int) bool {
	be, gen, ok := e.backend()
	if !ok || !be.SetAudioTrack(id) {
		return false
	}
	e.mu.Lock()
	if gen == e.gen {
		e.snap.AudioTrack = id
		e.publishLocked()
	}
	e.mu.Unlock()
	return true
}

// SetVideoTrack selects a video track by libVLC ID (-1 disables video).
func (e *Engine) SetVideoTrack(id int) bool {
	be, gen, ok := e.backend()
	if !ok || !be.SetVideoTrack(id) {
		return false
	}
	e.mu.Lock()
	if gen == e.gen {
		e.snap.VideoTrack = id
		e.publishLocked()
	}
	e.mu.Unlock()
	return true
}

// SetSubtitleTrack selects a subtitle track by libVLC ID (-1 disables).
func (e *Engine) SetSubtitleTrack(id int) bool {
	be, gen, ok := e.backend()
	if !ok || !be.SetSubtitleTrack(id) {
		return false
	}
	e.mu.Lock()
	if gen == e.gen {
		e.snap.SubtitleTrack = id
		e.publishLocked()
	}
	e.mu.Unlock()
	return true
}

// SetChapter jumps to a chapter by index.
func (e *Engine) SetChapter(i int) bool {
	be, gen, ok := e.backend()
	if !ok || !be.SetChapter(i) {
		return false
	}
	e.mu.Lock()
	if gen == e.gen {
		e.snap.Chapter = i
		e.publishLocked()
	}
	e.mu.Unlock()
	return true
}

// RefreshTracks re-reads tracks, subtitles and chapters from the backend.
// The UI calls it when it opens a menu or after a load settles.
func (e *Engine) RefreshTracks() {
	be, gen, ok := e.backend()
	if !ok {
		return
	}
	audio := be.AudioTracks()
	video := be.VideoTracks()
	subs := be.SubtitleTracks()
	audioID, videoID, subID := be.AudioTrack(), be.VideoTrack(), be.SubtitleTrack()
	chapters := be.Chapters()
	chapter := be.Chapter()
	e.mu.Lock()
	if gen == e.gen {
		e.snap.AudioTracks, e.snap.VideoTracks, e.snap.SubtitleTracks = audio, video, subs
		e.snap.AudioTrack, e.snap.VideoTrack, e.snap.SubtitleTrack = audioID, videoID, subID
		e.snap.Chapters, e.snap.Chapter = chapters, chapter
		e.publishLocked()
	}
	e.mu.Unlock()
}

// Stop stops playback and leaves the engine idle.
func (e *Engine) Stop() {
	be, gen, ok := e.backend()
	if !ok {
		return
	}
	be.Stop()
	e.optimistic(gen, Idle)
}

// Close stops and releases the backend. It is idempotent.
func (e *Engine) Close() {
	e.loadMu.Lock()
	defer e.loadMu.Unlock()
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	be := e.be
	quit := e.quit
	e.be = nil
	e.quit = nil
	e.snap.Phase = Idle
	e.publishLocked()
	e.mu.Unlock()
	if quit != nil {
		close(quit)
	}
	if be != nil {
		be.Close()
	}
}
