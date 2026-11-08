//go:build !darwin

package nativehost

// Stub for non-macOS builds: the bootstrap's native hosting is macOS-only.
// Callers must check ok from Resolve before using any handle.

type NSPoint struct{ X, Y float64 }
type NSSize struct{ Width, Height float64 }
type NSRect struct {
	Origin NSPoint
	Size   NSSize
}

type DIP struct{ X, Y, W, H float64 }

func Resolve(nswindow uintptr) (contentView, surfaceView uintptr, height float64, ok bool) {
	return 0, 0, 0, false
}

func ToMac(r DIP, height float64) NSRect {
	return NSRect{Origin: NSPoint{X: r.X, Y: height - r.Y - r.H}, Size: NSSize{Width: r.W, Height: r.H}}
}

func NewView() uintptr                                { return 0 }
func AddBelow(contentView, surfaceView, view uintptr) {}
func SetFrame(view uintptr, frame NSRect)             {}
func Frame(view uintptr) NSRect                       { return NSRect{} }
func Remove(view uintptr)                             {}
func Release(view uintptr)                            {}
