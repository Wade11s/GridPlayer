package engine

import (
	"sync"

	"gridplayer-go/internal/vlcmac"
)

// VLCOptions configures real libVLC backends.
type VLCOptions struct {
	// Verbose is libVLC's log level (0 = quiet). PoC/evidence runs use 2.
	Verbose int
	// View binds the NSView drawable before the first play. It must run on
	// the AppKit main thread. nil is a headless backend (dummy/test mode).
	View func() uintptr
	// Args are extra libVLC options (e.g. "--vout=dummy" for headless tests).
	Args []string
}

// NewVLCFactory returns a Factory backed by libVLC.
func NewVLCFactory(opts VLCOptions) Factory {
	return func(path string, loop bool) (Backend, error) {
		inst, err := vlcmac.NewInstanceArgs(opts.Verbose, opts.Args)
		if err != nil {
			return nil, err
		}
		pl, err := vlcmac.NewPlayer(inst)
		if err != nil {
			inst.Release()
			return nil, err
		}
		if opts.View != nil {
			pl.SetNSObject(opts.View())
		}
		if err := pl.SetMedia(path, loop); err != nil {
			pl.Release()
			inst.Release()
			return nil, err
		}
		b := &vlcBackend{
			inst: inst,
			pl:   pl,
			done: make(chan struct{}),
			out:  make(chan Event, 128),
		}
		go b.pump()
		return b, nil
	}
}

type vlcBackend struct {
	inst *vlcmac.Instance
	pl   *vlcmac.Player

	once sync.Once
	done chan struct{}
	out  chan Event
}

func (b *vlcBackend) pump() {
	defer close(b.out)
	events := b.pl.Events()
	for {
		select {
		case <-b.done:
			return
		case ev := <-events:
			switch ev.Type {
			case vlcmac.EventPlaying:
				b.send(Event{Kind: EventPlaying})
			case vlcmac.EventPaused:
				b.send(Event{Kind: EventPaused})
			case vlcmac.EventStopped:
				b.send(Event{Kind: EventStopped})
			case vlcmac.EventEndReached:
				b.send(Event{Kind: EventEnded})
			case vlcmac.EventError:
				b.send(Event{Kind: EventFailed})
			case vlcmac.EventTimeChanged:
				b.send(Event{Kind: EventTime, Time: ev.Time})
			}
		}
	}
}

func (b *vlcBackend) send(ev Event) {
	select {
	case b.out <- ev:
	case <-b.done:
	default:
	}
}

func (b *vlcBackend) Play()                { b.pl.Play() }
func (b *vlcBackend) Pause()               { b.pl.Pause() }
func (b *vlcBackend) Stop()                { b.pl.Stop() }
func (b *vlcBackend) SeekTo(ms int64)      { b.pl.SetTime(ms) }
func (b *vlcBackend) SetRate(rate float64) { b.pl.SetRate(float32(rate)) }
func (b *vlcBackend) SetVolume(volume int) { b.pl.SetVolume(volume) }
func (b *vlcBackend) SetMute(muted bool)   { b.pl.SetMute(muted) }
func (b *vlcBackend) Time() int64          { return b.pl.Time() }
func (b *vlcBackend) Length() int64        { return b.pl.Length() }

func (b *vlcBackend) SetLoop(bool) {
	// Loop is fixed when the media is created (input-repeat); the engine
	// reloads to change it.
}

func (b *vlcBackend) State() BackendState {
	switch b.pl.State() {
	case vlcmac.StateOpening, vlcmac.StateBuffering:
		return BackendLoading
	case vlcmac.StatePlaying:
		return BackendPlaying
	case vlcmac.StatePaused:
		return BackendPaused
	case vlcmac.StateEnded:
		return BackendEnded
	case vlcmac.StateError:
		return BackendFailed
	default:
		return BackendIdle
	}
}

func (b *vlcBackend) VideoSize() (int, int, bool) { return b.pl.VideoSize() }
func (b *vlcBackend) Events() <-chan Event        { return b.out }

func mapTracks(in []vlcmac.Track) []Track {
	if len(in) == 0 {
		return nil
	}
	out := make([]Track, len(in))
	for i, t := range in {
		out[i] = Track{ID: t.ID, Name: t.Name}
	}
	return out
}

func (b *vlcBackend) AudioTracks() []Track    { return mapTracks(b.pl.AudioTracks()) }
func (b *vlcBackend) VideoTracks() []Track    { return mapTracks(b.pl.VideoTracks()) }
func (b *vlcBackend) SubtitleTracks() []Track { return mapTracks(b.pl.SubtitleTracks()) }
func (b *vlcBackend) AudioTrack() int         { return b.pl.AudioTrack() }
func (b *vlcBackend) VideoTrack() int         { return b.pl.VideoTrack() }
func (b *vlcBackend) SubtitleTrack() int      { return b.pl.SubtitleTrack() }
func (b *vlcBackend) SetAudioTrack(id int) bool {
	return b.pl.SetAudioTrack(id)
}
func (b *vlcBackend) SetVideoTrack(id int) bool {
	return b.pl.SetVideoTrack(id)
}
func (b *vlcBackend) SetSubtitleTrack(id int) bool {
	return b.pl.SetSubtitleTrack(id)
}

func (b *vlcBackend) Chapters() []Chapter {
	in := b.pl.Chapters()
	if len(in) == 0 {
		return nil
	}
	out := make([]Chapter, len(in))
	for i, c := range in {
		out[i] = Chapter{Index: c.Index, Name: c.Name}
	}
	return out
}
func (b *vlcBackend) Chapter() int          { return b.pl.Chapter() }
func (b *vlcBackend) SetChapter(i int) bool { return b.pl.SetChapter(i) }
func (b *vlcBackend) SetAspectRatio(aspect string) bool {
	return b.pl.SetAspectRatio(aspect)
}
func (b *vlcBackend) SetScale(scale float64) bool { return b.pl.SetScale(scale) }
func (b *vlcBackend) SetCrop(crop string) bool    { return b.pl.SetCrop(crop) }
func (b *vlcBackend) Screenshot(path string) error {
	return b.pl.Screenshot(path)
}
func (b *vlcBackend) AddSubtitle(path string) bool {
	return b.pl.AddSubtitle(path)
}
func (b *vlcBackend) AddAudio(path string) bool { return b.pl.AddAudio(path) }

func (b *vlcBackend) Close() {
	b.once.Do(func() {
		close(b.done)
		b.pl.Stop()
		b.pl.Release()
		b.inst.Release()
	})
}
