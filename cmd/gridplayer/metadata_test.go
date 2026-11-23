package main

import (
	"encoding/json"
	"testing"

	"gridplayer-go/internal/playlist"
)

func TestCellMetadataFollowsURIAfterSwapAndInsert(t *testing.T) {
	d := &playlist.Document{
		Format: playlist.FormatID, Version: 1,
		Videos: []playlist.Video{
			{URI: "a.mp4", ID: "id-a", Title: "A", Extra: map[string]json.RawMessage{"custom": json.RawMessage(`"a"`)}},
			{URI: "b.mp4", ID: "id-b", Title: "B", Extra: map[string]json.RawMessage{"custom": json.RawMessage(`"b"`)}},
		},
	}
	a := &app{defaultVolume: 80, defaultRate: 1}
	a.applyDocument(d, "")
	assertMetadata := func(stage string, want map[string][3]string) {
		t.Helper()
		got := a.document()
		if len(got.Videos) != len(want) {
			t.Fatalf("%s videos=%d, want %d", stage, len(got.Videos), len(want))
		}
		for _, v := range got.Videos {
			expected := want[v.URI]
			var custom string
			if raw := v.Extra["custom"]; raw != nil {
				if err := json.Unmarshal(raw, &custom); err != nil {
					t.Fatal(err)
				}
			}
			if v.ID != expected[0] || v.Title != expected[1] || custom != expected[2] {
				t.Errorf("%s uri=%s metadata=(%q,%q,%q), want %v", stage, v.URI, v.ID, v.Title, custom, expected)
			}
		}
	}
	a.swapCells(0, 1)
	assertMetadata("swap", map[string][3]string{
		"a.mp4": {"id-a", "A", "a"}, "b.mp4": {"id-b", "B", "b"},
	})
	a.insertCells(0, []string{"x.mp4"})
	assertMetadata("insert", map[string][3]string{
		"x.mp4": {"", "", ""}, "a.mp4": {"id-a", "A", "a"}, "b.mp4": {"id-b", "B", "b"},
	})
}
