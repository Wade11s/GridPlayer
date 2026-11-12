package resolve

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

const persistentCookieFile = `# Netscape HTTP Cookie File
.example.com	TRUE	/	FALSE	4102444800	persistent	abc
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
	path := writeCookieFile(t, cookieFile)
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
	if strings.Contains(joined, "persistent=") || strings.Contains(joined, "expired=old") {
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

func TestUsableCookieFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(file, []byte(cookieFile), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := usableCookieFile(file); got != file {
		t.Fatalf("regular file %q", got)
	}
	for name, path := range map[string]string{
		"directory": dir,
		"missing":   filepath.Join(dir, "nope.txt"),
		"blank":     "",
	} {
		if got := usableCookieFile(path); got != "" {
			t.Fatalf("%s: %q", name, got)
		}
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
			t.Fatalf("%q = %v", tc.message, got)
		}
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
