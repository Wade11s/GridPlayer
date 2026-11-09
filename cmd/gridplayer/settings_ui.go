package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"gridplayer-go/internal/grid"
)

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
}

var (
	layoutOptions = []string{"Auto Rows", "Auto Columns", "Fixed", "Free Tiles", "Mosaic", "Single Mode"}
	sizeOptions   = []string{"1", "2", "3", "4", "5", "6", "7", "8"}
	sideOptions   = []string{"Auto", "Right", "Left", "Bottom", "Top"}
	viewOptions   = []string{"Fill (cover)", "Fit (contain)", "Stretch"}
	aspectOptions = []string{"Auto", "16:9", "4:3", "1:1", "2.35:1"}
	scaleOptions  = []string{"Fit", "1x", "1.5x", "2x"}
	cropOptions   = []string{"None", "16:9", "4:3", "2.35:1"}
)

func defaultSettingsDraft() settingsDraft {
	return settingsDraft{
		volume:     80,
		rate:       1,
		loop:       true,
		layout:     "Auto Rows",
		rows:       "3",
		cols:       "3",
		snap:       true,
		overlap:    true,
		singleSide: "Auto",
		view:       "Fill (cover)",
		aspect:     "Auto",
		scale:      "Fit",
		crop:       "None",
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
		ui.Tabs(c, &d.tab, "General", "Playback", "Grid", "Video")
		ui.Scroll(c).Fill().Children(func() {
			switch d.tab {
			case 0:
				a.settingsGeneral(c, d)
			case 1:
				a.settingsPlayback(c, d)
			case 2:
				a.settingsGrid(c, d)
			default:
				a.settingsVideo(c, d)
			}
		})
		ui.Divider(c)
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "Apply").Clicked() {
				a.applySettingsDraft(*d)
				if a.settingsWin != nil {
					a.settingsWin.Close()
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
		if c.eng != nil {
			c.eng.SetAspectRatio(a.aspect)
			c.eng.SetScale(a.scale)
			c.eng.SetCrop(a.crop)
		}
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
