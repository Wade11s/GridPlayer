// Command gridplayer is the Go + MyGo migration app (macOS/arm64).
//
// One MyGo native-UI window, N libVLC hardware panes (set_nsobject under
// Painter.Hole), one engine per cell, dynamic grid layout, menus and
// keyboard controls. Mirrors upstream macOS HW_SP: one libVLC instance per
// pane and the stop -> unbind -> detach -> release order.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"gridplayer-go/internal/engine"
	"gridplayer-go/internal/grid"
	"gridplayer-go/internal/nativehost"
	"gridplayer-go/internal/playlist"
	"gridplayer-go/internal/vlcmac"
)

type cell struct {
	label  string
	path   string
	eng    *engine.Engine
	view   uintptr
	clip   uintptr
	failed bool

	volume  int
	muted   bool
	rate    float64
	startAt int
}

type app struct {
	win          *mygo.Window
	resolve      func() (cv, sv uintptr, h float64, ok bool)
	cells        []*cell
	active       int
	cfg          grid.Config
	fill         bool
	closed       atomic.Bool
	status       string
	doc          *playlist.Document
	dirty        bool
	forceQuit    bool
	playlistPath string
}

func newApp(paths []string) *app {
	a := &app{cfg: grid.Config{Mode: grid.AutoRows}, fill: true}
	a.setPlaylist(paths)
	return a
}

func comparePaths(cells []*cell, paths []string) bool {
	if len(cells) != len(paths) {
		return false
	}
	for i := range paths {
		if cells[i].path != paths[i] {
			return false
		}
	}
	return true
}

func (a *app) paths() []string {
	out := make([]string, len(a.cells))
	for i, c := range a.cells {
		out[i] = c.path
	}
	return out
}

// setPlaylist replaces the cells. Main thread only.
func (a *app) setPlaylist(paths []string) {
	if len(paths) == 0 {
		for _, c := range a.cells {
			a.closeCell(c)
		}
		a.cells = nil
		a.active = 0
		a.dirty = true
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}
	if len(paths) > 16 {
		paths = paths[:16]
	}
	if comparePaths(a.cells, paths) {
		return
	}
	for i := len(paths); i < len(a.cells); i++ {
		a.closeCell(a.cells[i])
	}
	next := make([]*cell, len(paths))
	for i, p := range paths {
		if i < len(a.cells) && a.cells[i].path == p {
			next[i] = a.cells[i]
			continue
		}
		if i < len(a.cells) {
			a.closeCell(a.cells[i])
		}
		c := &cell{
			label:  strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)),
			path:   p,
			volume: 80,
			rate:   1,
		}
		if i > 0 {
			c.muted = true
		}
		next[i] = c
	}
	a.cells = next
	a.dirty = true
	if a.active >= len(a.cells) {
		a.active = len(a.cells) - 1
	}
	if a.active < 0 {
		a.active = 0
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) closeCell(c *cell) {
	if c == nil {
		return
	}
	if c.eng != nil {
		c.eng.Close()
		c.eng = nil
	}
	if c.view != 0 {
		nativehost.Release(c.view)
		c.view = 0
	}
	if c.clip != 0 {
		nativehost.Remove(c.clip)
		nativehost.Release(c.clip)
		c.clip = 0
	}
}

// ensure creates the native view + engine for cell i and sizes it to r.
// Main thread only (Draw callback).
func (a *app) ensure(i int, r grid.Rect) {
	if a.closed.Load() || i < 0 || i >= len(a.cells) {
		return
	}
	cv, sv, h, ok := a.resolve()
	if !ok {
		a.status = "nativehost: surface not found"
		return
	}
	c := a.cells[i]
	if c.failed {
		return
	}
	if c.view == 0 {
		clip := nativehost.NewClip()
		view := nativehost.NewView()
		if clip == 0 || view == 0 {
			c.failed = true
			a.status = "nativehost: no view"
			return
		}
		nativehost.AddBelow(cv, sv, clip)
		nativehost.AddSubview(clip, view)
		nativehost.SetFrame(clip, nativehost.ToMac(nativehost.DIP(r), h))
		eng := engine.New(engine.NewVLCFactory(engine.VLCOptions{
			View: func() uintptr { return view },
		}))
		eng.SetLoop(true)
		eng.SetVolume(c.volume)
		eng.SetMute(c.muted)
		eng.SetRate(c.rate)
		eng.SetStartPosition(int64(c.startAt))
		if err := eng.Load(c.path); err != nil {
			eng.Close()
			nativehost.Remove(clip)
			nativehost.Remove(view)
			nativehost.Release(view)
			nativehost.Release(clip)
			c.failed = true
			a.status = "vlc: " + err.Error()
			return
		}
		c.eng, c.view, c.clip = eng, view, clip
		go a.watch(eng)
	}
	nativehost.SetFrame(c.clip, nativehost.ToMac(nativehost.DIP(r), h))
	s := engine.Snapshot{}
	if c.eng != nil {
		s = c.eng.Snapshot()
	}
	vw, vh, vx, vy := float64(r.W), float64(r.H), 0.0, 0.0
	if a.fill && s.Width > 0 && s.Height > 0 {
		videoAspect := float64(s.Width) / float64(s.Height)
		cellAspect := r.W / r.H
		if cellAspect > videoAspect {
			vw = r.W
			vh = r.W / videoAspect
		} else {
			vh = r.H
			vw = r.H * videoAspect
		}
		vx = (r.W - vw) / 2
		vy = (r.H - vh) / 2
	}
	nativehost.SetFrame(c.view, nativehost.ToMac(
		nativehost.DIP{X: vx, Y: vy, W: vw, H: vh}, float64(r.H)))
}

func (a *app) watch(eng *engine.Engine) {
	for {
		select {
		case <-eng.Updates():
			if a.win != nil && !a.closed.Load() {
				a.win.Invalidate()
			}
		case <-time.After(500 * time.Millisecond):
			if a.closed.Load() {
				return
			}
		}
	}
}

func (a *app) activeEngine() *engine.Engine {
	if a.active < 0 || a.active >= len(a.cells) {
		return nil
	}
	return a.cells[a.active].eng
}

func (a *app) teardown() {
	if !a.closed.CompareAndSwap(false, true) {
		return
	}
	fmt.Println("gridplayer: teardown (stop->unbind->detach->release)...")
	for i, c := range a.cells {
		if c == nil {
			continue
		}
		if c.eng != nil {
			s := c.eng.Snapshot()
			fmt.Printf("gridplayer: cell %d time=%dms phase=%s\n", i, s.Time, s.Phase)
		}
		a.closeCell(c)
	}
	fmt.Println("gridplayer: teardown done")
}

func (a *app) view(c *ui.Context) {
	a.shortcuts(c)
	w, h := c.Size()
	rects := grid.Layout(len(a.cells), grid.Rect{W: float64(w), H: float64(h)}, 4, a.cfg)
	if len(rects) == 0 {
		ui.Text(c, "Open a video to begin").FontSize(20).Center()
		return
	}
	theme := c.Theme()
	ui.Box(c).Fill().Background(ui.RGB(10, 10, 12)).Children(func() {
		for i, r := range rects {
			a.cellView(c, i, r, theme)
		}
		if a.status != "" {
			ui.Text(c, a.status).Absolute().Left(10).Bottom(6).FontSize(12).
				TextColor(ui.RGB(255, 180, 120))
		}
	})
}

func (a *app) cellView(c *ui.Context, i int, r grid.Rect, theme *ui.Theme) {
	cell := a.cells[i]
	active := i == a.active
	border := ui.RGBA(0, 0, 0, 0)
	if active {
		border = ui.RGB(90, 150, 255)
	}
	s := engine.Snapshot{Phase: engine.Idle}
	if cell.eng != nil {
		s = cell.eng.Snapshot()
	}
	el := ui.Box(c.Key(fmt.Sprintf("cell-%d", i))).
		Absolute().Left(float32(r.X)).Top(float32(r.Y)).
		Width(float32(r.W)).Height(float32(r.H)).
		Radius(6).Background(ui.RGB(20, 20, 24)).
		Border(2, border).
		Draw(func(p *ui.Painter, box ui.Rect) {
			a.ensure(i, r)
			p.Hole(box, [4]float32{0, 0, 0, 0})
		}).
		Children(func() {
			badge := fmt.Sprintf(" %s  %dms  %s ", cell.label, s.Time, s.Phase)
			ui.Text(c, badge).Absolute().Top(6).Left(6).FontSize(14).
				Background(ui.RGBA(0, 0, 0, 190)).TextColor(ui.RGB(255, 255, 255))
			if s.Err != "" {
				ui.Text(c, " "+s.Err+" ").Absolute().Top(30).Left(6).FontSize(12).
					Background(ui.RGBA(120, 0, 0, 210)).TextColor(ui.RGB(255, 255, 255))
			}
			ui.Row(c).Absolute().Left(6).Bottom(6).Gap(4).Children(func() {
				label := "Play"
				if s.Phase == engine.Playing {
					label = "Pause"
				}
				if ui.Button(c.Key(fmt.Sprintf("play-%d", i)), label).Width(72).Clicked() && cell.eng != nil {
					cell.eng.Toggle()
				}
				if ui.Button(c.Key(fmt.Sprintf("back-%d", i)), "-5s").Width(60).Clicked() && cell.eng != nil {
					cell.eng.SeekTo(max(s.Time-5000, 0))
				}
				if ui.Button(c.Key(fmt.Sprintf("fwd-%d", i)), "+5s").Width(60).Clicked() && cell.eng != nil {
					cell.eng.SeekTo(s.Time + 5000)
				}
				mute := "Mute"
				if s.Muted {
					mute = "Unmute"
				}
				if ui.Button(c.Key(fmt.Sprintf("mute-%d", i)), mute).Width(88).Clicked() && cell.eng != nil {
					cell.eng.SetMute(!s.Muted)
				}
			})
		})
	if el.Clicked() {
		a.active = i
	}
}

func (a *app) shortcuts(c *ui.Context) {
	if len(a.cells) == 0 {
		return
	}
	eng := a.activeEngine()
	if c.Shortcut(0, ui.KeySpace) && !c.ShortcutRepeated() && eng != nil {
		eng.Toggle()
	}
	if c.Shortcut(0, ui.KeyLeft) && eng != nil {
		eng.SeekTo(max(eng.Snapshot().Time-5000, 0))
	}
	if c.Shortcut(0, ui.KeyRight) && eng != nil {
		eng.SeekTo(eng.Snapshot().Time + 5000)
	}
	if c.Shortcut(0, ui.KeyUp) && eng != nil {
		eng.SetVolume(min(eng.Snapshot().Volume+5, 100))
	}
	if c.Shortcut(0, ui.KeyDown) && eng != nil {
		eng.SetVolume(max(eng.Snapshot().Volume-5, 0))
	}
	if c.Shortcut(0, ui.KeyM) && eng != nil {
		eng.SetMute(!eng.Snapshot().Muted)
	}
	if c.Shortcut(0, ui.KeyL) && eng != nil {
		eng.SetLoop(!eng.Snapshot().Loop)
		_ = eng.Reload()
	}
	if c.Shortcut(0, ui.KeyB) {
		a.active = (a.active - 1 + len(a.cells)) % len(a.cells)
	}
	if c.Shortcut(0, ui.KeyN) {
		a.active = (a.active + 1) % len(a.cells)
	}
	if c.Shortcut(0, ui.KeyBracketLeft) && eng != nil {
		eng.SetRate(max(eng.Snapshot().Rate-0.25, 0.25))
	}
	if c.Shortcut(0, ui.KeyBracketRight) && eng != nil {
		eng.SetRate(min(eng.Snapshot().Rate+0.25, 4))
	}
}

func (a *app) openDialog(add bool) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:   a.win,
		Title:    "Open videos",
		Multiple: true,
		Filters: []mygo.FileFilter{{
			Name: "Media",
			Extensions: []string{
				"mp4", "mkv", "mov", "m4v", "avi", "webm", "ts", "m2ts",
				"mp3", "flac", "wav", "m4a", "ogg", "opus",
			},
		}},
	})
	if err != nil {
		a.status = err.Error()
		return
	}
	if len(paths) == 0 {
		return
	}
	if add {
		a.setPlaylist(append(a.paths(), paths...))
	} else {
		a.setPlaylist(paths)
	}
}

func (a *app) setGrid(cfg grid.Config) {
	a.cfg = cfg
	a.dirty = true
	if a.win != nil {
		a.win.Invalidate()
	}
}

func modeName(m grid.Mode) string {
	switch m {
	case grid.AutoCols:
		return "auto_cols"
	case grid.Fixed:
		return "fixed"
	default:
		return "auto_rows"
	}
}

func parseMode(s string) grid.Mode {
	switch s {
	case "auto_cols":
		return grid.AutoCols
	case "fixed":
		return grid.Fixed
	default:
		return grid.AutoRows
	}
}

// loadPlaylist replaces the cells and grid from a .gpls file. Main thread.
func (a *app) loadPlaylist(path string) error {
	d, err := playlist.Load(path)
	if err != nil {
		a.status = err.Error()
		return err
	}
	paths := make([]string, len(d.Videos))
	for i := range d.Videos {
		paths[i] = d.Videos[i].URI
	}
	a.doc = d
	a.setPlaylist(paths)
	for i := range d.Videos {
		if i >= len(a.cells) {
			break
		}
		c := a.cells[i]
		v := d.Videos[i]
		c.startAt = v.CurrentPosition
		c.muted = v.IsMuted
		if v.Volume > 0 {
			c.volume = int(v.Volume * 100)
		}
		if v.Rate > 0 {
			c.rate = v.Rate
		}
	}
	if mode, rows, cols := d.GridState(); mode != "" {
		a.cfg = grid.Config{Mode: parseMode(mode), Rows: rows, Cols: cols}
	}
	a.playlistPath = path
	a.dirty = false
	if a.win != nil {
		a.win.Invalidate()
	}
	return nil
}

func (a *app) document() *playlist.Document {
	d := &playlist.Document{}
	if a.doc != nil {
		d = a.doc
	}
	d.Format = playlist.FormatID
	d.Version = playlist.FormatVersion
	d.Videos = make([]playlist.Video, len(a.cells))
	for i, c := range a.cells {
		s := engine.Snapshot{}
		if c.eng != nil {
			s = c.eng.Snapshot()
		}
		v := playlist.Video{
			URI:             c.path,
			CurrentPosition: int(s.Time),
			IsMuted:         s.Muted,
			Volume:          float64(s.Volume) / 100,
			Rate:            s.Rate,
			PlaybackState:   s.Phase.String(),
			CellViewMode:    "fit",
		}
		if a.fill {
			v.CellViewMode = "fill"
		}
		if a.doc != nil && i < len(a.doc.Videos) {
			v.ID = a.doc.Videos[i].ID
			v.Title = a.doc.Videos[i].Title
			v.Extra = a.doc.Videos[i].Extra
		}
		d.Videos[i] = v
	}
	d.SetGridState(modeName(a.cfg.Mode), a.cfg.Rows, a.cfg.Cols)
	return d
}

func (a *app) savePlaylist(path string) error {
	d := a.document()
	if err := d.Save(path); err != nil {
		a.status = err.Error()
		return err
	}
	a.doc = d
	a.playlistPath = path
	a.dirty = false
	return nil
}

func (a *app) saveDialog() bool {
	path := a.playlistPath
	if path == "" {
		p, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: a.win,
			Title:  "Save playlist",
			Filters: []mygo.FileFilter{{
				Name:       "GridPlayer playlist",
				Extensions: []string{"gpls"},
			}},
		})
		if err != nil || p == "" {
			return false
		}
		if !strings.HasSuffix(strings.ToLower(p), ".gpls") {
			p += ".gpls"
		}
		path = p
	}
	return a.savePlaylist(path) == nil
}

func (a *app) openPlaylistDialog() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.win,
		Title:  "Open playlist",
		Filters: []mygo.FileFilter{{
			Name:       "GridPlayer playlist",
			Extensions: []string{"gpls"},
		}},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	_ = a.loadPlaylist(paths[0])
}

func (a *app) closeRequested(e *mygo.CloseEvent) {
	if a.forceQuit || !a.dirty {
		a.teardown()
		return
	}
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:       a.win,
		Type:         mygo.MessageQuestion,
		Title:        "Save playlist?",
		Message:      "The playlist has unsaved changes.",
		Buttons:      []string{"Save", "Discard", "Cancel"},
		CancelButton: 2,
	})
	if err != nil {
		e.PreventDefault()
		return
	}
	switch res.Button {
	case 0:
		if a.saveDialog() {
			a.teardown()
		} else {
			e.PreventDefault()
		}
	case 1:
		a.teardown()
	default:
		e.PreventDefault()
	}
}

func (a *app) toggleLoop() {
	if eng := a.activeEngine(); eng != nil {
		eng.SetLoop(!eng.Snapshot().Loop)
		_ = eng.Reload()
	}
}

func (a *app) cycleAudio() {
	eng := a.activeEngine()
	if eng == nil {
		return
	}
	eng.RefreshTracks()
	s := eng.Snapshot()
	if len(s.AudioTracks) == 0 {
		return
	}
	idx := 0
	for i, tr := range s.AudioTracks {
		if tr.ID == s.AudioTrack {
			idx = i
		}
	}
	next := s.AudioTracks[(idx+1)%len(s.AudioTracks)]
	eng.SetAudioTrack(next.ID)
}

func (a *app) cycleSubtitle() {
	eng := a.activeEngine()
	if eng == nil {
		return
	}
	eng.RefreshTracks()
	s := eng.Snapshot()
	if len(s.SubtitleTracks) == 0 {
		return
	}
	idx := -1
	for i, tr := range s.SubtitleTracks {
		if tr.ID == s.SubtitleTrack {
			idx = i
		}
	}
	next := s.SubtitleTracks[(idx+1+len(s.SubtitleTracks))%len(s.SubtitleTracks)]
	eng.SetSubtitleTrack(next.ID)
}

func (a *app) disableSubtitles() {
	if eng := a.activeEngine(); eng != nil {
		eng.SetSubtitleTrack(-1)
	}
}

func (a *app) chapter(step int) {
	eng := a.activeEngine()
	if eng == nil {
		return
	}
	eng.RefreshTracks()
	s := eng.Snapshot()
	if len(s.Chapters) == 0 {
		return
	}
	i := (s.Chapter + step + len(s.Chapters)) % len(s.Chapters)
	eng.SetChapter(i)
}

func (a *app) setRate(rate float64) {
	if eng := a.activeEngine(); eng != nil {
		eng.SetRate(rate)
	}
}

func (a *app) menu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: []*mygo.MenuItem{
			{Label: "Open…", Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) { a.openDialog(false) }},
			{Label: "Add…", Accelerator: "CmdOrCtrl+Shift+O", Click: func(*mygo.MenuItem, *mygo.Window) { a.openDialog(true) }},
			{Type: mygo.MenuItemSeparator},
			{Label: "Open Playlist…", Click: func(*mygo.MenuItem, *mygo.Window) { a.openPlaylistDialog() }},
			{Label: "Save Playlist", Accelerator: "CmdOrCtrl+S", Click: func(*mygo.MenuItem, *mygo.Window) { a.saveDialog() }},
			{Label: "Save Playlist As…", Accelerator: "CmdOrCtrl+Shift+S", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.playlistPath = ""
				a.saveDialog()
			}},
			{Type: mygo.MenuItemSeparator},
			{Role: mygo.RoleClose},
		}},
		{Label: "Playback", Submenu: []*mygo.MenuItem{
			{Label: "Play/Pause", Accelerator: "Space", Click: func(*mygo.MenuItem, *mygo.Window) {
				if eng := a.activeEngine(); eng != nil {
					eng.Toggle()
				}
			}},
			{Label: "Stop", Click: func(*mygo.MenuItem, *mygo.Window) {
				if eng := a.activeEngine(); eng != nil {
					eng.Stop()
				}
			}},
			{Type: mygo.MenuItemSeparator},
			{Label: "Seek -5s", Accelerator: "Left", Click: func(*mygo.MenuItem, *mygo.Window) {
				if eng := a.activeEngine(); eng != nil {
					eng.SeekTo(max(eng.Snapshot().Time-5000, 0))
				}
			}},
			{Label: "Seek +5s", Accelerator: "Right", Click: func(*mygo.MenuItem, *mygo.Window) {
				if eng := a.activeEngine(); eng != nil {
					eng.SeekTo(eng.Snapshot().Time + 5000)
				}
			}},
			{Type: mygo.MenuItemSeparator},
			{Label: "Mute", Accelerator: "M", Click: func(*mygo.MenuItem, *mygo.Window) {
				if eng := a.activeEngine(); eng != nil {
					eng.SetMute(!eng.Snapshot().Muted)
				}
			}},
			{Label: "Loop", Accelerator: "L", Click: func(*mygo.MenuItem, *mygo.Window) { a.toggleLoop() }},
			{Label: "Rate", Submenu: []*mygo.MenuItem{
				{Label: "0.5x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setRate(0.5) }},
				{Label: "1x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setRate(1) }},
				{Label: "1.5x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setRate(1.5) }},
				{Label: "2x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setRate(2) }},
			}},
			{Type: mygo.MenuItemSeparator},
			{Label: "Previous Cell", Accelerator: "B", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.active = (a.active - 1 + len(a.cells)) % len(a.cells)
			}},
			{Label: "Next Cell", Accelerator: "N", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.active = (a.active + 1) % len(a.cells)
			}},
		}},
		{Label: "Grid", Submenu: []*mygo.MenuItem{
			{Label: "Auto Rows", Click: func(*mygo.MenuItem, *mygo.Window) { a.setGrid(grid.Config{Mode: grid.AutoRows}) }},
			{Label: "Auto Columns", Click: func(*mygo.MenuItem, *mygo.Window) { a.setGrid(grid.Config{Mode: grid.AutoCols}) }},
			{Type: mygo.MenuItemSeparator},
			{Label: "Fixed 2×2", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2})
			}},
			{Label: "Fixed 3×3", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3})
			}},
			{Label: "Fixed 4×4", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 4, Cols: 4})
			}},
		}},
		{Label: "View", Submenu: []*mygo.MenuItem{
			{Label: "Fill (cover)", Type: mygo.MenuItemCheckbox, Checked: a.fill, Click: func(*mygo.MenuItem, *mygo.Window) {
				a.fill = true
				if a.win != nil {
					a.win.Invalidate()
				}
			}},
			{Label: "Fit (contain)", Type: mygo.MenuItemCheckbox, Checked: !a.fill, Click: func(*mygo.MenuItem, *mygo.Window) {
				a.fill = false
				if a.win != nil {
					a.win.Invalidate()
				}
			}},
		}},
		{Label: "Tracks", Submenu: []*mygo.MenuItem{
			{Label: "Next Audio Track", Click: func(*mygo.MenuItem, *mygo.Window) { a.cycleAudio() }},
			{Label: "Next Subtitle Track", Click: func(*mygo.MenuItem, *mygo.Window) { a.cycleSubtitle() }},
			{Label: "Disable Subtitles", Click: func(*mygo.MenuItem, *mygo.Window) { a.disableSubtitles() }},
			{Type: mygo.MenuItemSeparator},
			{Label: "Previous Chapter", Click: func(*mygo.MenuItem, *mygo.Window) { a.chapter(-1) }},
			{Label: "Next Chapter", Click: func(*mygo.MenuItem, *mygo.Window) { a.chapter(1) }},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleWindowMenu},
		{Role: mygo.RoleHelp},
	})
}

func main() {
	fixtures := flag.String("out", "testdata/fixtures", "fixture dir")
	autoquit := flag.Int("autoquit", 0, "quit after N seconds (0 = manual)")
	logFile := flag.String("log", "testdata/gridplayer-vlc.log", "libVLC log file")
	files := flag.String("files", "", "comma-separated media paths (default: 4 fixtures)")
	playlistPath := flag.String("playlist", "", "load a .gpls playlist")
	savePath := flag.String("save-playlist", "", "save the current playlist after 3s (automation)")
	flag.Parse()

	if lf, err := os.Create(*logFile); err == nil {
		vlcmac.LogSink = func(level int, msg string) {
			fmt.Fprintf(lf, "[vlc %d] %s\n", level, msg)
		}
	}
	var paths []string
	if *files != "" {
		paths = strings.Split(*files, ",")
	} else {
		paths = []string{
			filepath.Join(*fixtures, "plane-a.mp4"),
			filepath.Join(*fixtures, "plane-b.mp4"),
			filepath.Join(*fixtures, "plane-c.mp4"),
			filepath.Join(*fixtures, "plane-d.mp4"),
		}
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			log.Printf("missing media %s: %v", p, err)
		}
	}
	a := newApp(paths)
	if *playlistPath != "" {
		if err := a.loadPlaylist(*playlistPath); err != nil {
			log.Printf("playlist: %v", err)
		}
	}
	a.dirty = false
	mygo.App.WhenReady(func() {
		w := mygo.NewWindow(mygo.WindowOptions{
			Title:          "GridPlayer",
			Width:          1280,
			Height:         720,
			UseContentSize: true,
			Content:        ui.View(a.view),
		})
		a.win = w
		a.resolve = func() (uintptr, uintptr, float64, bool) {
			return nativehost.Resolve(w.NativeHandle())
		}
		w.OnClose(func(e *mygo.CloseEvent) { a.closeRequested(e) })
		mygo.App.SetMenu(a.menu())
		if *savePath != "" {
			go func() {
				time.Sleep(3 * time.Second)
				mygo.RunOnMain(func() {
					if err := a.savePlaylist(*savePath); err != nil {
						log.Printf("save playlist: %v", err)
					}
				})
			}()
		}
		go func() {
			for {
				time.Sleep(200 * time.Millisecond)
				if a.closed.Load() {
					return
				}
				if a.win != nil {
					a.win.Invalidate()
				}
			}
		}()
		if *autoquit > 0 {
			go func() {
				time.Sleep(time.Duration(*autoquit) * time.Second)
				mygo.RunOnMain(func() {
					a.forceQuit = true
					if a.win != nil {
						a.win.Close()
					}
				})
			}()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	a.teardown()
	fmt.Println("gridplayer: bye")
}
