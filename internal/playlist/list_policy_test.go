package playlist

import (
	"encoding/json"
	"testing"
)

func TestListEndJSONUsesUpstreamVocabulary(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		canonical  string
		legacyName bool
	}{
		{name: "Go loop", value: "loop", canonical: "start_over"},
		{name: "upstream start over", value: "start_over", canonical: "start_over"},
		{name: "pause", value: "pause", canonical: "pause"},
		{name: "stop", value: "stop", canonical: "stop"},
		{name: "close", value: "close", canonical: "close"},
		{name: "legacy next cell", value: "next", canonical: "start_over", legacyName: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(Video{URI: "a.mp4", ListEnd: tt.value})
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			var got string
			if err := json.Unmarshal(fields["list_end_action"], &got); err != nil {
				t.Fatalf("canonical field in %s: %v", raw, err)
			}
			if got != tt.canonical {
				t.Fatalf("canonical value %q, want %q (%s)", got, tt.canonical, raw)
			}
			_, hasLegacy := fields["list_end"]
			if hasLegacy != tt.legacyName {
				t.Fatalf("legacy field presence=%v, want %v (%s)", hasLegacy, tt.legacyName, raw)
			}
			if hasLegacy {
				if err := json.Unmarshal(fields["list_end"], &got); err != nil || got != "next" {
					t.Fatalf("legacy value %q, err=%v", got, err)
				}
			}
		})
	}
}

func TestCanonicalListEndPrecedesLegacyAlias(t *testing.T) {
	d, err := Parse([]byte(`{"format":"gridplayer-playlist","videos":[{"uri":"a.mp4","list_end_action":"close","list_end":"next"}]}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Videos[0].ListEnd; got != "close" {
		t.Fatalf("canonical value lost to legacy alias: %q", got)
	}
}

func TestMalformedLegacyListPolicyIsRejected(t *testing.T) {
	src := `{"format":"gridplayer-playlist","videos":[{"uri":"x","list_end":123,"list_fail":true,"list_fail_wait":"9"}]}`
	if _, err := Parse([]byte(src), ""); err == nil {
		t.Fatal("malformed legacy policy was silently converted to zero values")
	}
}
