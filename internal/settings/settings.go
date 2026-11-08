// Package settings stores GridPlayer settings in a QSettings-compatible
// INI file. Keys use GridPlayer's slash form ("playlist/grid_mode"), which
// becomes a [playlist] section with a grid_mode entry, as QSettings writes.
package settings

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store is an in-memory settings file.
type Store struct {
	path   string
	values map[string]string
}

// Load reads path; a missing file is an empty store.
func Load(path string) (*Store, error) {
	s := &Store{path: path, values: map[string]string{}}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if section != "" {
			k = section + "/" + k
		}
		s.values[k] = v
	}
	return s, sc.Err()
}

// Path is the file this store came from.
func (s *Store) Path() string { return s.path }

// Get returns the value for key, or def.
func (s *Store) Get(key, def string) string {
	if v, ok := s.values[key]; ok {
		return v
	}
	return def
}

// Set stores key.
func (s *Store) Set(key, value string) { s.values[key] = value }

// Delete removes key.
func (s *Store) Delete(key string) { delete(s.values, key) }

// Save writes the file atomically, sections and keys sorted.
func (s *Store) Save() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	sections := map[string][]string{}
	for k := range s.values {
		sec, key := "", k
		if i := strings.IndexByte(k, '/'); i >= 0 {
			sec, key = k[:i], k[i+1:]
		}
		sections[sec] = append(sections[sec], key)
	}
	names := make([]string, 0, len(sections))
	for sec := range sections {
		names = append(names, sec)
	}
	sort.Strings(names)
	for _, sec := range names {
		if sec != "" {
			b.WriteString("[" + sec + "]\n")
		}
		keys := sections[sec]
		sort.Strings(keys)
		for _, key := range keys {
			full := key
			if sec != "" {
				full = sec + "/" + key
			}
			b.WriteString(key + "=" + s.values[full] + "\n")
		}
		if sec != "" {
			b.WriteString("\n")
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, s.path)
}
