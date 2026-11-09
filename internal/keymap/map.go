package keymap

import "fmt"

// Actions are the editable shortcuts, in menu order.
var Actions = []string{
	"play_pause",
	"seek_back",
	"seek_forward",
	"volume_up",
	"volume_down",
	"mute",
	"loop",
	"prev_cell",
	"next_cell",
	"rate_down",
	"rate_up",
}

// Labels are the user-facing action names.
var Labels = map[string]string{
	"play_pause":   "Play/Pause",
	"seek_back":    "Seek -5s",
	"seek_forward": "Seek +5s",
	"volume_up":    "Volume up",
	"volume_down":  "Volume down",
	"mute":         "Mute",
	"loop":         "Loop",
	"prev_cell":    "Previous cell",
	"next_cell":    "Next cell",
	"rate_down":    "Rate down",
	"rate_up":      "Rate up",
}

// Map is a set of action bindings.
type Map struct {
	bindings map[string]Binding
}

// Default returns the built-in bindings.
func Default() Map {
	m := Map{bindings: map[string]Binding{}}
	for action, value := range map[string]string{
		"play_pause":   "space",
		"seek_back":    "left",
		"seek_forward": "right",
		"volume_up":    "up",
		"volume_down":  "down",
		"mute":         "m",
		"loop":         "l",
		"prev_cell":    "b",
		"next_cell":    "n",
		"rate_down":    "[",
		"rate_up":      "]",
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
// invalid values.
func Load(get func(string) string) Map {
	m := Default()
	for _, action := range Actions {
		raw := get("keymap/" + action)
		if raw == "" {
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
	for _, action := range Actions {
		set("keymap/"+action, m.Get(action).String())
	}
}
