package resolve

import (
	"bufio"
	"bytes"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// cookiePair is one Netscape cookie ready for streamlink's --http-cookie.
type cookiePair struct {
	Name       string
	Value      string
	Persistent bool
}

// cookiesForHost reads a Netscape cookies.txt and returns the cookies that
// apply to u. The file is only read when the user configured one; a missing or
// malformed file yields no cookies rather than an error, matching yt-dlp's
// tolerant behaviour.
func cookiesForHost(path string, u *url.URL, now time.Time) []cookiePair {
	path = strings.TrimSpace(path)
	if path == "" || u == nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return parseCookies(data, u, now)
}

func parseCookies(data []byte, u *url.URL, now time.Time) []cookiePair {
	if u == nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	requestPath := u.EscapedPath()
	if requestPath == "" {
		requestPath = "/"
	}
	var cookies []cookiePair
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		line := strings.TrimPrefix(scanner.Text(), "#HttpOnly_")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 7 {
			continue
		}
		domain := strings.TrimPrefix(strings.ToLower(fields[0]), ".")
		if !cookieDomainMatches(host, domain) {
			continue
		}
		if cookiePath := fields[2]; !cookiePathMatches(requestPath, cookiePath) {
			continue
		}
		expiry, err := strconv.ParseInt(fields[4], 10, 64)
		if err == nil && expiry > 0 && time.Unix(expiry, 0).Before(now) {
			continue
		}
		name, value := fields[5], fields[6]
		if name == "" {
			continue
		}
		cookies = append(cookies, cookiePair{Name: name, Value: value, Persistent: expiry > 0})
	}
	return cookies
}

// sessionCookies are the cookies a jar load drops when it keeps persistent
// cookies only. Passing them as --http-cookie keeps them working while the
// jar supplies the rest.
func sessionCookies(cookies []cookiePair) []cookiePair {
	var sessions []cookiePair
	for _, cookie := range cookies {
		if !cookie.Persistent {
			sessions = append(sessions, cookie)
		}
	}
	return sessions
}

func cookieDomainMatches(host, domain string) bool {
	if domain == "" {
		return false
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// cookiePathMatches follows RFC 6265 section 5.1.4: an identical path, a
// cookie path ending in "/", or the next request character being "/".
func cookiePathMatches(requestPath, cookiePath string) bool {
	if cookiePath == "" {
		return true
	}
	if requestPath == cookiePath {
		return true
	}
	if !strings.HasPrefix(requestPath, cookiePath) {
		return false
	}
	if strings.HasSuffix(cookiePath, "/") {
		return true
	}
	return len(requestPath) > len(cookiePath) && requestPath[len(cookiePath)] == '/'
}

func cookieArgs(cookies []cookiePair) []string {
	args := make([]string, 0, len(cookies)*2)
	for _, cookie := range cookies {
		args = append(args, "--http-cookie", cookie.Name+"="+cookie.Value)
	}
	return args
}
