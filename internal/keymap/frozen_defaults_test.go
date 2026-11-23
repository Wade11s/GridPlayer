package keymap

import "testing"

// Values are independently anchored to frozen params/actions.py (0.5.5).
func TestFrozenDefaultBindings(t *testing.T) {
	m := Default()
	want := map[string]string{
		"play_pause":                 "ctrl+space",
		"play_pause_all":             "space",
		"seek_back":                  "ctrl+left",
		"seek_forward":               "ctrl+right",
		"seek_percent_back":          "left",
		"seek_percent_forward":       "right",
		"seek_percent_back_large":    "[",
		"seek_percent_forward_large": "]",
		"seek_percent_back_huge":     ";",
		"seek_percent_forward_huge":  "'",
		"rate_down":                  "none",
		"rate_up":                    "none",
		"rate_increase":              "c",
		"rate_decrease":              "x",
		"rate_reset":                 "z",
		"zoom_in":                    "+",
		"zoom_out":                   "-",
		"zoom_reset":                 "*",
		"crop_top_less":              "p",
		"crop_bottom_less":           "alt+p",
		"previous_file_all":          "shift+pageup",
		"next_file_all":              "shift+pagedown",
	}
	for action, expected := range want {
		if got := m.Get(action).String(); got != expected {
			t.Errorf("%s=%q, want %q", action, got, expected)
		}
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("frozen defaults contain duplicate bindings: %v", err)
	}
}

func TestLoadMigratesOldGoDefaultsOnce(t *testing.T) {
	old := map[string]string{
		"keymap/play_pause":       "space",
		"keymap/seek_back":        "left",
		"keymap/seek_forward":     "right",
		"keymap/rate_down":        "[",
		"keymap/rate_up":          "]",
		"keymap/zoom_in":          "shift+=",
		"keymap/zoom_out":         "shift+-",
		"keymap/zoom_reset":       "shift+`",
		"keymap/crop_top_less":    "alt+p",
		"keymap/crop_bottom_less": "q",
	}
	m := Load(func(key string) string { return old[key] })
	for action, expected := range map[string]string{
		"play_pause": "ctrl+space", "play_pause_all": "space",
		"seek_back": "ctrl+left", "seek_forward": "ctrl+right",
		"rate_down": "none", "rate_up": "none",
		"seek_percent_back_large": "[", "seek_percent_forward_large": "]",
		"zoom_in": "+", "zoom_out": "-", "zoom_reset": "*",
		"crop_top_less": "p", "crop_bottom_less": "alt+p",
	} {
		if got := m.Get(action).String(); got != expected {
			t.Errorf("migrated %s=%q, want %q", action, got, expected)
		}
	}
}

func TestSchemaTwoPreservesIntentionalCustomValues(t *testing.T) {
	m := Load(func(key string) string {
		switch key {
		case "keymap/schema":
			return "2"
		case "keymap/play_pause":
			return "p"
		case "keymap/zoom_in":
			return "shift+="
		default:
			return ""
		}
	})
	if got := m.Get("play_pause").String(); got != "p" {
		t.Fatalf("custom play binding %q", got)
	}
	if got := m.Get("zoom_in").String(); got != "shift+=" {
		t.Fatalf("schema-2 custom zoom binding %q", got)
	}
}
