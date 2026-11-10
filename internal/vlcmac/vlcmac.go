// Package vlcmac is a thin cgo wrapper over libVLC (VLC 3.0.24 SDK).
//
// It owns no UI and no goroutine policy. Callers (internal/engine) own
// session/generation fencing, the bounded frame queue and main-thread
// posting. All exported methods are safe to call from any goroutine.
//
// Release order for each player is fixed and must not be reordered:
// stop -> detach events -> unbind NSView (set_nsobject nil) -> release
// player -> release media -> release instance. The AppKit-side detach
// (removeFromSuperview) belongs to the caller and runs on the main thread
// between unbind and release.
//
// Build prerequisites (see scripts/build.sh):
//
//	CGO_CFLAGS="-I/Applications/VLC.app/Contents/MacOS/include"
//	CGO_LDFLAGS="-L/Applications/VLC.app/Contents/MacOS/lib -lvlc -lvlccore -Wl,-rpath,/Applications/VLC.app/Contents/MacOS/lib -framework CoreFoundation -framework CoreServices -framework CFNetwork -framework CoreGraphics -framework QuartzCore -framework OpenGL -framework AppKit"
//
// For a non-default VLC, set VLC_APP and rebuild with those paths.
package vlcmac

/*
#cgo LDFLAGS: -lvlc
#include <stdlib.h>
#include <string.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdint.h>
#include <vlc/libvlc.h>
#include <vlc/libvlc_media.h>
#include <vlc/libvlc_events.h>
#include <vlc/libvlc_renderer_discoverer.h>
#include <vlc/libvlc_media_player.h>

extern float libvlc_media_player_get_fps(libvlc_media_player_t *p_mi);

extern void GoVLCEvent(void *player, int ev_type, int64_t time_ms);

static void call_event(const struct libvlc_event_t *ev, void *opaque) {
    long long t = 0;
    if (ev->type == libvlc_MediaPlayerTimeChanged)
        t = (long long)ev->u.media_player_time_changed.new_time;
    GoVLCEvent(opaque, ev->type, t);
}

extern void GoVLCLog(void *data, int level, char *ctx, char *fmt, char *msg);

static void call_log(void *data, int level, const libvlc_log_t *ctx, const char *fmt, va_list args) {
    char buf[1024];
    vsnprintf(buf, sizeof(buf), fmt, args);
    GoVLCLog(data, level, NULL, (char *)fmt, buf);
}

extern unsigned GoVLCSetup(void **opaque, char *chroma, unsigned *width, unsigned *height, unsigned *pitches, unsigned *lines);
extern void GoVLCleanup(void *opaque);
extern void *GoVLCLock(void *opaque, void *planes);
extern void GoVLCUnlock(void *opaque, void *picture, void *planes);
extern void GoVLCDisplay(void *opaque, void *picture);

static unsigned call_setup(void **opaque, char *chroma, unsigned *w, unsigned *h, unsigned *pitches, unsigned *lines) {
    return GoVLCSetup(opaque, chroma, w, h, pitches, lines);
}
static void call_cleanup(void *opaque) { GoVLCleanup(opaque); }
static void *call_lock(void *opaque, void **planes) { return GoVLCLock(opaque, planes); }
static void call_unlock(void *opaque, void *picture, void *const *planes) { GoVLCUnlock(opaque, picture, (void *)planes); }
static void call_display(void *opaque, void *picture) { GoVLCDisplay(opaque, picture); }

static int attach_player_events(libvlc_event_manager_t *mgr, void *player, int types[], size_t n) {
    for (size_t i = 0; i < n; i++) {
        int rc = libvlc_event_attach(mgr, types[i], call_event, player);
        if (rc != 0)
            return rc;
    }
    return 0;
}

static void set_video_callbacks(libvlc_media_player_t *mp, void *opaque) {
    libvlc_video_set_callbacks(mp, call_lock, call_unlock, call_display, opaque);
    libvlc_video_set_format_callbacks(mp, call_setup, call_cleanup);
}

static void set_log_callback(libvlc_instance_t *inst) {
    libvlc_log_set(inst, call_log, NULL);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// LogSink, when set, receives libVLC log lines (decoder evidence).
var LogSink func(level int, msg string)

//export GoVLCLog
func GoVLCLog(data unsafe.Pointer, level C.int, ctx *C.char, format *C.char, msg *C.char) {
	if LogSink == nil {
		return
	}
	LogSink(int(level), C.GoString(msg))
}

type videoCbState struct {
	chroma        string
	width, height int
	pitch         int
	planes        [3]unsafe.Pointer
	buf           []byte
	picture       unsafe.Pointer
	pinner        *runtime.Pinner
	owner         *Player
}

var cbMu sync.Mutex
var cbStates = map[unsafe.Pointer]*videoCbState{}

func stateFor(opaque unsafe.Pointer) *videoCbState {
	cbMu.Lock()
	defer cbMu.Unlock()
	s, ok := cbStates[opaque]
	if !ok {
		s = &videoCbState{}
		cbStates[opaque] = s
	}
	return s
}

//export GoVLCSetup
func GoVLCSetup(opaque *unsafe.Pointer, chroma *C.char, width *C.unsigned, height *C.unsigned, pitches *C.unsigned, lines *C.unsigned) C.unsigned {
	// Accept the decoder's native size in RV32 (RGB) for texture upload.
	s := stateFor(unsafe.Pointer(*opaque))
	w := int(*width)
	h := int(*height)
	if w <= 0 || h <= 0 {
		return 0
	}
	pitch := w * 4
	*pitches = C.unsigned(pitch)
	*lines = C.unsigned(h)
	// chroma: first byte 'R' keeps fourcc handling in libVLC; force RV32.
	// force RV32 fourcc into all four bytes (C string "RV32" + NUL)
	bs := (*[5]byte)(unsafe.Pointer(chroma))
	bs[0], bs[1], bs[2], bs[3], bs[4] = 'R', 'V', '3', '2', 0
	buf := make([]byte, pitch*h)
	s.chroma = "RV32"
	s.width, s.height, s.pitch = w, h, pitch
	s.buf = buf
	if s.pinner != nil {
		s.pinner.Unpin()
	}
	s.pinner = &runtime.Pinner{}
	s.pinner.Pin(&s.buf[0])
	return 1
}

//export GoVLCleanup
func GoVLCleanup(opaque unsafe.Pointer) {
	cbMu.Lock()
	s := cbStates[opaque]
	delete(cbStates, opaque)
	cbMu.Unlock()
	if s != nil && s.pinner != nil {
		s.pinner.Unpin()
	}
}

//export GoVLCLock
func GoVLCLock(opaque unsafe.Pointer, planes unsafe.Pointer) unsafe.Pointer {
	s := stateFor(opaque)
	if s.buf == nil {
		return nil
	}
	planesArr := (*[3]unsafe.Pointer)(planes)
	planesArr[0] = unsafe.Pointer(&s.buf[0])
	return unsafe.Pointer(&s.buf[0])
}

//export GoVLCUnlock
func GoVLCUnlock(opaque unsafe.Pointer, picture unsafe.Pointer, planes unsafe.Pointer) {}

//export GoVLCDisplay
func GoVLCDisplay(opaque unsafe.Pointer, picture unsafe.Pointer) {
	s := stateFor(opaque)
	if s.buf == nil || s.owner == nil {
		return
	}
	pix := make([]byte, len(s.buf))
	copy(pix, s.buf)
	f := Frame{W: s.width, H: s.height, Pix: pix, Stride: s.pitch, Chroma: s.chroma}
	// bounded: drop oldest rather than blocking the decode thread
	select {
	case s.owner.frames <- f:
	default:
		select {
		case <-s.owner.frames:
		default:
		}
		select {
		case s.owner.frames <- f:
		default:
		}
	}
}

// EnableCallbackVideo renders decoded frames into FrameCh (RV32). Call
// before SetMedia. Honest caveat: libVLC docs warn this path disables HW
// decode or forces a slow copy; callers must read decoder logs.
func (p *Player) EnableCallbackVideo() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released || p.mp == nil {
		return
	}
	opaque := unsafe.Pointer(p.mp)
	st := stateFor(opaque)
	st.owner = p
	C.set_video_callbacks(p.mp, opaque)
}

// Event is a libVLC media-player event worth knowing about.
type Event struct {
	Type int
	Time int64 // ms, for TimeChanged
}

// Player event types (libvlc_event_e).
const (
	EventMediaChanged = int(C.libvlc_MediaPlayerMediaChanged)
	EventPlaying      = int(C.libvlc_MediaPlayerPlaying)
	EventPaused       = int(C.libvlc_MediaPlayerPaused)
	EventStopped      = int(C.libvlc_MediaPlayerStopped)
	EventEndReached   = int(C.libvlc_MediaPlayerEndReached)
	EventError        = int(C.libvlc_MediaPlayerEncounteredError)
	EventTimeChanged  = int(C.libvlc_MediaPlayerTimeChanged)
	EventVout         = int(C.libvlc_MediaPlayerVout)
)

// libvlc_state_t values.
const (
	StateNothingSpecial = 0
	StateOpening        = 1
	StateBuffering      = 2
	StatePlaying        = 3
	StatePaused         = 4
	StateStopped        = 5
	StateEnded          = 6
	StateError          = 7
)

// Frame is one decoded video frame copied out of libVLC buffers (RV32 path).
type Frame struct {
	W, H int
	Pix  []byte
	// Stride is bytes per row for planar/YUV; for RV32 it is W*4.
	Stride int
	// Chroma is the 4-byte chroma of the buffer (e.g. "RV32").
	Chroma string
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

// registry routes libVLC callbacks (which carry the player pointer) to the
// owning Player. Guarded by regMu; callbacks never block.
var (
	regMu    sync.RWMutex
	registry = map[unsafe.Pointer]*Player{}
)

//export GoVLCEvent
func GoVLCEvent(player unsafe.Pointer, evType C.int, timeMs C.int64_t) {
	regMu.RLock()
	p := registry[player]
	regMu.RUnlock()
	if p == nil {
		return
	}
	select {
	case p.events <- Event{Type: int(evType), Time: int64(timeMs)}:
	default:
	}
}

// Instance is a libVLC instance (one per HW_SP player, mirroring upstream).
type Instance struct {
	mu       sync.Mutex
	inst     *C.libvlc_instance_t
	released bool
}

// NewInstance creates a libVLC instance with the upstream option set.
// verbose>0 adds --verbose=N for decode-path evidence (PoC only).
func NewInstance(verbose int) (*Instance, error) {
	return NewInstanceArgs(verbose, nil)
}

// NewInstanceArgs is NewInstance with extra libVLC options (e.g.
// "--vout=dummy" for headless integration tests).
func NewInstanceArgs(verbose int, extra []string) (*Instance, error) {
	l, err := Discover()
	if err != nil {
		return nil, err
	}
	// python-vlc does the same (vlc.py:677): plugin discovery is via env,
	// not a libvlc_new argument (VLC 3.0.24 has no --plugin-path option).
	os.Setenv("VLC_PLUGIN_PATH", l.PluginsDir)
	args := []string{
		"--quiet", "--no-disable-screensaver", "--no-sub-autodetect-file",
		"--no-lua", "--no-osd", "--no-snapshot-preview", "--no-interact",
		"--no-keyboard-events", "--no-mouse-events",
	}
	if verbose > 0 {
		args = append(args, fmt.Sprintf("--verbose=%d", verbose))
	}
	args = append(args, extra...)
	argv := make([]*C.char, len(args))
	for i, a := range args {
		argv[i] = C.CString(a)
		defer C.free(unsafe.Pointer(argv[i]))
	}
	inst := C.libvlc_new(C.int(len(args)), &argv[0])
	if inst == nil {
		return nil, errors.New("vlcmac: libvlc_new failed (check VLC_APP=" + l.AppDir + ")")
	}
	C.set_log_callback(inst)
	return &Instance{inst: inst}, nil
}

// Release frees the instance. Call after each player's Release.
func (in *Instance) Release() {
	if in == nil {
		return
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.released || in.inst == nil {
		return
	}
	in.released = true
	C.libvlc_release(in.inst)
	in.inst = nil
}

// Player wraps one libvlc_media_player_t.
type Player struct {
	mu       sync.Mutex
	inst     *Instance
	mp       *C.libvlc_media_player_t
	md       *C.libvlc_media_t
	attached bool
	released bool
	events   chan Event
	frames   chan Frame
}

// NewPlayer creates a player. The caller must SetNSObject before SetMedia,
// mirroring upstream init_player order.
func NewPlayer(in *Instance) (*Player, error) {
	if in == nil {
		return nil, errors.New("vlcmac: nil instance")
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.released || in.inst == nil {
		return nil, errors.New("vlcmac: instance released")
	}
	mp := C.libvlc_media_player_new(in.inst)
	if mp == nil {
		return nil, errors.New("vlcmac: media_player_new failed")
	}
	p := &Player{inst: in, mp: mp, events: make(chan Event, 128), frames: make(chan Frame, 8)}
	regMu.Lock()
	registry[unsafe.Pointer(mp)] = p
	regMu.Unlock()
	return p, nil
}

// SetNSObject binds an NSView address or 0 to unbind. Call on the process
// main thread for non-zero values.
//
// libvlc_media_player_set_nsobject is invoked through purego SyscallN so
// no Go<->C uintptr roundtrip exists (go vet clean; same style as MyGo).
func (p *Player) SetNSObject(view uintptr) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released || p.mp == nil {
		return
	}
	setNSObjectPurego(p.mp, view)
}

var setNSObjectFn uintptr
var setNSObjectOnce sync.Once

func setNSObjectPurego(mp *C.libvlc_media_player_t, view uintptr) {
	setNSObjectOnce.Do(func() {
		layout, _ := Discover()
		path := "/Applications/VLC.app/Contents/MacOS/lib/libvlc.dylib"
		if layout != nil {
			path = layout.LibVLC
		}
		h, err := purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			panic(err)
		}
		setNSObjectFn, err = purego.Dlsym(h, "libvlc_media_player_set_nsobject")
		if err != nil {
			panic(err)
		}
	})
	_, _, _ = purego.SyscallN(setNSObjectFn, uintptr(unsafe.Pointer(mp)), view)
}

// SetMedia loads a file path. With loop, it enables in-place file repeat
// (upstream INPUT_REPEAT_PASSES semantics: no demuxer restart).
func (p *Player) SetMedia(path string, loop bool) error {
	if p == nil {
		return errors.New("vlcmac: nil player")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released || p.mp == nil {
		return errors.New("vlcmac: released")
	}
	isURL := strings.Contains(path, "://")
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var md *C.libvlc_media_t
	if isURL {
		md = C.libvlc_media_new_location(p.inst.inst, cpath)
	} else {
		md = C.libvlc_media_new_path(p.inst.inst, cpath)
	}
	if md == nil {
		return fmt.Errorf("vlcmac: media_new failed for %s", path)
	}
	if loop {
		opt := C.CString(":input-repeat=10000")
		C.libvlc_media_add_option(md, opt)
		C.free(unsafe.Pointer(opt))
	}
	C.libvlc_media_player_set_media(p.mp, md)
	if p.md != nil {
		C.libvlc_media_release(p.md)
	}
	p.md = md
	if !p.attached {
		mgr := C.libvlc_media_player_event_manager(p.mp)
		types := []C.int{C.libvlc_MediaPlayerTimeChanged, C.libvlc_MediaPlayerPlaying,
			C.libvlc_MediaPlayerPaused, C.libvlc_MediaPlayerStopped,
			C.libvlc_MediaPlayerEndReached, C.libvlc_MediaPlayerEncounteredError,
			C.libvlc_MediaPlayerMediaChanged, C.libvlc_MediaPlayerVout}
		typesC := make([]C.int, len(types))
		copy(typesC, types)
		if rc := C.attach_player_events(mgr, unsafe.Pointer(p.mp), &typesC[0], C.size_t(len(typesC))); rc != 0 {
			return fmt.Errorf("vlcmac: event attach failed rc=%d", int(rc))
		}
		p.attached = true
	}
	return nil
}

// SetTransform applies libVLC's transform filter to the current media.
// Call after SetMedia and before Play; "none" removes the filter.
func (p *Player) SetTransform(transform string) bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released || p.md == nil {
		return false
	}
	if transform == "" || transform == "none" {
		return true
	}
	filter := C.CString(":video-filter=transform")
	defer C.free(unsafe.Pointer(filter))
	C.libvlc_media_add_option(p.md, filter)
	kind := C.CString(":transform-type=" + transform)
	defer C.free(unsafe.Pointer(kind))
	C.libvlc_media_add_option(p.md, kind)
	return true
}

func (p *Player) with(fn func(mp *C.libvlc_media_player_t)) bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released || p.mp == nil {
		return false
	}
	fn(p.mp)
	return true
}

func (p *Player) Play() { p.with(func(mp *C.libvlc_media_player_t) { C.libvlc_media_player_play(mp) }) }
func (p *Player) Pause() {
	p.with(func(mp *C.libvlc_media_player_t) { C.libvlc_media_player_pause(mp) })
}

// SetPaused pauses or resumes without toggling.
func (p *Player) SetPaused(paused bool) {
	p.with(func(mp *C.libvlc_media_player_t) {
		if paused {
			C.libvlc_media_player_set_pause(mp, 1)
		} else {
			C.libvlc_media_player_set_pause(mp, 0)
		}
	})
}

// NextFrame pauses, then steps one decoded frame forward.
func (p *Player) NextFrame() {
	p.with(func(mp *C.libvlc_media_player_t) {
		C.libvlc_media_player_set_pause(mp, 1)
		C.libvlc_media_player_next_frame(mp)
	})
}

// FPS is the playing media rate, or 0 when libVLC does not know it.
func (p *Player) FPS() float64 {
	var fps float64
	p.with(func(mp *C.libvlc_media_player_t) {
		fps = float64(C.libvlc_media_player_get_fps(mp))
	})
	return fps
}
func (p *Player) Stop() { p.with(func(mp *C.libvlc_media_player_t) { C.libvlc_media_player_stop(mp) }) }
func (p *Player) SetTime(ms int64) {
	p.with(func(mp *C.libvlc_media_player_t) { C.libvlc_media_player_set_time(mp, C.libvlc_time_t(ms)) })
}
func (p *Player) SetRate(r float32) {
	p.with(func(mp *C.libvlc_media_player_t) { C.libvlc_media_player_set_rate(mp, C.float(r)) })
}
func (p *Player) SetVolume(v int) {
	p.with(func(mp *C.libvlc_media_player_t) { C.libvlc_audio_set_volume(mp, C.int(v)) })
}

// SetSubtitleDelay offsets subtitles by milliseconds. libVLC takes microseconds.
func (p *Player) SetSubtitleDelay(ms int64) {
	p.with(func(mp *C.libvlc_media_player_t) {
		C.libvlc_video_set_spu_delay(mp, C.int64_t(ms*1000))
	})
}

// SetAudioDelay offsets audio by milliseconds. libVLC takes microseconds.
func (p *Player) SetAudioDelay(ms int64) {
	p.with(func(mp *C.libvlc_media_player_t) {
		C.libvlc_audio_set_delay(mp, C.int64_t(ms*1000))
	})
}

func (p *Player) SetMute(m bool) {
	p.with(func(mp *C.libvlc_media_player_t) {
		if m {
			C.libvlc_audio_set_mute(mp, 1)
		} else {
			C.libvlc_audio_set_mute(mp, 0)
		}
	})
}
func (p *Player) Time() (ms int64) {
	p.with(func(mp *C.libvlc_media_player_t) { ms = int64(C.libvlc_media_player_get_time(mp)) })
	return ms
}
func (p *Player) Length() (ms int64) {
	p.with(func(mp *C.libvlc_media_player_t) { ms = int64(C.libvlc_media_player_get_length(mp)) })
	return ms
}
func (p *Player) State() (state int) {
	p.with(func(mp *C.libvlc_media_player_t) { state = int(C.libvlc_media_player_get_state(mp)) })
	return state
}

func (p *Player) VideoSize() (w, h int, ok bool) {
	p.with(func(mp *C.libvlc_media_player_t) {
		var px, py C.unsigned
		if C.libvlc_video_get_size(mp, 0, &px, &py) == 0 {
			w, h, ok = int(px), int(py), true
		}
	})
	return w, h, ok
}

func trackList(head *C.libvlc_track_description_t) []Track {
	if head == nil {
		return nil
	}
	defer C.libvlc_track_description_list_release(head)
	var out []Track
	for d := head; d != nil; d = d.p_next {
		name := ""
		if d.psz_name != nil {
			name = C.GoString(d.psz_name)
		}
		out = append(out, Track{ID: int(d.i_id), Name: name})
	}
	return out
}

func (p *Player) AudioTracks() (out []Track) {
	p.with(func(mp *C.libvlc_media_player_t) {
		out = trackList(C.libvlc_audio_get_track_description(mp))
	})
	return out
}

func (p *Player) VideoTracks() (out []Track) {
	p.with(func(mp *C.libvlc_media_player_t) {
		out = trackList(C.libvlc_video_get_track_description(mp))
	})
	return out
}

func (p *Player) SubtitleTracks() (out []Track) {
	p.with(func(mp *C.libvlc_media_player_t) {
		out = trackList(C.libvlc_video_get_spu_description(mp))
	})
	return out
}

func (p *Player) SetAudioTrack(id int) (ok bool) {
	p.with(func(mp *C.libvlc_media_player_t) {
		ok = C.libvlc_audio_set_track(mp, C.int(id)) == 0
	})
	return ok
}

func (p *Player) SetVideoTrack(id int) (ok bool) {
	p.with(func(mp *C.libvlc_media_player_t) {
		ok = C.libvlc_video_set_track(mp, C.int(id)) == 0
	})
	return ok
}

func (p *Player) SetSubtitleTrack(id int) (ok bool) {
	p.with(func(mp *C.libvlc_media_player_t) {
		ok = C.libvlc_video_set_spu(mp, C.int(id)) == 0
	})
	return ok
}

func (p *Player) AudioTrack() (id int) {
	p.with(func(mp *C.libvlc_media_player_t) { id = int(C.libvlc_audio_get_track(mp)) })
	return id
}

func (p *Player) VideoTrack() (id int) {
	p.with(func(mp *C.libvlc_media_player_t) { id = int(C.libvlc_video_get_track(mp)) })
	return id
}

func (p *Player) SubtitleTrack() (id int) {
	p.with(func(mp *C.libvlc_media_player_t) { id = int(C.libvlc_video_get_spu(mp)) })
	return id
}

func (p *Player) Chapters() (out []Chapter) {
	p.with(func(mp *C.libvlc_media_player_t) {
		var descs **C.struct_libvlc_chapter_description_t
		n := int(C.libvlc_media_player_get_full_chapter_descriptions(mp, -1, &descs))
		if n <= 0 || descs == nil || *descs == nil {
			return
		}
		defer C.libvlc_chapter_descriptions_release(descs, C.unsigned(n))
		// descs points at an array of chapter pointers.
		arr := unsafe.Slice(descs, n)
		for i := range arr {
			name := ""
			if arr[i] != nil && arr[i].psz_name != nil {
				name = strings.TrimSpace(C.GoString(arr[i].psz_name))
			}
			out = append(out, Chapter{Index: i, Name: name})
		}
	})
	return out
}

func (p *Player) SetChapter(i int) (ok bool) {
	p.with(func(mp *C.libvlc_media_player_t) {
		C.libvlc_media_player_set_chapter(mp, C.int(i))
		ok = true
	})
	return ok
}

func (p *Player) Chapter() (i int) {
	p.with(func(mp *C.libvlc_media_player_t) { i = int(C.libvlc_media_player_get_chapter(mp)) })
	return i
}

// SetAspectRatio sets the display aspect ("16:9"), or "" for the source's.
func (p *Player) SetAspectRatio(aspect string) (ok bool) {
	p.with(func(mp *C.libvlc_media_player_t) {
		if aspect == "" {
			C.libvlc_video_set_aspect_ratio(mp, nil)
			ok = true
			return
		}
		c := C.CString(aspect)
		defer C.free(unsafe.Pointer(c))
		C.libvlc_video_set_aspect_ratio(mp, c)
		ok = true
	})
	return ok
}

// SetScale sets the video scale (0 = fit to the drawable).
func (p *Player) SetScale(scale float64) (ok bool) {
	p.with(func(mp *C.libvlc_media_player_t) {
		C.libvlc_video_set_scale(mp, C.float(scale))
		ok = true
	})
	return ok
}

// SetCrop sets crop geometry ("16:9"), or "" for none.
func (p *Player) SetCrop(crop string) (ok bool) {
	p.with(func(mp *C.libvlc_media_player_t) {
		if crop == "" {
			C.libvlc_video_set_crop_geometry(mp, nil)
			ok = true
			return
		}
		c := C.CString(crop)
		defer C.free(unsafe.Pointer(c))
		C.libvlc_video_set_crop_geometry(mp, c)
		ok = true
	})
	return ok
}

// AddSubtitle attaches a subtitle file to the current media.
func fileMRL(path string) string {
	if strings.HasPrefix(path, "file://") {
		return path
	}
	if !strings.HasPrefix(path, "/") && !strings.Contains(path, "://") {
		abs, err := filepath.Abs(path)
		if err == nil {
			path = abs
		}
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

// AddSubtitle attaches a subtitle file to the current media.
func (p *Player) AddSubtitle(path string) (ok bool) {
	uri := fileMRL(path)
	p.with(func(mp *C.libvlc_media_player_t) {
		c := C.CString(uri)
		defer C.free(unsafe.Pointer(c))
		ok = C.libvlc_media_player_add_slave(mp, C.libvlc_media_slave_type_subtitle, c, false) == 0
	})
	return ok
}

// AddAudio attaches an audio file to the current media.
func (p *Player) AddAudio(path string) (ok bool) {
	uri := fileMRL(path)
	p.with(func(mp *C.libvlc_media_player_t) {
		c := C.CString(uri)
		defer C.free(unsafe.Pointer(c))
		ok = C.libvlc_media_player_add_slave(mp, C.libvlc_media_slave_type_audio, c, false) == 0
	})
	return ok
}

// Screenshot writes the current video frame to path.
func (p *Player) Screenshot(path string) (err error) {
	if path == "" {
		return errors.New("vlcmac: empty screenshot path")
	}
	if !p.with(func(mp *C.libvlc_media_player_t) {
		c := C.CString(path)
		defer C.free(unsafe.Pointer(c))
		if C.libvlc_video_take_snapshot(mp, 0, c, 0, 0) != 0 {
			err = errors.New("vlcmac: video not found for snapshot")
		}
	}) {
		return errors.New("vlcmac: released player")
	}
	return err
}

// Release runs the fixed teardown: unbind NSView -> release player ->
// release media. Call Stop first; detach the AppKit view (caller, main
// thread) between unbind and release.
func (p *Player) Release() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released || p.mp == nil {
		return
	}
	p.released = true
	mp := p.mp
	md := p.md
	p.mp = nil
	p.md = nil
	regMu.Lock()
	delete(registry, unsafe.Pointer(mp))
	regMu.Unlock()
	setNSObjectPurego(mp, 0)
	C.libvlc_media_player_release(mp)
	if md != nil {
		C.libvlc_media_release(md)
	}
}

// Events returns this player's event stream. It stops receiving once the
// player is released (the channel is not closed: in-flight libVLC
// callbacks may still send).
func (p *Player) Events() <-chan Event { return p.events }

// Frames returns this player's callback-frame stream (Attempt B only).
func (p *Player) Frames() <-chan Frame { return p.frames }
