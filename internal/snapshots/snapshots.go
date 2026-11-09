// Package snapshots stores named playlist/grid snapshots in one JSON file.
package snapshots

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gridplayer-go/internal/playlist"
)

// Snapshot is one saved document.
type Snapshot struct {
	Name     string          `json:"name"`
	Saved    int64           `json:"saved_unix"`
	Document json.RawMessage `json:"document"`
}

// Store is the snapshots file.
type Store struct {
	path      string
	Snapshots []Snapshot `json:"snapshots"`
}

// New returns an empty store writing to path.
func New(path string) *Store {
	return &Store{path: path}
}

// Load reads path; a missing file is empty.
func Load(path string) (*Store, error) {
	s := New(path)
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
	if s.Snapshots == nil {
		s.Snapshots = []Snapshot{}
	}
	return s, nil
}

// List returns the snapshots sorted by name.
func (s *Store) List() []Snapshot {
	out := append([]Snapshot(nil), s.Snapshots...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns the document stored under name.
func (s *Store) Get(name string) (*playlist.Document, bool) {
	for _, snap := range s.Snapshots {
		if snap.Name == name {
			var d playlist.Document
			if err := json.Unmarshal(snap.Document, &d); err != nil {
				return nil, false
			}
			return &d, true
		}
	}
	return nil, false
}

// Save stores d under name, replacing an existing snapshot.
func (s *Store) Save(name string, d *playlist.Document) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("snapshot name is empty")
	}
	if d == nil {
		return errors.New("snapshot document is nil")
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	snap := Snapshot{Name: name, Saved: time.Now().Unix(), Document: raw}
	next := append([]Snapshot(nil), s.Snapshots...)
	replaced := false
	for i := range next {
		if next[i].Name == name {
			next[i] = snap
			replaced = true
			break
		}
	}
	if !replaced {
		next = append(next, snap)
	}
	if err := s.writeList(next); err != nil {
		return err
	}
	s.Snapshots = next
	return nil
}

// Remove deletes a snapshot by name.
func (s *Store) Remove(name string) error {
	out := make([]Snapshot, 0, len(s.Snapshots))
	for _, snap := range s.Snapshots {
		if snap.Name != name {
			out = append(out, snap)
		}
	}
	if err := s.writeList(out); err != nil {
		return err
	}
	s.Snapshots = out
	return nil
}

func (s *Store) writeList(list []Snapshot) error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(struct {
		Snapshots []Snapshot `json:"snapshots"`
	}{list}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".snapshots-*")
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
