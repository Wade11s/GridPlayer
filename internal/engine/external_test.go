package engine

import (
	"reflect"
	"testing"
)

func TestSetExternalFilesForLoadReplacesAttachments(t *testing.T) {
	var backends []*fakeBackend
	e := New(func(string, bool, string) (Backend, error) {
		f := newFake()
		backends = append(backends, f)
		return f, nil
	})
	defer e.Close()
	audio := []string{"/tmp/one.m4a", "/tmp/two.m4a"}
	subtitles := []string{"/tmp/de.srt"}
	if !e.SetExternalFilesForLoad(audio, subtitles) {
		t.Fatal("initial external file set was not a change")
	}
	if err := e.Load("a.mp4"); err != nil {
		t.Fatal(err)
	}
	if got := trackNames(backends[0].audio); !reflect.DeepEqual(got, audio) {
		t.Fatalf("audio attachments %v, want %v", got, audio)
	}
	if got := trackNames(backends[0].subs); !reflect.DeepEqual(got, subtitles) {
		t.Fatalf("subtitle attachments %v, want %v", got, subtitles)
	}
	if e.SetExternalFilesForLoad(audio, subtitles) {
		t.Fatal("identical external file set reported a change")
	}
	if !e.SetExternalFilesForLoad(nil, nil) {
		t.Fatal("clearing external files was not a change")
	}
	if err := e.Load("b.mp4"); err != nil {
		t.Fatal(err)
	}
	if len(backends) != 2 || len(backends[1].audio) != 0 || len(backends[1].subs) != 0 {
		t.Fatalf("external files leaked onto next item: %+v", backends[1])
	}
}

func trackNames(tracks []Track) []string {
	names := make([]string, len(tracks))
	for i, track := range tracks {
		names[i] = track.Name
	}
	return names
}
