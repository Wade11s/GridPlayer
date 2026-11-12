package resolve

import (
	"bufio"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// cookiePair is one Netscape cookie ready for streamlink's --http-cookie.
type cookiePair struct {
	Name  string
	Value string
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
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	host := strings.ToLower(u.Hostname())
	requestPath := u.EscapedPath()
	if requestPath == "" {
		requestPath = "/"
	}
	var cookies []cookiePair
	scanner := bufio.NewScanner(file)
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
		if cookiePath := fields[2]; cookiePath != "" && cookiePath != "/" &&
			!strings.HasPrefix(requestPath, cookiePath) {
			continue
		}
		if expiry, err := strconv.ParseInt(fields[4], 10, 64); err == nil && expiry > 0 &&
			time.Unix(expiry, 0).Before(now) {
			continue
		}
		name, value := fields[5], fields[6]
		if name == "" {
			continue
		}
		cookies = append(cookies, cookiePair{Name: name, Value: value})
	}
	return cookies
}

func cookieDomainMatches(host, domain string) bool {
	if domain == "" {
		return false
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func cookieArgs(cookies []cookiePair) []string {
	args := make([]string, 0, len(cookies)*2)
	for _, cookie := range cookies {
		args = append(args, "--http-cookie", cookie.Name+"="+cookie.Value)
	}
	return args
}
