package resolve

import (
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
	args := r.streamlinkArgs(u, "https://example.com/watch")
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
