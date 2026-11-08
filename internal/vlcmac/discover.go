package vlcmac

import (
	"errors"
	"os"
	"path/filepath"
)

// VLCLayout describes a discovered libVLC SDK install.
type VLCLayout struct {
	AppDir     string // /Applications/VLC.app
	IncludeDir string // .../Contents/MacOS/include
	LibDir     string // .../Contents/MacOS/lib
	PluginsDir string // .../Contents/MacOS/plugins
	LibVLC     string
	LibVLCCore string
}

// Discover finds libVLC from $VLC_APP or /Applications/VLC.app.
func Discover() (*VLCLayout, error) {
	app := os.Getenv("VLC_APP")
	if app == "" {
		app = "/Applications/VLC.app"
	}
	base := filepath.Join(app, "Contents", "MacOS")
	l := &VLCLayout{
		AppDir:     app,
		IncludeDir: filepath.Join(base, "include"),
		LibDir:     filepath.Join(base, "lib"),
		PluginsDir: filepath.Join(base, "plugins"),
		LibVLC:     filepath.Join(base, "lib", "libvlc.dylib"),
		LibVLCCore: filepath.Join(base, "lib", "libvlccore.dylib"),
	}
	for _, p := range []string{l.LibVLC, l.LibVLCCore, l.PluginsDir, filepath.Join(l.IncludeDir, "vlc", "libvlc.h")} {
		if _, err := os.Stat(p); err != nil {
			return nil, errors.New("vlcmac: libVLC SDK incomplete, missing " + p + " (set VLC_APP)")
		}
	}
	return l, nil
}
