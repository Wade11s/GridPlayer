// Package bookmarks stores per-file positions in a small JSON file.
package bookmarks

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Bookmark is a named position in a file.
type Bookmark struct {
	Name     string `json:"name"`
	Position int64  `json:"position_ms"`
	Created  int64  `json:"created_unix"`
}

// Store is a file's bookmarks by media path.
type Store struct {
	path  string
	Files map[string][]Bookmark `json:"files"`
}

// Load reads path; a missing file is empty.
func Load(path string) (*Store, error) {
	s := &Store{path: path, Files: map[string][]Bookmark{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	if s.Files == nil {
		s.Files = map[string][]Bookmark{}
	}
	return s, nil
}

// Add appends a bookmark, replacing one with the same name.
func (s *Store) Add(file, name string, pos int64) {
	if pos < 0 {
		pos = 0
	}
	list := s.Files[file]
	for i := range list {
		if list[i].Name == name {
			list[i].Position = pos
			list[i].Created = time.Now().Unix()
			s.Files[file] = list
			return
		}
	}
	list = append(list, Bookmark{Name: name, Position: pos, Created: time.Now().Unix()})
	sort.Slice(list, func(i, j int) bool { return list[i].Position < list[j].Position })
	s.Files[file] = list
}

// List returns a copy of file's bookmarks.
func (s *Store) List(file string) []Bookmark {
	return append([]Bookmark(nil), s.Files[file]...)
}

// Remove deletes a bookmark by name.
func (s *Store) Remove(file, name string) {
	list := s.Files[file]
	out := list[:0]
	for _, b := range list {
		if b.Name != name {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		delete(s.Files, file)
		return
	}
	s.Files[file] = out
}

// Save writes the store atomically.
func (s *Store) Save() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".bookmarks-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
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
