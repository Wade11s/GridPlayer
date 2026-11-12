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
	joined := strings.Join(names, ",")
	if joined != "session,secret" {
		t.Fatalf("cookies %q", joined)
	}
}

func TestCookiesForHostHonoursCookiePath(t *testing.T) {
	path := writeCookieFile(t, cookieFile)
	u, err := url.Parse("https://example.com/private/page")
	if err != nil {
		t.Fatal(err)
	}
	got := cookiesForHost(path, u, time.Unix(2_000_000_000, 0))
	seen := map[string]bool{}
	for _, cookie := range got {
		seen[cookie.Name] = true
	}
	if !seen["scoped"] {
		t.Fatalf("scoped cookie missing: %+v", got)
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

func TestStreamlinkArgsPassCookies(t *testing.T) {
	path := writeCookieFile(t, cookieFile)
	r := &Resolver{Streamlink: "/tmp/streamlink", Proxy: "http://127.0.0.1:8080", Cookies: path}
	u, _ := url.Parse("https://example.com/watch")
	args := r.streamlinkArgs("https://example.com/watch", path, cookiesForHost(path, u, time.Unix(2_000_000_000, 0)), false)
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
}

func TestStreamlinkArgsPreferCookiesFile(t *testing.T) {
	path := writeCookieFile(t, cookieFile)
	r := &Resolver{Streamlink: "/tmp/streamlink", Cookies: path}
	u, _ := url.Parse("https://example.com/watch")
	args := r.streamlinkArgs("https://example.com/watch", path, cookiesForHost(path, u, time.Unix(2_000_000_000, 0)), true)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--http-cookies-file "+path) {
		t.Fatalf("missing cookies file in %q", joined)
	}
	if strings.Contains(joined, "--http-cookie ") {
		t.Fatalf("cookie values leaked into args: %q", joined)
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

func TestInspectCookieJarRejectsUnusableFiles(t *testing.T) {
	u, _ := url.Parse("https://example.com/watch")
	dir := t.TempDir()

	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	noMagic := filepath.Join(dir, "nomagic.txt")
	if err := os.WriteFile(noMagic, []byte(".example.com\tTRUE\t/\tFALSE\t0\tsession\tabc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	otherHost := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(otherHost, []byte("# Netscape HTTP Cookie File\n.other.com\tTRUE\t/\tFALSE\t0\tx\ty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"empty":      empty,
		"no magic":   noMagic,
		"other host": otherHost,
		"directory":  dir,
		"missing":    filepath.Join(dir, "nope.txt"),
		"blank":      "",
	} {
		if got, cookies := inspectCookieJar(path, u); got != "" || cookies != nil {
			t.Fatalf("%s: got path %q cookies %+v", name, got, cookies)
		}
	}
}

func TestUnsupportedStreamlinkOptionIsScopedToTheFlag(t *testing.T) {
	cases := []struct {
		message string
		want    bool
	}{
		{"Error: no such option: --http-cookies-file", true},
		{"error: unrecognized arguments: --http-cookies-file x", true},
		{"error: unrecognized arguments: --some-other-option", false},
		{"error: streamlink: unknown option --http-cookies-file", true},
		{"network error", false},
	}
	for _, tc := range cases {
		err := errors.New(tc.message)
		if got := unsupportedStreamlinkOption(err); got != tc.want {
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
	cookies := writeCookieFile(t, "# Netscape HTTP Cookie File\n.example.com\tTRUE\t/\tFALSE\t0\tsession\tabc\n")
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
