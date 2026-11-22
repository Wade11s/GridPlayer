package playlist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDocumentPreservesUpstreamTopLevelFields(t *testing.T) {
	src := []byte(`{
		"format":"gridplayer-playlist","version":1,"videos":[],
		"settings":{"grid_state":{"mode":"auto_rows"}},
		"lists":[{"id":"7ca7e0ca-1f7c-4f7f-a06e-e6c0f50b6a5d","items":[{"uri":"a.mp4"}]}],
		"bookmarks":[{"uri":"a.mp4","bookmarks":[{"name":"one","position":1000}]}],
		"snapshots":{"1":{"grid_state":{"mode":"fixed"},"videos":[]}},
		"window_state":"normal",
		"custom_top":{"keep":true}
	}`)
	d, err := Parse(src, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var original, saved map[string]json.RawMessage
	if err := json.Unmarshal(src, &original); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"lists", "bookmarks", "snapshots", "window_state", "custom_top"} {
		if !reflect.DeepEqual(original[key], saved[key]) {
			t.Errorf("%s changed on round trip:\noriginal=%s\nsaved=%s", key, original[key], saved[key])
		}
	}
}

func TestExternalTrackFilesResolveAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := `{"format":"gridplayer-playlist","videos":[{` +
		`"uri":"movie.mp4","external_audio":["audio/a.m4a"],` +
		`"external_subtitles":["subs/de.srt"],` +
		`"audio_selection":{"kind":"external","file":"audio/a.m4a","track":0},` +
		`"subtitle_selection":{"kind":"external","file":"subs/de.srt","track":0}}]}`
	d, err := Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	v := d.Videos[0]
	if !reflect.DeepEqual(v.ExternalAudio, []string{filepath.Join(dir, "audio/a.m4a")}) ||
		!reflect.DeepEqual(v.ExternalSubtitles, []string{filepath.Join(dir, "subs/de.srt")}) {
		t.Fatalf("resolved external files: audio=%v subtitles=%v", v.ExternalAudio, v.ExternalSubtitles)
	}
	path := filepath.Join(dir, "saved.gpls")
	if err := d.Save(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Videos []struct {
			ExternalAudio     []string        `json:"external_audio"`
			ExternalSubtitles []string        `json:"external_subtitles"`
			AudioSelection    json.RawMessage `json:"audio_selection"`
			SubtitleSelection json.RawMessage `json:"subtitle_selection"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	got := out.Videos[0]
	if !reflect.DeepEqual(got.ExternalAudio, v.ExternalAudio) ||
		!reflect.DeepEqual(got.ExternalSubtitles, v.ExternalSubtitles) {
		t.Fatalf("saved external files: %+v", got)
	}
	if len(got.AudioSelection) == 0 || len(got.SubtitleSelection) == 0 {
		t.Fatalf("selection fields were dropped: %s", raw)
	}
}
