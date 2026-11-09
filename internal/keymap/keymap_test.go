package keymap

import "testing"

func TestParseRoundTrip(t *testing.T) {
	cases := map[string]string{
		"space":         "space",
		"left":          "left",
		"m":             "m",
		"cmd+shift+p":   "cmd+shift+p",
		"shift+cmd+p":   "cmd+shift+p",
		"option+space":  "alt+space",
		"control+alt+k": "ctrl+alt+k",
		"[":             "[",
		"f12":           "f12",
		"none":          "none",
	}
	for in, want := range cases {
		b, err := Parse(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got := b.String(); got != want {
			t.Fatalf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, in := range []string{"", "cmd", "cmd+cmd+p", "cmd+unknown", "hyper+p"} {
		if _, err := Parse(in); err == nil {
			t.Fatalf("%q accepted", in)
		}
	}
}

func TestDisplayAndAccelerator(t *testing.T) {
	b, err := Parse("shift+cmd+p")
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Display(); got != "Cmd+Shift+P" {
		t.Fatalf("display %q", got)
	}
	if got := b.Accelerator(); got != "CmdOrCtrl+Shift+P" {
		t.Fatalf("accelerator %q", got)
	}
}

func TestMapDefaultsLoadSaveValidate(t *testing.T) {
	m := Default()
	if got := m.Get("play_pause").String(); got != "space" {
		t.Fatalf("default play %q", got)
	}
	if err := m.Set("play_pause", "p"); err != nil {
		t.Fatal(err)
	}
	if err := m.Set("mute", "p"); err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err == nil {
		t.Fatal("duplicate binding accepted")
	}
	if err := m.Set("mute", "none"); err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}

	values := map[string]string{}
	m.Save(func(k, v string) { values[k] = v })
	loaded := Load(func(k string) string { return values[k] })
	if got := loaded.Get("play_pause").String(); got != "p" {
		t.Fatalf("loaded play %q", got)
	}
	if !loaded.Get("mute").Empty() {
		t.Fatalf("loaded mute %q", loaded.Get("mute").String())
	}
}

func TestLoadKeepsDefaultsOnInvalidValues(t *testing.T) {
	m := Load(func(k string) string {
		if k == "keymap/play_pause" {
			return "hyper+p"
		}
		return ""
	})
	if got := m.Get("play_pause").String(); got != "space" {
		t.Fatalf("invalid value replaced default: %q", got)
	}
}
