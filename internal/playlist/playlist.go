// Package playlist reads and writes GridPlayer's JSON playlist format v1.
//
// It preserves unknown settings and per-video fields so a file written by
// the Python app survives a load/save round trip even when this Go build
// does not understand every field yet.
package playlist

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

const (
	FormatID      = "gridplayer-playlist"
	FormatVersion = 1
)

// Document is a .gpls playlist.
type Document struct {
	Format   string                     `json:"format"`
	Version  int                        `json:"version"`
	Videos   []Video                    `json:"videos"`
	Settings map[string]json.RawMessage `json:"settings,omitempty"`
}

// Video is one cell. Unknown fields are kept in Extra.
type Video struct {
	ID              string                     `json:"id,omitempty"`
	URI             string                     `json:"uri"`
	Title           string                     `json:"title,omitempty"`
	CurrentPosition int                        `json:"current_position,omitempty"`
	IsMuted         bool                       `json:"is_muted,omitempty"`
	Volume          float64                    `json:"volume,omitempty"`
	Rate            float64                    `json:"rate,omitempty"`
	CellViewMode    string                     `json:"cell_view_mode,omitempty"`
	PlaybackState   string                     `json:"playback_state,omitempty"`
	ListIndex       int                        `json:"list_index,omitempty"`
	ListEnd         string                     `json:"list_end,omitempty"`
	EndAction       string                     `json:"end_action,omitempty"`
	ListFail        string                     `json:"list_fail,omitempty"`
	ListFailWait    *int                       `json:"list_fail_wait,omitempty"`
	AudioDelay      int64                      `json:"audio_delay_ms,omitempty"`
	SubtitleDelay   int64                      `json:"subtitle_delay_ms,omitempty"`
	LoopStart       int64                      `json:"loop_start,omitempty"`
	LoopEnd         int64                      `json:"loop_end,omitempty"`
	Transform       string                     `json:"transform,omitempty"`
	SponsorMode     string                     `json:"sponsor_mode,omitempty"`
	SponsorModes    map[string]string          `json:"sponsor_modes,omitempty"`
	Extra           map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON keeps fields this build does not know.
func (v *Video) UnmarshalJSON(data []byte) error {
	type alias Video
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*v = Video(a)
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	for _, k := range []string{"id", "uri", "title", "current_position", "is_muted",
		"volume", "rate", "cell_view_mode", "playback_state", "list_index", "list_end",
		"list_fail", "list_fail_wait", "end_action", "audio_delay_ms",
		"subtitle_delay_ms", "loop_start", "loop_end", "transform",
		"sponsor_mode", "sponsor_modes"} {
		delete(all, k)
	}
	if len(all) > 0 {
		v.Extra = all
	}
	return nil
}

// MarshalJSON merges Extra back in.
func (v Video) MarshalJSON() ([]byte, error) {
	type alias Video
	b, err := json.Marshal(alias(v))
	if err != nil {
		return nil, err
	}
	if len(v.Extra) == 0 {
		return b, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for k, raw := range v.Extra {
		if _, ok := m[k]; !ok {
			m[k] = raw
		}
	}
	return json.Marshal(m)
}

// Load reads a .gpls file.
func Load(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data, filepath.Dir(path))
}

// Parse reads JSON playlist text; relative URIs resolve against baseDir.
func Parse(data []byte, baseDir string) (*Document, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	var d Document
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("playlist: %w", err)
	}
	if d.Format != FormatID {
		return nil, errors.New("playlist: not a GridPlayer playlist")
	}
	if d.Version == 0 {
		d.Version = 1
	}
	if d.Version < 1 || d.Version > FormatVersion {
		return nil, fmt.Errorf("playlist: unsupported version %d", d.Version)
	}
	for i := range d.Videos {
		d.Videos[i].URI = ResolveURI(d.Videos[i].URI, baseDir)
	}
	return &d, nil
}

// ResolveURI turns a file URI or relative path into an absolute path;
// network URLs pass through unchanged.
func ResolveURI(uri, baseDir string) string {
	if uri == "" {
		return ""
	}
	if u, err := url.Parse(uri); err == nil && u.Scheme != "" {
		if u.Scheme == "file" {
			p, err := url.PathUnescape(u.Path)
			if err == nil {
				return p
			}
		}
		return uri
	}
	if filepath.IsAbs(uri) || baseDir == "" {
		return uri
	}
	return filepath.Join(baseDir, uri)
}

// Save writes the document atomically as UTF-8 JSON v1.
func (d *Document) Save(path string) error {
	d.Format = FormatID
	d.Version = FormatVersion
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".gpls-*")
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
	return os.Rename(name, path)
}

// GridState reads our grid subset from settings ("grid_state").
func (d *Document) GridState() (mode string, rows, cols int) {
	raw, ok := d.Settings["grid_state"]
	if !ok {
		return "", 0, 0
	}
	var s struct {
		Mode string `json:"mode"`
		Rows int    `json:"rows"`
		Cols int    `json:"cols"`
	}
	_ = json.Unmarshal(raw, &s)
	return s.Mode, s.Rows, s.Cols
}

// SetGridState writes the grid subset, preserving other grid fields.
func (d *Document) SetGridState(mode string, rows, cols int) {
	if d.Settings == nil {
		d.Settings = map[string]json.RawMessage{}
	}
	m := map[string]json.RawMessage{}
	if raw, ok := d.Settings["grid_state"]; ok {
		_ = json.Unmarshal(raw, &m)
	}
	m["mode"], _ = json.Marshal(mode)
	m["rows"], _ = json.Marshal(rows)
	m["cols"], _ = json.Marshal(cols)
	d.Settings["grid_state"], _ = json.Marshal(m)
}
