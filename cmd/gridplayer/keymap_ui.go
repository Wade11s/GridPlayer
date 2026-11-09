package main

import (
	"github.com/egoist/mygo/ui"

	"gridplayer-go/internal/keymap"
)

func (a *app) shortcut(c *ui.Context, action string) bool {
	b := a.keys.Get(action)
	if b.Empty() {
		return false
	}
	mods, key, ok := bindingUI(b)
	return ok && c.Shortcut(mods, key)
}

func bindingUI(b keymap.Binding) (ui.Modifiers, ui.Key, bool) {
	var mods ui.Modifiers
	for _, m := range b.Mods {
		switch m {
		case "cmd":
			mods |= ui.Cmd
		case "shift":
			mods |= ui.Shift
		case "ctrl":
			mods |= ui.Ctrl
		case "alt":
			mods |= ui.Alt
		}
	}
	key, ok := keyByName[b.Key]
	return mods, key, ok
}

var keyByName = map[string]ui.Key{
	"space":     ui.KeySpace,
	"left":      ui.KeyLeft,
	"right":     ui.KeyRight,
	"up":        ui.KeyUp,
	"down":      ui.KeyDown,
	"enter":     ui.KeyEnter,
	"escape":    ui.KeyEscape,
	"backspace": ui.KeyBackspace,
	"delete":    ui.KeyDelete,
	"tab":       ui.KeyTab,
	"home":      ui.KeyHome,
	"end":       ui.KeyEnd,
	"pageup":    ui.KeyPageUp,
	"pagedown":  ui.KeyPageDown,
	"f1":        ui.KeyF1,
	"f2":        ui.KeyF2,
	"f3":        ui.KeyF3,
	"f4":        ui.KeyF4,
	"f5":        ui.KeyF5,
	"f6":        ui.KeyF6,
	"f7":        ui.KeyF7,
	"f8":        ui.KeyF8,
	"f9":        ui.KeyF9,
	"f10":       ui.KeyF10,
	"f11":       ui.KeyF11,
	"f12":       ui.KeyF12,
	"-":         ui.KeyMinus,
	"=":         ui.KeyEqual,
	",":         ui.KeyComma,
	".":         ui.KeyPeriod,
	"/":         ui.KeySlash,
	";":         ui.KeySemicolon,
	"'":         ui.KeyQuote,
	"[":         ui.KeyBracketLeft,
	"]":         ui.KeyBracketRight,
	"\\":        ui.KeyBackslash,
	"`":         ui.KeyBackquote,
	"0":         ui.Key0,
	"1":         ui.Key1,
	"2":         ui.Key2,
	"3":         ui.Key3,
	"4":         ui.Key4,
	"5":         ui.Key5,
	"6":         ui.Key6,
	"7":         ui.Key7,
	"8":         ui.Key8,
	"9":         ui.Key9,
	"a":         ui.KeyA,
	"b":         ui.KeyB,
	"c":         ui.KeyC,
	"d":         ui.KeyD,
	"e":         ui.KeyE,
	"f":         ui.KeyF,
	"g":         ui.KeyG,
	"h":         ui.KeyH,
	"i":         ui.KeyI,
	"j":         ui.KeyJ,
	"k":         ui.KeyK,
	"l":         ui.KeyL,
	"m":         ui.KeyM,
	"n":         ui.KeyN,
	"o":         ui.KeyO,
	"p":         ui.KeyP,
	"q":         ui.KeyQ,
	"r":         ui.KeyR,
	"s":         ui.KeyS,
	"t":         ui.KeyT,
	"u":         ui.KeyU,
	"v":         ui.KeyV,
	"w":         ui.KeyW,
	"x":         ui.KeyX,
	"y":         ui.KeyY,
	"z":         ui.KeyZ,
}
