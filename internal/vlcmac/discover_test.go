package vlcmac

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundledLayout(t *testing.T) {
	contents := t.TempDir()
	base := filepath.Join(contents, "Frameworks")
	plugins := filepath.Join(contents, "Resources", "vlc", "plugins")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(plugins, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"libvlc.dylib", "libvlccore.dylib"} {
		if err := os.WriteFile(filepath.Join(base, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	l, err := bundledLayout(contents)
	if err != nil {
		t.Fatal(err)
	}
	if l.LibVLC != filepath.Join(base, "libvlc.dylib") ||
		l.LibVLCCore != filepath.Join(base, "libvlccore.dylib") ||
		l.PluginsDir != plugins {
		t.Fatalf("layout %+v", l)
	}
	if err := os.Remove(filepath.Join(base, "libvlc.dylib")); err != nil {
		t.Fatal(err)
	}
	if _, err := bundledLayout(contents); err == nil {
		t.Fatal("missing libvlc accepted")
	}
}
