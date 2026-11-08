//go:build darwin

// Command poc-hw is Attempt A: isolated hardware-path proof.
//
// One AppKit window, 4 NSViews, 4 libVLC players bound with
// set_nsobject (mirroring Qt QMacCocoaViewContainer + set_nsobject in
// upstream video_frame_vlc_hw_sp.py). No MyGo, no callback frames.
// AppKit via purego (as in MyGo internal/darwin); libVLC via internal/vlcmac.
//
// Usage: scripts/build.sh poc-hw && ./bin/poc-hw [--verbose=N] [--out=DIR]
// Exits 0 only after stop -> unbind -> detach -> release.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"gridplayer-go/internal/vlcmac"
)

type id = objc.ID

type NSPoint struct{ X, Y float64 }
type NSSize struct{ Width, Height float64 }
type NSRect struct {
	Origin NSPoint
	Size   NSSize
}

var (
	msgInitRect   func(obj id, sel objc.SEL, r NSRect) id
	msgInitWindow func(obj id, sel objc.SEL, r NSRect, style uint, backing uint, deferFlag bool) id
	msgSend       func(obj id, sel objc.SEL, args ...any) id
	poolPush      func() uintptr
	poolPop       func(pool uintptr)
)

func loadAppKit() {
	open := func(path string) uintptr {
		h, err := purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			panic(err)
		}
		return h
	}
	objcLib := open("/usr/lib/libobjc.A.dylib")
	open("/System/Library/Frameworks/Foundation.framework/Foundation")
	open("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
	open("/System/Library/Frameworks/AppKit.framework/AppKit")
	p, _ := purego.Dlsym(objcLib, "objc_msgSend")
	purego.RegisterFunc(&msgSend, p)
	purego.RegisterFunc(&msgInitRect, p)
	purego.RegisterFunc(&msgInitWindow, p)
	pp, _ := purego.Dlsym(objcLib, "objc_autoreleasePoolPush")
	purego.RegisterFunc(&poolPush, pp)
	qq, _ := purego.Dlsym(objcLib, "objc_autoreleasePoolPop")
	purego.RegisterFunc(&poolPop, qq)
}

func sel(name string) objc.SEL { return objc.RegisterName(name) }

func send(obj id, name string, args ...any) id { return msgSend(obj, sel(name), args...) }

func class(name string) id { return id(objc.GetClass(name)) }

func nsString(s string) id {
	return send(class("NSString"), "stringWithUTF8String:", s)
}

func pooled() func() {
	runtime.LockOSThread()
	p := poolPush()
	return func() {
		poolPop(p)
		runtime.UnlockOSThread()
	}
}

// runFor pumps the main runloop for d seconds so AppKit draws.
func runFor(d float64) {
	rl := send(class("NSRunLoop"), "mainRunLoop")
	date := send(class("NSDate"), "dateWithTimeIntervalSinceNow:", d)
	send(rl, "runUntilDate:", date)
}

type cell struct {
	view id
	inst *vlcmac.Instance
	pl   *vlcmac.Player
}

func main() {
	runtime.LockOSThread()
	verbose := flag.Int("verbose", 0, "libVLC verbose level (0=quiet)")
	out := flag.String("out", "testdata/fixtures", "fixture dir")
	logPath := flag.String("log", "testdata/poc-hw-vlc.log", "libVLC log file")
	flag.Parse()
	if lf, err := os.Create(*logPath); err == nil {
		vlcmac.LogSink = func(level int, msg string) {
			fmt.Fprintf(lf, "[vlc %d] %s\n", level, msg)
		}
		fmt.Println("poc-hw: log ->", *logPath)
	}
	loadAppKit()
	defer pooled()()

	app := send(class("NSApplication"), "sharedApplication")
	send(app, "setActivationPolicy:", 0) // Regular
	winFrame := NSRect{Origin: NSPoint{X: 100, Y: 100}, Size: NSSize{Width: 1280, Height: 720}}
	win := msgInitWindow(send(class("NSWindow"), "alloc"), sel("initWithContentRect:styleMask:backing:defer:"),
		winFrame, 15, 2, true)
	send(win, "setTitle:", nsString("poc-hw"))
	content := send(win, "contentView")
	// 2x2 cells, bottom-left origin: row 0 at top.
	views := make([]id, 0, 4)
	for i := 0; i < 4; i++ {
		x := float64((i % 2) * 640)
		y := float64(360 - (i/2)*360)
		v := msgInitRect(send(class("NSView"), "alloc"), sel("initWithFrame:"),
			NSRect{Origin: NSPoint{X: x, Y: y}, Size: NSSize{Width: 640, Height: 360}})
		send(v, "setWantsLayer:", 1)
		send(content, "addSubview:", v)
		views = append(views, v)
	}
	send(win, "makeKeyAndOrderFront:", id(0))
	send(app, "activateIgnoringOtherApps:", true)

	paths := []string{
		filepath.Join(*out, "plane-a.mp4"),
		filepath.Join(*out, "plane-b.mp4"),
		filepath.Join(*out, "plane-c.mp4"),
		filepath.Join(*out, "plane-d.mp4"),
	}
	var cells []cell
	for i, path := range paths {
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(os.Stderr, "poc-hw: missing fixture %s: %v\n", path, err)
			os.Exit(2)
		}
		inst, err := vlcmac.NewInstance(*verbose)
		if err != nil {
			fmt.Fprintf(os.Stderr, "poc-hw: %v\n", err)
			os.Exit(2)
		}
		pl, err := vlcmac.NewPlayer(inst)
		if err != nil {
			fmt.Fprintf(os.Stderr, "poc-hw: %v\n", err)
			os.Exit(2)
		}
		pl.SetNSObject(uintptr(views[i]))
		if err := pl.SetMedia(path, true); err != nil {
			fmt.Fprintf(os.Stderr, "poc-hw: %v\n", err)
			os.Exit(2)
		}
		pl.Play()
		cells = append(cells, cell{view: views[i], inst: inst, pl: pl})
	}
	fmt.Println("poc-hw: 4 players started, waiting 5s for decode...")
	for d := 0; d < 50; d++ {
		runFor(0.1)
	}
	failed := false
	for i, c := range cells {
		w, h, ok := c.pl.VideoSize()
		fmt.Printf("poc-hw: cell %d time=%dms length=%dms state=%d %dx%d ok=%v\n",
			i, c.pl.Time(), c.pl.Length(), c.pl.State(), w, h, ok)
		if c.pl.Length() <= 0 || w == 0 || h == 0 {
			failed = true
		}
	}
	if len(cells) > 0 {
		cells[0].pl.Pause()
		time.Sleep(300 * time.Millisecond)
		fmt.Printf("poc-hw: pause time=%dms\n", cells[0].pl.Time())
		cells[0].pl.SetTime(3000)
		time.Sleep(600 * time.Millisecond)
		cells[0].pl.Play()
		time.Sleep(800 * time.Millisecond)
		fmt.Printf("poc-hw: after seek 3000ms+resume time=%dms (want >=2500)\n", cells[0].pl.Time())
		if cells[0].pl.Time() < 2500 {
			fmt.Printf("poc-hw: SEEK FAILED time=%d\n", cells[0].pl.Time())
			failed = true
		}
	}
	fmt.Println("poc-hw: tearing down (stop->unbind->detach->release)...")
	for _, c := range cells {
		c.pl.Stop()
		time.Sleep(100 * time.Millisecond)
		c.pl.Release()
	}
	for _, c := range cells {
		c.inst.Release()
	}
	for _, v := range views {
		send(v, "removeFromSuperview")
	}
	send(win, "close")
	if failed {
		fmt.Println("poc-hw: FAIL")
		os.Exit(1)
	}
	fmt.Println("poc-hw: PASS")
}
