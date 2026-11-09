// Package resolve turns a page URL into a direct media URL with yt-dlp.
//
// It never reads the user's yt-dlp config, cookies or browser data:
// --ignore-config is always passed. It does not download media, and it
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

// ErrUnavailable means yt-dlp is not installed.
var ErrUnavailable = errors.New("resolve: yt-dlp not found")

// Resolver shells out to yt-dlp.
type Resolver struct {
	Path    string
	Timeout time.Duration
}

// New finds yt-dlp on PATH.
func New() *Resolver {
	p, err := exec.LookPath("yt-dlp")
	if err != nil {
		return &Resolver{Timeout: 30 * time.Second}
	}
	return &Resolver{Path: p, Timeout: 30 * time.Second}
}

// Available reports whether yt-dlp was found.
func (r *Resolver) Available() bool { return r != nil && r.Path != "" }

// Resolve returns a direct media URL. For schemes yt-dlp does not handle,
// it returns raw unchanged. A yt-dlp failure is returned as an error; the
// caller may choose to play raw directly.
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
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{
		"--ignore-config", "--no-warnings", "--skip-download", "--no-playlist",
		"-g", "-f", "best",
	}
	if u.Scheme == "file" {
		args = append(args, "--enable-file-urls")
	}
	args = append(args, "--", raw)
	cmd := exec.CommandContext(cctx, r.Path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if cctx.Err() != nil {
			return "", fmt.Errorf("resolve: yt-dlp timed out after %s", timeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 400 {
			msg = msg[:400]
		}
		return "", fmt.Errorf("resolve: yt-dlp failed: %v: %s", err, msg)
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line, nil
		}
	}
	return "", errors.New("resolve: yt-dlp returned no URL")
}
