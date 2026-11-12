package resolve

import (
	"bufio"
	"bytes"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// cookieMagic is the header CPython's MozillaCookieJar looks for. It accepts
// "# Netscape HTTP Cookie File" and "# HTTP Cookie File" case-insensitively.
var cookieMagic = regexp.MustCompile(`(?i)#( netscape)? http cookie file`)

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

// parseStrict mirrors what CPython's MozillaCookieJar.load accepts, because
// streamlink treats a rejected jar as fatal. It returns ok=false on the first
// line the loader would refuse, and only reports the cookies that survive
// load() with ignore_discard=false (persistent, unexpired, matching u).
func parseStrict(data []byte, u *url.URL, now time.Time) (cookies []cookiePair, ok bool) {
	if u == nil {
		return nil, false
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	first := true
	host := strings.ToLower(u.Hostname())
	requestPath := u.EscapedPath()
	if requestPath == "" {
		requestPath = "/"
	}
	for scanner.Scan() {
		raw := scanner.Text()
		if first {
			first = false
			if !cookieMagic.MatchString(raw) {
				return nil, false
			}
			continue
		}
		line := strings.TrimPrefix(raw, "#HttpOnly_")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 7 {
			return nil, false
		}
		domain := fields[0]
		switch strings.ToUpper(fields[1]) {
		case "TRUE":
			if !strings.HasPrefix(domain, ".") {
				return nil, false
			}
		case "FALSE":
			if strings.HasPrefix(domain, ".") {
				return nil, false
			}
		default:
			return nil, false
		}
		session := fields[4] == ""
		if session {
			continue
		}
		expiry, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			return nil, false
		}
		if expiry <= now.Unix() {
			continue
		}
		bareDomain := strings.TrimPrefix(strings.ToLower(domain), ".")
		if !cookieDomainMatches(host, bareDomain) {
			continue
		}
		if !cookiePathMatches(requestPath, fields[2]) {
			continue
		}
		if fields[5] == "" {
			continue
		}
		cookies = append(cookies, cookiePair{Name: fields[5], Value: fields[6], Persistent: true})
	}
	return cookies, true
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
