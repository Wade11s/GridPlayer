package keymap

import "fmt"

// SchemaVersion prevents old Go defaults from being migrated repeatedly
// after a user intentionally customizes a key to one of those values.
const SchemaVersion = "2"

var legacyGoDefaults = map[string]string{
	"play_pause":       "space",
	"seek_back":        "left",
	"seek_forward":     "right",
	"rate_down":        "[",
	"rate_up":          "]",
	"zoom_in":          "shift+=",
	"zoom_out":         "shift+-",
	"zoom_reset":       "shift+`",
	"crop_top_less":    "alt+p",
	"crop_bottom_less": "q",
}

// Actions are the editable shortcuts, in menu order.
var Actions = []string{
	"play_pause",
	"play_pause_all",
	"seek_back",
	"seek_forward",
	"seek_percent_back",
	"seek_percent_forward",
	"seek_percent_back_large",
	"seek_percent_forward_large",
	"seek_percent_back_huge",
	"seek_percent_forward_huge",
	"volume_up",
	"volume_down",
	"mute",
	"loop",
	"prev_cell",
	"next_cell",
	"rate_down",
	"rate_up",
	"frame_next",
	"frame_prev",
	"audio_later",
	"audio_earlier",
	"seek_percent_0",
	"seek_percent_1",
	"seek_percent_2",
	"seek_percent_3",
	"seek_percent_4",
	"seek_percent_5",
	"seek_percent_6",
	"seek_percent_7",
	"seek_percent_8",
	"seek_percent_9",
	"seek_random",
	"seek_start",
	"seek_medium_back",
	"seek_long_back",
	"seek_medium_forward",
	"seek_long_forward",
	"loop_start",
	"loop_end",
	"loop_reset",
	"loop_chapter",
	"rate_reset",
	"zoom_in",
	"zoom_out",
	"zoom_reset",
	"crop_left_more",
	"crop_left_less",
	"crop_top_more",
	"crop_top_less",
	"crop_right_more",
	"crop_right_less",
	"crop_bottom_more",
	"crop_bottom_less",
	"crop_reset",
	"shift_left",
	"shift_right",
	"shift_up",
	"shift_down",
	"sub_delay_later",
	"sub_delay_earlier",
	"reload",
	"timecode",
	"take_screenshot",
	"rate_increase",
	"rate_decrease",
	"next_file",
	"previous_file",
	"next_file_all",
	"previous_file_all",
	"crop_off",
}

// Labels are the user-facing action names.
var Labels = map[string]string{
	"play_pause":                 "Play/Pause (active)",
	"play_pause_all":             "Play/Pause (all)",
	"seek_back":                  "Seek -5s",
	"seek_forward":               "Seek +5s",
	"seek_percent_back":          "Seek -1%",
	"seek_percent_forward":       "Seek +1%",
	"seek_percent_back_large":    "Seek -5%",
	"seek_percent_forward_large": "Seek +5%",
	"seek_percent_back_huge":     "Seek -10%",
	"seek_percent_forward_huge":  "Seek +10%",
	"volume_up":                  "Volume up",
	"volume_down":                "Volume down",
	"mute":                       "Mute",
	"loop":                       "Loop",
	"prev_cell":                  "Previous cell",
	"next_cell":                  "Next cell",
	"rate_down":                  "Rate down",
	"rate_up":                    "Rate up",
	"frame_next":                 "Next frame",
	"frame_prev":                 "Previous frame",
	"audio_later":                "Audio later",
	"audio_earlier":              "Audio earlier",
	"seek_percent_0":             "Seek 0%",
	"seek_percent_1":             "Seek 10%",
	"seek_percent_2":             "Seek 20%",
	"seek_percent_3":             "Seek 30%",
	"seek_percent_4":             "Seek 40%",
	"seek_percent_5":             "Seek 50%",
	"seek_percent_6":             "Seek 60%",
	"seek_percent_7":             "Seek 70%",
	"seek_percent_8":             "Seek 80%",
	"seek_percent_9":             "Seek 90%",
	"seek_random":                "Seek random",
	"seek_start":                 "Seek start",
	"seek_medium_back":           "Seek -15s",
	"seek_long_back":             "Seek -30s",
	"seek_medium_forward":        "Seek +15s",
	"seek_long_forward":          "Seek +30s",
	"loop_start":                 "Set Loop Start",
	"loop_end":                   "Set Loop End",
	"loop_reset":                 "Loop Reset",
	"loop_chapter":               "Loop chapter",
	"rate_reset":                 "Reset rate",
	"zoom_in":                    "Zoom in",
	"zoom_out":                   "Zoom out",
	"zoom_reset":                 "Reset zoom",
	"crop_left_more":             "Crop left +",
	"crop_left_less":             "Crop left -",
	"crop_top_more":              "Crop top +",
	"crop_top_less":              "Crop top -",
	"crop_right_more":            "Crop right +",
	"crop_right_less":            "Crop right -",
	"crop_bottom_more":           "Crop bottom +",
	"crop_bottom_less":           "Crop bottom -",
	"crop_reset":                 "Reset crop",
	"shift_left":                 "Move left",
	"shift_right":                "Move right",
	"shift_up":                   "Move up",
	"shift_down":                 "Move down",
	"sub_delay_later":            "Subtitle delay later",
	"sub_delay_earlier":          "Subtitle delay earlier",
	"reload":                     "Reload",
	"timecode":                   "Go to timecode",
	"take_screenshot":            "Take screenshot",
	"rate_increase":              "Faster",
	"rate_decrease":              "Slower",
	"next_file":                  "Next file",
	"previous_file":              "Previous file",
	"next_file_all":              "Next file (all)",
	"previous_file_all":          "Previous file (all)",
	"crop_off":                   "Crop off",
}

// Map is a set of action bindings.
type Map struct {
	bindings map[string]Binding
}

// Default returns the built-in bindings.
func Default() Map {
	m := Map{bindings: map[string]Binding{}}
	for action, value := range map[string]string{
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
		"volume_up":                  "up",
		"volume_down":                "down",
		"mute":                       "m",
		"loop":                       "l",
		"prev_cell":                  "b",
		"next_cell":                  "n",
		"rate_down":                  "none",
		"rate_up":                    "none",
		"frame_next":                 "s",
		"frame_prev":                 "d",
		"audio_later":                "k",
		"audio_earlier":              "j",
		"seek_percent_0":             "0",
		"seek_percent_1":             "1",
		"seek_percent_2":             "2",
		"seek_percent_3":             "3",
		"seek_percent_4":             "4",
		"seek_percent_5":             "5",
		"seek_percent_6":             "6",
		"seek_percent_7":             "7",
		"seek_percent_8":             "8",
		"seek_percent_9":             "9",
		"seek_random":                "r",
		"seek_start":                 "alt+0",
		"seek_medium_back":           "ctrl+[",
		"seek_long_back":             "ctrl+;",
		"seek_medium_forward":        "ctrl+]",
		"seek_long_forward":          "ctrl+'",
		"loop_start":                 ",",
		"loop_end":                   ".",
		"loop_reset":                 "/",
		"rate_reset":                 "z",
		"zoom_in":                    "+",
		"zoom_out":                   "-",
		"zoom_reset":                 "*",
		"crop_left_more":             "u",
		"crop_left_less":             "i",
		"crop_top_more":              "o",
		"crop_top_less":              "p",
		"crop_right_more":            "alt+u",
		"crop_right_less":            "alt+i",
		"crop_bottom_more":           "alt+o",
		"crop_bottom_less":           "alt+p",
		"crop_reset":                 "y",
		"shift_left":                 "alt+left",
		"shift_right":                "alt+right",
		"shift_up":                   "alt+up",
		"shift_down":                 "alt+down",
		"sub_delay_later":            "h",
		"sub_delay_earlier":          "g",
		"reload":                     "f5",
		"timecode":                   "t",
		"take_screenshot":            "alt+s",
		"rate_increase":              "c",
		"rate_decrease":              "x",
		"next_file":                  "pagedown",
		"previous_file":              "pageup",
		"next_file_all":              "shift+pagedown",
		"previous_file_all":          "shift+pageup",
		"crop_off":                   "shift+y",
	} {
		b, err := Parse(value)
		if err != nil {
			panic(err)
		}
		m.bindings[action] = b
	}
	return m
}

// Get returns the binding for action.
func (m Map) Get(action string) Binding {
	if m.bindings == nil {
		return Binding{}
	}
	return m.bindings[action]
}

// Set parses and stores a binding.
func (m *Map) Set(action, value string) error {
	b, err := Parse(value)
	if err != nil {
		return err
	}
	if m.bindings == nil {
		m.bindings = map[string]Binding{}
	}
	m.bindings[action] = b
	return nil
}

// Bindings returns a copy of all bindings.
func (m Map) Bindings() map[string]Binding {
	out := map[string]Binding{}
	for action, b := range m.bindings {
		out[action] = b
	}
	return out
}

// Validate rejects duplicate non-empty bindings.
func (m Map) Validate() error {
	seen := map[string]string{}
	for _, action := range Actions {
		b := m.Get(action)
		if b.Empty() {
			continue
		}
		key := b.String()
		if other, ok := seen[key]; ok {
			return fmt.Errorf("%s and %s both use %s", Labels[other], Labels[action], key)
		}
		seen[key] = action
	}
	return nil
}

// Load reads keymap/<action> from get, keeping defaults for missing or
// invalid values. Pre-schema-2 values equal to an old Go default are skipped
// once so the frozen replacement default can take effect.
func Load(get func(string) string) Map {
	m := Default()
	migrate := get("keymap/schema") != SchemaVersion
	for _, action := range Actions {
		raw := get("keymap/" + action)
		if raw == "" {
			continue
		}
		if migrate && raw == legacyGoDefaults[action] {
			continue
		}
		if err := m.Set(action, raw); err != nil {
			continue
		}
	}
	return m
}

// Save writes every binding through set.
func (m Map) Save(set func(string, string)) {
	set("keymap/schema", SchemaVersion)
	for _, action := range Actions {
		set("keymap/"+action, m.Get(action).String())
	}
}
