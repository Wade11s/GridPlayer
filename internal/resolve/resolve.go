// Package resolve turns a page URL into a direct media URL with yt-dlp,
// falling back to streamlink.
//
// It never reads the user's yt-dlp config: --ignore-config is always passed.
// Cookies and a proxy are opt-in settings. It does not download media, and it
// does not expand playlists. Direct schemes (rtsp, rtmp, ...) pass through.
package resolve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

const maxOutput = 64 << 10

type limitedBuffer struct {
	buf bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := maxOutput - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string { return b.buf.String() }
func (b *limitedBuffer) Len() int       { return b.buf.Len() }

// ErrUnavailable means neither yt-dlp nor streamlink is installed.
var ErrUnavailable = errors.New("resolve: yt-dlp/streamlink not found")

// Resolver shells out to yt-dlp and streamlink.
type Resolver struct {
	Path       string // yt-dlp
	Streamlink string
	Proxy      string
	Cookies    string
	Format     string
	Timeout    time.Duration
}

// New finds yt-dlp and streamlink on PATH.
func New() *Resolver {
	r := &Resolver{Format: "best", Timeout: 30 * time.Second}
	if p, err := exec.LookPath("yt-dlp"); err == nil {
		r.Path = p
	}
	if p, err := exec.LookPath("streamlink"); err == nil {
		r.Streamlink = p
	}
	return r
}

// Available reports whether at least one resolver was found.
func (r *Resolver) Available() bool {
	return r != nil && (r.Path != "" || r.Streamlink != "")
}

// Resolve returns a direct media URL. For schemes the resolvers do not
// handle, it returns raw unchanged. yt-dlp is tried first, then streamlink;
// both failures are reported together.
func (r *Resolver) Resolve(ctx context.Context, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("resolve: empty URL")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return "", fmt.Errorf("resolve: not a URL: %q", raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "file":
	default:
		return raw, nil
	}
	if !r.Available() {
		return "", ErrUnavailable
	}

	var failures []string
	if r.Path != "" {
		got, err := r.run(ctx, r.Path, r.ytdlpArgs(u, raw))
		if err == nil {
			return got, nil
		}
		failures = append(failures, "yt-dlp: "+err.Error())
	}
	if u.Scheme != "file" && r.Streamlink != "" {
		got, err := r.run(ctx, r.Streamlink, r.streamlinkArgs(u, raw))
		if err == nil {
			return got, nil
		}
		failures = append(failures, "streamlink: "+err.Error())
	}
	msg := strings.Join(failures, "; ")
	if len(msg) > 800 {
		msg = msg[:800]
	}
	return "", fmt.Errorf("resolve: %s", msg)
}

func (r *Resolver) ytdlpArgs(u *url.URL, raw string) []string {
	format := strings.TrimSpace(r.Format)
	if format == "" {
		format = "best"
	}
	args := []string{
		"--ignore-config", "--no-warnings", "--skip-download", "--no-playlist",
		"-g", "-f", format,
	}
	if proxy := strings.TrimSpace(r.Proxy); proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	if cookies := strings.TrimSpace(r.Cookies); cookies != "" {
		args = append(args, "--cookies", cookies)
	}
	if u.Scheme == "file" {
		args = append(args, "--enable-file-urls")
	}
	return append(args, "--", raw)
}

func (r *Resolver) streamlinkArgs(u *url.URL, raw string) []string {
	args := []string{"--stream-url", "--default-stream", "best"}
	if proxy := strings.TrimSpace(r.Proxy); proxy != "" {
		args = append(args, "--http-proxy", proxy)
	}
	if cookies := strings.TrimSpace(r.Cookies); cookies != "" {
		args = append(args, cookieArgs(cookiesForHost(cookies, u, time.Now()))...)
	}
	return append(args, "--", raw)
}

func (r *Resolver) run(ctx context.Context, path string, args []string) (string, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, path, args...)
	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if cctx.Err() != nil {
			return "", fmt.Errorf("timed out after %s", timeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 400 {
			msg = msg[:400]
		}
		return "", fmt.Errorf("%v: %s", err, msg)
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line, nil
		}
	}
	return "", errors.New("returned no URL")
}
