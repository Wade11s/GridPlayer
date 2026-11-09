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
	if l, err := discoverBundled(); err == nil {
		return l, nil
	}
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

// discoverBundled finds the libs/plugins copied into a .app by
// scripts/build_app.sh. Development binaries outside a bundle fall through
// to the installed VLC.
func discoverBundled() (*VLCLayout, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return bundledLayout(filepath.Dir(filepath.Dir(exe)))
}

func bundledLayout(contents string) (*VLCLayout, error) {
	base := filepath.Join(contents, "Frameworks")
	plugins := filepath.Join(contents, "Resources", "vlc", "plugins")
	l := &VLCLayout{
		AppDir:     contents,
		IncludeDir: filepath.Join(contents, "Resources", "vlc", "include"),
		LibDir:     base,
		PluginsDir: plugins,
		LibVLC:     filepath.Join(base, "libvlc.dylib"),
		LibVLCCore: filepath.Join(base, "libvlccore.dylib"),
	}
	for _, p := range []string{l.LibVLC, l.LibVLCCore, l.PluginsDir} {
		if _, err := os.Stat(p); err != nil {
			return nil, err
		}
	}
	return l, nil
}
