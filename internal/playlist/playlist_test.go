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
	mode, rows, cols, preallocate := d.GridState()
	if mode != "fixed" || rows != 2 || cols != 3 || preallocate {
		t.Fatalf("grid %q %d %d %v", mode, rows, cols, preallocate)
	}
	d.SetGridState("fixed", 2, 3, true)
	if _, _, _, preallocate := d.GridState(); !preallocate {
		t.Fatal("preallocate did not round-trip")
	}
	d.SetGridState("fixed", 2, 3, false)
	if _, _, _, preallocate := d.GridState(); preallocate {
		t.Fatal("clearing preallocate did not round-trip")
	}
	d.SetGridState("auto_rows", 0, 0, false)
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
	mode2, _, _, _ := d2.GridState()
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
	for _, want := range []string{`"list_index":2`, `"list_end_action":"start_over"`, `"list_end":"next"`, `"list_fail_action":"show_error"`, `"list_fail_wait_sec":0`, `"audio_delay_ms":-450`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %s in %s", want, raw)
		}
	}
}

func TestSponsorOverridesRoundTrip(t *testing.T) {
	src := `{"format":"gridplayer-playlist","videos":[{` +
		`"uri":"https://youtu.be/dQw4w9WgXcQ","sponsor_mode":"off",` +
		`"sponsor_modes":{"intro":"skip","selfpromo":"off"},` +
		`"sponsor_url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ"}]}`
	d, err := Parse([]byte(src), "")
	if err != nil {
		t.Fatal(err)
	}
	v := d.Videos[0]
	if v.SponsorMode != "off" {
		t.Fatalf("mode %q", v.SponsorMode)
	}
	if v.SponsorModes["intro"] != "skip" || v.SponsorModes["selfpromo"] != "off" {
		t.Fatalf("modes %+v", v.SponsorModes)
	}
	if v.SponsorURL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("url %q", v.SponsorURL)
	}
	if len(v.Extra) != 0 {
		t.Fatalf("known fields leaked into extra: %+v", v.Extra)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"sponsor_mode":"off"`, `"intro":"skip"`, `"selfpromo":"off"`,
		`"sponsor_url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %s in %s", want, raw)
		}
	}
}

func TestViewShapeFieldsRoundTrip(t *testing.T) {
	src := `{"format":"gridplayer-playlist","videos":[{` +
		`"uri":"a.mp4","aspect_ratio":"4:3","scale":1.5,"crop_ratio":"2.35:1"}]}`
	d, err := Parse([]byte(src), "")
	if err != nil {
		t.Fatal(err)
	}
	v := d.Videos[0]
	if v.AspectRatio == nil || *v.AspectRatio != "4:3" ||
		v.Scale == nil || *v.Scale != 1.5 ||
		v.CropRatio == nil || *v.CropRatio != "2.35:1" {
		t.Fatalf("view shape %+v", v)
	}
	if len(v.Extra) != 0 {
		t.Fatalf("known fields leaked into extra: %+v", v.Extra)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"aspect_ratio":"4:3"`, `"scale":1.5`, `"crop_ratio":"2.35:1"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %s in %s", want, raw)
		}
	}
}

func TestCropMarginsRoundTrip(t *testing.T) {
	src := `{"format":"gridplayer-playlist","videos":[{` +
		`"uri":"a.mp4","crop_margins":[10,20,30,40]}]}`
	d, err := Parse([]byte(src), "")
	if err != nil {
		t.Fatal(err)
	}
	v := d.Videos[0]
	if len(v.CropMargins) != 4 || v.CropMargins[0] != 10 || v.CropMargins[3] != 40 {
		t.Fatalf("margins %+v", v.CropMargins)
	}
	if len(v.Extra) != 0 {
		t.Fatalf("known field leaked into extra: %+v", v.Extra)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"crop_margins":[10,20,30,40]`) {
		t.Fatalf("missing crop margins in %s", raw)
	}
}

func TestExplicitEmptyViewShapeSurvivesJSON(t *testing.T) {
	src := `{"format":"gridplayer-playlist","videos":[{` +
		`"uri":"a.mp4","aspect_ratio":"","scale":0,"crop_ratio":""}]}`
	d, err := Parse([]byte(src), "")
	if err != nil {
		t.Fatal(err)
	}
	v := d.Videos[0]
	if v.AspectRatio == nil || *v.AspectRatio != "" {
		t.Fatalf("aspect presence lost: %+v", v.AspectRatio)
	}
	if v.Scale == nil || *v.Scale != 0 {
		t.Fatalf("scale presence lost: %+v", v.Scale)
	}
	if v.CropRatio == nil || *v.CropRatio != "" {
		t.Fatalf("crop presence lost: %+v", v.CropRatio)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"aspect_ratio":""`, `"scale":0`, `"crop_ratio":""`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("explicit field missing %s in %s", want, raw)
		}
	}
}

func TestUpstreamWholeFileLoopPresence(t *testing.T) {
	d, err := Parse([]byte(`{"format":"gridplayer-playlist","version":1,"videos":[{"uri":"a.mp4","loop_start":0}]}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if v := d.Videos[0]; v.LoopActive == nil || !*v.LoopActive {
		t.Fatalf("loop_start presence lost: %+v", v.LoopActive)
	}
	d, err = Parse([]byte(`{"format":"gridplayer-playlist","version":1,"videos":[{"uri":"a.mp4"}]}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if v := d.Videos[0]; v.LoopActive != nil {
		t.Fatal("absent loop_start became active")
	}
}

func TestLoopActiveResetDropsExtension(t *testing.T) {
	d, err := Parse([]byte(`{"format":"gridplayer-playlist","version":1,"videos":[{"uri":"a.mp4","loop_active":true}]}`), "")
	if err != nil {
		t.Fatal(err)
	}
	v := d.Videos[0]
	if v.LoopActive == nil || !*v.LoopActive {
		t.Fatalf("loop_active not parsed: %+v", v.LoopActive)
	}
	v.LoopActive = nil
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "loop_active") {
		t.Fatalf("resetting kept the extension: %s", raw)
	}
}

func TestAnchorAndCropRoundTrip(t *testing.T) {
	// Upstream AutoName serializes TOP_LEFT as top_left.
	src := `{"format":"gridplayer-playlist","videos":[{` +
		`"uri":"a.mp4","anchor":"top_left","crop":[10,20,30,40],"crop_margins":[1,2,3,4]}]}`
	d, err := Parse([]byte(src), "")
	if err != nil {
		t.Fatal(err)
	}
	v := d.Videos[0]
	if v.Anchor != "top_left" {
		t.Fatalf("anchor %q", v.Anchor)
	}
	if len(v.Crop) != 4 || v.Crop[0] != 10 || v.Crop[3] != 40 {
		t.Fatalf("upstream crop %+v", v.Crop)
	}
	if len(v.CropMargins) != 4 || v.CropMargins[0] != 1 {
		t.Fatalf("legacy crop_margins %+v", v.CropMargins)
	}
	if len(v.Extra) != 0 {
		t.Fatalf("known fields leaked into extra: %+v", v.Extra)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"anchor":"top_left"`, `"crop":[10,20,30,40]`, `"crop_margins":[1,2,3,4]`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %s in %s", want, raw)
		}
	}
}

// Upstream 0.5.5 serializes these list policies with _action/_sec suffixes.
// Older Go documents used shorter names; both must load, while new writes
// use the upstream names that a Python round trip can consume directly.
func TestUpstreamListPolicyFieldNames(t *testing.T) {
	src := `{"format":"gridplayer-playlist","videos":[{` +
		`"uri":"list.m3u","list_end_action":"stop",` +
		`"list_fail_action":"show_error","list_fail_wait_sec":5}]}`
	d, err := Parse([]byte(src), "")
	if err != nil {
		t.Fatal(err)
	}
	v := d.Videos[0]
	if v.ListEnd != "stop" || v.ListFail != "show_error" ||
		v.ListFailWait == nil || *v.ListFailWait != 5 {
		t.Fatalf("upstream fields %+v", v)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"list_end_action":"stop"`,
		`"list_fail_action":"show_error"`,
		`"list_fail_wait_sec":5`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %s in %s", want, raw)
		}
	}
	for _, gone := range []string{`"list_end":`, `"list_fail":`, `"list_fail_wait":`} {
		if strings.Contains(string(raw), gone) {
			t.Fatalf("legacy field %s remains in %s", gone, raw)
		}
	}
}

func TestLegacyListPolicyFieldNamesStillLoad(t *testing.T) {
	src := `{"format":"gridplayer-playlist","videos":[{` +
		`"uri":"list.m3u","list_end":"next",` +
		`"list_fail":"play_next","list_fail_wait":0}]}`
	d, err := Parse([]byte(src), "")
	if err != nil {
		t.Fatal(err)
	}
	v := d.Videos[0]
	if v.ListEnd != "next" || v.ListFail != "play_next" ||
		v.ListFailWait == nil || *v.ListFailWait != 0 {
		t.Fatalf("legacy fields %+v", v)
	}
	if len(v.Extra) != 0 {
		t.Fatalf("legacy fields leaked into extra: %+v", v.Extra)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"list_end_action":"start_over"`,
		`"list_end":"next"`, // Go-only Next Cell behavior survives for Go.
		`"list_fail_action":"play_next"`,
		`"list_fail_wait_sec":0`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %s in %s", want, raw)
		}
	}
}
