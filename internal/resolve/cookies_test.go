package resolve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const cookieFile = `# Netscape HTTP Cookie File
.example.com	TRUE	/	FALSE	0	session	abc
#HttpOnly_.example.com	TRUE	/	FALSE	0	secret	xyz
.other.com	TRUE	/	FALSE	0	foreign	no
.example.com	TRUE	/private	FALSE	0	scoped	yes
.example.com	TRUE	/	FALSE	1000000000	expired	old
`

const mixedCookieFile = `# Netscape HTTP Cookie File
.example.com	TRUE	/	FALSE	0	session	abc
.example.com	TRUE	/	FALSE	4102444800	persistent	def
`

func writeCookieFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCookiesForHostMatchesDomainPathAndExpiry(t *testing.T) {
	path := writeCookieFile(t, cookieFile)
	u, err := url.Parse("https://www.example.com/watch?v=1")
	if err != nil {
		t.Fatal(err)
	}
	got := cookiesForHost(path, u, time.Unix(2_000_000_000, 0))
	var names []string
	for _, cookie := range got {
		names = append(names, cookie.Name)
	}
	if joined := strings.Join(names, ","); joined != "session,secret" {
		t.Fatalf("cookies %q", joined)
	}
}

func TestCookiesForHostHonoursCookiePath(t *testing.T) {
	path := writeCookieFile(t, cookieFile)
	u, err := url.Parse("https://example.com/private/page")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, cookie := range cookiesForHost(path, u, time.Unix(2_000_000_000, 0)) {
		seen[cookie.Name] = true
	}
	if !seen["scoped"] {
		t.Fatalf("scoped cookie missing")
	}
}

func TestCookiesForHostToleratesMissingFile(t *testing.T) {
	u, _ := url.Parse("https://example.com/")
	if got := cookiesForHost(filepath.Join(t.TempDir(), "nope.txt"), u, time.Now()); got != nil {
		t.Fatalf("cookies %+v", got)
	}
	if got := cookiesForHost("", u, time.Now()); got != nil {
		t.Fatalf("cookies %+v", got)
	}
}

func TestSessionCookiesFiltersPersistent(t *testing.T) {
	path := writeCookieFile(t, cookieFile)
	u, _ := url.Parse("https://example.com/watch")
	all := cookiesForHost(path, u, time.Unix(2_000_000_000, 0))
	sessions := sessionCookies(all)
	if len(sessions) != 2 {
		t.Fatalf("sessions %+v", sessions)
	}
	for _, cookie := range sessions {
		if cookie.Persistent {
			t.Fatalf("persistent cookie in session list: %+v", cookie)
		}
	}
}

func TestStreamlinkArgsPassCookiesOnFallback(t *testing.T) {
	path := writeCookieFile(t, cookieFile)
	r := &Resolver{Streamlink: "/tmp/streamlink", Proxy: "http://127.0.0.1:8080", Cookies: path}
	u, _ := url.Parse("https://example.com/watch")
	args := r.streamlinkArgs("https://example.com/watch", "", cookiesForHost(path, u, time.Unix(2_000_000_000, 0)))
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--stream-url",
		"--http-proxy http://127.0.0.1:8080",
		"--http-cookie session=abc",
		"--http-cookie secret=xyz",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
	if strings.Contains(joined, "foreign=no") {
		t.Fatalf("foreign cookie leaked: %q", joined)
	}
	if strings.Contains(joined, "--http-cookies-file") {
		t.Fatalf("file flag in fallback: %q", joined)
	}
}

func TestStreamlinkArgsPreferCookiesFileAndKeepSessionCookies(t *testing.T) {
	path := writeCookieFile(t, mixedCookieFile)
	r := &Resolver{Streamlink: "/tmp/streamlink", Cookies: path}
	u, _ := url.Parse("https://example.com/watch")
	all := cookiesForHost(path, u, time.Unix(2_000_000_000, 0))
	args := r.streamlinkArgs("https://example.com/watch", path, sessionCookies(all))
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--http-cookies-file "+path) {
		t.Fatalf("missing cookies file in %q", joined)
	}
	if !strings.Contains(joined, "--http-cookie session=abc") {
		t.Fatalf("session cookie missing from the file path: %q", joined)
	}
	if strings.Contains(joined, "persistent=def") {
		t.Fatalf("persistent cookies must stay in the jar: %q", joined)
	}
}

func TestCookiePathMatchesRFC6265(t *testing.T) {
	cases := []struct {
		request, cookie string
		want            bool
	}{
		{"/private", "/private", true},
		{"/private/page", "/private", true},
		{"/privateer", "/private", false},
		{"/privateX", "/private", false},
		{"/anything", "/", true},
		{"/a/b", "", true},
		{"/a/b", "/a/", true},
	}
	for _, tc := range cases {
		if got := cookiePathMatches(tc.request, tc.cookie); got != tc.want {
			t.Fatalf("%s vs %s = %v, want %v", tc.request, tc.cookie, got, tc.want)
		}
	}
}

func TestReadCookieFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(file, []byte(cookieFile), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, ok := readCookieFile(file); !ok || !strings.Contains(string(data), "session") {
		t.Fatalf("regular file not read: ok=%v", ok)
	}
	for name, path := range map[string]string{
		"directory": dir,
		"missing":   filepath.Join(dir, "nope.txt"),
		"blank":     "",
	} {
		if _, ok := readCookieFile(path); ok {
			t.Fatalf("%s: read", name)
		}
	}

	large := filepath.Join(dir, "large.txt")
	if err := os.WriteFile(large, bytes.Repeat([]byte("a"), maxCookieFile+2), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readCookieFile(large); ok {
		t.Fatal("oversized file read")
	}

	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	if _, ok := readCookieFile(fifo); ok {
		t.Fatal("fifo read")
	}
}

func TestUnsupportedCookieFileOption(t *testing.T) {
	cases := []struct {
		message string
		want    bool
	}{
		{"Error: no such option: --http-cookies-file", true},
		{"error: unrecognized arguments: --http-cookies-file x", true},
		{"error: streamlink: unknown option --http-cookies-file", true},
		{"error: unrecognized arguments: --some-other-option", false},
		{"OSError: Error while loading cookies from file: x", false},
		{"network error", false},
	}
	for _, tc := range cases {
		if got := unsupportedCookieFileOption(errors.New(tc.message)); got != tc.want {
			t.Fatalf("%q = %v", tc.message, tc.want)
		}
	}

	// The displayed message keeps only 400 bytes, but classification must
	// see the diagnostic that follows a realistic argparse usage block.
	long := &commandError{
		err:    errors.New("exit status 2"),
		stderr: strings.Repeat("usage: streamlink [--very-long-option VALUE] ...\n", 80) + "Error: no such option: --http-cookies-file\n",
	}
	if len(long.Error()) > 500 {
		t.Fatalf("display message must stay bounded: %d", len(long.Error()))
	}
	if !unsupportedCookieFileOption(long) {
		t.Fatal("long usage preamble hid the option error")
	}
	if got := unsupportedCookieFileOption(fmt.Errorf("resolve: %w", long)); !got {
		t.Fatal("wrapped option error not recognised")
	}
}

func TestResolveStreamlinkFallsBackToCookieArgs(t *testing.T) {
	yt := fakeTool(t, "yt-dlp", `echo "yt fail" >&2; exit 1`)
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("ARGS_FILE", argsFile)
	sl := fakeTool(t, "streamlink", `
for arg in "$@"; do
  if [ "$arg" = "--http-cookies-file" ]; then
    echo "Error: no such option: --http-cookies-file" >&2
    exit 2
  fi
done
printf '%s\n' "$@" > "$ARGS_FILE"
echo "https://stream.example/live.m3u8"`)
	cookies := writeCookieFile(t, "# Netscape HTTP Cookie File\n.example.com\tTRUE\t/\tFALSE\t4102444800\tsession\tabc\n")
	r := &Resolver{Path: yt, Streamlink: sl, Cookies: cookies}
	got, err := r.Resolve(context.Background(), "https://example.com/live")
	if err != nil || got != "https://stream.example/live.m3u8" {
		t.Fatalf("resolve %q %v", got, err)
	}
	args := readArgs(t, argsFile)
	if !strings.Contains(args, "--http-cookie") || !strings.Contains(args, "session=abc") {
		t.Fatalf("fallback args missing cookies:\n%s", args)
	}
}

func TestResolveStreamlinkFallsBackOnLongArgparsePreamble(t *testing.T) {
	yt := fakeTool(t, "yt-dlp", `echo "yt fail" >&2; exit 1`)
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("ARGS_FILE", argsFile)
	sl := fakeTool(t, "streamlink", `
for arg in "$@"; do
  if [ "$arg" = "--http-cookies-file" ]; then
    i=0
    while [ "$i" -lt 120 ]; do
      echo "usage: streamlink [--very-long-option-$i VALUE] ..." >&2
      i=$((i+1))
    done
    echo "Error: no such option: --http-cookies-file" >&2
    exit 2
  fi
done
printf '%s\n' "$@" > "$ARGS_FILE"
echo "https://stream.example/live.m3u8"`)
	cookies := writeCookieFile(t, mixedCookieFile)
	r := &Resolver{Path: yt, Streamlink: sl, Cookies: cookies}
	got, err := r.Resolve(context.Background(), "https://example.com/live")
	if err != nil || got != "https://stream.example/live.m3u8" {
		t.Fatalf("resolve %q %v", got, err)
	}
	args := readArgs(t, argsFile)
	if strings.Contains(args, "--http-cookies-file") {
		t.Fatalf("fallback kept the file flag:\n%s", args)
	}
	for _, want := range []string{"--http-cookie", "session=abc", "persistent=def"} {
		if !strings.Contains(args, want) {
			t.Fatalf("fallback args missing %q:\n%s", want, args)
		}
	}
}

func TestResolveStreamlinkPrefersCookieFile(t *testing.T) {
	yt := fakeTool(t, "yt-dlp", `echo "yt fail" >&2; exit 1`)
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("ARGS_FILE", argsFile)
	sl := fakeTool(t, "streamlink", `printf '%s\n' "$@" > "$ARGS_FILE"; echo "https://stream.example/live.m3u8"`)
	cookies := writeCookieFile(t, mixedCookieFile)
	r := &Resolver{Path: yt, Streamlink: sl, Cookies: cookies}
	got, err := r.Resolve(context.Background(), "https://example.com/live")
	if err != nil || got != "https://stream.example/live.m3u8" {
		t.Fatalf("resolve %q %v", got, err)
	}
	args := readArgs(t, argsFile)
	if !strings.Contains(args, "--http-cookies-file") || !strings.Contains(args, cookies) {
		t.Fatalf("preferred args missing the jar:\n%s", args)
	}
	if !strings.Contains(args, "session=abc") {
		t.Fatalf("session cookie missing:\n%s", args)
	}
	if strings.Contains(args, "persistent=def") {
		t.Fatalf("persistent value leaked into argv:\n%s", args)
	}
}

func TestResolveStreamlinkDoesNotFallBackOnJarLoadError(t *testing.T) {
	yt := fakeTool(t, "yt-dlp", `echo "yt fail" >&2; exit 1`)
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("ARGS_FILE", argsFile)
	sl := fakeTool(t, "streamlink", `echo "OSError: Error while loading cookies from file" >&2; exit 2`)
	cookies := writeCookieFile(t, mixedCookieFile)
	r := &Resolver{Path: yt, Streamlink: sl, Cookies: cookies}
	if _, err := r.Resolve(context.Background(), "https://example.com/live"); err == nil {
		t.Fatal("jar load failure was swallowed")
	}
	if _, err := os.Stat(argsFile); !os.IsNotExist(err) {
		t.Fatalf("unexpected fallback invocation: %v", err)
	}
}

func TestCookieExpiryBoundaryAndMalformedExpiry(t *testing.T) {
	body := "# Netscape HTTP Cookie File\n" +
		".example.com\tTRUE\t/\tFALSE\t2000000000\tboundary\tv\n" +
		".example.com\tTRUE\t/\tFALSE\tsoon\tmalformed\tv\n" +
		".example.com\tTRUE\t/\tFALSE\t\tempty\tv\n"
	path := writeCookieFile(t, body)
	u, _ := url.Parse("https://example.com/")
	now := time.Unix(2_000_000_000, 0)
	seen := map[string]bool{}
	for _, cookie := range cookiesForHost(path, u, now) {
		seen[cookie.Name] = true
	}
	if seen["boundary"] {
		t.Fatal("cookie expiring exactly now was retained")
	}
	if seen["malformed"] {
		t.Fatal("malformed expiry was reinterpreted as a session cookie")
	}
	if !seen["empty"] {
		t.Fatal("empty expiry should be a session cookie")
	}
}

func TestCookieDomainMatches(t *testing.T) {
	cases := []struct {
		host, domain string
		want         bool
	}{
		{"example.com", "example.com", true},
		{"www.example.com", "example.com", true},
		{"badexample.com", "example.com", false},
		{"example.com", "", false},
	}
	for _, tc := range cases {
		if got := cookieDomainMatches(tc.host, tc.domain); got != tc.want {
			t.Fatalf("%s vs %s = %v", tc.host, tc.domain, got)
		}
	}
}
