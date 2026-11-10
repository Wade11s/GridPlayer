// Command gridplayer is the Go + MyGo migration app (macOS/arm64).
//
// One MyGo native-UI window, N libVLC hardware panes (set_nsobject under
// Painter.Hole), one engine per cell, dynamic grid layout, menus and
// keyboard controls. Mirrors upstream macOS HW_SP: one libVLC instance per
// pane and the stop -> unbind -> detach -> release order.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"gridplayer-go/internal/bookmarks"
	"gridplayer-go/internal/engine"
	"gridplayer-go/internal/grid"
	"gridplayer-go/internal/i18n"
	"gridplayer-go/internal/keymap"
	"gridplayer-go/internal/medialist"
	"gridplayer-go/internal/nativehost"
	"gridplayer-go/internal/playlist"
	"gridplayer-go/internal/resolve"
	"gridplayer-go/internal/settings"
	"gridplayer-go/internal/snapshots"
	"gridplayer-go/internal/vlcmac"
)

type cell struct {
	label   string
	source  string
	path    string
	eng     *engine.Engine
	view    uintptr
	clip    uintptr
	failed  bool
	listErr string

	volume           int
	muted            bool
	rate             float64
	audioDelay       int64
	subDelay         int64
	loopStart        int64
	loopEnd          int64
	cropMargins      cropMargins
	startAt          int
	startPaused      bool
	pauseOnPlay      atomic.Bool
	retries          atomic.Int32
	viewMode         string
	zoom             float64
	anchorX, anchorY float64
	shiftX, shiftY   float64

	list      []string
	listIndex int
	listEnd   string
	endAction string
	listFail  string
	// listFailWait is set when this cell overrides the default wait.
	// Zero means move on at once.
	listFailWait  *int
	failStreak    int
	netFailStreak int
	skipGen       uint64
	loop          bool
	advancing     bool
	endedGen      uint64
}

type app struct {
	win                 *mygo.Window
	resolve             func() (cv, sv uintptr, h float64, ok bool)
	cells               []*cell
	active              int
	cfg                 grid.Config
	fill                bool
	stretch             bool
	single              bool
	singleSide          grid.StripSide
	singleShare         *float64
	free                bool
	freeTiles           []grid.Tile
	mosaic              bool
	mosaicTiles         []grid.Tile
	mosaicLocked        map[int]bool
	snap                bool
	overlap             bool
	freeArea            [2]float64
	moveIdx             int
	moveLast            [2]float32
	moveOK              bool
	aspect              string
	scale               float64
	crop                string
	closed              atomic.Bool
	status              string
	doc                 *playlist.Document
	dirty               bool
	forceQuit           bool
	playlistPath        string
	urlWin              *mygo.Window
	urlInput            string
	settingsWin         *mygo.Window
	settingsDraft       settingsDraft
	resolver            *resolve.Resolver
	keys                keymap.Map
	language            string
	translator          *i18n.Translator
	subtitle            subtitleStyle
	bookmarks           *bookmarks.Store
	bookmarkWin         *mygo.Window
	bookmarkRenameWin   *mygo.Window
	bookmarkRenameKey   string
	bookmarkRenameOld   string
	bookmarkRenameInput string
	snapshots           *snapshots.Store
	snapshotSaveWin     *mygo.Window
	timecodeWin         *mygo.Window
	timecodeInput       string
	snapshotSaveName    string
	pending             []pendingDrop
	settings            *settings.Store
	recent              []string
	defaultVolume       int
	defaultMuted        bool
	defaultRate         float64
	defaultLoop         bool
	defaultZoom         float64
	defaultAnchor       string
	defaultEnd          string
	retryMode           atomic.Int32
	retryTimes          atomic.Int32
	listFailMode        atomic.Int32
	listFailWait        atomic.Int32
	autoReloadMin       atomic.Int64
}

func newApp(paths []string) *app {
	a := &app{
		cfg:           grid.Config{Mode: grid.AutoRows},
		fill:          true,
		snap:          true,
		overlap:       true,
		defaultVolume: 80,
		defaultRate:   1,
		defaultLoop:   true,
		defaultZoom:   1,
		defaultAnchor: "center",
		resolver:      resolve.New(),
		keys:          keymap.Default(),
		language:      "en_US",
		subtitle:      subtitleStyle{Opacity: 255},
	}
	a.setRetryPolicy("times", defaultRetryTimes)
	a.setListFailPolicy(listFailPlayNext, defaultListFailWait)
	a.setPlaylist(paths)
	return a
}

func (a *app) loadSettings() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return
	}
	st, err := settings.Load(filepath.Join(dir, "settings.ini"))
	if err != nil {
		log.Printf("settings: %v", err)
		return
	}
	a.settings = st
	a.language = st.Get("player/language", "en_US")
	a.loadTranslator()
	a.keys = keymap.Load(func(k string) string { return st.Get(k, "") })
	a.cfg = grid.Config{
		Mode: parseMode(st.Get("playlist/grid_mode", "auto_rows")),
		Rows: atoi(st.Get("playlist/grid_rows", "3"), 3),
		Cols: atoi(st.Get("playlist/grid_cols", "3"), 3),
	}
	a.single = st.Get("playlist/single_mode_strip", "off") != "off"
	a.free = st.Get("playlist/grid_free", "false") == "true"
	a.mosaic = st.Get("playlist/grid_mosaic", "false") == "true"
	a.snap = st.Get("playlist/grid_snap", "true") == "true"
	a.overlap = st.Get("playlist/grid_overlap", "true") == "true"
	if raw := st.Get("playlist/grid_mosaic_tiles", ""); raw != "" {
		var tiles []grid.Tile
		if err := json.Unmarshal([]byte(raw), &tiles); err == nil && len(tiles) == len(a.cells) {
			valid := true
			for _, t := range tiles {
				if !t.Valid() {
					valid = false
					break
				}
			}
			if valid {
				a.mosaicTiles = tiles
			}
		}
	}
	if raw := st.Get("playlist/grid_mosaic_locked", ""); raw != "" {
		var locked []int
		if err := json.Unmarshal([]byte(raw), &locked); err == nil {
			a.mosaicLocked = map[int]bool{}
			for _, i := range locked {
				if i >= 0 && i < len(a.cells) {
					a.mosaicLocked[i] = true
				}
			}
		}
	}
	switch st.Get("playlist/single_mode_strip_side", "auto") {
	case "right":
		a.singleSide = grid.StripRight
	case "left":
		a.singleSide = grid.StripLeft
	case "bottom":
		a.singleSide = grid.StripBottom
	case "top":
		a.singleSide = grid.StripTop
	default:
		a.singleSide = grid.StripAuto
	}
	switch st.Get("video_defaults/cell_view_mode", "fill") {
	case "fit":
		a.fill, a.stretch = false, false
	case "stretch":
		a.fill, a.stretch = false, true
	default:
		a.fill, a.stretch = true, false
	}
	if v := atof(st.Get("video_defaults/volume", "0.8"), 0.8); v > 0 {
		a.defaultVolume = int(v * 100)
	}
	a.defaultMuted = st.Get("video_defaults/muted", "false") == "true"
	a.defaultRate = clampPlaybackRate(atof(st.Get("video_defaults/rate", "1"), 1))
	a.defaultLoop = st.Get("video_defaults/loop", "true") == "true"
	if raw := st.Get("video_defaults/end_action", ""); raw != "" {
		a.defaultEnd = normalizeEndAction(raw)
		if a.defaultEnd == endLoop {
			a.defaultLoop = true
		} else if a.defaultEnd != "" {
			a.defaultLoop = false
		}
	}
	a.setRetryPolicy(
		st.Get("video_defaults/network_retry_mode", "times"),
		atoi(st.Get("video_defaults/network_retry_times", "3"), defaultRetryTimes),
	)
	a.autoReloadMin.Store(int64(atoi(st.Get("video_defaults/auto_reload_timer", "0"), 0)))
	a.setListFailPolicy(
		st.Get("video_defaults/list_fail_action", listFailPlayNext),
		atoi(st.Get("video_defaults/list_fail_wait", "2"), defaultListFailWait),
	)
	a.aspect = st.Get("video_defaults/aspect_ratio", "")
	a.scale = atof(st.Get("video_defaults/scale", "0"), 0)
	if a.scale < 0 {
		a.scale = 0
	}
	if z := atof(st.Get("video_defaults/zoom", "1"), 1); z > 0 {
		a.defaultZoom = clampZoom(z)
	} else {
		a.defaultZoom = 1
	}
	a.defaultAnchor = st.Get("video_defaults/anchor", "center")
	a.crop = st.Get("video_defaults/crop_ratio", "")
	a.loadNetworkSettings(st)
	a.subtitle.Font = st.Get("subtitles/font", "")
	a.subtitle.Size = subtitleSize(st.Get("subtitles/font_size", "0"))
	a.subtitle.Color = st.Get("subtitles/color", "")
	a.subtitle.OutlineColor = st.Get("subtitles/outline_color", "")
	a.subtitle.Outline = subtitleOutlineValue(st.Get("subtitles/outline", "None"))
	a.subtitle.Bold = st.Get("subtitles/bold", "false") == "true"
	a.subtitle.Opacity = subtitleOpacity(st.Get("subtitles/opacity", "255"))
	a.subtitle.Encoding = st.Get("subtitles/encoding", "")
	a.subtitle.Autodetect = st.Get("subtitles/autodetect", "false") == "true"
	for i, c := range a.cells {
		c.volume = a.defaultVolume
		c.muted = i > 0 || a.defaultMuted
		c.rate = a.defaultRate
		c.loop = a.defaultLoop
		c.zoom = a.defaultZoom
		c.anchorX, c.anchorY = parseAnchor(a.defaultAnchor)
		c.viewMode = viewModeName(a.fill, a.stretch)
	}
	if raw := st.Get("app/recent", ""); raw != "" {
		_ = json.Unmarshal([]byte(raw), &a.recent)
	}
	if b, err := bookmarks.Load(filepath.Join(dir, "bookmarks.json")); err == nil {
		a.bookmarks = b
	} else {
		log.Printf("bookmarks: %v", err)
	}
	if s, err := snapshots.Load(filepath.Join(dir, "snapshots.json")); err == nil {
		a.snapshots = s
	} else {
		log.Printf("snapshots: %v", err)
		a.snapshots = snapshots.New(filepath.Join(dir, "snapshots.json"))
	}
}

func (a *app) loadTranslator() {
	dir, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return
	}
	translator, err := i18n.Load(filepath.Join(dir, "translations"), a.language)
	if err != nil {
		log.Printf("i18n: %v", err)
		translator, _ = i18n.Load(filepath.Join(dir, "translations"), "en_US")
	}
	a.translator = translator
}

func (a *app) tr(context, source string) string {
	if a.translator == nil {
		return source
	}
	return a.translator.Tr(context, source)
}

func (a *app) loadNetworkSettings(st *settings.Store) {
	if a.resolver == nil {
		a.resolver = resolve.New()
	}
	a.resolver.Proxy = st.Get("network/proxy", "")
	a.resolver.Cookies = st.Get("network/cookies", "")
	a.resolver.Format = st.Get("network/format", "best")
	a.resolver.Path = st.Get("network/ytdlp_path", a.resolver.Path)
	a.resolver.Streamlink = st.Get("network/streamlink_path", a.resolver.Streamlink)
}

func atoi(s string, def int) int {
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return def
	}
	return v
}

func atof(s string, def float64) float64 {
	var v float64
	if _, err := fmt.Sscanf(s, "%g", &v); err != nil {
		return def
	}
	return v
}

func visibleOnDisplays(x, y, w, h int) bool {
	for _, d := range mygo.Screen.Displays() {
		wa := d.WorkArea
		iw := min(x+w, wa.X+wa.Width) - max(x, wa.X)
		ih := min(y+h, wa.Y+wa.Height) - max(y, wa.Y)
		if iw >= 64 && ih >= 64 {
			return true
		}
	}
	return false
}

func (a *app) saveSettings() {
	if a.settings == nil {
		return
	}
	a.settings.Set("playlist/grid_mode", modeName(a.cfg.Mode))
	a.settings.Set("playlist/grid_rows", fmt.Sprint(a.cfg.Rows))
	a.settings.Set("playlist/grid_cols", fmt.Sprint(a.cfg.Cols))
	a.settings.Set("playlist/grid_free", fmt.Sprint(a.free))
	a.settings.Set("playlist/grid_mosaic", fmt.Sprint(a.mosaic))
	a.settings.Set("playlist/grid_snap", fmt.Sprint(a.snap))
	a.settings.Set("playlist/grid_overlap", fmt.Sprint(a.overlap))
	if a.mosaic {
		a.ensureMosaic()
		if raw, err := json.Marshal(a.mosaicTiles); err == nil {
			a.settings.Set("playlist/grid_mosaic_tiles", string(raw))
		}
		locked := make([]int, 0, len(a.mosaicLocked))
		for i := range a.mosaicLocked {
			locked = append(locked, i)
		}
		sort.Ints(locked)
		if raw, err := json.Marshal(locked); err == nil {
			a.settings.Set("playlist/grid_mosaic_locked", string(raw))
		}
	} else {
		a.settings.Delete("playlist/grid_mosaic_tiles")
		a.settings.Delete("playlist/grid_mosaic_locked")
	}
	strip := "off"
	if a.single {
		strip = "swap"
	}
	a.settings.Set("playlist/single_mode_strip", strip)
	side := "auto"
	switch a.singleSide {
	case grid.StripRight:
		side = "right"
	case grid.StripLeft:
		side = "left"
	case grid.StripBottom:
		side = "bottom"
	case grid.StripTop:
		side = "top"
	}
	a.settings.Set("playlist/single_mode_strip_side", side)
	view := "fill"
	if a.stretch {
		view = "stretch"
	} else if !a.fill {
		view = "fit"
	}
	a.settings.Set("video_defaults/cell_view_mode", view)
	a.settings.Set("player/language", a.language)
	a.settings.Set("video_defaults/aspect_ratio", a.aspect)
	a.settings.Set("video_defaults/scale", fmt.Sprintf("%g", a.scale))
	a.settings.Set("video_defaults/zoom", fmt.Sprintf("%g", a.defaultZoom))
	a.settings.Set("video_defaults/anchor", a.defaultAnchor)
	a.settings.Set("video_defaults/crop_ratio", a.crop)
	a.settings.Set("video_defaults/volume", fmt.Sprintf("%.2f", float64(a.defaultVolume)/100))
	a.settings.Set("video_defaults/muted", fmt.Sprint(a.defaultMuted))
	a.settings.Set("video_defaults/rate", fmt.Sprintf("%g", a.defaultRate))
	a.settings.Set("video_defaults/loop", fmt.Sprint(a.defaultLoop))
	end := normalizeEndAction(a.defaultEnd)
	if end == "" {
		if a.defaultLoop {
			end = endLoop
		} else {
			end = endStop
		}
	}
	a.settings.Set("video_defaults/end_action", end)
	switch a.retryMode.Load() {
	case retryOff:
		a.settings.Set("video_defaults/network_retry_mode", "off")
	case retryInfinite:
		a.settings.Set("video_defaults/network_retry_mode", "infinite")
	default:
		a.settings.Set("video_defaults/network_retry_mode", "times")
	}
	times := a.retryTimes.Load()
	if times < 1 {
		times = defaultRetryTimes
	}
	a.settings.Set("video_defaults/network_retry_times", fmt.Sprint(times))
	if a.listFailMode.Load() == listFailShow {
		a.settings.Set("video_defaults/list_fail_action", listFailShowError)
	} else {
		a.settings.Set("video_defaults/list_fail_action", listFailPlayNext)
	}
	a.settings.Set("video_defaults/list_fail_wait", fmt.Sprint(a.listFailWait.Load()))
	a.settings.Set("video_defaults/auto_reload_timer", fmt.Sprint(a.autoReloadMin.Load()))
	if a.resolver != nil {
		a.settings.Set("network/proxy", a.resolver.Proxy)
		a.settings.Set("network/cookies", a.resolver.Cookies)
		a.settings.Set("network/format", a.resolver.Format)
		a.settings.Set("network/ytdlp_path", a.resolver.Path)
		a.settings.Set("network/streamlink_path", a.resolver.Streamlink)
	}
	a.settings.Set("subtitles/font", a.subtitle.Font)
	a.settings.Set("subtitles/font_size", fmt.Sprint(a.subtitle.Size))
	a.settings.Set("subtitles/color", a.subtitle.Color)
	a.settings.Set("subtitles/outline_color", a.subtitle.OutlineColor)
	a.settings.Set("subtitles/outline", subtitleOutlineName(a.subtitle.Outline))
	a.settings.Set("subtitles/bold", fmt.Sprint(a.subtitle.Bold))
	a.settings.Set("subtitles/opacity", fmt.Sprint(a.subtitle.Opacity))
	a.settings.Set("subtitles/encoding", a.subtitle.Encoding)
	a.settings.Set("subtitles/autodetect", fmt.Sprint(a.subtitle.Autodetect))
	if raw, err := json.Marshal(a.recent); err == nil {
		a.settings.Set("app/recent", string(raw))
	}
	a.keys.Save(func(k, v string) { a.settings.Set(k, v) })
	if err := a.settings.Save(); err != nil {
		log.Printf("settings save: %v", err)
	}
}

func (a *app) saveWindowState() {
	if a.settings == nil || a.win == nil {
		return
	}
	b := a.win.ContentBounds()
	if b.Width <= 0 || b.Height <= 0 {
		return
	}
	a.settings.Set("window/x", fmt.Sprint(b.X))
	a.settings.Set("window/y", fmt.Sprint(b.Y))
	a.settings.Set("window/width", fmt.Sprint(b.Width))
	a.settings.Set("window/height", fmt.Sprint(b.Height))
	if err := a.settings.Save(); err != nil {
		log.Printf("settings save: %v", err)
	}
}

func (a *app) addRecent(path string) {
	if path == "" {
		return
	}
	out := []string{path}
	for _, p := range a.recent {
		if p != path && len(out) < 10 {
			out = append(out, p)
		}
	}
	a.recent = out
	a.saveSettings()
	if a.win != nil {
		mygo.App.SetMenu(a.menu())
	}
}

func (a *app) openRecent(path string) {
	if strings.EqualFold(filepath.Ext(path), ".gpls") {
		_ = a.loadPlaylist(path)
		return
	}
	a.setPlaylist([]string{path})
	a.addRecent(path)
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
		a.mosaicTiles = nil
		a.mosaicLocked = nil
		a.dirty = true
		a.refreshMenu()
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
		next[i] = a.newCell(i, p)
	}
	a.cells = next
	// Free tiles are re-laid out for the new playlist.
	a.freeTiles = grid.FreeLayout(len(a.cells))
	a.resetMosaic()
	a.dirty = true
	if a.active >= len(a.cells) {
		a.active = len(a.cells) - 1
	}
	if a.active < 0 {
		a.active = 0
	}
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) newCell(i int, p string) *cell {
	c := &cell{
		label:    labelFor(p),
		source:   p,
		path:     p,
		volume:   a.defaultVolume,
		rate:     a.defaultRate,
		loop:     a.defaultLoop,
		viewMode: viewModeName(a.fill, a.stretch),
	}
	if i > 0 || a.defaultMuted {
		c.muted = true
	}
	if !strings.Contains(p, "://") {
		if st, err := os.Stat(p); err == nil && (st.IsDir() || medialist.IsList(p)) {
			if items, err := medialist.Expand(p, false, 500); err != nil {
				c.listErr = err.Error()
			} else if len(items) > 0 {
				c.list = items
				c.listIndex = 0
				c.path = items[0]
				c.loop = false
				c.listEnd = "loop"
				c.endAction = endNext
				if action := normalizeEndAction(a.defaultEnd); action != "" && action != endLoop {
					c.endAction = action
				}
			}
		}
	}
	return c
}

func labelFor(p string) string {
	s := strings.TrimSuffix(p, "/")
	base := s
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		base = s[i+1:]
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// insertCells keeps existing cell structs (engines/views) and splices new
// ones in at index at.
func (a *app) insertCells(at int, files []string) {
	if len(files) == 0 {
		return
	}
	if at < 0 {
		at = 0
	}
	if at > len(a.cells) {
		at = len(a.cells)
	}
	next := make([]*cell, 0, len(a.cells)+len(files))
	next = append(next, a.cells[:at]...)
	for j, p := range files {
		next = append(next, a.newCell(at+j, p))
	}
	next = append(next, a.cells[at:]...)
	a.cells = next
	a.freeTiles = grid.FreeLayout(len(a.cells))
	a.resetMosaic()
	a.active = at
	a.dirty = true
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) dropFiles(at int, files []string) {
	if len(files) == 0 {
		return
	}
	if len(files) == 1 && strings.EqualFold(filepath.Ext(files[0]), ".gpls") {
		// Immediate entry point (window drops, dialogs): load now.
		_ = a.loadPlaylist(files[0])
		return
	}
	a.insertCells(at, files)
	a.addRecent(files[0])
}

// queueDrop is used from within a draw loop, where loading a .gpls would
// resize a.cells mid-frame. It defers single-playlist drops to after the
// frame and executes media inserts immediately.
func (a *app) queueDrop(at int, files []string) {
	if len(files) == 1 && strings.EqualFold(filepath.Ext(files[0]), ".gpls") {
		a.pending = append(a.pending, pendingDrop{at: at, files: files})
		return
	}
	a.dropFiles(at, files)
}

// pendingDrop is deferred until after a frame completes (dropping a
// .gpls on a cell would otherwise resize a.cells mid-loop).
type pendingDrop struct {
	at    int
	files []string
}

// handleDropped drains pending drops after the view completes.
func (a *app) handleDropped() {
	pending := a.pending
	a.pending = nil
	for _, d := range pending {
		a.dropFiles(d.at, d.files)
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

// reloadCells recreates every live engine so instance-level options such as
// subtitle styling take effect, resuming each cell at its current position.
func (a *app) reloadCells() {
	for _, c := range a.cells {
		if c.eng == nil {
			continue
		}
		s := c.eng.Snapshot()
		if s.Phase != engine.Loading {
			c.startAt = int(s.Time)
			c.startPaused = s.Phase != engine.Playing
		}
		a.closeCell(c)
	}
	if a.win != nil {
		a.win.Invalidate()
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
	if c.listErr != "" {
		c.failed = true
		a.status = c.listErr
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
			Args: a.subtitle.args(),
		}))
		eng.SetLoop(c.loop)
		eng.SetVolume(c.volume)
		eng.SetMute(c.muted)
		eng.SetRate(c.rate)
		eng.SetAudioDelay(c.audioDelay)
		eng.SetSubtitleDelay(c.subDelay)
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
		if c.startPaused {
			c.pauseOnPlay.Store(true)
		}
		c.eng, c.view, c.clip = eng, view, clip
		go a.watch(c, eng)
		go a.autoReloadLoop(c, eng)
	}
	nativehost.SetFrame(c.clip, nativehost.ToMac(nativehost.DIP(r), h))
	fill, stretch := viewModeFlags(a.cellViewMode(c))
	s := engine.Snapshot{}
	if c.eng != nil {
		s = c.eng.Snapshot()
		if stretch {
			c.eng.SetAspectRatio(fmt.Sprintf("%.0f:%.0f", r.W, r.H))
		} else {
			c.eng.SetAspectRatio(a.aspect)
		}
		c.eng.SetScale(a.scale)
	}
	place := placePicture(videoShape{W: float64(s.Width), H: float64(s.Height)},
		r.W, r.H, fill, stretch, viewZoom{Scale: c.zoom, Crop: c.cropMargins.combined(parseCropRatio(a.crop, videoShape{W: float64(s.Width), H: float64(s.Height)})), AnchorX: c.anchorX, AnchorY: c.anchorY, ShiftX: c.shiftX, ShiftY: c.shiftY})
	nativehost.SetFrame(c.view, nativehost.ToMac(
		nativehost.DIP{X: place.X, Y: place.Y, W: place.W, H: place.H}, float64(r.H)))
}

// autoReloadLoop reloads a live cell on the auto reload timer.
// 0 minutes disables it.
func (a *app) autoReloadLoop(c *cell, eng *engine.Engine) {
	for {
		minutes := int(a.autoReloadMin.Load())
		if minutes <= 0 {
			time.Sleep(time.Second)
			if a.closed.Load() {
				return
			}
			continue
		}
		time.Sleep(autoReloadInterval(minutes))
		if a.closed.Load() || c.eng != eng {
			return
		}
		mygo.RunOnMain(func() {
			if a.closed.Load() || c.eng != eng {
				return
			}
			a.autoReloadReloadsCell(c)
		})
	}
}

func (a *app) watch(c *cell, eng *engine.Engine) {
	var lastRetryGen uint64
	wasPlaying := false
	for {
		select {
		case <-eng.Updates():
			s := eng.Snapshot()
			if s.Phase == engine.Playing {
				c.retries.Store(0)
				lastRetryGen = 0
				if !wasPlaying {
					mygo.RunOnMain(func() {
						c.failStreak = 0
						c.netFailStreak = 0
						c.skipGen = 0
						if strings.HasPrefix(a.status, "retrying ") || strings.HasPrefix(a.status, "retry limit ") ||
							strings.HasPrefix(a.status, "Next file in ") || strings.HasPrefix(a.status, "Stopped: ") {
							a.status = ""
						}
					})
				}
				wasPlaying = true
			} else {
				wasPlaying = false
			}
			if c.pauseOnPlay.Load() && s.Phase == engine.Playing {
				c.pauseOnPlay.Store(false)
				eng.Pause()
			}
			if s.Phase == engine.Failed && s.Gen != lastRetryGen {
				lastRetryGen = s.Gen
				armed := false
				if retryable(c.path) {
					limit := a.networkRetryLimit()
					if limit != 0 {
						attempt := c.retries.Add(1)
						name := filepath.Base(c.path)
						if limit > 0 && int(attempt) > limit {
							mygo.RunOnMain(func() {
								a.status = "retry limit reached for " + name
							})
						} else {
							armed = true
							delay := retryDelay(attempt)
							gen := s.Gen
							mygo.RunOnMain(func() {
								a.status = retryStatus(name, delay, attempt, limit)
							})
							own := eng
							time.AfterFunc(delay, func() {
								mygo.RunOnMain(func() {
									if a.closed.Load() {
										return
									}
									now := a.networkRetryLimit()
									if now == 0 || (now > 0 && int(attempt) > now) {
										return
									}
									_ = own.ReloadIfFailed(gen)
								})
							})
						}
					}
				}
				if !armed && len(c.list) > 0 {
					gen := s.Gen
					path := c.path
					items := len(c.list)
					own := eng
					mygo.RunOnMain(func() {
						if a.closed.Load() {
							return
						}
						skip, wait, status := a.noteListFailure(c, path, items)
						if status != "" {
							a.status = status
						}
						if !skip {
							return
						}
						c.skipGen = gen
						time.AfterFunc(wait, func() {
							mygo.RunOnMain(func() {
								if a.closed.Load() || c.skipGen != gen {
									return
								}
								snap := own.Snapshot()
								if snap.Gen != gen || snap.Phase != engine.Failed {
									return
								}
								a.stepOnFailure(c)
							})
						})
					})
				}
			}
			if s.Phase == engine.Ended && c.shouldAdvance(s.Gen) {
				mygo.RunOnMain(func() { a.finishVideo(c) })
			}
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

// shouldAdvance reports once per generation that the list ended, so the
// Stopped event VLC sends after EndReached does not advance twice.
func (c *cell) shouldAdvance(gen uint64) bool {
	if c.endedGen == gen {
		return false
	}
	c.endedGen = gen
	return true
}

func (a *app) advanceList(c *cell) {
	if c == nil {
		return
	}
	c.skipGen = 0
	if c.advancing || len(c.list) == 0 {
		return
	}
	next := c.listIndex + 1
	if next >= len(c.list) {
		switch c.listEnd {
		case "stop":
			return
		case "next":
			a.nextCellFrom(c)
			return
		default:
			next = 0
		}
	}
	c.advancing = true
	defer func() { c.advancing = false }()
	c.listIndex = next
	c.path = c.list[next]
	c.startAt = 0
	c.retries.Store(0)
	if c.eng != nil {
		c.eng.SetStartPosition(0)
		if err := c.eng.Load(c.path); err != nil {
			a.status = err.Error()
		}
	}
	a.refreshMenu()
	if a.bookmarkWin != nil {
		a.bookmarkWin.Invalidate()
	}
}

func (a *app) nextCellFrom(c *cell) {
	idx := -1
	for i, x := range a.cells {
		if x == c {
			idx = i
			break
		}
	}
	if idx < 0 || idx+1 >= len(a.cells) {
		return
	}
	a.setActive(idx + 1)
	if eng := a.activeEngine(); eng != nil {
		eng.Play()
	}
}

func (a *app) previousList(c *cell) {
	if c == nil {
		return
	}
	c.skipGen = 0
	if c.advancing || len(c.list) == 0 {
		return
	}
	prev := c.listIndex - 1
	if prev < 0 {
		prev = len(c.list) - 1
	}
	c.advancing = true
	defer func() { c.advancing = false }()
	c.listIndex = prev
	c.path = c.list[prev]
	c.startAt = 0
	c.retries.Store(0)
	if c.eng != nil {
		c.eng.SetStartPosition(0)
		if err := c.eng.Load(c.path); err != nil {
			a.status = err.Error()
		}
	}
	a.refreshMenu()
	if a.bookmarkWin != nil {
		a.bookmarkWin.Invalidate()
	}
}

func (a *app) activeEngine() *engine.Engine {
	if a.active < 0 || a.active >= len(a.cells) {
		return nil
	}
	return a.cells[a.active].eng
}

// setActive changes the active cell and refreshes the menu so the
// Bookmarks submenu follows the newly active source.
func (a *app) nudgeAudioDelay(delta int64) {
	c := a.activeCell()
	eng := a.activeEngine()
	if c == nil || eng == nil {
		return
	}
	c.audioDelay = eng.NudgeAudioDelay(delta)
	a.status = fmt.Sprintf("audio delay %+dms", c.audioDelay)
	a.dirty = true
}

func (a *app) resetAudioDelay() {
	c := a.activeCell()
	eng := a.activeEngine()
	if c == nil || eng == nil {
		return
	}
	c.audioDelay = 0
	eng.SetAudioDelay(0)
	a.status = "audio delay +0ms"
	a.dirty = true
}

func (a *app) setActive(i int) {
	if i < 0 || i >= len(a.cells) || a.active == i {
		return
	}
	a.active = i
	a.refreshMenu()
}

func (a *app) activeCell() *cell {
	if a.active < 0 || a.active >= len(a.cells) {
		return nil
	}
	return a.cells[a.active]
}

func viewModeName(fill, stretch bool) string {
	if stretch {
		return "stretch"
	}
	if fill {
		return "fill"
	}
	return "fit"
}

func viewModeFlags(mode string) (fill, stretch bool) {
	switch mode {
	case "stretch":
		return false, true
	case "fit":
		return false, false
	default:
		return true, false
	}
}

func (a *app) cellViewMode(c *cell) string {
	if c != nil && c.viewMode != "" {
		return c.viewMode
	}
	return viewModeName(a.fill, a.stretch)
}

func (a *app) activeViewFlags() (fill, stretch bool) {
	if c := a.activeCell(); c != nil {
		return viewModeFlags(a.cellViewMode(c))
	}
	return a.fill, a.stretch
}

func (a *app) setListEnd(mode string) {
	c := a.activeCell()
	if c == nil || len(c.list) == 0 {
		return
	}
	c.listEnd = mode
	a.dirty = true
	a.refreshMenu()
}

func cloneWait(wait *int) *int {
	if wait == nil {
		return nil
	}
	copied := *wait
	return &copied
}

func (a *app) finishVideo(c *cell) {
	if c == nil {
		return
	}
	switch c.finishAction() {
	case endNext:
		if len(c.list) > 0 {
			a.advanceList(c)
		}
	case endPrevious:
		a.stepPrevious(c)
	case endShuffle:
		a.shuffleList(c)
	case endStop:
		if c.eng != nil {
			c.eng.Stop()
		}
	case endPause:
		if c.eng != nil {
			c.eng.Pause()
		}
	}
}

func (a *app) stepOnFailure(c *cell) {
	if c == nil {
		return
	}
	switch c.finishAction() {
	case endPrevious:
		a.stepPrevious(c)
	case endShuffle:
		a.shuffleList(c)
	default:
		a.advanceList(c)
	}
}

func (a *app) stepPrevious(c *cell) {
	if c == nil || len(c.list) == 0 {
		return
	}
	if c.listIndex == 0 {
		switch c.listEnd {
		case "stop":
			return
		case "next":
			a.previousCellFrom(c)
			return
		}
	}
	a.previousList(c)
}

func (a *app) previousCellFrom(c *cell) {
	idx := -1
	for i, x := range a.cells {
		if x == c {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return
	}
	a.setActive(idx - 1)
	if eng := a.activeEngine(); eng != nil {
		eng.Play()
	}
}

func (a *app) shuffleList(c *cell) {
	if c == nil || len(c.list) == 0 {
		return
	}
	if len(c.list) == 1 {
		a.advanceList(c)
		return
	}
	index := pickShuffle(len(c.list), c.listIndex, randIntN)
	a.loadListIndex(c, index)
}

func randIntN(n int) int { return rand.IntN(n) }

func (a *app) loadListIndex(c *cell, index int) {
	if c == nil || index < 0 || index >= len(c.list) || c.advancing {
		return
	}
	c.skipGen = 0
	c.advancing = true
	defer func() { c.advancing = false }()
	c.listIndex = index
	c.path = c.list[index]
	c.startAt = 0
	c.retries.Store(0)
	if c.eng != nil {
		c.eng.SetStartPosition(0)
		if err := c.eng.Load(c.path); err != nil {
			a.status = err.Error()
		}
	}
	a.refreshMenu()
	if a.bookmarkWin != nil {
		a.bookmarkWin.Invalidate()
	}
}

func (a *app) setEndAction(mode string) {
	c := a.activeCell()
	if c == nil {
		return
	}
	mode = normalizeEndAction(mode)
	if mode == "" {
		mode = endLoop
	}
	c.endAction = mode
	c.loop = mode == endLoop
	if c.eng != nil {
		c.eng.SetLoop(c.loop)
	}
	a.dirty = true
	a.refreshMenu()
}

func (a *app) setListFail(mode string) {
	c := a.activeCell()
	if c == nil || len(c.list) == 0 {
		return
	}
	c.listFail = mode
	c.skipGen = 0
	a.dirty = true
	a.refreshMenu()
}

func (a *app) ensureMosaic() {
	if len(a.mosaicTiles) != len(a.cells) {
		a.mosaicTiles = grid.Mosaic(len(a.cells))
		a.mosaicLocked = map[int]bool{}
	}
	if a.mosaicLocked == nil {
		a.mosaicLocked = map[int]bool{}
	}
}

func (a *app) resetMosaic() {
	a.mosaicTiles = grid.Mosaic(len(a.cells))
	a.mosaicLocked = map[int]bool{}
}

func (a *app) moveTile(tiles []grid.Tile, i int, dx, dy float64, forceOverlap bool) []grid.Tile {
	var next []grid.Tile
	if !forceOverlap && !a.overlap {
		next = grid.MoveTileNoOverlap(tiles, i, dx, dy, a.freeArea[0], a.freeArea[1])
	} else {
		next = grid.MoveTile(tiles, i, dx, dy, a.freeArea[0], a.freeArea[1])
	}
	if a.snap {
		snapped := grid.SnapMove(next, i, a.freeArea[0], a.freeArea[1], 8)
		if forceOverlap || a.overlap || !grid.Overlaps(snapped, i) ||
			grid.OverlapArea(snapped, i) <= grid.OverlapArea(next, i) {
			next = snapped
		}
	}
	return next
}

func (a *app) resizeTile(tiles []grid.Tile, i int, dx, dy float64, forceOverlap bool) []grid.Tile {
	var next []grid.Tile
	if !forceOverlap && !a.overlap {
		next = grid.ResizeTileNoOverlap(tiles, i, dx, dy, a.freeArea[0], a.freeArea[1])
	} else {
		next = grid.ResizeTile(tiles, i, dx, dy, a.freeArea[0], a.freeArea[1])
	}
	if a.snap {
		snapped := grid.SnapResize(next, i, a.freeArea[0], a.freeArea[1], 8)
		if forceOverlap || a.overlap || !grid.Overlaps(snapped, i) ||
			grid.OverlapArea(snapped, i) <= grid.OverlapArea(next, i) {
			next = snapped
		}
	}
	return next
}

func (a *app) arrangeMosaic() {
	if !a.mosaic {
		return
	}
	a.ensureMosaic()
	a.mosaicTiles = grid.Arrange(len(a.cells), a.mosaicTiles, a.mosaicLocked)
	a.dirty = true
	a.saveSettings()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) toggleMosaicLock() {
	if !a.mosaic || a.active < 0 || a.active >= len(a.cells) {
		return
	}
	a.ensureMosaic()
	if a.mosaicLocked[a.active] {
		delete(a.mosaicLocked, a.active)
	} else {
		a.mosaicLocked[a.active] = true
	}
	a.dirty = true
	a.saveSettings()
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) unlockMosaic() {
	if !a.mosaic {
		return
	}
	a.mosaicLocked = map[int]bool{}
	a.dirty = true
	a.saveSettings()
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) teardown() {
	if !a.closed.CompareAndSwap(false, true) {
		return
	}
	a.saveSettings()
	a.saveWindowState()
	if a.settingsWin != nil {
		a.settingsWin.Close()
		a.settingsWin = nil
	}
	if a.bookmarkRenameWin != nil {
		a.bookmarkRenameWin.Close()
		a.bookmarkRenameWin = nil
	}
	if a.bookmarkWin != nil {
		a.bookmarkWin.Close()
		a.bookmarkWin = nil
	}
	if a.snapshotSaveWin != nil {
		a.snapshotSaveWin.Close()
		a.snapshotSaveWin = nil
	}
	if a.timecodeWin != nil {
		a.timecodeWin.Close()
		a.timecodeWin = nil
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
	defer a.handleDropped()
	w, h := c.Size()
	area := grid.Rect{W: float64(w), H: float64(h)}
	a.freeArea = [2]float64{float64(w), float64(h)}
	rects := grid.Layout(len(a.cells), area, 4, a.cfg)
	if a.mosaic {
		a.ensureMosaic()
		tiles := a.mosaicTiles
		rects = make([]grid.Rect, len(tiles))
		for i, t := range tiles {
			rects[i] = t.Rect(area, 4)
		}
	} else if a.free {
		if len(a.freeTiles) != len(a.cells) {
			a.freeTiles = grid.FreeLayout(len(a.cells))
		}
		tiles := a.freeTiles
		rects = make([]grid.Rect, len(tiles))
		for i, t := range tiles {
			rects[i] = t.Rect(area, 4)
		}
	}
	if a.single && len(a.cells) > 1 {
		rects, _ = grid.SingleLayout(len(a.cells), a.active, area, 4, a.singleSide, a.singleShare, 16.0/9.0)
	}
	if len(rects) == 0 {
		ui.Column(c).Fill().Center().Gap(8).Children(func() {
			ui.Text(c, "Open a video to begin").FontSize(20)
			if a.status != "" {
				ui.Text(c, a.status).FontSize(12).TextColor(ui.RGB(255, 180, 120))
			}
		})
		return
	}
	theme := c.Theme()
	ui.Box(c).Fill().Background(ui.RGB(10, 10, 12)).Children(func() {
		for i, r := range rects {
			a.cellView(c, i, r, theme)
		}
		if a.free || a.mosaic {
			for i, r := range rects {
				if a.mosaic && a.mosaicLocked[i] {
					continue
				}
				h := ui.Box(c.Key(fmt.Sprintf("resize-%d", i))).
					Absolute().Left(float32(r.X + r.W - 16)).Top(float32(r.Y + r.H - 16)).
					Width(16).Height(16).Radius(3).Background(theme.Accent)
				if dx, dy, ok := h.Dragged(); ok && (dx != 0 || dy != 0) {
					if a.free {
						a.freeTiles = a.resizeTile(a.freeTiles, i, float64(dx), float64(dy), false)
					} else {
						a.mosaicTiles = a.resizeTile(a.mosaicTiles, i, float64(dx), float64(dy), true)
					}
					a.dirty = true
				}
			}
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
	base := ui.Box(c.Key(fmt.Sprintf("cell-%d", i))).
		Absolute().Left(float32(r.X)).Top(float32(r.Y)).
		Width(float32(r.W)).Height(float32(r.H)).
		Radius(6).Background(ui.RGB(20, 20, 24))
	if !a.free && !a.mosaic {
		base = base.Drag(i)
	}
	if over, ok := ui.DragOver[int](base); ok && over != i {
		border = theme.Accent
	}
	el := base.Border(2, border).
		Draw(func(p *ui.Painter, box ui.Rect) {
			a.ensure(i, r)
			p.Hole(box, [4]float32{0, 0, 0, 0})
		}).
		Children(func() {
			badge := fmt.Sprintf(" %s  %dms  %s ", cell.label, s.Time, s.Phase)
			if len(cell.list) > 0 {
				badge = fmt.Sprintf(" %s %d/%d  %dms  %s ", cell.label, cell.listIndex+1, len(cell.list), s.Time, s.Phase)
			}
			if a.mosaic && a.mosaicLocked[i] {
				badge = strings.TrimSuffix(badge, " ") + " locked "
			}
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
				if ui.Button(c.Key(fmt.Sprintf("shot-%d", i)), "Shot").Width(64).Clicked() && cell.eng != nil {
					a.saveFrame(cell.eng)
				}
			})
		})
	if el.Clicked() {
		a.setActive(i)
	}
	if a.free && len(a.freeTiles) > i {
		if dx, dy, ok := el.Dragged(); ok && (dx != 0 || dy != 0) {
			a.freeTiles = a.moveTile(a.freeTiles, i, float64(dx), float64(dy), false)
			a.dirty = true
		}
	} else if a.mosaic && len(a.mosaicTiles) > i && !a.mosaicLocked[i] {
		if dx, dy, ok := el.Dragged(); ok && (dx != 0 || dy != 0) {
			a.mosaicTiles = a.moveTile(a.mosaicTiles, i, float64(dx), float64(dy), true)
			a.dirty = true
		}
	}
	// In grid modes the cell stays a drag-swap source; Free and Mosaic move
	// their own tile instead.
	_ = border
	if el.FileDragOver() && border == ui.RGBA(0, 0, 0, 0) {
		el = el.Border(2, theme.Accent)
	}
	if files := el.DroppedFiles(); len(files) > 0 {
		a.queueDrop(i, files)
	}
	if from, ok := ui.Drop[int](el); ok && from != i {
		a.swapCells(from, i)
	}
}

func (a *app) swapCells(from, to int) {
	if from < 0 || to < 0 || from >= len(a.cells) || to >= len(a.cells) {
		return
	}
	a.cells[from], a.cells[to] = a.cells[to], a.cells[from]
	a.active = to
	a.dirty = true
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) shortcuts(c *ui.Context) {
	if len(a.cells) == 0 {
		return
	}
	eng := a.activeEngine()
	if a.shortcut(c, "play_pause") && !c.ShortcutRepeated() && eng != nil {
		eng.Toggle()
	}
	if a.shortcut(c, "seek_back") && eng != nil {
		eng.SeekTo(max(eng.Snapshot().Time-5000, 0))
	}
	if a.shortcut(c, "seek_forward") && eng != nil {
		eng.SeekTo(eng.Snapshot().Time + 5000)
	}
	if a.shortcut(c, "volume_up") && eng != nil {
		eng.SetVolume(min(eng.Snapshot().Volume+5, 100))
	}
	if a.shortcut(c, "volume_down") && eng != nil {
		eng.SetVolume(max(eng.Snapshot().Volume-5, 0))
	}
	if a.shortcut(c, "mute") && eng != nil {
		eng.SetMute(!eng.Snapshot().Muted)
	}
	if a.shortcut(c, "loop") && eng != nil {
		eng.SetLoop(!eng.Snapshot().Loop)
		_ = eng.Reload()
	}
	if a.shortcut(c, "prev_cell") {
		a.setActive((a.active - 1 + len(a.cells)) % len(a.cells))
	}
	if a.shortcut(c, "next_cell") {
		a.setActive((a.active + 1) % len(a.cells))
	}
	if a.shortcut(c, "rate_down") && eng != nil {
		a.nudgeRate(-0.25)
	}
	if a.shortcut(c, "rate_up") && eng != nil {
		a.nudgeRate(0.25)
	}
	if a.shortcut(c, "frame_next") && eng != nil {
		eng.NextFrame()
	}
	if a.shortcut(c, "frame_prev") && eng != nil {
		eng.PreviousFrame()
	}
	if a.shortcut(c, "audio_later") {
		a.nudgeAudioDelay(engine.AudioDelayStepMS)
	}
	if a.shortcut(c, "audio_earlier") {
		a.nudgeAudioDelay(-engine.AudioDelayStepMS)
	}
	for digit := 0; digit <= 9; digit++ {
		if a.shortcut(c, "seek_percent_"+strconv.Itoa(digit)) {
			a.seekPercentActive(float64(digit) / 10)
		}
	}
	if a.shortcut(c, "seek_random") {
		if eng != nil {
			lo, hi := int64(0), eng.Snapshot().Length
			if cell := a.activeCell(); cell != nil && cell.loopEnd > 0 {
				lo, hi = cell.loopStart, cell.loopEnd
			}
			a.seekActive(randomSeekTarget(lo, hi, rand.Int64N))
		}
	}
	if a.shortcut(c, "seek_start") {
		a.seekActive(0)
	}
	if a.shortcut(c, "seek_medium_back") {
		a.seekActive(a.activeSnapshot().Time - 15000)
	}
	if a.shortcut(c, "seek_long_back") {
		a.seekActive(a.activeSnapshot().Time - 30000)
	}
	if a.shortcut(c, "seek_medium_forward") {
		a.seekActive(a.activeSnapshot().Time + 15000)
	}
	if a.shortcut(c, "seek_long_forward") {
		a.seekActive(a.activeSnapshot().Time + 30000)
	}
	if a.shortcut(c, "loop_start") {
		a.setLoopPoint(true)
	}
	if a.shortcut(c, "loop_end") {
		a.setLoopPoint(false)
	}
	if a.shortcut(c, "loop_reset") {
		a.resetLoopPoints()
	}
	if a.shortcut(c, "rate_reset") {
		a.setRate(1)
	}
	if a.shortcut(c, "zoom_in") {
		a.stepZoom(1)
	}
	if a.shortcut(c, "zoom_out") {
		a.stepZoom(-1)
	}
	if a.shortcut(c, "zoom_reset") {
		a.resetZoom()
	}
	cropSteps := []struct {
		action string
		side   string
		delta  float64
	}{
		{"crop_left_more", "left", 10}, {"crop_left_less", "left", -10},
		{"crop_top_more", "top", 10}, {"crop_top_less", "top", -10},
		{"crop_right_more", "right", 10}, {"crop_right_less", "right", -10},
		{"crop_bottom_more", "bottom", 10}, {"crop_bottom_less", "bottom", -10},
	}
	for _, step := range cropSteps {
		if a.shortcut(c, step.action) {
			a.nudgeCrop(step.side, step.delta)
		}
	}
	if a.shortcut(c, "crop_reset") {
		for _, cell := range a.cells {
			cell.cropMargins = cropMargins{}
		}
		a.setCrop("")
	}
	if a.shortcut(c, "shift_left") {
		a.shiftPicture(-shiftStepShare, 0)
	}
	if a.shortcut(c, "shift_right") {
		a.shiftPicture(shiftStepShare, 0)
	}
	if a.shortcut(c, "shift_up") {
		a.shiftPicture(0, -shiftStepShare)
	}
	if a.shortcut(c, "shift_down") {
		a.shiftPicture(0, shiftStepShare)
	}
	if a.shortcut(c, "sub_delay_later") {
		a.nudgeSubtitleDelay(engine.SubtitleDelayStepMS)
	}
	if a.shortcut(c, "sub_delay_earlier") {
		a.nudgeSubtitleDelay(-engine.SubtitleDelayStepMS)
	}
	if a.shortcut(c, "reload") {
		a.reloadActive()
	}
	if a.shortcut(c, "timecode") {
		a.openTimecodeDialog()
	}
	if a.shortcut(c, "take_screenshot") {
		a.saveFrame(a.activeEngine())
	}
	if a.shortcut(c, "rate_increase") {
		a.nudgeRate(0.25)
	}
	if a.shortcut(c, "rate_decrease") {
		a.nudgeRate(-0.25)
	}
	if a.shortcut(c, "next_file") {
		if cell := a.activeCell(); cell != nil {
			a.advanceList(cell)
		}
	}
	if a.shortcut(c, "previous_file") {
		if cell := a.activeCell(); cell != nil {
			a.previousList(cell)
		}
	}
	if a.shortcut(c, "crop_off") {
		for _, cell := range a.cells {
			cell.cropMargins = cropMargins{}
		}
		a.setCrop("")
	}
}

func (a *app) activeSnapshot() engine.Snapshot {
	if eng := a.activeEngine(); eng != nil {
		return eng.Snapshot()
	}
	return engine.Snapshot{}
}

func (a *app) seekActive(ms int64) {
	if eng := a.activeEngine(); eng != nil {
		c := a.activeCell()
		if c != nil {
			ms = seekableTime(ms, c.loopStart, c.loopEnd)
		}
		eng.SeekTo(ms)
	}
}

func (a *app) seekPercentActive(percent float64) {
	if eng := a.activeEngine(); eng != nil {
		a.seekActive(seekPercentTarget(eng.Snapshot().Length, percent))
	}
}

func (a *app) nudgeRate(delta float64) {
	if eng := a.activeEngine(); eng != nil {
		eng.SetRate(clampPlaybackRate(eng.Snapshot().Rate + delta))
	}
}

func (a *app) stepZoom(direction int) {
	a.defaultZoom = zoomRung(a.defaultZoom, direction, defaultZoomStep, false)
	for _, c := range a.cells {
		c.zoom = a.defaultZoom
	}
	a.saveSettings()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) resetZoom() {
	a.setZoom(1)
}

func (a *app) nudgeCrop(side string, delta float64) {
	for _, c := range a.cells {
		c.cropMargins = cropStep(c.cropMargins, side, delta)
	}
	a.dirty = true
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) setLoopPoint(isStart bool) {
	c := a.activeCell()
	if c == nil {
		return
	}
	at := a.activeSnapshot().Time
	var start, end *int64
	if isStart {
		start = &c.loopStart
		if c.loopEnd != 0 {
			end = &c.loopEnd
		}
	} else {
		end = &c.loopEnd
		if c.loopStart != 0 {
			start = &c.loopStart
		}
	}
	if !setLoopPoint(start, end, at, isStart) {
		return
	}
	a.dirty = true
	a.status = fmt.Sprintf("loop %d-%dms", c.loopStart, c.loopEnd)
}

func (a *app) resetLoopPoints() {
	for _, c := range a.cells {
		c.loopStart, c.loopEnd = 0, 0
	}
	a.dirty = true
	a.status = "loop reset"
}

func (a *app) nudgeSubtitleDelay(delta int64) {
	for _, c := range a.cells {
		c.subDelay = subtitleDelayNudge(c.subDelay, delta)
		if c.eng != nil {
			c.eng.SetSubtitleDelay(c.subDelay)
		}
	}
	a.dirty = true
	if c := a.activeCell(); c != nil {
		a.status = fmt.Sprintf("subtitle delay %+dms", c.subDelay)
	}
}

func (a *app) reloadActive() {
	if eng := a.activeEngine(); eng != nil {
		_ = eng.ReloadIfFailed(eng.Snapshot().Gen)
		_ = eng.Reload()
	}
}

func (a *app) openTimecodeDialog() {
	if a.timecodeWin != nil {
		return
	}
	a.timecodeInput = ""
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:          "Go to timecode",
		Width:          320,
		Height:         150,
		UseContentSize: true,
		Content:        ui.View(a.timecodeView),
	})
	a.timecodeWin = w
	w.OnClose(func(*mygo.CloseEvent) { a.timecodeWin = nil })
}

func (a *app) timecodeView(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		ui.TextInput(c.Key("timecode"), &a.timecodeInput).Label("hh:mm:ss.mmm")
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c.Key("timecode-ok"), "Go").Clicked() {
				if ms, ok := parseTimecode(a.timecodeInput); ok {
					a.seekActive(ms)
				} else {
					a.status = "invalid timecode"
				}
				if a.timecodeWin != nil {
					a.timecodeWin.Close()
				}
			}
			if ui.Button(c.Key("timecode-cancel"), "Cancel").Clicked() && a.timecodeWin != nil {
				a.timecodeWin.Close()
			}
		})
	})
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
	a.addRecent(paths[0])
}

func (a *app) openURLDialog() {
	if a.urlWin != nil {
		return
	}
	a.urlInput = ""
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:          "Open URL",
		Width:          400,
		Height:         170,
		UseContentSize: true,
		Content:        ui.View(a.urlView),
	})
	a.urlWin = w
	w.OnClose(func(*mygo.CloseEvent) { a.urlWin = nil })
}

func (a *app) urlView(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		ui.Text(c, "Stream URL (http/https/rtsp/HLS) or file:// URI").FontSize(13)
		ui.TextInput(c.Key("url"), &a.urlInput).Label("URL")
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c.Key("url-ok"), "Open").Clicked() && strings.TrimSpace(a.urlInput) != "" {
				raw := strings.TrimSpace(a.urlInput)
				if a.urlWin != nil {
					a.urlWin.Close()
				}
				a.playURL(raw)
			}
			if ui.Button(c.Key("url-cancel"), "Cancel").Clicked() && a.urlWin != nil {
				a.urlWin.Close()
			}
		})
	})
}

func (a *app) playURL(raw string) {
	a.addRecent(raw)
	if a.resolver == nil {
		a.resolver = resolve.New()
	}
	res := *a.resolver
	a.status = "resolving " + raw
	go func() {
		direct, err := res.Resolve(context.Background(), raw)
		mygo.RunOnMain(func() {
			if err != nil {
				if playableDirect(raw) {
					a.status = "resolver: " + err.Error() + " (direct)"
					a.setPlaylist([]string{raw})
				} else {
					a.status = err.Error()
				}
				return
			}
			a.status = ""
			a.setPlaylist([]string{direct})
		})
	}()
}

func playableDirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "file", "rtsp", "rtmp", "udp", "mms":
		return true
	}
	return false
}

func (a *app) setGrid(cfg grid.Config) {
	a.cfg = cfg
	a.dirty = true
	a.saveSettings()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) setSingleSide(side grid.StripSide) {
	a.singleSide = side
	a.saveSettings()
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

func (a *app) layoutName() string {
	switch {
	case a.single:
		return "single"
	case a.mosaic:
		return "mosaic"
	case a.free:
		return "free"
	default:
		return modeName(a.cfg.Mode)
	}
}

func stripSideName(s grid.StripSide) string {
	switch s {
	case grid.StripRight:
		return "right"
	case grid.StripLeft:
		return "left"
	case grid.StripBottom:
		return "bottom"
	case grid.StripTop:
		return "top"
	default:
		return "auto"
	}
}

func stripSideFromName(s string) grid.StripSide {
	switch s {
	case "right":
		return grid.StripRight
	case "left":
		return grid.StripLeft
	case "bottom":
		return grid.StripBottom
	case "top":
		return grid.StripTop
	default:
		return grid.StripAuto
	}
}

func validTiles(tiles []grid.Tile) bool {
	for _, t := range tiles {
		if !t.Valid() {
			return false
		}
	}
	return true
}

func setRawSetting(d *playlist.Document, key string, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	if d.Settings == nil {
		d.Settings = map[string]json.RawMessage{}
	}
	d.Settings[key] = raw
}

func getRawSetting(d *playlist.Document, key string, value any) bool {
	raw, ok := d.Settings[key]
	if !ok {
		return false
	}
	return json.Unmarshal(raw, value) == nil
}

// loadPlaylist replaces the cells and grid from a .gpls file. Main thread.
func (a *app) loadPlaylist(path string) error {
	d, err := playlist.Load(path)
	if err != nil {
		a.status = err.Error()
		return err
	}
	a.applyDocument(d, path)
	a.addRecent(path)
	return nil
}

// applyDocument replaces the cells and grid from a playlist document.
func (a *app) applyDocument(d *playlist.Document, path string) {
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
		switch v.CellViewMode {
		case "fill", "fit", "stretch":
			c.viewMode = v.CellViewMode
		}
		c.startPaused = strings.EqualFold(v.PlaybackState, "paused") ||
			strings.EqualFold(v.PlaybackState, "stopped")
		oldPath := c.path
		if len(c.list) > 0 {
			if v.ListIndex >= 0 && v.ListIndex < len(c.list) {
				c.listIndex = v.ListIndex
				c.path = c.list[c.listIndex]
			}
			switch v.ListEnd {
			case "loop", "stop", "next":
				c.listEnd = v.ListEnd
			}
			if action := normalizeEndAction(v.EndAction); action != "" {
				c.endAction = action
				c.loop = action == endLoop
			}
			switch v.ListFail {
			case listFailPlayNext, listFailShowError:
				c.listFail = v.ListFail
			}
			if v.ListFailWait != nil {
				wait := *v.ListFailWait
				c.listFailWait = &wait
			}
		}
		c.audioDelay = engine.ClampAudioDelay(v.AudioDelay)
		c.subDelay = engine.ClampSubtitleDelay(v.SubtitleDelay)
		c.loopStart = v.LoopStart
		c.loopEnd = v.LoopEnd
		if c.eng != nil {
			reload := c.path != oldPath
			if reload {
				c.eng.SetStartPosition(int64(c.startAt))
				if err := c.eng.Load(c.path); err != nil {
					a.status = err.Error()
				}
			} else {
				c.eng.SeekTo(int64(c.startAt))
			}
			c.eng.SetVolume(c.volume)
			c.eng.SetMute(c.muted)
			c.eng.SetRate(c.rate)
			c.eng.SetAudioDelay(c.audioDelay)
			c.eng.SetSubtitleDelay(c.subDelay)
			c.eng.SetLoop(c.loop)
			if c.startPaused {
				c.pauseOnPlay.Store(true)
				c.eng.Pause()
			} else {
				c.pauseOnPlay.Store(false)
				c.eng.Play()
			}
		}
	}
	if mode, rows, cols := d.GridState(); mode != "" {
		a.cfg = grid.Config{Mode: parseMode(mode), Rows: rows, Cols: cols}
	}
	a.free, a.mosaic, a.single = false, false, false
	a.singleShare = nil
	layout := ""
	_ = getRawSetting(d, "grid_layout", &layout)
	switch layout {
	case "free":
		a.free = true
	case "mosaic":
		a.mosaic = true
	case "single":
		a.single = true
		var side string
		if getRawSetting(d, "single_side", &side) {
			a.singleSide = stripSideFromName(side)
		} else {
			a.singleSide = grid.StripAuto
		}
		var share float64
		if getRawSetting(d, "single_share", &share) && share > 0 {
			if share < 0.05 {
				share = 0.05
			}
			if share > 0.4 {
				share = 0.4
			}
			a.singleShare = &share
		}
	}
	if a.free {
		var tiles []grid.Tile
		if getRawSetting(d, "grid_free_tiles", &tiles) && len(tiles) == len(a.cells) && validTiles(tiles) {
			a.freeTiles = tiles
		} else {
			a.freeTiles = grid.FreeLayout(len(a.cells))
		}
	}
	if a.mosaic {
		a.ensureMosaic()
		var tiles []grid.Tile
		if getRawSetting(d, "grid_mosaic_tiles", &tiles) && len(tiles) == len(a.cells) && validTiles(tiles) {
			a.mosaicTiles = tiles
		}
		var locked []int
		if getRawSetting(d, "grid_mosaic_locked", &locked) {
			a.mosaicLocked = map[int]bool{}
			for _, i := range locked {
				if i >= 0 && i < len(a.cells) {
					a.mosaicLocked[i] = true
				}
			}
		}
	}
	a.playlistPath = path
	a.dirty = false
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
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
		source := c.source
		if source == "" {
			source = c.path
		}
		v := playlist.Video{
			URI:             source,
			CurrentPosition: int(s.Time),
			IsMuted:         s.Muted,
			Volume:          float64(s.Volume) / 100,
			Rate:            s.Rate,
			PlaybackState:   s.Phase.String(),
			CellViewMode:    a.cellViewMode(c),
			ListIndex:       c.listIndex,
			ListEnd:         c.listEnd,
			EndAction:       c.endAction,
			ListFail:        c.listFail,
			ListFailWait:    cloneWait(c.listFailWait),
			AudioDelay:      c.audioDelay,
			SubtitleDelay:   c.subDelay,
			LoopStart:       c.loopStart,
			LoopEnd:         c.loopEnd,
		}
		if a.doc != nil && i < len(a.doc.Videos) {
			v.ID = a.doc.Videos[i].ID
			v.Title = a.doc.Videos[i].Title
			v.Extra = a.doc.Videos[i].Extra
		}
		d.Videos[i] = v
	}
	d.SetGridState(modeName(a.cfg.Mode), a.cfg.Rows, a.cfg.Cols)
	setRawSetting(d, "grid_layout", a.layoutName())
	if a.single {
		setRawSetting(d, "single_side", stripSideName(a.singleSide))
		if a.singleShare != nil {
			setRawSetting(d, "single_share", *a.singleShare)
		} else {
			delete(d.Settings, "single_share")
		}
	} else {
		delete(d.Settings, "single_side")
		delete(d.Settings, "single_share")
	}
	if a.free {
		setRawSetting(d, "grid_free_tiles", a.freeTiles)
	} else {
		delete(d.Settings, "grid_free_tiles")
	}
	if a.mosaic {
		a.ensureMosaic()
		setRawSetting(d, "grid_mosaic_tiles", a.mosaicTiles)
		locked := make([]int, 0, len(a.mosaicLocked))
		for i := range a.mosaicLocked {
			locked = append(locked, i)
		}
		sort.Ints(locked)
		setRawSetting(d, "grid_mosaic_locked", locked)
	} else {
		delete(d.Settings, "grid_mosaic_tiles")
		delete(d.Settings, "grid_mosaic_locked")
	}
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

func (a *app) setAspect(aspect string) {
	a.aspect = aspect
	for _, c := range a.cells {
		if c.eng != nil {
			c.eng.SetAspectRatio(aspect)
		}
	}
	a.saveSettings()
}

func (a *app) setZoom(scale float64) {
	a.defaultZoom = clampZoom(scale)
	for _, c := range a.cells {
		c.zoom = a.defaultZoom
	}
	a.saveSettings()
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) setAnchor(name string) {
	a.defaultAnchor = name
	x, y := parseAnchor(name)
	for _, c := range a.cells {
		c.anchorX, c.anchorY = x, y
	}
	a.saveSettings()
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) shiftPicture(dx, dy float64) {
	for _, c := range a.cells {
		c.shiftX = clampFloat(c.shiftX+dx, -1, 1)
		c.shiftY = clampFloat(c.shiftY+dy, -1, 1)
	}
	a.saveSettings()
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) setScale(scale float64) {
	a.scale = scale
	for _, c := range a.cells {
		if c.eng != nil {
			c.eng.SetScale(scale)
		}
	}
	a.saveSettings()
}

func (a *app) setCrop(crop string) {
	a.crop = crop
	a.saveSettings()
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) setViewMode(fill bool, stretch bool) {
	a.fill = fill
	a.stretch = stretch
	mode := viewModeName(fill, stretch)
	for _, c := range a.cells {
		c.viewMode = mode
	}
	a.saveSettings()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) refreshMenu() {
	if a.win != nil {
		mygo.App.SetMenu(a.menu())
	}
	if a.bookmarkWin != nil {
		a.bookmarkWin.Invalidate()
	}
}

func (a *app) addTrack(kind string) {
	eng := a.activeEngine()
	if eng == nil {
		return
	}
	title := "Load Subtitle File…"
	if kind == "audio" {
		title = "Load Audio File…"
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.win,
		Title:  title,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	for _, p := range paths {
		if kind == "subtitle" {
			eng.AddSubtitle(p)
		} else {
			eng.AddAudio(p)
		}
	}
	eng.RefreshTracks()
	if a.win != nil {
		a.win.Invalidate()
	}
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

func (a *app) activeSource() string {
	if a.active < 0 || a.active >= len(a.cells) {
		return ""
	}
	c := a.cells[a.active]
	if c.source != "" {
		return c.source
	}
	return c.path
}

// bookmarkKey is the current file, not the playlist/list source, so a
// playlist's bookmarks follow the item that is actually playing.
func (a *app) bookmarkKey() string {
	if a.active < 0 || a.active >= len(a.cells) {
		return ""
	}
	c := a.cells[a.active]
	if c.path != "" {
		return c.path
	}
	return c.source
}

func fmtClock(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	s := ms / 1000
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

func (a *app) addBookmark() {
	eng := a.activeEngine()
	key := a.bookmarkKey()
	if eng == nil || key == "" || a.bookmarks == nil {
		return
	}
	pos := eng.Snapshot().Time
	name := a.bookmarkName(key)
	a.bookmarks.Add(key, name, pos)
	if err := a.bookmarks.Save(); err != nil {
		a.status = err.Error()
	}
	a.refreshMenu()
	if a.bookmarkWin != nil {
		a.bookmarkWin.Invalidate()
	}
}

func (a *app) bookmarkName(key string) string {
	for n := len(a.bookmarks.List(key)) + 1; ; n++ {
		name := fmt.Sprintf("Bookmark %d", n)
		if !a.bookmarks.Has(key, name) {
			return name
		}
	}
}

func (a *app) jumpBookmark(pos int64) {
	if eng := a.activeEngine(); eng != nil {
		eng.SeekTo(pos)
	}
}

func (a *app) clearBookmarks() {
	key := a.bookmarkKey()
	if a.bookmarks == nil || key == "" {
		return
	}
	a.bookmarks.Clear(key)
	if err := a.bookmarks.Save(); err != nil {
		a.status = err.Error()
	}
	a.refreshMenu()
	if a.bookmarkWin != nil {
		a.bookmarkWin.Invalidate()
	}
}

func (a *app) saveFrame(eng *engine.Engine) {
	if eng == nil {
		return
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent: a.win,
		Title:  "Save frame",
		Filters: []mygo.FileFilter{{
			Name:       "PNG image",
			Extensions: []string{"png"},
		}},
	})
	if err != nil || path == "" {
		return
	}
	if !strings.HasSuffix(strings.ToLower(path), ".png") {
		path += ".png"
	}
	if err := eng.Screenshot(path); err != nil {
		a.status = err.Error()
		return
	}
	a.addRecent(path)
	a.status = "saved " + filepath.Base(path)
}

func (a *app) menu() *mygo.Menu {
	recentItems := make([]*mygo.MenuItem, 0, len(a.recent))
	for _, p := range a.recent {
		path := p
		recentItems = append(recentItems, &mygo.MenuItem{
			Label: filepath.Base(path),
			Click: func(*mygo.MenuItem, *mygo.Window) { a.openRecent(path) },
		})
	}
	if len(recentItems) == 0 {
		recentItems = append(recentItems, &mygo.MenuItem{Label: "No Recent Files", Disabled: true})
	}
	bookmarkItems := make([]*mygo.MenuItem, 0)
	if a.bookmarks != nil {
		for _, b := range a.bookmarks.List(a.bookmarkKey()) {
			b := b
			bookmarkItems = append(bookmarkItems, &mygo.MenuItem{
				Label: fmt.Sprintf("%s — %s", b.Name, fmtClock(b.Position)),
				Click: func(*mygo.MenuItem, *mygo.Window) { a.jumpBookmark(b.Position) },
			})
		}
	}
	if len(bookmarkItems) == 0 {
		bookmarkItems = append(bookmarkItems, &mygo.MenuItem{Label: "No Bookmarks", Disabled: true})
	}
	fill, stretch := a.activeViewFlags()
	listEnd := "loop"
	listFail := listFailPlayNext
	endAction := endLoop
	hasList := false
	if c := a.activeCell(); c != nil {
		endAction = c.finishAction()
		if len(c.list) > 0 {
			hasList = true
			if c.listEnd != "" {
				listEnd = c.listEnd
			}
			listFail = a.listFailAction(c)
		}
	}
	snapshotRestore, snapshotRemove := a.snapshotMenuItems()
	snapshotMenu := []*mygo.MenuItem{
		{Label: "Save Current…", Click: func(*mygo.MenuItem, *mygo.Window) { a.openSnapshotSave() }},
		{Type: mygo.MenuItemSeparator},
	}
	snapshotMenu = append(snapshotMenu, snapshotRestore...)
	snapshotMenu = append(snapshotMenu,
		&mygo.MenuItem{Type: mygo.MenuItemSeparator},
		&mygo.MenuItem{Label: "Delete", Submenu: snapshotRemove},
	)
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: a.tr("Actions", "File"), Submenu: []*mygo.MenuItem{
			{Label: a.tr("Actions", "Open…"), Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) { a.openDialog(false) }},
			{Label: a.tr("Actions", "Add…"), Accelerator: "CmdOrCtrl+Shift+O", Click: func(*mygo.MenuItem, *mygo.Window) { a.openDialog(true) }},
			{Type: mygo.MenuItemSeparator},
			{Label: a.tr("Actions", "Open URL…"), Accelerator: "CmdOrCtrl+U", Click: func(*mygo.MenuItem, *mygo.Window) { a.openURLDialog() }},
			{Label: a.tr("Actions", "Open Playlist…"), Click: func(*mygo.MenuItem, *mygo.Window) { a.openPlaylistDialog() }},
			{Label: "Open Recent", Submenu: recentItems},
			{Label: a.tr("Actions", "Save Playlist"), Accelerator: "CmdOrCtrl+S", Click: func(*mygo.MenuItem, *mygo.Window) { a.saveDialog() }},
			{Label: a.tr("Actions", "Save Playlist As…"), Accelerator: "CmdOrCtrl+Shift+S", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.playlistPath = ""
				a.saveDialog()
			}},
			{Type: mygo.MenuItemSeparator},
			{Label: a.tr("Actions", "Settings…"), Accelerator: "CmdOrCtrl+,", Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }},
			{Type: mygo.MenuItemSeparator},
			{Role: mygo.RoleClose},
		}},
		{Label: a.tr("Actions", "Playback"), Submenu: []*mygo.MenuItem{
			{Label: a.tr("Actions", "Play/Pause"), Accelerator: a.keys.Get("play_pause").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				if eng := a.activeEngine(); eng != nil {
					eng.Toggle()
				}
			}},
			{Label: a.tr("Actions", "Stop"), Click: func(*mygo.MenuItem, *mygo.Window) {
				if eng := a.activeEngine(); eng != nil {
					eng.Stop()
				}
			}},
			{Label: a.tr("Actions", "Previous in List"), Click: func(*mygo.MenuItem, *mygo.Window) {
				if a.active >= 0 && a.active < len(a.cells) {
					a.previousList(a.cells[a.active])
				}
			}},
			{Label: a.tr("Actions", "Next in List"), Click: func(*mygo.MenuItem, *mygo.Window) {
				if a.active >= 0 && a.active < len(a.cells) {
					a.advanceList(a.cells[a.active])
				}
			}},
			{Label: a.tr("Actions", "End of List"), Submenu: []*mygo.MenuItem{
				{Label: a.tr("Actions", "Loop"), Type: mygo.MenuItemRadio, Checked: listEnd == "loop", Disabled: !hasList,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setListEnd("loop") }},
				{Label: a.tr("Actions", "Stop"), Type: mygo.MenuItemRadio, Checked: listEnd == "stop", Disabled: !hasList,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setListEnd("stop") }},
				{Label: a.tr("Actions", "Next Cell"), Type: mygo.MenuItemRadio, Checked: listEnd == "next", Disabled: !hasList,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setListEnd("next") }},
			}},
			{Label: a.tr("Actions", "When Finished"), Submenu: []*mygo.MenuItem{
				{Label: a.tr("Actions", "Loop File"), Type: mygo.MenuItemRadio, Checked: endAction == endLoop,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setEndAction(endLoop) }},
				{Label: a.tr("Actions", "Next File"), Type: mygo.MenuItemRadio, Checked: endAction == endNext, Disabled: !hasList,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setEndAction(endNext) }},
				{Label: a.tr("Actions", "Previous File"), Type: mygo.MenuItemRadio, Checked: endAction == endPrevious, Disabled: !hasList,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setEndAction(endPrevious) }},
				{Label: a.tr("Actions", "Shuffle"), Type: mygo.MenuItemRadio, Checked: endAction == endShuffle, Disabled: !hasList,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setEndAction(endShuffle) }},
				{Label: a.tr("Actions", "Pause"), Type: mygo.MenuItemRadio, Checked: endAction == endPause,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setEndAction(endPause) }},
				{Label: a.tr("Actions", "Stop"), Type: mygo.MenuItemRadio, Checked: endAction == endStop,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setEndAction(endStop) }},
			}},
			{Label: a.tr("Actions", "When Video Fails"), Submenu: []*mygo.MenuItem{
				{Label: a.tr("Actions", "Play Next"), Type: mygo.MenuItemRadio, Checked: listFail == listFailPlayNext, Disabled: !hasList,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setListFail(listFailPlayNext) }},
				{Label: a.tr("Actions", "Show Error"), Type: mygo.MenuItemRadio, Checked: listFail == listFailShowError, Disabled: !hasList,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setListFail(listFailShowError) }},
			}},
			{Label: a.tr("Actions", "Save Frame…"), Click: func(*mygo.MenuItem, *mygo.Window) { a.saveFrame(a.activeEngine()) }},
			{Type: mygo.MenuItemSeparator},
			{Label: a.tr("Actions", "Seek -5s"), Accelerator: a.keys.Get("seek_back").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				if eng := a.activeEngine(); eng != nil {
					eng.SeekTo(max(eng.Snapshot().Time-5000, 0))
				}
			}},
			{Label: a.tr("Actions", "Seek +5s"), Accelerator: a.keys.Get("seek_forward").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				if eng := a.activeEngine(); eng != nil {
					eng.SeekTo(eng.Snapshot().Time + 5000)
				}
			}},
			{Type: mygo.MenuItemSeparator},
			{Label: a.tr("Actions", "Next Frame"), Accelerator: a.keys.Get("frame_next").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				if eng := a.activeEngine(); eng != nil {
					eng.NextFrame()
				}
			}},
			{Label: a.tr("Actions", "Previous Frame"), Accelerator: a.keys.Get("frame_prev").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				if eng := a.activeEngine(); eng != nil {
					eng.PreviousFrame()
				}
			}},
			{Label: a.tr("Actions", "Audio Later"), Accelerator: a.keys.Get("audio_later").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				a.nudgeAudioDelay(engine.AudioDelayStepMS)
			}},
			{Label: a.tr("Actions", "Audio Earlier"), Accelerator: a.keys.Get("audio_earlier").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				a.nudgeAudioDelay(-engine.AudioDelayStepMS)
			}},
			{Label: a.tr("Actions", "Reset Audio Delay"), Click: func(*mygo.MenuItem, *mygo.Window) { a.resetAudioDelay() }},
			{Label: a.tr("Actions", "Mute"), Accelerator: a.keys.Get("mute").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				if eng := a.activeEngine(); eng != nil {
					eng.SetMute(!eng.Snapshot().Muted)
				}
			}},
			{Label: a.tr("Actions", "Loop"), Accelerator: a.keys.Get("loop").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) { a.toggleLoop() }},
			{Label: "Rate", Submenu: []*mygo.MenuItem{
				{Label: "0.5x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setRate(0.5) }},
				{Label: "1x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setRate(1) }},
				{Label: "1.5x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setRate(1.5) }},
				{Label: "2x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setRate(2) }},
			}},
			{Type: mygo.MenuItemSeparator},
			{Label: a.tr("Actions", "Previous Cell"), Accelerator: a.keys.Get("prev_cell").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				a.setActive((a.active - 1 + len(a.cells)) % len(a.cells))
			}},
			{Label: a.tr("Actions", "Next Cell"), Accelerator: a.keys.Get("next_cell").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				a.setActive((a.active + 1) % len(a.cells))
			}},
		}},
		{Label: a.tr("Actions", "Grid"), Submenu: []*mygo.MenuItem{
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
			{Label: "Free Tiles", Type: mygo.MenuItemCheckbox, Checked: a.free, Click: func(*mygo.MenuItem, *mygo.Window) {
				a.free = !a.free
				if a.free {
					a.mosaic = false
					a.single = false
				}
				a.saveSettings()
				a.refreshMenu()
				if a.win != nil {
					a.win.Invalidate()
				}
			}},
			{Label: "Mosaic", Type: mygo.MenuItemCheckbox, Checked: a.mosaic, Click: func(*mygo.MenuItem, *mygo.Window) {
				a.mosaic = !a.mosaic
				if a.mosaic {
					a.free = false
					a.single = false
					a.ensureMosaic()
				}
				a.saveSettings()
				a.refreshMenu()
				if a.win != nil {
					a.win.Invalidate()
				}
			}},
			{Label: "Arrange Mosaic", Disabled: !a.mosaic, Click: func(*mygo.MenuItem, *mygo.Window) {
				a.arrangeMosaic()
			}},
			{Label: "Lock Active Tile", Type: mygo.MenuItemCheckbox, Checked: a.mosaic && a.mosaicLocked[a.active],
				Disabled: !a.mosaic || len(a.cells) == 0, Click: func(*mygo.MenuItem, *mygo.Window) {
					a.toggleMosaicLock()
				}},
			{Label: "Unlock All Tiles", Disabled: !a.mosaic, Click: func(*mygo.MenuItem, *mygo.Window) {
				a.unlockMosaic()
			}},
			{Label: "Snap to Edges", Type: mygo.MenuItemCheckbox, Checked: a.snap, Click: func(*mygo.MenuItem, *mygo.Window) {
				a.snap = !a.snap
				a.saveSettings()
				a.refreshMenu()
			}},
			{Label: "Free: Allow Tile Overlap", Type: mygo.MenuItemCheckbox, Checked: a.overlap, Click: func(*mygo.MenuItem, *mygo.Window) {
				a.overlap = !a.overlap
				a.saveSettings()
				a.refreshMenu()
			}},
			{Type: mygo.MenuItemSeparator},
			{Label: "Single Mode", Type: mygo.MenuItemCheckbox, Checked: a.single, Click: func(*mygo.MenuItem, *mygo.Window) {
				a.single = !a.single
				if a.single {
					a.free = false
					a.mosaic = false
				}
				a.saveSettings()
				a.refreshMenu()
				if a.win != nil {
					a.win.Invalidate()
				}
			}},
			{Label: "Strip Side", Submenu: []*mygo.MenuItem{
				{Label: "Auto", Click: func(*mygo.MenuItem, *mygo.Window) { a.setSingleSide(grid.StripAuto) }},
				{Label: "Right", Click: func(*mygo.MenuItem, *mygo.Window) { a.setSingleSide(grid.StripRight) }},
				{Label: "Left", Click: func(*mygo.MenuItem, *mygo.Window) { a.setSingleSide(grid.StripLeft) }},
				{Label: "Bottom", Click: func(*mygo.MenuItem, *mygo.Window) { a.setSingleSide(grid.StripBottom) }},
				{Label: "Top", Click: func(*mygo.MenuItem, *mygo.Window) { a.setSingleSide(grid.StripTop) }},
			}},
		}},
		{Label: a.tr("Actions", "View"), Submenu: []*mygo.MenuItem{
			{Label: "Fill (cover)", Type: mygo.MenuItemCheckbox, Checked: fill, Click: func(*mygo.MenuItem, *mygo.Window) {
				a.setViewMode(true, false)
				a.refreshMenu()
			}},
			{Label: "Fit (contain)", Type: mygo.MenuItemCheckbox, Checked: !fill && !stretch, Click: func(*mygo.MenuItem, *mygo.Window) {
				a.setViewMode(false, false)
				a.refreshMenu()
			}},
			{Label: "Stretch (cell shape)", Type: mygo.MenuItemCheckbox, Checked: stretch, Click: func(*mygo.MenuItem, *mygo.Window) {
				a.setViewMode(false, true)
				a.refreshMenu()
			}},
			{Type: mygo.MenuItemSeparator},
			{Label: a.tr("Actions", "Aspect Ratio"), Submenu: []*mygo.MenuItem{
				{Label: "Auto", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAspect("") }},
				{Label: "16:9", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAspect("16:9") }},
				{Label: "4:3", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAspect("4:3") }},
				{Label: "1:1", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAspect("1:1") }},
				{Label: "2.35:1", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAspect("2.35:1") }},
			}},
			{Label: a.tr("Actions", "Scale"), Submenu: []*mygo.MenuItem{
				{Label: "Fit", Click: func(*mygo.MenuItem, *mygo.Window) { a.setScale(0) }},
				{Label: "1x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setScale(1) }},
				{Label: "1.5x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setScale(1.5) }},
				{Label: "2x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setScale(2) }},
			}},
			{Label: a.tr("Actions", "Zoom"), Submenu: []*mygo.MenuItem{
				{Label: "Reset 1x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setZoom(1) }},
				{Label: "2x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setZoom(2) }},
				{Label: "5x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setZoom(5) }},
				{Label: "10x", Click: func(*mygo.MenuItem, *mygo.Window) { a.setZoom(10) }},
			}},
			{Label: a.tr("Actions", "Anchor"), Submenu: []*mygo.MenuItem{
				{Label: "Center", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAnchor("center") }},
				{Label: "Top", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAnchor("top") }},
				{Label: "Bottom", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAnchor("bottom") }},
				{Label: "Left", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAnchor("left") }},
				{Label: "Right", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAnchor("right") }},
				{Label: "Top-left", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAnchor("top-left") }},
				{Label: "Top-right", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAnchor("top-right") }},
				{Label: "Bottom-left", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAnchor("bottom-left") }},
				{Label: "Bottom-right", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAnchor("bottom-right") }},
			}},
			{Type: mygo.MenuItemSeparator},
			{Label: a.tr("Actions", "Move Picture"), Submenu: []*mygo.MenuItem{
				{Label: "Left", Click: func(*mygo.MenuItem, *mygo.Window) { a.shiftPicture(-0.1, 0) }},
				{Label: "Right", Click: func(*mygo.MenuItem, *mygo.Window) { a.shiftPicture(0.1, 0) }},
				{Label: "Up", Click: func(*mygo.MenuItem, *mygo.Window) { a.shiftPicture(0, -0.1) }},
				{Label: "Down", Click: func(*mygo.MenuItem, *mygo.Window) { a.shiftPicture(0, 0.1) }},
				{Label: "Center", Click: func(*mygo.MenuItem, *mygo.Window) { a.shiftPicture(-1, -1) }},
			}},
			{Label: a.tr("Actions", "Crop"), Submenu: []*mygo.MenuItem{
				{Label: "None", Click: func(*mygo.MenuItem, *mygo.Window) { a.setCrop("") }},
				{Label: "16:9", Click: func(*mygo.MenuItem, *mygo.Window) { a.setCrop("16:9") }},
				{Label: "4:3", Click: func(*mygo.MenuItem, *mygo.Window) { a.setCrop("4:3") }},
				{Label: "2.35:1", Click: func(*mygo.MenuItem, *mygo.Window) { a.setCrop("2.35:1") }},
			}},
		}},
		{Label: a.tr("Actions", "Bookmarks"), Submenu: append([]*mygo.MenuItem{
			{Label: "Add Bookmark", Click: func(*mygo.MenuItem, *mygo.Window) { a.addBookmark() }},
			{Label: "Manage Bookmarks…", Click: func(*mygo.MenuItem, *mygo.Window) { a.openBookmarkManager() }},
			{Label: "Clear This File's Bookmarks", Click: func(*mygo.MenuItem, *mygo.Window) { a.clearBookmarks() }},
			{Type: mygo.MenuItemSeparator},
		}, bookmarkItems...)},
		{Label: a.tr("Actions", "Snapshots"), Submenu: snapshotMenu},
		{Label: a.tr("Actions", "Tracks"), Submenu: []*mygo.MenuItem{
			{Label: "Load Subtitle File…", Click: func(*mygo.MenuItem, *mygo.Window) { a.addTrack("subtitle") }},
			{Label: "Load Audio File…", Click: func(*mygo.MenuItem, *mygo.Window) { a.addTrack("audio") }},
			{Type: mygo.MenuItemSeparator},
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
	screenshot := flag.String("screenshot", "", "save a frame of the active cell to path after 2s")
	single := flag.Bool("single", false, "start in single mode")
	free := flag.Bool("free", false, "start with free tiles")
	mosaic := flag.Bool("mosaic", false, "start with mosaic layout")
	openURL := flag.String("url", "", "open a URL through yt-dlp resolution at startup")
	userData := flag.String("user-data-dir", "", "override the user data directory")
	flag.Parse()
	if *userData != "" {
		mygo.App.SetPath(mygo.PathUserData, *userData)
	}

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
	if *openURL != "" {
		// The URL is resolved after startup; do not create (and then have
		// to close) the default fixture cells first.
		paths = nil
	}
	a := newApp(paths)
	a.dirty = false
	mygo.App.WhenReady(func() {
		a.loadSettings()
		if *single {
			a.single = true
			a.free = false
			a.mosaic = false
		}
		if *mosaic {
			a.mosaic = true
			a.free = false
			a.single = false
		}
		if *free {
			a.free = true
			a.mosaic = false
			a.single = false
		}
		if *playlistPath != "" {
			if err := a.loadPlaylist(*playlistPath); err != nil {
				log.Printf("playlist: %v", err)
			}
		}
		a.dirty = false
		opts := mygo.WindowOptions{
			Title:          "GridPlayer",
			Width:          1280,
			Height:         720,
			UseContentSize: true,
			Content:        ui.View(a.view),
		}
		if a.settings != nil {
			if v := atoi(a.settings.Get("window/width", ""), 0); v > 0 {
				opts.Width = v
			}
			if v := atoi(a.settings.Get("window/height", ""), 0); v > 0 {
				opts.Height = v
			}
			if s := a.settings.Get("window/x", ""); s != "" {
				x := atoi(s, 0)
				if sy := a.settings.Get("window/y", ""); sy != "" {
					y := atoi(sy, 0)
					if visibleOnDisplays(x, y, opts.Width, opts.Height) {
						opts.X, opts.Y = x, y
					}
				}
			}
		}
		w := mygo.NewWindow(opts)
		a.win = w
		a.resolve = func() (uintptr, uintptr, float64, bool) {
			return nativehost.Resolve(w.NativeHandle())
		}
		w.OnClose(func(e *mygo.CloseEvent) { a.closeRequested(e) })
		w.OnFileDrop(func(e *mygo.FileDropEvent) {
			if len(e.Paths) > 0 {
				a.dropFiles(len(a.cells), e.Paths)
			}
		})
		mygo.App.SetMenu(a.menu())
		if *openURL != "" {
			a.playURL(*openURL)
		}
		if *screenshot != "" {
			go func() {
				time.Sleep(3 * time.Second)
				mygo.RunOnMain(func() {
					if eng := a.activeEngine(); eng != nil {
						if err := eng.Screenshot(*screenshot); err != nil {
							log.Printf("screenshot: %v", err)
						} else {
							log.Printf("screenshot saved: %s", *screenshot)
						}
					}
				})
			}()
		}
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
