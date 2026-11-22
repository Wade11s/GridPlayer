package main

import (
	"reflect"
	"testing"

	"gridplayer-go/internal/dummy"
	"gridplayer-go/internal/engine"
	"gridplayer-go/internal/playlist"
)

func TestDocumentPersistsExternalTrackFiles(t *testing.T) {
	a := &app{cells: []*cell{{
		path:              "movie.mp4",
		externalAudio:     []string{"one.m4a", "two.m4a"},
		externalSubtitles: []string{"de.srt"},
	}}}
	v := a.document().Videos[0]
	if !reflect.DeepEqual(v.ExternalAudio, []string{"one.m4a", "two.m4a"}) ||
		!reflect.DeepEqual(v.ExternalSubtitles, []string{"de.srt"}) {
		t.Fatalf("document external files: %+v", v)
	}
}

func TestApplyDocumentReloadsWhenExternalFilesChange(t *testing.T) {
	loads := 0
	eng := engine.New(func(string, bool, string) (engine.Backend, error) {
		loads++
		return dummy.New(), nil
	})
	defer eng.Close()
	if err := eng.Load("movie.mp4"); err != nil {
		t.Fatal(err)
	}
	c := &cell{source: "movie.mp4", path: "movie.mp4", eng: eng}
	a := &app{cells: []*cell{c}, defaultVolume: 80, defaultRate: 1}
	d := &playlist.Document{
		Format: playlist.FormatID, Version: 1,
		Videos: []playlist.Video{{
			URI: "movie.mp4", ExternalAudio: []string{"voice.m4a"},
			ExternalSubtitles: []string{"de.srt"},
		}},
	}
	a.applyDocument(d, "")
	if loads != 2 {
		t.Fatalf("external file change did not reload live engine: loads=%d", loads)
	}
	if !reflect.DeepEqual(c.externalAudio, []string{"voice.m4a"}) ||
		!reflect.DeepEqual(c.externalSubtitles, []string{"de.srt"}) {
		t.Fatalf("restored cell files: audio=%v subtitles=%v", c.externalAudio, c.externalSubtitles)
	}
}

func TestListItemChangeClearsExternalFiles(t *testing.T) {
	eng := engine.New(func(string, bool, string) (engine.Backend, error) {
		return dummy.New(), nil
	})
	defer eng.Close()
	if err := eng.Load("a.mp4"); err != nil {
		t.Fatal(err)
	}
	c := &cell{
		list: []string{"a.mp4", "b.mp4"}, path: "a.mp4", eng: eng,
		externalAudio: []string{"a.m4a"}, externalSubtitles: []string{"a.srt"},
	}
	eng.SetExternalFilesForLoad(c.externalAudio, c.externalSubtitles)
	a := &app{cells: []*cell{c}}
	a.nextList(c)
	if c.path != "b.mp4" || len(c.externalAudio) != 0 || len(c.externalSubtitles) != 0 {
		t.Fatalf("external files leaked onto next item: %+v", c)
	}
	v := a.document().Videos[0]
	if len(v.ExternalAudio) != 0 || len(v.ExternalSubtitles) != 0 {
		t.Fatalf("cleared files persisted on next item: %+v", v)
	}
}
