package resolve

import (
	"bytes"
	"context"
	"errors"
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
	if r.Path == "" {
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

func fakeTool(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveYTDLPArgs(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("ARGS_FILE", argsFile)
	tool := fakeTool(t, "yt-dlp",
		`printf '%s\n' "$@" > "$ARGS_FILE"; echo "https://cdn.example/direct.m3u8"`)
	r := &Resolver{
		Path:    tool,
		Proxy:   "http://proxy:8080",
		Cookies: "/tmp/cookies.txt",
		Format:  "best",
	}
	got, err := r.Resolve(context.Background(), "https://example.com/watch")
	if err != nil || got != "https://cdn.example/direct.m3u8" {
		t.Fatalf("resolve %q %v", got, err)
	}
	args := readArgs(t, argsFile)
	for _, want := range []string{
		"--ignore-config", "--no-playlist", "--proxy", "http://proxy:8080",
		"--cookies", "/tmp/cookies.txt", "-f", "best", "https://example.com/watch",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("args missing %q:\n%s", want, args)
		}
	}
}

func TestResolveFileURLAddsEnableFileURLs(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("ARGS_FILE", argsFile)
	tool := fakeTool(t, "yt-dlp",
		`printf '%s\n' "$@" > "$ARGS_FILE"; echo "file:///tmp/x.m3u8"`)
	r := &Resolver{Path: tool}
	if _, err := r.Resolve(context.Background(), "file:///tmp/x.m3u8"); err != nil {
		t.Fatal(err)
	}
	if args := readArgs(t, argsFile); !strings.Contains(args, "--enable-file-urls") {
		t.Fatalf("args missing --enable-file-urls:\n%s", args)
	}
}

func TestResolveFallsBackToStreamlink(t *testing.T) {
	fail := fakeTool(t, "yt-dlp", `echo "yt fail" >&2; exit 1`)
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("ARGS_FILE", argsFile)
	streamlink := fakeTool(t, "streamlink",
		`printf '%s\n' "$@" > "$ARGS_FILE"; echo "https://stream.example/live.m3u8"`)
	r := &Resolver{Path: fail, Streamlink: streamlink, Proxy: "http://proxy:8080"}
	got, err := r.Resolve(context.Background(), "https://example.com/live")
	if err != nil || got != "https://stream.example/live.m3u8" {
		t.Fatalf("resolve %q %v", got, err)
	}
	args := readArgs(t, argsFile)
	for _, want := range []string{"--stream-url", "--default-stream", "best", "--http-proxy", "http://proxy:8080"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args missing %q:\n%s", want, args)
		}
	}
}

func TestResolveBothFail(t *testing.T) {
	y := fakeTool(t, "yt-dlp", `echo "yt fail" >&2; exit 1`)
	s := fakeTool(t, "streamlink", `echo "sl fail" >&2; exit 1`)
	r := &Resolver{Path: y, Streamlink: s}
	_, err := r.Resolve(context.Background(), "https://example.com/live")
	if err == nil || !strings.Contains(err.Error(), "yt-dlp") || !strings.Contains(err.Error(), "streamlink") {
		t.Fatalf("combined error: %v", err)
	}
}

func TestResolveFileURLDoesNotUseStreamlink(t *testing.T) {
	y := fakeTool(t, "yt-dlp", `echo "yt fail" >&2; exit 1`)
	s := fakeTool(t, "streamlink", `echo "https://stream.example/should-not"`)
	r := &Resolver{Path: y, Streamlink: s}
	_, err := r.Resolve(context.Background(), "file:///tmp/x.m3u8")
	if err == nil || strings.Contains(err.Error(), "streamlink") {
		t.Fatalf("file URL used streamlink: %v", err)
	}
}

func TestResolveNoTools(t *testing.T) {
	r := &Resolver{}
	if _, err := r.Resolve(context.Background(), "https://example.com/live"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func readArgs(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
