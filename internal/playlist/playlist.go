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
	Extra    map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON retains upstream lists, bookmarks, snapshots, window state,
// and future top-level fields even when this build does not interpret them.
func (d *Document) UnmarshalJSON(data []byte) error {
	type alias Document
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*d = Document(a)
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	for _, key := range []string{"format", "version", "videos", "settings"} {
		delete(all, key)
	}
	if len(all) > 0 {
		d.Extra = all
	}
	return nil
}

// MarshalJSON merges preserved top-level fields back into the document.
func (d Document) MarshalJSON() ([]byte, error) {
	type alias Document
	b, err := json.Marshal(alias(d))
	if err != nil {
		return nil, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, err
	}
	for key, raw := range d.Extra {
		if _, exists := all[key]; !exists {
			all[key] = raw
		}
	}
	return json.Marshal(all)
}

// Video is one cell. Unknown fields are kept in Extra.
type Video struct {
	ID                string                     `json:"id,omitempty"`
	URI               string                     `json:"uri"`
	Title             string                     `json:"title,omitempty"`
	CurrentPosition   int                        `json:"current_position,omitempty"`
	IsMuted           bool                       `json:"is_muted,omitempty"`
	Volume            float64                    `json:"volume,omitempty"`
	Rate              float64                    `json:"rate,omitempty"`
	CellViewMode      string                     `json:"cell_view_mode,omitempty"`
	AspectRatio       *string                    `json:"aspect_ratio,omitempty"`
	Scale             *float64                   `json:"scale,omitempty"`
	CropRatio         *string                    `json:"crop_ratio,omitempty"`
	PlaybackState     string                     `json:"playback_state,omitempty"`
	ListIndex         int                        `json:"list_index,omitempty"`
	ListEnd           string                     `json:"list_end_action,omitempty"`
	EndAction         string                     `json:"end_action,omitempty"`
	ListFail          string                     `json:"list_fail_action,omitempty"`
	ListFailWait      *int                       `json:"list_fail_wait_sec,omitempty"`
	ExternalAudio     []string                   `json:"external_audio,omitempty"`
	ExternalSubtitles []string                   `json:"external_subtitles,omitempty"`
	AudioDelay        int64                      `json:"audio_delay_ms,omitempty"`
	SubtitleDelay     int64                      `json:"subtitle_delay_ms,omitempty"`
	LoopStart         int64                      `json:"loop_start,omitempty"`
	LoopEnd           int64                      `json:"loop_end,omitempty"`
	Transform         string                     `json:"transform,omitempty"`
	SponsorMode       string                     `json:"sponsor_mode,omitempty"`
	SponsorModes      map[string]string          `json:"sponsor_modes,omitempty"`
	SponsorURL        string                     `json:"sponsor_url,omitempty"`
	Anchor            string                     `json:"anchor,omitempty"`
	Crop              []float64                  `json:"crop,omitempty"`
	CropMargins       []float64                  `json:"crop_margins,omitempty"`
	Extra             map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON keeps fields this build does not know.
func (v *Video) UnmarshalJSON(data []byte) error {
	type alias Video
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*v = Video(a)
	// Older Go builds wrote these fields without upstream's action/sec
	// suffixes. Read both; a canonical field wins whenever it is present.
	var legacy struct {
		ListEnd      string `json:"list_end"`
		ListFail     string `json:"list_fail"`
		ListFailWait *int   `json:"list_fail_wait"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	if _, present := all["list_end_action"]; !present {
		v.ListEnd = legacy.ListEnd
	} else if v.ListEnd == "start_over" && legacy.ListEnd == "next" {
		// Our dual-field Next Cell representation uses start_over only as
		// upstream's fallback; Go restores the extension on the next load.
		v.ListEnd = legacy.ListEnd
	}
	if _, present := all["list_fail_action"]; !present {
		v.ListFail = legacy.ListFail
	}
	if _, present := all["list_fail_wait_sec"]; !present {
		v.ListFailWait = legacy.ListFailWait
	}
	for _, k := range []string{"id", "uri", "title", "current_position", "is_muted",
		"volume", "rate", "cell_view_mode", "playback_state", "list_index", "list_end",
		"list_end_action", "list_fail", "list_fail_wait", "list_fail_action",
		"list_fail_wait_sec", "aspect_ratio", "scale", "crop_ratio",
		"end_action", "audio_delay_ms", "external_audio", "external_subtitles",
		"subtitle_delay_ms", "loop_start", "loop_end", "transform",
		"sponsor_mode", "sponsor_modes", "sponsor_url", "anchor", "crop", "crop_margins"} {
		delete(all, k)
	}
	if len(all) > 0 {
		v.Extra = all
	}
	return nil
}

// listEndJSONFields maps the old Go values to upstream's ListEndAction
// vocabulary. Next Cell is a Go-only extension: upstream gets its safe
// start_over fallback while the old key preserves the behavior for Go.
func listEndJSONFields(value string) (canonical, legacy string) {
	switch value {
	case "loop", "start_over":
		return "start_over", ""
	case "next":
		return "start_over", "next"
	default:
		// pause/stop/close are already upstream names. Unknown values are
		// echoed rather than silently replaced by a value we do not know.
		return value, ""
	}
}

// MarshalJSON merges Extra back in and writes upstream-valid list actions.
func (v Video) MarshalJSON() ([]byte, error) {
	type alias Video
	b, err := json.Marshal(alias(v))
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	canonical, legacy := listEndJSONFields(v.ListEnd)
	if canonical == "" {
		delete(m, "list_end_action")
	} else {
		m["list_end_action"], _ = json.Marshal(canonical)
	}
	if legacy == "" {
		delete(m, "list_end")
	} else {
		m["list_end"], _ = json.Marshal(legacy)
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
		for j, uri := range d.Videos[i].ExternalAudio {
			d.Videos[i].ExternalAudio[j] = ResolveURI(uri, baseDir)
		}
		for j, uri := range d.Videos[i].ExternalSubtitles {
			d.Videos[i].ExternalSubtitles[j] = ResolveURI(uri, baseDir)
		}
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
