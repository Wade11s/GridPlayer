package playlist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTripPreservesUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.gpls")
	src := `{
  "format": "gridplayer-playlist",
  "version": 1,
  "videos": [
    {"id":"x","uri":"a.mp4","current_position":3000,"custom_field":{"k":1},"is_muted":true}
  ],
  "settings": {"grid_state":{"mode":"fixed","rows":2,"cols":3,"custom":true},"other":"keep"}
}`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Videos[0].URI; got != filepath.Join(dir, "a.mp4") {
		t.Fatalf("resolved uri %q", got)
	}
	if !d.Videos[0].IsMuted || d.Videos[0].CurrentPosition != 3000 {
		t.Fatalf("video %+v", d.Videos[0])
	}
	mode, rows, cols := d.GridState()
	if mode != "fixed" || rows != 2 || cols != 3 {
		t.Fatalf("grid %q %d %d", mode, rows, cols)
	}
	d.SetGridState("auto_rows", 0, 0)
	if err := d.Save(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"custom_field"`) ||
		!strings.Contains(string(raw), `"other"`) ||
		!strings.Contains(string(raw), `"custom"`) {
		t.Fatalf("unknown fields lost:\n%s", raw)
	}
	d2, err := Parse(raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	mode2, _, _ := d2.GridState()
	if mode2 != "auto_rows" {
		t.Fatalf("grid mode after save %q", mode2)
	}
}

func TestValidationAndRelativePaths(t *testing.T) {
	if _, err := Parse([]byte(`{"format":"nope","videos":[]}`), ""); err == nil {
		t.Fatal("want invalid format error")
	}
	if _, err := Parse([]byte(`{"format":"gridplayer-playlist","version":2}`), ""); err == nil {
		t.Fatal("want unsupported version error")
	}
	d, err := Parse([]byte(`{"format":"gridplayer-playlist","videos":[{"uri":"sub/a.mp4"},{"uri":"https://x/y.m3u8"},{"uri":"file:///tmp/b.mp4"}]}`), "/base")
	if err != nil {
		t.Fatal(err)
	}
	if d.Version != 1 {
		t.Fatalf("default version %d", d.Version)
	}
	if d.Videos[0].URI != "/base/sub/a.mp4" {
		t.Fatalf("relative %q", d.Videos[0].URI)
	}
	if d.Videos[1].URI != "https://x/y.m3u8" || d.Videos[2].URI != "/tmp/b.mp4" {
		t.Fatalf("uris %q %q", d.Videos[1].URI, d.Videos[2].URI)
	}
}

func TestListItemFieldsRoundTrip(t *testing.T) {
	src := `{"format":"gridplayer-playlist","videos":[{` +
		`"uri":"list.m3u","list_index":2,"list_end":"next",` +
		`"list_fail":"show_error","list_fail_wait":0,"audio_delay_ms":-450,` +
		`"cell_view_mode":"stretch","playback_state":"paused"}]}`
	d, err := Parse([]byte(src), "")
	if err != nil {
		t.Fatal(err)
	}
	v := d.Videos[0]
	if v.ListIndex != 2 || v.ListEnd != "next" ||
		v.CellViewMode != "stretch" || v.PlaybackState != "paused" ||
		v.ListFail != "show_error" || v.ListFailWait == nil || *v.ListFailWait != 0 || v.AudioDelay != -450 {
		t.Fatalf("video %+v", v)
	}
	if len(v.Extra) != 0 {
		t.Fatalf("known fields leaked into extra: %+v", v.Extra)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"list_index":2`, `"list_end":"next"`, `"list_fail":"show_error"`, `"list_fail_wait":0`, `"audio_delay_ms":-450`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %s in %s", want, raw)
		}
	}
}
