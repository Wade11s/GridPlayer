package resolve

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLimitedBufferCaps(t *testing.T) {
	var b limitedBuffer
	n, err := io.Copy(&b, bytes.NewReader(make([]byte, 200000)))
	if err != nil || n != 200000 {
		t.Fatalf("copy n=%d err=%v", n, err)
	}
	if b.Len() != maxOutput {
		t.Fatalf("buffer len=%d, want %d", b.Len(), maxOutput)
	}
}

func TestDirectSchemesPassThrough(t *testing.T) {
	r := &Resolver{}
	for _, raw := range []string{"rtsp://example/a", "rtmp://example/b", "udp://239.0.0.1:1234"} {
		got, err := r.Resolve(context.Background(), raw)
		if err != nil || got != raw {
			t.Fatalf("%q -> %q %v", raw, got, err)
		}
	}
}

func TestResolveLocalHLS(t *testing.T) {
	master, err := filepath.Abs("../../testdata/fixtures/hls/master.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(master); err != nil {
		t.Skip("hls fixture missing")
	}
	r := New()
	if !r.Available() {
		t.Skip("yt-dlp not installed")
	}
	got, err := r.Resolve(context.Background(), "file://"+master)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil || u.Scheme != "file" {
		t.Fatalf("resolved %q", got)
	}
	if !strings.HasSuffix(u.Path, ".m3u8") {
		t.Fatalf("resolved %q", got)
	}
	if _, err := os.Stat(u.Path); err != nil {
		t.Fatalf("resolved path missing: %v", err)
	}
}

func TestResolveRejectsEmpty(t *testing.T) {
	r := &Resolver{}
	if _, err := r.Resolve(context.Background(), ""); err == nil {
		t.Fatal("want error")
	}
	if _, err := r.Resolve(context.Background(), "not a url"); err == nil {
		t.Fatal("want error")
	}
}
