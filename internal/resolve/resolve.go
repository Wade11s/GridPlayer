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
	"os"
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

// commandError keeps the complete captured stderr for classification while its
// message stays bounded for display.
type commandError struct {
	err    error
	stderr string
}

func (e *commandError) Error() string {
	msg := strings.TrimSpace(e.stderr)
	if len(msg) > 400 {
		msg = msg[:400]
	}
	if msg == "" {
		return e.err.Error()
	}
	return fmt.Sprintf("%v: %s", e.err, msg)
}

func (e *commandError) Unwrap() error { return e.err }

func (b *limitedBuffer) String() string { return b.buf.String() }
func (b *limitedBuffer) Len() int       { return b.buf.Len() }

// ErrUnavailable means neither yt-dlp nor streamlink is installed.
var ErrUnavailable = errors.New("resolve: yt-dlp/streamlink not found")

// Resolver shells out to yt-dlp and streamlink.
type Resolver struct {
	Path       string // yt-dlp
	Streamlink string
	// ProxyMode is the frozen network/proxy_mode: system, none, or custom.
	// Empty keeps the older Proxy-only behavior.
	ProxyMode string
	Proxy     string
	Cookies   string
	Format    string
	Timeout   time.Duration
}

// proxyMode normalizes the stored mode; an unknown value keeps the legacy
// Proxy-only behavior rather than silently choosing a proxy source.
func (r *Resolver) proxyMode() string {
	switch strings.ToLower(strings.TrimSpace(r.ProxyMode)) {
	case "system", "none", "custom":
		return strings.ToLower(strings.TrimSpace(r.ProxyMode))
	default:
		return ""
	}
}

// mustRefuseProxyEnv reports whether the child must not inherit a proxy
// from the environment. None refuses one, and so does custom with nothing
// filled in: the frozen source reads that as no proxy rather than as the
// machine's.
func (r *Resolver) mustRefuseProxyEnv() bool {
	mode := r.proxyMode()
	return mode == "none" || (mode == "custom" && strings.TrimSpace(r.Proxy) == "")
}

// proxyURL is the proxy the resolver requests; system and none leave the
// client on its environment or with no proxy at all.
func (r *Resolver) proxyURL() string {
	switch r.proxyMode() {
	case "system", "none":
		return ""
	case "custom":
		return strings.TrimSpace(r.Proxy)
	default:
		return strings.TrimSpace(r.Proxy)
	}
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
		got, err := r.resolveStreamlink(ctx, u, raw)
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
	if proxy := r.proxyURL(); proxy != "" {
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

// resolveStreamlink prefers streamlink's own cookies file handling, which lets
// streamlink apply the jar's attributes and keeps cookie values off the
// command line. Older streamlink releases lack the flag, so a failure that
// names the option falls back to per-cookie arguments.
func (r *Resolver) resolveStreamlink(ctx context.Context, u *url.URL, raw string) (string, error) {
	path := strings.TrimSpace(r.Cookies)
	data, ok := readCookieFile(path)
	var file string
	var cookies []cookiePair
	if ok {
		file = path
		cookies = parseCookies(data, u, time.Now())
	}
	if file == "" {
		return r.run(ctx, r.Streamlink, r.streamlinkArgs(raw, "", cookies))
	}
	// Streamlink loads the jar itself, so only the cookies its loader drops
	// (session cookies) are passed separately.
	got, err := r.run(ctx, r.Streamlink, r.streamlinkArgs(raw, file, sessionCookies(cookies)))
	if err == nil {
		return got, nil
	}
	if !unsupportedCookieFileOption(err) {
		return got, err
	}
	return r.run(ctx, r.Streamlink, r.streamlinkArgs(raw, "", cookies))
}

// withoutProxyEnv drops the proxy variables a child would otherwise pick
// up, so "none" can mean none rather than the machine's settings.
func withoutProxyEnv(env []string) []string {
	out := env[:0]
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			out = append(out, entry)
			continue
		}
		switch strings.ToLower(name) {
		case "http_proxy", "https_proxy", "all_proxy", "ftp_proxy":
			continue
		}
		out = append(out, entry)
	}
	return out
}

// unsupportedCookieFileOption reports whether streamlink rejected the flag
// itself, which is the only case a per-cookie retry can help with. A jar the
// loader refuses stays a hard error.
func unsupportedCookieFileOption(err error) bool {
	if err == nil {
		return false
	}
	// Argparse prints its usage block before the diagnostic, and the
	// returned message only keeps the first 400 bytes. Classify against the
	// complete captured stderr so a long preamble cannot hide the option
	// error.
	message := err.Error()
	var cmdErr *commandError
	if errors.As(err, &cmdErr) {
		message = cmdErr.stderr
	}
	message = strings.ToLower(message)
	if !strings.Contains(message, "http-cookies-file") {
		return false
	}
	return strings.Contains(message, "no such option") ||
		strings.Contains(message, "unrecognized arguments") ||
		strings.Contains(message, "unknown option")
}

func (r *Resolver) streamlinkArgs(raw, cookieFile string, cookies []cookiePair) []string {
	args := []string{"--stream-url", "--default-stream", "best"}
	if proxy := r.proxyURL(); proxy != "" {
		args = append(args, "--http-proxy", proxy)
	}
	if cookieFile != "" {
		args = append(args, "--http-cookies-file", cookieFile)
	}
	args = append(args, cookieArgs(cookies)...)
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
	if r.mustRefuseProxyEnv() {
		cmd.Env = withoutProxyEnv(os.Environ())
	}
	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if cctx.Err() != nil {
			return "", fmt.Errorf("timed out after %s", timeout)
		}
		return "", &commandError{err: err, stderr: stderr.String()}
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line, nil
		}
	}
	return "", errors.New("returned no URL")
}
