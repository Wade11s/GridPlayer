// Package dummy is a fake playback backend for layout tests: a deterministic
// 100s clock at 25fps with no decoding. It never claims hardware decode.
package dummy

import (
	"sync"
	"sync/atomic"
	"time"

	"gridplayer-go/internal/engine"
)

const (
	LengthMs = 100000
	FPS      = 25
)

type Backend struct {
	mu     sync.Mutex
	events chan engine.Event
	start  atomic.Int64 // UnixNano of play start
	offset int64
	paused bool
	closed atomic.Bool
}

// New returns a dummy backend whose clock starts now.
func New() *Backend {
	b := &Backend{
		events: make(chan engine.Event, 32),
	}
	b.start.Store(time.Now().UnixNano())
	return b
}

func (b *Backend) now() int64 {
	if b.paused {
		return b.offset
	}
	return b.offset + (time.Now().UnixNano()-b.start.Load())/1e6
}

func (b *Backend) Play()  { b.paused = false; b.start.Store(time.Now().UnixNano()) }
func (b *Backend) Pause() { b.offset = b.now(); b.paused = true }
func (b *Backend) Stop()  { b.offset = 0; b.paused = false; b.start.Store(time.Now().UnixNano()) }
func (b *Backend) SeekTo(ms int64) {
	if ms < 0 {
		ms = 0
	}
	b.offset = ms % LengthMs
	b.start.Store(time.Now().UnixNano())
}
func (b *Backend) SetRate(float64)            {}
func (b *Backend) SetVolume(int)              {}
func (b *Backend) SetMute(bool)               {}
func (b *Backend) SetLoop(bool)               {}
func (b *Backend) SetAspectRatio(string) bool { return true }
func (b *Backend) SetScale(float64) bool      { return true }
func (b *Backend) SetCrop(string) bool        { return true }
func (b *Backend) AddSubtitle(string) bool    { return true }
func (b *Backend) AddAudio(string) bool       { return true }
func (b *Backend) SetAudioTrack(int) bool     { return true }
func (b *Backend) SetVideoTrack(int) bool     { return true }
func (b *Backend) SetSubtitleTrack(int) bool  { return true }
func (b *Backend) SetChapter(int) bool        { return true }
func (b *Backend) AudioTracks() []engine.Track {
	return []engine.Track{{ID: 1, Name: "Dummy Audio"}}
}
func (b *Backend) VideoTracks() []engine.Track {
	return []engine.Track{{ID: 1, Name: "Dummy Video"}}
}
func (b *Backend) SubtitleTracks() []engine.Track { return nil }
func (b *Backend) AudioTrack() int                { return 1 }
func (b *Backend) VideoTrack() int                { return 1 }
func (b *Backend) SubtitleTrack() int             { return -1 }
func (b *Backend) Chapters() []engine.Chapter     { return nil }
func (b *Backend) Chapter() int                   { return 0 }
func (b *Backend) Time() int64                    { return b.now() }
func (b *Backend) Length() int64                  { return LengthMs }
func (b *Backend) VideoSize() (int, int, bool)    { return 640, 360, true }
func (b *Backend) Events() <-chan engine.Event    { return b.events }
func (b *Backend) Tick() {
	if b.closed.Load() {
		return
	}
	select {
	case b.events <- engine.Event{Kind: engine.EventPlaying}:
	default:
	}
}
func (b *Backend) Screenshot(string) error { return nil }

func (b *Backend) State() engine.BackendState {
	if b.closed.Load() {
		return engine.BackendIdle
	}
	if b.paused {
		return engine.BackendPaused
	}
	return engine.BackendPlaying
}

func (b *Backend) Close() { b.closed.Store(true) }
