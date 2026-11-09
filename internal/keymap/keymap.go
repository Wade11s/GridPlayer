// Package keymap parses and stores GridPlayer's keyboard shortcuts.
package keymap

import (
	"errors"
	"fmt"
	"strings"
)

// Binding is a normalized shortcut such as "space", "left" or
// "cmd+shift+p". An empty binding means the action has no shortcut.
type Binding struct {
	Mods []string
	Key  string
}

// Empty reports whether the action has no shortcut.
func (b Binding) Empty() bool { return b.Key == "" }

// String returns the settings form, or "none" for an empty binding.
func (b Binding) String() string {
	if b.Empty() {
		return "none"
	}
	parts := append([]string(nil), b.Mods...)
	parts = append(parts, b.Key)
	return strings.Join(parts, "+")
}

// Display returns a human-readable form for the settings UI.
func (b Binding) Display() string {
	if b.Empty() {
		return ""
	}
	var parts []string
	for _, m := range b.Mods {
		switch m {
		case "cmd":
			parts = append(parts, "Cmd")
		case "shift":
			parts = append(parts, "Shift")
		case "ctrl":
			parts = append(parts, "Ctrl")
		case "alt":
			parts = append(parts, "Alt")
		}
	}
	parts = append(parts, displayKey(b.Key))
	return strings.Join(parts, "+")
}

// Accelerator returns a MyGo menu accelerator string.
func (b Binding) Accelerator() string {
	if b.Empty() {
		return ""
	}
	var parts []string
	for _, m := range b.Mods {
		switch m {
		case "cmd":
			parts = append(parts, "CmdOrCtrl")
		case "shift":
			parts = append(parts, "Shift")
		case "ctrl":
			parts = append(parts, "Ctrl")
		case "alt":
			parts = append(parts, "Alt")
		}
	}
	parts = append(parts, displayKey(b.Key))
	return strings.Join(parts, "+")
}

// Parse normalizes a shortcut.
func Parse(s string) (Binding, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return Binding{}, errors.New("empty shortcut")
	}
	if s == "none" {
		return Binding{}, nil
	}
	parts := strings.Split(s, "+")
	var b Binding
	seen := map[string]bool{}
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return Binding{}, errors.New("empty shortcut part")
		}
		if i == len(parts)-1 {
			key, ok := normalizeKey(part)
			if !ok {
				return Binding{}, fmt.Errorf("unknown key %q", part)
			}
			b.Key = key
			continue
		}
		mod, ok := normalizeMod(part)
		if !ok {
			return Binding{}, fmt.Errorf("unknown modifier %q", part)
		}
		if seen[mod] {
			return Binding{}, fmt.Errorf("duplicate modifier %q", mod)
		}
		seen[mod] = true
	}
	if b.Key == "" {
		return Binding{}, errors.New("missing key")
	}
	for _, mod := range []string{"cmd", "shift", "ctrl", "alt"} {
		if seen[mod] {
			b.Mods = append(b.Mods, mod)
		}
	}
	return b, nil
}

func normalizeMod(s string) (string, bool) {
	switch s {
	case "cmd", "command", "cmdorctrl", "meta", "super":
		return "cmd", true
	case "shift":
		return "shift", true
	case "ctrl", "control":
		return "ctrl", true
	case "alt", "option", "opt":
		return "alt", true
	default:
		return "", false
	}
}

func normalizeKey(s string) (string, bool) {
	switch s {
	case "space", "spacebar":
		return "space", true
	case "left", "arrowleft":
		return "left", true
	case "right", "arrowright":
		return "right", true
	case "up", "arrowup":
		return "up", true
	case "down", "arrowdown":
		return "down", true
	case "enter", "return":
		return "enter", true
	case "esc", "escape":
		return "escape", true
	case "backspace":
		return "backspace", true
	case "delete", "del":
		return "delete", true
	case "tab":
		return "tab", true
	case "home":
		return "home", true
	case "end":
		return "end", true
	case "pageup":
		return "pageup", true
	case "pagedown":
		return "pagedown", true
	}
	if len(s) == 1 {
		c := s[0]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			strings.ContainsRune("[]-=,./;'\\`", rune(c)) {
			return s, true
		}
	}
	if len(s) == 2 && s[0] == 'f' && s[1] >= '1' && s[1] <= '9' {
		return s, true
	}
	if s == "f10" || s == "f11" || s == "f12" {
		return s, true
	}
	return "", false
}

func displayKey(k string) string {
	switch k {
	case "space":
		return "Space"
	case "left":
		return "Left"
	case "right":
		return "Right"
	case "up":
		return "Up"
	case "down":
		return "Down"
	case "enter":
		return "Enter"
	case "escape":
		return "Escape"
	case "backspace":
		return "Backspace"
	case "delete":
		return "Delete"
	case "tab":
		return "Tab"
	case "home":
		return "Home"
	case "end":
		return "End"
	case "pageup":
		return "PageUp"
	case "pagedown":
		return "PageDown"
	}
	if len(k) == 1 {
		return strings.ToUpper(k)
	}
	return strings.ToUpper(k[:1]) + k[1:]
}
