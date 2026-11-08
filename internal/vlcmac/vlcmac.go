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
#include <vlc/libvlc.h>
#include <vlc/libvlc_media.h>
#include <vlc/libvlc_events.h>
#include <vlc/libvlc_renderer_discoverer.h>
#include <vlc/libvlc_media_player.h>

extern void GoVLCEvent(void *player, int ev_type, long long time_ms);

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
	"os"
	"runtime"
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

// FrameCh is drained by the engine. Bounded: the oldest frame is dropped
// when full so libVLC threads never block.
var FrameCh = make(chan Frame, 8)

type videoCbState struct {
	chroma        string
	width, height int
	pitch         int
	planes        [3]unsafe.Pointer
	buf           []byte
	picture       unsafe.Pointer
	pinner        *runtime.Pinner
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
	if s.buf == nil {
		return
	}
	pix := make([]byte, len(s.buf))
	copy(pix, s.buf)
	f := Frame{W: s.width, H: s.height, Pix: pix, Stride: s.pitch, Chroma: s.chroma}
	// bounded: drop oldest rather than blocking the decode thread
	select {
	case FrameCh <- f:
	default:
		select {
		case <-FrameCh:
		default:
		}
		select {
		case FrameCh <- f:
		default:
		}
	}
}

// EnableCallbackVideo renders decoded frames into FrameCh (RV32). Call
// before SetMedia. Honest caveat: libVLC docs warn this path disables HW
// decode or forces a slow copy; callers must read decoder logs.
func (p *Player) EnableCallbackVideo() {
	if p == nil || p.mp == nil {
		return
	}
	opaque := unsafe.Pointer(p.mp)
	stateFor(opaque)
	C.set_video_callbacks(p.mp, opaque)
}

// Event is a libVLC media-player event worth knowing about.
type Event struct {
	Type int
	Time int64 // ms, for TimeChanged
}

// Frame is one decoded video frame copied out of libVLC buffers (RV32 path).
type Frame struct {
	W, H int
	Pix  []byte
	// Stride is bytes per row for planar/YUV; for RV32 it is W*4.
	Stride int
	// Chroma is the 4-byte chroma of the buffer (e.g. "RV32").
	Chroma string
}

// eventCh receives player events; engine drains it. Buffered so libVLC
// never blocks when the UI is slow.
var eventCh = make(chan Event, 256)

//export GoVLCEvent
func GoVLCEvent(player unsafe.Pointer, evType C.int, timeMs C.int64_t) {
	// Must never block or panic on libVLC threads.
	select {
	case eventCh <- Event{Type: int(evType), Time: int64(timeMs)}:
	default:
	}
}

// Instance is a libVLC instance (one per HW_SP player, mirroring upstream).
type Instance struct {
	inst *C.libvlc_instance_t
}

// NewInstance creates a libVLC instance with the upstream option set.
// verbose>0 adds --verbose=N for decode-path evidence (PoC only).
func NewInstance(verbose int) (*Instance, error) {
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
	if in == nil || in.inst == nil {
		return
	}
	C.libvlc_release(in.inst)
	in.inst = nil
}

// Player wraps one libvlc_media_player_t.
type Player struct {
	inst *Instance
	mp   *C.libvlc_media_player_t
	md   *C.libvlc_media_t
}

// NewPlayer creates a player. The caller must SetNSObject before SetMedia,
// mirroring upstream init_player order.
func NewPlayer(in *Instance) (*Player, error) {
	mp := C.libvlc_media_player_new(in.inst)
	if mp == nil {
		return nil, errors.New("vlcmac: media_player_new failed")
	}
	return &Player{inst: in, mp: mp}, nil
}

// SetNSObject binds an NSView address or 0 to unbind. Call on the process
// main thread for non-zero values.
//
// libvlc_media_player_set_nsobject is invoked through purego SyscallN so
// no Go<->C uintptr roundtrip exists (go vet clean; same style as MyGo).
func (p *Player) SetNSObject(view uintptr) {
	if p == nil || p.mp == nil {
		return
	}
	setNSObjectPurego(p.mp, view)
}

var setNSObjectFn uintptr

func setNSObjectPurego(mp *C.libvlc_media_player_t, view uintptr) {
	if setNSObjectFn == 0 {
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
	}
	_, _, _ = purego.SyscallN(setNSObjectFn, uintptr(unsafe.Pointer(mp)), view)
}

// SetMedia loads a file path and enables in-place file repeat loop
// (upstream INPUT_REPEAT_PASSES semantics: no demuxer restart).
func (p *Player) SetMedia(path string) error {
	if p == nil || p.mp == nil {
		return errors.New("vlcmac: nil player")
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	md := C.libvlc_media_new_path(p.inst.inst, cpath)
	if md == nil {
		return fmt.Errorf("vlcmac: media_new_path failed for %s", path)
	}
	opt := C.CString(":input-repeat=10000")
	C.libvlc_media_add_option(md, opt)
	C.free(unsafe.Pointer(opt))
	C.libvlc_media_player_set_media(p.mp, md)
	if p.md != nil {
		C.libvlc_media_release(p.md)
	}
	p.md = md
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
	return nil
}

func (p *Player) Play()             { C.libvlc_media_player_play(p.mp) }
func (p *Player) Pause()            { C.libvlc_media_player_pause(p.mp) }
func (p *Player) Stop()             { C.libvlc_media_player_stop(p.mp) }
func (p *Player) SetTime(ms int64)  { C.libvlc_media_player_set_time(p.mp, C.libvlc_time_t(ms)) }
func (p *Player) SetRate(r float32) { C.libvlc_media_player_set_rate(p.mp, C.float(r)) }
func (p *Player) SetVolume(v int)   { C.libvlc_audio_set_volume(p.mp, C.int(v)) }
func (p *Player) SetMute(m bool) {
	if m {
		C.libvlc_audio_set_mute(p.mp, 1)
	} else {
		C.libvlc_audio_set_mute(p.mp, 0)
	}
}
func (p *Player) Time() int64   { return int64(C.libvlc_media_player_get_time(p.mp)) }
func (p *Player) Length() int64 { return int64(C.libvlc_media_player_get_length(p.mp)) }
func (p *Player) State() int    { return int(C.libvlc_media_player_get_state(p.mp)) }

func (p *Player) VideoSize() (w, h int, ok bool) {
	var px, py C.unsigned
	if C.libvlc_video_get_size(p.mp, 0, &px, &py) != 0 {
		return 0, 0, false
	}
	return int(px), int(py), true
}

// Release runs the fixed teardown: unbind NSView -> release player ->
// release media. Call Stop first; detach the AppKit view (caller, main
// thread) between unbind and release.
func (p *Player) Release() {
	if p == nil || p.mp == nil {
		return
	}
	mp := p.mp
	md := p.md
	p.mp = nil
	p.md = nil
	C.libvlc_media_player_set_nsobject(mp, nil)
	C.libvlc_media_player_release(mp)
	if md != nil {
		C.libvlc_media_release(md)
	}
}

// Events returns the shared event channel (drained by engine).
func Events() <-chan Event { return eventCh }
