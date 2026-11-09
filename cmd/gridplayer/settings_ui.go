package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"gridplayer-go/internal/grid"
	"gridplayer-go/internal/keymap"
	"gridplayer-go/internal/resolve"
)

type keyBindingDraft struct {
	Action string
	Label  string
	Value  string
}

type settingsDraft struct {
	tab int

	volume float64
	muted  bool
	rate   float64
	loop   bool

	layout     string
	rows       string
	cols       string
	snap       bool
	overlap    bool
	singleSide string

	view   string
	aspect string
	scale  string
	crop   string

	subFont         string
	subSize         string
	subColor        string
	subOutlineColor string
	subOutline      string
	subBold         bool
	subOpacity      float64
	subEncoding     string
	subAutodetect   bool

	proxy      string
	cookies    string
	ytdlp      string
	streamlink string
	format     string

	keys        []keyBindingDraft
	keymapError string
}

var (
	layoutOptions   = []string{"Auto Rows", "Auto Columns", "Fixed", "Free Tiles", "Mosaic", "Single Mode"}
	sizeOptions     = []string{"1", "2", "3", "4", "5", "6", "7", "8"}
	sideOptions     = []string{"Auto", "Right", "Left", "Bottom", "Top"}
	viewOptions     = []string{"Fill (cover)", "Fit (contain)", "Stretch"}
	aspectOptions   = []string{"Auto", "16:9", "4:3", "1:1", "2.35:1"}
	scaleOptions    = []string{"Fit", "1x", "1.5x", "2x"}
	cropOptions     = []string{"None", "16:9", "4:3", "2.35:1"}
	formatOptions   = []string{"Best", "Worst", "720p", "480p"}
	outlineOptions  = []string{"None", "Thin", "Normal", "Thick"}
	encodingOptions = []string{"Auto", "UTF-8", "GB18030", "Windows-1252", "Shift_JIS"}
)

func defaultSettingsDraft() settingsDraft {
	return settingsDraft{
		volume:      80,
		rate:        1,
		loop:        true,
		layout:      "Auto Rows",
		rows:        "3",
		cols:        "3",
		snap:        true,
		overlap:     true,
		singleSide:  "Auto",
		view:        "Fill (cover)",
		aspect:      "Auto",
		scale:       "Fit",
		crop:        "None",
		subOutline:  "None",
		subOpacity:  100,
		subEncoding: "Auto",
		subSize:     "0",
		format:      "Best",
		keys:        defaultKeyBindingDrafts(),
	}
}

func (a *app) openSettings() {
	if a.settingsWin != nil {
		return
	}
	a.settingsDraft = a.currentSettingsDraft()
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:          "Settings",
		Width:          560,
		Height:         560,
		UseContentSize: true,
		Content:        ui.View(a.settingsView),
	})
	a.settingsWin = w
	w.OnClose(func(*mygo.CloseEvent) { a.settingsWin = nil })
}

func (a *app) settingsView(c *ui.Context) {
	d := &a.settingsDraft
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Tabs(c, &d.tab, "General", "Playback", "Grid", "Video", "Subtitles", "Network", "Keymap")
		ui.Scroll(c).Fill().Children(func() {
			switch d.tab {
			case 0:
				a.settingsGeneral(c, d)
			case 1:
				a.settingsPlayback(c, d)
			case 2:
				a.settingsGrid(c, d)
			case 3:
				a.settingsVideo(c, d)
			case 4:
				a.settingsSubtitles(c, d)
			case 5:
				a.settingsNetwork(c, d)
			default:
				a.settingsKeymap(c, d)
			}
		})
		ui.Divider(c)
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "Apply").Clicked() {
				if err := d.validateKeymap(); err != nil {
					d.keymapError = err.Error()
				} else {
					d.keymapError = ""
					a.applySettingsDraft(*d)
					if a.settingsWin != nil {
						a.settingsWin.Close()
					}
				}
			}
			if ui.Button(c, "Cancel").Clicked() && a.settingsWin != nil {
				a.settingsWin.Close()
			}
			ui.Spacer(c)
			if ui.Button(c, "Reset Defaults").Clicked() {
				*d = defaultSettingsDraft()
			}
		})
	})
}

func (a *app) settingsGeneral(c *ui.Context, d *settingsDraft) {
	ui.Form(c, func() {
		ui.Field(c, "Default volume", func() {
			ui.Slider(c, &d.volume, 0, 100)
		}).Description(fmt.Sprintf("%.0f%% — applies to newly opened cells", d.volume))
		ui.Field(c, "Default muted", func() {
			ui.Checkbox(c, &d.muted, "Start new cells muted")
		})
	})
}

func (a *app) settingsPlayback(c *ui.Context, d *settingsDraft) {
	ui.Form(c, func() {
		ui.Field(c, "Default rate", func() {
			ui.Slider(c, &d.rate, 0.25, 4)
		}).Description(fmt.Sprintf("%.2fx — applies to newly opened cells", d.rate))
		ui.Field(c, "Default loop", func() {
			ui.Checkbox(c, &d.loop, "Loop new cells")
		})
		ui.Field(c, "Decode", func() {
			ui.Text(c, "VideoToolbox via libVLC")
		})
		ui.Field(c, "Resolver", func() {
			ui.Text(c, "yt-dlp with direct fallback")
		})
	})
}

func (a *app) settingsGrid(c *ui.Context, d *settingsDraft) {
	ui.Form(c, func() {
		ui.Field(c, "Layout", func() {
			ui.Select(c, &d.layout, layoutOptions)
		})
		if d.layout == "Fixed" {
			ui.Field(c, "Rows", func() {
				ui.Select(c, &d.rows, sizeOptions)
			})
			ui.Field(c, "Columns", func() {
				ui.Select(c, &d.cols, sizeOptions)
			})
		}
		if d.layout == "Single Mode" {
			ui.Field(c, "Strip side", func() {
				ui.Select(c, &d.singleSide, sideOptions)
			})
		}
		ui.Field(c, "Snap", func() {
			ui.Checkbox(c, &d.snap, "Snap edges and middles within 8 DIPs")
		})
		ui.Field(c, "Free overlap", func() {
			ui.Checkbox(c, &d.overlap, "Allow free tiles to overlap")
		})
	})
}

func (a *app) settingsVideo(c *ui.Context, d *settingsDraft) {
	ui.Form(c, func() {
		ui.Field(c, "View mode", func() {
			ui.Select(c, &d.view, viewOptions)
		})
		ui.Field(c, "Aspect ratio", func() {
			ui.Select(c, &d.aspect, aspectOptions)
		})
		ui.Field(c, "Scale", func() {
			ui.Select(c, &d.scale, scaleOptions)
		})
		ui.Field(c, "Crop", func() {
			ui.Select(c, &d.crop, cropOptions)
		})
	})
}

func (a *app) settingsSubtitles(c *ui.Context, d *settingsDraft) {
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, "Changing the style reloads live cells at their current position.").FontSize(12)
		ui.Form(c, func() {
			ui.Field(c, "Font", func() {
				ui.TextInput(c, &d.subFont)
			}).Description("Blank = VLC default")
			ui.Field(c, "Font size", func() {
				ui.TextInput(c, &d.subSize)
			}).Description("0 = VLC default; 12-96 pixels")
			ui.Field(c, "Text color", func() {
				ui.TextInput(c, &d.subColor)
			}).Description("#RRGGBB; blank = default")
			ui.Field(c, "Outline color", func() {
				ui.TextInput(c, &d.subOutlineColor)
			}).Description("#RRGGBB; blank = default")
			ui.Field(c, "Outline", func() {
				ui.Select(c, &d.subOutline, outlineOptions)
			})
			ui.Field(c, "Bold", func() {
				ui.Checkbox(c, &d.subBold, "Bold subtitles")
			})
			ui.Field(c, "Opacity", func() {
				ui.Slider(c, &d.subOpacity, 0, 100)
			}).Description(fmt.Sprintf("%.0f%%", d.subOpacity))
			ui.Field(c, "Encoding", func() {
				ui.Select(c, &d.subEncoding, encodingOptions)
			})
			ui.Field(c, "Autodetect", func() {
				ui.Checkbox(c, &d.subAutodetect, "Find sidecar subtitle files")
			})
		})
	})
}

func (a *app) settingsNetwork(c *ui.Context, d *settingsDraft) {
	ui.Form(c, func() {
		ui.Field(c, "Proxy", func() {
			ui.TextInput(c, &d.proxy)
		}).Description("http://host:port — passed to yt-dlp and streamlink")
		ui.Field(c, "Cookies file", func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c, &d.cookies)
				if ui.Button(c, "Browse…").Clicked() {
					if paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
						Parent: a.settingsWin,
						Title:  "Cookies file",
					}); err == nil && len(paths) > 0 {
						d.cookies = paths[0]
					}
				}
			})
		})
		ui.Field(c, "yt-dlp path", func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c, &d.ytdlp)
				if ui.Button(c, "Browse…").Clicked() {
					if paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
						Parent: a.settingsWin,
						Title:  "yt-dlp",
					}); err == nil && len(paths) > 0 {
						d.ytdlp = paths[0]
					}
				}
			})
		})
		ui.Field(c, "Streamlink path", func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c, &d.streamlink)
				if ui.Button(c, "Browse…").Clicked() {
					if paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
						Parent: a.settingsWin,
						Title:  "streamlink",
					}); err == nil && len(paths) > 0 {
						d.streamlink = paths[0]
					}
				}
			})
		})
		ui.Field(c, "Quality", func() {
			ui.Select(c, &d.format, formatOptions)
		}).Description("yt-dlp format; streamlink uses best")
	})
}

func (a *app) settingsKeymap(c *ui.Context, d *settingsDraft) {
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, "Use space, left, m, cmd+shift+p, or none.").FontSize(12)
		ui.Form(c, func() {
			for i := range d.keys {
				k := &d.keys[i]
				ui.Field(c, k.Label, func() {
					ui.TextInput(c, &k.Value)
				})
			}
		})
		if d.keymapError != "" {
			ui.Text(c, d.keymapError).FontSize(12).TextColor(ui.RGB(255, 120, 120))
		}
	})
}

func defaultKeyBindingDrafts() []keyBindingDraft {
	return keyBindingDrafts(keymap.Default())
}

func keyBindingDrafts(m keymap.Map) []keyBindingDraft {
	out := make([]keyBindingDraft, 0, len(keymap.Actions))
	for _, action := range keymap.Actions {
		out = append(out, keyBindingDraft{
			Action: action,
			Label:  keymap.Labels[action],
			Value:  m.Get(action).String(),
		})
	}
	return out
}

func (d settingsDraft) validateKeymap() error {
	m := keymap.Default()
	for _, k := range d.keys {
		if err := m.Set(k.Action, k.Value); err != nil {
			return fmt.Errorf("%s: %w", k.Label, err)
		}
	}
	return m.Validate()
}

func (a *app) currentSettingsDraft() settingsDraft {
	d := defaultSettingsDraft()
	d.volume = float64(a.defaultVolume)
	d.muted = a.defaultMuted
	d.rate = a.defaultRate
	d.loop = a.defaultLoop
	d.snap = a.snap
	d.overlap = a.overlap
	d.rows = fmt.Sprint(clampInt(a.cfg.Rows, 1, 8))
	d.cols = fmt.Sprint(clampInt(a.cfg.Cols, 1, 8))
	switch {
	case a.single:
		d.layout = "Single Mode"
	case a.mosaic:
		d.layout = "Mosaic"
	case a.free:
		d.layout = "Free Tiles"
	default:
		switch a.cfg.Mode {
		case grid.AutoCols:
			d.layout = "Auto Columns"
		case grid.Fixed:
			d.layout = "Fixed"
		default:
			d.layout = "Auto Rows"
		}
	}
	d.singleSide = sideName(a.singleSide)
	d.view = viewName(a.fill, a.stretch)
	d.aspect = aspectName(a.aspect)
	d.scale = scaleName(a.scale)
	d.crop = cropName(a.crop)
	if a.resolver != nil {
		d.proxy = a.resolver.Proxy
		d.cookies = a.resolver.Cookies
		d.ytdlp = a.resolver.Path
		d.streamlink = a.resolver.Streamlink
		d.format = formatName(a.resolver.Format)
	}
	d.subFont = a.subtitle.Font
	d.subSize = fmt.Sprint(a.subtitle.Size)
	d.subColor = a.subtitle.Color
	d.subOutlineColor = a.subtitle.OutlineColor
	d.subOutline = subtitleOutlineName(a.subtitle.Outline)
	d.subBold = a.subtitle.Bold
	d.subOpacity = float64(a.subtitle.Opacity) * 100 / 255
	d.subEncoding = subtitleEncodingName(a.subtitle.Encoding)
	d.subAutodetect = a.subtitle.Autodetect
	d.keys = keyBindingDrafts(a.keys)
	return d
}

func (a *app) applySettingsDraft(d settingsDraft) {
	a.defaultVolume = clampInt(int(math.Round(d.volume)), 0, 100)
	a.defaultMuted = d.muted
	a.defaultRate = clampFloat(d.rate, 0.25, 4)
	a.defaultLoop = d.loop
	a.snap = d.snap
	a.overlap = d.overlap
	a.singleSide = sideFromName(d.singleSide)
	a.fill, a.stretch = viewFromName(d.view)
	a.aspect = aspectFromName(d.aspect)
	a.scale = scaleFromName(d.scale)
	a.crop = cropFromName(d.crop)
	oldSub := a.subtitle
	a.subtitle = subtitleStyle{
		Font:         strings.TrimSpace(d.subFont),
		Size:         clampInt(atoi(d.subSize, 0), 0, 96),
		Color:        strings.TrimSpace(d.subColor),
		OutlineColor: strings.TrimSpace(d.subOutlineColor),
		Outline:      subtitleOutlineValue(d.subOutline),
		Bold:         d.subBold,
		Opacity:      clampInt(int(math.Round(d.subOpacity*255/100)), 0, 255),
		Encoding:     subtitleEncodingValue(d.subEncoding),
		Autodetect:   d.subAutodetect,
	}
	if a.resolver == nil {
		a.resolver = resolve.New()
	}
	a.resolver.Proxy = strings.TrimSpace(d.proxy)
	a.resolver.Cookies = strings.TrimSpace(d.cookies)
	a.resolver.Path = strings.TrimSpace(d.ytdlp)
	a.resolver.Streamlink = strings.TrimSpace(d.streamlink)
	a.resolver.Format = formatFromName(d.format)
	for _, k := range d.keys {
		if err := a.keys.Set(k.Action, k.Value); err != nil {
			a.status = err.Error()
			break
		}
	}
	oldCfg, oldFree, oldMosaic, oldSingle := a.cfg, a.free, a.mosaic, a.single
	switch d.layout {
	case "Auto Columns":
		a.cfg = grid.Config{Mode: grid.AutoCols}
		a.free, a.mosaic, a.single = false, false, false
	case "Fixed":
		a.cfg = grid.Config{Mode: grid.Fixed, Rows: atoi(d.rows, 3), Cols: atoi(d.cols, 3)}
		a.free, a.mosaic, a.single = false, false, false
	case "Free Tiles":
		a.free, a.mosaic, a.single = true, false, false
	case "Mosaic":
		a.free, a.mosaic, a.single = false, true, false
		a.ensureMosaic()
	case "Single Mode":
		a.free, a.mosaic, a.single = false, false, true
	default:
		a.cfg = grid.Config{Mode: grid.AutoRows}
		a.free, a.mosaic, a.single = false, false, false
	}
	for _, c := range a.cells {
		c.viewMode = viewModeName(a.fill, a.stretch)
		if c.eng != nil {
			c.eng.SetAspectRatio(a.aspect)
			c.eng.SetScale(a.scale)
			c.eng.SetCrop(a.crop)
		}
	}
	if a.subtitle != oldSub {
		a.reloadCells()
	}
	if a.cfg != oldCfg || a.free != oldFree || a.mosaic != oldMosaic || a.single != oldSingle {
		a.dirty = true
	}
	a.saveSettings()
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func sideName(s grid.StripSide) string {
	switch s {
	case grid.StripRight:
		return "Right"
	case grid.StripLeft:
		return "Left"
	case grid.StripBottom:
		return "Bottom"
	case grid.StripTop:
		return "Top"
	default:
		return "Auto"
	}
}

func sideFromName(s string) grid.StripSide {
	switch s {
	case "Right":
		return grid.StripRight
	case "Left":
		return grid.StripLeft
	case "Bottom":
		return grid.StripBottom
	case "Top":
		return grid.StripTop
	default:
		return grid.StripAuto
	}
}

func viewName(fill, stretch bool) string {
	if stretch {
		return "Stretch"
	}
	if fill {
		return "Fill (cover)"
	}
	return "Fit (contain)"
}

func viewFromName(s string) (fill, stretch bool) {
	switch s {
	case "Stretch":
		return false, true
	case "Fit (contain)":
		return false, false
	default:
		return true, false
	}
}

func aspectName(s string) string {
	if s == "" {
		return "Auto"
	}
	for _, o := range aspectOptions {
		if o == s {
			return s
		}
	}
	return "Auto"
}

func aspectFromName(s string) string {
	if s == "Auto" {
		return ""
	}
	return s
}

func scaleName(v float64) string {
	switch v {
	case 1:
		return "1x"
	case 1.5:
		return "1.5x"
	case 2:
		return "2x"
	default:
		return "Fit"
	}
}

func scaleFromName(s string) float64 {
	switch s {
	case "1x":
		return 1
	case "1.5x":
		return 1.5
	case "2x":
		return 2
	default:
		return 0
	}
}

func formatName(s string) string {
	switch s {
	case "worst":
		return "Worst"
	case "best[height<=720]":
		return "720p"
	case "best[height<=480]":
		return "480p"
	default:
		return "Best"
	}
}

func formatFromName(s string) string {
	switch s {
	case "Worst":
		return "worst"
	case "720p":
		return "best[height<=720]"
	case "480p":
		return "best[height<=480]"
	default:
		return "best"
	}
}

func cropName(s string) string {
	if s == "" {
		return "None"
	}
	for _, o := range cropOptions {
		if o == s {
			return s
		}
	}
	return "None"
}

func cropFromName(s string) string {
	if s == "None" {
		return ""
	}
	return s
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
