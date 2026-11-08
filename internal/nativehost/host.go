// Package nativehost hosts foreign NSViews (libVLC macosx vout drawables)
// under a MyGo native-UI window's surface, using only public app APIs:
// *mygo.Window.NativeHandle (NSWindow*) + ui Painter.Hole.
//
// Layout mirrors MyGo's own webview hosting (internal/darwin/webview.go):
// our NSView is added to the window's contentView *below* the
// MyGoSurfaceView; the surface paints a transparent Hole over our rect so
// VLC shows through, and content painted after the Hole composites above.
// The surface keeps pointer input (it only forwards through holes where
// it placed webviews, which we do not use), so overlay controls stay
// clickable — VLC never needs the pointer for playback control.
//
// Coordinates: ui DIPs are top-left origin, 1 DIP = 1 point. AppKit frames
// are bottom-left origin points: macY = H - dipY - dipH, macX = dipX.
//
// Every function requires the process main thread (AppKit rule); callers
// use mygo.RunOnMain or run inside a ui view builder / Draw callback.
package nativehost

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

type id = objc.ID

// NSPoint, NSSize, NSRect mirror CoreGraphics structs (64-bit).
type NSPoint struct{ X, Y float64 }
type NSSize struct{ Width, Height float64 }
type NSRect struct {
	Origin NSPoint
	Size   NSSize
}

// DIP is a rect in ui top-left DIPs.
type DIP struct{ X, Y, W, H float64 }

var (
	once       sync.Once
	msgSend    func(obj id, sel objc.SEL, args ...any) id
	msgInitRec func(obj id, sel objc.SEL, r NSRect) id
	msgSetRec  func(obj id, sel objc.SEL, r NSRect)
	msgRect    func(obj id, sel objc.SEL) NSRect
)

// nsWindowBelow is NSWindowBelow (-1), for addSubview:positioned:relativeTo:.
const nsWindowBelow = uintptr(^uint(0))

func initAppKit() {
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
	p, err := purego.Dlsym(objcLib, "objc_msgSend")
	if err != nil {
		panic(err)
	}
	stret := p
	if runtime.GOARCH == "amd64" {
		stret, _ = purego.Dlsym(objcLib, "objc_msgSend_stret")
	}
	purego.RegisterFunc(&msgSend, p)
	purego.RegisterFunc(&msgInitRec, p)
	purego.RegisterFunc(&msgSetRec, p)
	purego.RegisterFunc(&msgRect, stret)
}

func sel(name string) objc.SEL { return objc.RegisterName(name) }

func class(name string) id { return id(objc.GetClass(name)) }

func send(obj id, name string, args ...any) id { return msgSend(obj, sel(name), args...) }

func nsString(s string) id { return send(class("NSString"), "stringWithUTF8String:", s) }

// goString reads an NSString* (class names are ASCII, so byte length works).
func goString(str id) string {
	if str == 0 {
		return ""
	}
	// No typed wrapper for NSString length; read the C string with a
	// bounded scan (class names are short).
	ptr := uintptr(send(str, "UTF8String"))
	if ptr == 0 {
		return ""
	}
	// vet-clean idiom (as in MyGo): copy the address into a variable and
	// take its address so the conversion keeps provenance.
	a := ptr
	cptr := *(*unsafe.Pointer)(unsafe.Pointer(&a))
	buf := unsafe.Slice((*byte)(cptr), 256)
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return string(buf[:n])
}

// Resolve maps an NSWindow* (Window.NativeHandle) to its contentView and
// MyGoSurfaceView. ok is false when the hierarchies do not match.
func Resolve(nswindow uintptr) (contentView, surfaceView uintptr, height float64, ok bool) {
	once.Do(initAppKit)
	win := id(nswindow)
	cv := send(win, "contentView")
	if cv == 0 {
		return 0, 0, 0, false
	}
	subs := send(cv, "subviews")
	enum := send(subs, "objectEnumerator")
	for {
		v := send(enum, "nextObject")
		if v == 0 {
			break
		}
		if goString(send(v, "className")) == "MyGoSurfaceView" {
			h := msgRect(cv, sel("bounds")).Size.Height
			return uintptr(cv), uintptr(v), h, true
		}
	}
	return uintptr(cv), 0, 0, false
}

// ToMac converts a top-left DIP rect to bottom-left AppKit points.
func ToMac(r DIP, height float64) NSRect {
	return NSRect{
		Origin: NSPoint{X: r.X, Y: height - r.Y - r.H},
		Size:   NSSize{Width: r.W, Height: r.H},
	}
}

// NewView creates a layer-backed NSView with a zero frame (+1 owned).
func NewView() uintptr {
	once.Do(initAppKit)
	v := msgInitRec(send(class("NSView"), "alloc"), sel("initWithFrame:"),
		NSRect{Size: NSSize{}})
	send(v, "setWantsLayer:", 1)
	return uintptr(v)
}

// AddBelow adds view to contentView below surfaceView (webview order).
func AddBelow(contentView, surfaceView, view uintptr) {
	once.Do(initAppKit)
	send(id(contentView), "addSubview:positioned:relativeTo:", id(view), nsWindowBelow, id(surfaceView))
}

// SetFrame moves a native view (AppKit points, bottom-left).
func SetFrame(view uintptr, frame NSRect) {
	once.Do(initAppKit)
	msgSetRec(id(view), sel("setFrame:"), frame)
}

// Frame returns the view's current frame (points, bottom-left).
func Frame(view uintptr) NSRect {
	once.Do(initAppKit)
	return msgRect(id(view), sel("frame"))
}

// Remove detaches a view from its superview.
func Remove(view uintptr) {
	once.Do(initAppKit)
	send(id(view), "removeFromSuperview")
}

// Release drops our +1 ownership from NewView.
func Release(view uintptr) {
	once.Do(initAppKit)
	send(id(view), "release")
}
