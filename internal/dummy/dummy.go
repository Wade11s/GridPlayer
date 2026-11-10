// Package dummy is a fake playback backend for layout tests: a deterministic
// 100s clock at 25fps (frame = time / 40ms) with no decoding. It never
// claims hardware decode.
package dummy

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"gridplayer-go/internal/engine"
)

const (
	LengthMs = 100000
	FPS      = 25
	FrameMs  = 1000 / FPS
)

type Backend struct {
	mu       sync.Mutex
	events   chan engine.Event
	start    atomic.Int64 // UnixNano of play start
	offset   int64
	delay    int64
	subDelay int64
	paused   bool
	stopped  bool
	closed   atomic.Bool
}

// New returns a dummy backend whose clock starts now.
func New() *Backend {
	b := &Backend{events: make(chan engine.Event, 32)}
	b.start.Store(time.Now().UnixNano())
	return b
}

func (b *Backend) now() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.paused || b.stopped {
		return b.offset
	}
	elapsed := (time.Now().UnixNano() - b.start.Load()) / 1e6
	return (b.offset + elapsed) % LengthMs
}

func (b *Backend) Play() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.paused {
		b.paused = false
		b.stopped = false
		b.start.Store(time.Now().UnixNano())
	} else if b.stopped {
		b.offset = 0
		b.paused = false
		b.stopped = false
		b.start.Store(time.Now().UnixNano())
	}
	// Already playing: do nothing (idempotent; does not rewind).
}

func (b *Backend) Pause() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.paused && !b.stopped {
		b.offset = b.nowLocked()
		b.paused = true
	}
}

func (b *Backend) nowLocked() int64 {
	if b.paused || b.stopped {
		return b.offset
	}
	return (b.offset + (time.Now().UnixNano()-b.start.Load())/1e6) % LengthMs
}

func (b *Backend) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.offset = 0
	b.paused = false
	b.stopped = true
}

func (b *Backend) NextFrame() {
	b.Pause()
	b.mu.Lock()
	b.offset += FrameMs
	if b.offset > LengthMs-FrameMs {
		b.offset = LengthMs - FrameMs
	}
	b.mu.Unlock()
}

func (b *Backend) PreviousFrame() {
	b.Pause()
	b.mu.Lock()
	b.offset = engine.FrameStepBack(b.offset, FPS)
	b.mu.Unlock()
}

func (b *Backend) SetAudioDelay(ms int64) {
	b.mu.Lock()
	b.delay = ms
	b.mu.Unlock()
}

func (b *Backend) SetSubtitleDelay(ms int64) {
	b.mu.Lock()
	b.subDelay = ms
	b.mu.Unlock()
}

func (b *Backend) SeekTo(ms int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ms < 0 {
		ms = 0
	}
	b.offset = ms % LengthMs
	b.stopped = false
	b.paused = false
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
	b.mu.Lock()
	stopped, paused := b.stopped, b.paused
	b.mu.Unlock()
	if b.closed.Load() || stopped || paused {
		return
	}
	select {
	case b.events <- engine.Event{Kind: engine.EventPlaying}:
	default:
	}
}

// Screenshot writes a deterministic placeholder PNG (a single color indexed
// by the current frame) so layout acceptance has a file artifact. It is
// not a decode.
func (b *Backend) Screenshot(path string) error {
	if path == "" {
		return errors.New("dummy: empty screenshot path")
	}
	t := b.now()
	img := image.NewRGBA(image.Rect(0, 0, 64, 36))
	c := color.RGBA{R: uint8(t % 256), G: uint8((t / 256) % 256), B: 64, A: 255}
	for y := 0; y < 36; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func (b *Backend) State() engine.BackendState {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed.Load() || b.stopped {
		return engine.BackendIdle
	}
	if b.paused {
		return engine.BackendPaused
	}
	return engine.BackendPlaying
}

func (b *Backend) Close() { b.closed.Store(true) }
