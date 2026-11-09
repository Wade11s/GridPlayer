// Package medialist reads the local media lists GridPlayer opens into one
// cell: m3u/m3u8, pls and xspf, plus a folder's files. HLS manifests
// (#EXT-X-) are passed through as a single URL, not expanded.
package medialist

import (
	"bufio"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// IsList reports whether path is a list file this package reads.
func IsList(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m3u", ".m3u8", ".pls", ".xspf":
		return true
	}
	return false
}

// Expand returns the media a path opens into one cell: a folder's files
// (recursive optional), or a list's entries resolved against its folder.
// A path that is neither is returned as itself.
func Expand(path string, recursive bool, max int) ([]string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return scanFolder(path, recursive, max)
	}
	if !IsList(path) {
		return []string{path}, nil
	}
	return parseList(path, max)
}

func scanFolder(dir string, recursive bool, max int) ([]string, error) {
	var out []string
	walk := func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		if isMedia(p) {
			out = append(out, p)
			if max > 0 && len(out) >= max {
				return filepath.SkipAll
			}
		}
		return nil
	}
	if err := filepath.WalkDir(dir, walk); err != nil && !errors.Is(err, filepath.SkipAll) {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func isMedia(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".mp4", ".mkv", ".mov", ".m4v", ".avi", ".webm", ".ts", ".m2ts",
		".mp3", ".flac", ".wav", ".m4a", ".ogg", ".opus":
		return true
	}
	return false
}

func parseList(path string, max int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	if strings.Contains(text, "#EXT-X-") {
		// HLS manifest: libVLC opens it as one stream.
		return []string{path}, nil
	}
	base := filepath.Dir(path)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pls":
		return parsePLS(text, base, max)
	case ".xspf":
		return parseXSPF(data, base, max)
	default:
		out := parseM3U(text, base, max)
		if len(out) == 0 {
			return nil, errors.New("medialist: m3u has no entries")
		}
		return out, nil
	}
}

func parseM3U(text, base string, max int) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, resolve(line, base))
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

func parsePLS(text, base string, max int) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(strings.ToLower(line), "file") {
			continue
		}
		_, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out = append(out, resolve(strings.TrimSpace(v), base))
		if max > 0 && len(out) >= max {
			break
		}
	}
	if len(out) == 0 {
		return nil, errors.New("medialist: pls has no File entries")
	}
	return out, nil
}

type xspfPlaylist struct {
	Tracks []struct {
		Location string `xml:"location"`
	} `xml:"trackList>track"`
}

func parseXSPF(data []byte, base string, max int) ([]string, error) {
	var p xspfPlaylist
	if err := xml.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	var out []string
	for _, t := range p.Tracks {
		if t.Location == "" {
			continue
		}
		out = append(out, resolve(t.Location, base))
		if max > 0 && len(out) >= max {
			break
		}
	}
	if len(out) == 0 {
		return nil, errors.New("medialist: xspf has no tracks")
	}
	return out, nil
}

func resolve(p, base string) string {
	if strings.Contains(p, "://") || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}
