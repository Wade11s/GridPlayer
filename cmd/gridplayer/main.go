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
	"sync"
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
	"gridplayer-go/internal/sponsorblock"
	"gridplayer-go/internal/vlcmac"
)

// cellGap is the space left between cells. The frozen CellLayout sets
// spacing 0 and every grid Box shares its edge with its neighbour, so cells
// meet edge to edge; any positive value is a Go-only gap.
const cellGap = 0.0

// maxFixedSize is the frozen MAX_FIXED_SIZE: the largest rows or columns a
// fixed grid accepts. It bounds a hand-edited document, not normal use.
const maxFixedSize = 100

type cell struct {
	label   string
	source  string
	path    string
	id      string
	title   string
	extra   map[string]json.RawMessage
	eng     *engine.Engine
	view    uintptr
	clip    uintptr
	failed  bool
	listErr string

	volume            int
	muted             bool
	rate              float64
	audioDelay        int64
	subDelay          int64
	externalAudio     []string
	externalSubtitles []string
	segment           atomic.Pointer[loopSegment]
	cropMargins       cropMargins
	startAt           int
	startPaused       bool
	pauseOnPlay       atomic.Bool
	retries           atomic.Int32
	viewMode          string
	aspect            string
	scale             float64
	crop              string
	zoom              float64
	anchorX, anchorY  float64
	shiftX, shiftY    float64
	transform         string

	list      []string
	listIndex int
	listEnd   string
	// listEndRaw/listFailRaw retain an unrecognised document enum verbatim.
	listEndRaw  string
	endAction   string
	listFail    string
	listFailRaw string
	shuffleBag  shuffleBag
	// listFailWait is set when this cell overrides the default wait.
	// Zero means move on at once.
	listFailWait    *int
	failStreak      int
	netFailStreak   int
	skipGen         uint64
	sponsorSkipEnd  atomic.Int64
	sponsorFetchGen atomic.Uint64
	// sponsorOverrideState is 0 to follow the global toggle, 1 for on, 2 for
	// off. It is atomic because the watch goroutine reads it.
	sponsorOverrideState atomic.Int32
	// sponsorOverrideModes holds per-category overrides for this cell. It is
	// guarded by app.sponsorMu.
	sponsorOverrideModes map[string]string
	// sponsorSource keeps the original page URL when the cell plays a resolved
	// direct URL, so SponsorBlock can still find the video.
	sponsorSource string
	loop          bool
	advancing     bool
	endedGen      uint64
}

// loopSegment is the cell's fragment-loop bounds. It is one pointer so the
// watch goroutine never observes a new start beside a stale end.
type loopSegment struct {
	start  int64
	end    int64
	active bool
}

func (c *cell) loopState() (start, end int64, active bool) {
	if p := c.segment.Load(); p != nil {
		return p.start, p.end, p.active
	}
	return 0, 0, false
}

func (c *cell) loopBounds() (start, end int64) {
	start, end, _ = c.loopState()
	return start, end
}

func (c *cell) setLoopBounds(start, end int64) {
	c.setLoopSegment(start, end, start > 0 || end > 0)
}

func (c *cell) setLoopSegment(start, end int64, active bool) {
	c.segment.Store(&loopSegment{start: start, end: end, active: active})
}

type app struct {
	win     *mygo.Window
	resolve func() (cv, sv uintptr, h float64, ok bool)
	cells   []*cell
	active  int
	cfg     grid.Config
	// askOverflow asks before a smaller fixed grid closes videos; nil opens
	// the frozen dialog. Tests inject one.
	askOverflow         func(over, capacity int) bool
	template            string
	fill                bool
	stretch             bool
	single              bool
	singleSide          grid.StripSide
	singleShare         *float64
	free                bool
	freeTiles           []grid.Tile
	freeOrder           []int
	mosaic              bool
	mosaicTiles         []grid.Tile
	mosaicOrder         []int
	mosaicLocked        map[int]bool
	snap                bool
	overlap             bool
	showGridLines       bool
	freeArea            [2]float64
	nativeClips         []uintptr
	moveIdx             int
	moveLast            [2]float32
	moveOK              bool
	aspect              string
	scale               float64
	crop                string
	fillLimit           int
	zoomToCursor        bool
	pauseMinimized      bool
	minimizePaused      map[*cell]bool
	inhibitScreensaver  bool
	minimized           bool
	screensaver         screensaverGuard
	stayOnTop           bool
	startMaximized      bool
	startFullscreen     bool
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
	sponsorEnabled      atomic.Bool
	sponsorModes        map[string]string
	sponsorClient       *sponsorblock.Client
	sponsorMu           sync.Mutex
	sponsorSpans        map[*cell][]sponsorblock.Span
	sponsorSegments     map[*cell][]sponsorblock.Segment
	sponsorLoading      map[*cell]bool
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
	defaultTransform    string
	defaultEnd          string
	retryMode           atomic.Int32
	retryTimes          atomic.Int32
	listFailMode        atomic.Int32
	listFailWait        atomic.Int32
	autoReloadMin       atomic.Int64
}

func newApp(paths []string) *app {
	a := &app{
		cfg:                grid.Config{Mode: grid.AutoRows},
		fill:               true,
		snap:               true,
		overlap:            true,
		showGridLines:      true,
		defaultVolume:      80,
		defaultRate:        1,
		defaultLoop:        true,
		defaultZoom:        1,
		defaultAnchor:      "center",
		defaultTransform:   "none",
		fillLimit:          100,
		pauseMinimized:     true,
		inhibitScreensaver: true,
		resolver:           resolve.New(),
		keys:               keymap.Default(),
		language:           "en_US",
		sponsorModes:       sponsorblock.DefaultModes(),
		sponsorClient:      sponsorblock.New(),
		sponsorSpans:       map[*cell][]sponsorblock.Span{},
		sponsorSegments:    map[*cell][]sponsorblock.Segment{},
		sponsorLoading:     map[*cell]bool{},
		subtitle:           subtitleStyle{Opacity: 255},
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
	a.pauseMinimized = st.Get("playlist/pause_minimized", "true") == "true"
	a.zoomToCursor = st.Get("player/zoom_to_cursor", "false") == "true"
	a.inhibitScreensaver = st.Get("player/inhibit_screensaver", "true") == "true"
	a.stayOnTop = st.Get("player/stay_on_top", "false") == "true"
	a.startMaximized = st.Get("player/start_maximized", "false") == "true"
	a.startFullscreen = st.Get("player/start_fullscreen", "false") == "true"
	a.keys = keymap.Load(func(k string) string { return st.Get(k, "") })
	a.cfg = grid.Config{
		Mode:        parseMode(st.Get("playlist/grid_mode", "auto_rows")),
		Rows:        clampInt(atoi(st.Get("playlist/grid_rows", "3"), 3), 1, maxFixedSize),
		Cols:        clampInt(atoi(st.Get("playlist/grid_cols", "3"), 3), 1, maxFixedSize),
		Preallocate: st.Get("playlist/grid_preallocate", "false") == "true",
	}
	a.template = st.Get("playlist/grid_template", "")
	if _, ok := grid.FindTemplate(a.template); !ok || a.cfg.Mode != grid.Fixed {
		a.template = ""
	}
	a.single = st.Get("playlist/single_mode_strip", "off") != "off"
	a.free = st.Get("playlist/grid_free", "false") == "true"
	a.mosaic = st.Get("playlist/grid_mosaic", "false") == "true"
	a.snap = st.Get("playlist/grid_snap", "true") == "true"
	a.overlap = st.Get("playlist/grid_overlap", "true") == "true"
	a.showGridLines = st.Get("playlist/show_grid_lines", "true") == "true"
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
	if raw := st.Get("playlist/grid_mosaic_order", ""); raw != "" {
		var order []int
		if err := json.Unmarshal([]byte(raw), &order); err == nil {
			a.mosaicOrder = grid.Restack(order, len(a.cells), a.mosaicLocked)
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
	a.defaultTransform = st.Get("video_defaults/transform", "none")
	a.sponsorEnabled.Store(st.Get("sponsorblock/enabled", "false") == "true")
	for _, category := range sponsorblock.Categories {
		a.sponsorModes[category.Name] = st.Get("sponsorblock/"+category.Name, category.Mode)
	}
	a.crop = st.Get("video_defaults/crop_ratio", "")
	a.fillLimit = clampInt(atoi(st.Get("video_defaults/fill_limit", "100"), 100), 0, 100)
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
		if len(c.list) == 0 {
			c.loop = a.defaultLoop
			a.applyDefaultEnd(c)
		}
		c.zoom = a.defaultZoom
		c.anchorX, c.anchorY = parseAnchor(a.defaultAnchor)
		c.transform = a.defaultTransform
		c.viewMode = viewModeName(a.fill, a.stretch)
		c.aspect = a.aspect
		c.scale = a.scale
		c.crop = a.crop
		a.fetchSponsor(c, c.source)
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
	a.resolver.ProxyMode, a.resolver.Proxy = networkProxySettings(st)
	a.resolver.Cookies = st.Get("network/cookies", "")
	a.resolver.Format = st.Get("network/format", "best")
	a.resolver.Path = st.Get("network/ytdlp_path", a.resolver.Path)
	a.resolver.Streamlink = st.Get("network/streamlink_path", a.resolver.Streamlink)
}

// networkProxySettings reads the frozen network/proxy_mode and
// network/proxy_url, migrating the single URL older Go builds wrote.
func networkProxySettings(st *settings.Store) (mode, url string) {
	mode = strings.ToLower(strings.TrimSpace(st.Get("network/proxy_mode", "")))
	url = strings.TrimSpace(st.Get("network/proxy_url", ""))
	switch mode {
	case "system", "none", "custom":
		return mode, url
	}
	if legacy := strings.TrimSpace(st.Get("network/proxy", "")); legacy != "" {
		return "custom", legacy
	}
	return "system", ""
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
	a.settings.Set("playlist/grid_preallocate", fmt.Sprint(a.cfg.Preallocate))
	a.settings.Set("playlist/grid_template", a.template)
	a.settings.Set("playlist/grid_free", fmt.Sprint(a.free))
	a.settings.Set("playlist/grid_mosaic", fmt.Sprint(a.mosaic))
	a.settings.Set("playlist/grid_snap", fmt.Sprint(a.snap))
	a.settings.Set("playlist/grid_overlap", fmt.Sprint(a.overlap))
	a.settings.Set("playlist/show_grid_lines", fmt.Sprint(a.showGridLines))
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
		if raw, err := json.Marshal(a.mosaicOrder); err == nil {
			a.settings.Set("playlist/grid_mosaic_order", string(raw))
		}
	} else {
		a.settings.Delete("playlist/grid_mosaic_tiles")
		a.settings.Delete("playlist/grid_mosaic_locked")
		a.settings.Delete("playlist/grid_mosaic_order")
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
	a.settings.Set("playlist/pause_minimized", fmt.Sprint(a.pauseMinimized))
	a.settings.Set("player/inhibit_screensaver", fmt.Sprint(a.inhibitScreensaver))
	a.settings.Set("player/stay_on_top", fmt.Sprint(a.stayOnTop))
	a.settings.Set("player/start_maximized", fmt.Sprint(a.startMaximized))
	a.settings.Set("player/start_fullscreen", fmt.Sprint(a.startFullscreen))
	a.settings.Set("video_defaults/aspect_ratio", a.aspect)
	a.settings.Set("video_defaults/scale", fmt.Sprintf("%g", a.scale))
	a.settings.Set("video_defaults/zoom", fmt.Sprintf("%g", a.defaultZoom))
	a.settings.Set("video_defaults/anchor", a.defaultAnchor)
	a.settings.Set("video_defaults/transform", a.defaultTransform)
	a.settings.Set("sponsorblock/enabled", fmt.Sprint(a.sponsorEnabled.Load()))
	for _, category := range sponsorblock.Categories {
		a.settings.Set("sponsorblock/"+category.Name, a.sponsorModes[category.Name])
	}
	a.settings.Set("video_defaults/crop_ratio", a.crop)
	a.settings.Set("video_defaults/fill_limit", fmt.Sprint(a.fillLimit))
	a.settings.Set("player/zoom_to_cursor", fmt.Sprint(a.zoomToCursor))
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
		a.settings.Set("network/proxy_mode", a.resolver.ProxyMode)
		a.settings.Set("network/proxy_url", a.resolver.Proxy)
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

// applyDefaultEnd gives a plain file the configured end action. The helper
// skips list cells entirely: a loop default is carried by c.loop, and newCell
// applies a non-loop default to list cells through its own list branch.
func (a *app) applyDefaultEnd(c *cell) {
	if c == nil || len(c.list) > 0 {
		return
	}
	if action := normalizeEndAction(a.defaultEnd); action != "" && action != endLoop {
		c.endAction = action
	}
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
	a.resetFreeTiles()
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
		label:     labelFor(p),
		source:    p,
		path:      p,
		volume:    a.defaultVolume,
		rate:      a.defaultRate,
		loop:      a.defaultLoop,
		viewMode:  viewModeName(a.fill, a.stretch),
		aspect:    a.aspect,
		scale:     a.scale,
		crop:      a.crop,
		zoom:      a.defaultZoom,
		transform: a.defaultTransform,
	}
	c.anchorX, c.anchorY = parseAnchor(a.defaultAnchor)
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
	// A plain file takes the default end action too; a loop default is
	// carried by c.loop rather than an action value.
	a.applyDefaultEnd(c)
	a.fetchSponsor(c, p)
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
	a.resetFreeTiles()
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
	c.sponsorFetchGen.Add(1)
	a.sponsorMu.Lock()
	delete(a.sponsorSpans, c)
	delete(a.sponsorSegments, c)
	delete(a.sponsorLoading, c)
	a.sponsorMu.Unlock()
	a.refreshKeepAwake()
}

func (a *app) fetchSponsor(c *cell, raw string) {
	if c == nil {
		return
	}
	videoID, ok := sponsorblock.YouTubeID(raw)
	if !ok {
		return
	}
	// Remember the page URL even when the fetch is off, so enabling
	// SponsorBlock later still finds the video behind a resolved direct URL.
	c.sponsorSource = raw
	if !a.sponsorEnabledFor(c) || a.sponsorClient == nil {
		return
	}
	a.sponsorMu.Lock()
	if a.sponsorLoading[c] || a.sponsorSpans[c] != nil || a.sponsorSegments[c] != nil {
		a.sponsorMu.Unlock()
		return
	}
	a.sponsorLoading[c] = true
	a.sponsorMu.Unlock()
	gen := c.sponsorFetchGen.Add(1)
	modes := a.sponsorModesFor(c)
	categories := sponsorCategoriesFrom(modes)
	if len(categories) == 0 {
		a.sponsorMu.Lock()
		delete(a.sponsorLoading, c)
		a.sponsorMu.Unlock()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		segments, err := a.sponsorClient.Fetch(ctx, videoID, categories)
		a.sponsorMu.Lock()
		if c.sponsorFetchGen.Load() == gen {
			delete(a.sponsorLoading, c)
			if err == nil {
				a.sponsorSegments[c] = segments
				a.sponsorSpans[c] = sponsorblock.SkipSpans(segments, modes)
			}
		}
		a.sponsorMu.Unlock()
		if err == nil {
			mygo.RunOnMain(func() {
				if a.win != nil {
					a.win.Invalidate()
				}
			})
		}
	}()
}

func (a *app) resetSponsor(c *cell, raw string) {
	if c == nil {
		return
	}
	if _, ok := sponsorblock.YouTubeID(raw); ok {
		c.sponsorSource = raw
	} else if raw != "" {
		c.sponsorSource = ""
	}
	c.sponsorFetchGen.Add(1)
	a.sponsorMu.Lock()
	delete(a.sponsorSpans, c)
	delete(a.sponsorSegments, c)
	delete(a.sponsorLoading, c)
	a.sponsorMu.Unlock()
	c.sponsorSkipEnd.Store(0)
	a.fetchSponsor(c, raw)
}

// resetAllSponsor drops every cell's spans and segments and fences off any
// fetch still in flight, so a settings change cannot land stale results.
func (a *app) resetAllSponsor() {
	a.sponsorMu.Lock()
	defer a.sponsorMu.Unlock()
	for _, c := range a.cells {
		c.sponsorFetchGen.Add(1)
		c.sponsorSkipEnd.Store(0)
	}
	a.sponsorSpans = map[*cell][]sponsorblock.Span{}
	a.sponsorSegments = map[*cell][]sponsorblock.Segment{}
	a.sponsorLoading = map[*cell]bool{}
}

func (a *app) maybeSkipSponsor(c *cell, eng *engine.Engine, s engine.Snapshot) {
	if c == nil || eng == nil || !a.sponsorEnabledFor(c) || s.Phase != engine.Playing || s.Length <= 0 {
		return
	}
	a.sponsorMu.Lock()
	spans := append([]sponsorblock.Span(nil), a.sponsorSpans[c]...)
	a.sponsorMu.Unlock()
	span, ok := sponsorblock.SpanAt(spans, s.Time)
	if !ok || !sponsorSpanIsSkip(a.sponsorModesFor(c), span) {
		c.sponsorSkipEnd.Store(0)
		return
	}
	if span.End-s.Time < sponsorblock.SkipMinMS {
		return
	}
	target := span.End
	if s.Length > 0 && span.End >= s.Length-sponsorblock.SkipEndMargin {
		target = s.Length
	}
	if target == c.sponsorSkipEnd.Load() {
		return
	}
	c.sponsorSkipEnd.Store(target)
	start, end, active := c.loopState()
	eng.SeekTo(clampSegmentSeek(start, end, s.Length, target, active))
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
		eng.SetTransform(c.transform)
		eng.SetStartPosition(int64(c.startAt))
		eng.SetExternalFilesForLoad(c.externalAudio, c.externalSubtitles)
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
		a.fetchSponsor(c, c.path)
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
			c.eng.SetAspectRatio(c.aspect)
		}
		c.eng.SetScale(c.scale)
	}
	shape, crop := a.cellShape(c, s)
	place := placePicture(shape,
		r.W, r.H, fill, stretch, viewZoom{Scale: c.zoom, Crop: crop, FillLimit: a.fillLimit, AnchorX: c.anchorX, AnchorY: c.anchorY, ShiftX: c.shiftX, ShiftY: c.shiftY})
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
	loopSeeking := false
	loopSeekFrom := int64(0)
	loopSeekTries := 0
	for {
		select {
		case <-eng.Updates():
			s := eng.Snapshot()
			loopStart, loopEnd, loopActive := c.loopState()
			if s.Phase == engine.Playing && loopActive {
				end := loopEnd
				if end <= 0 {
					end = s.Length
				}
				if target, seek := loopSeekTarget(s.Time, loopStart, end); seek {
					if !loopSeeking || (loopSeekFrom != s.Time && loopSeekTries < 2) {
						loopSeeking = true
						loopSeekFrom = s.Time
						loopSeekTries++
						log.Printf("loop segment: seek %d -> %dms", s.Time, target)
						eng.SeekTo(target)
					}
				} else {
					loopSeeking = false
					loopSeekTries = 0
				}
			} else {
				loopSeeking = false
				loopSeekTries = 0
			}
			if s.Phase == engine.Playing {
				c.retries.Store(0)
				lastRetryGen = 0
				if !wasPlaying {
					mygo.RunOnMain(func() {
						a.refreshKeepAwake()
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
				if wasPlaying {
					mygo.RunOnMain(func() { a.refreshKeepAwake() })
				}
				wasPlaying = false
			}
			if c.pauseOnPlay.Load() && s.Phase == engine.Playing {
				c.pauseOnPlay.Store(false)
				eng.Pause()
			}
			a.maybeSkipSponsor(c, eng, s)
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
			if s.Phase == engine.Ended {
				if loopActive {
					if c.shouldAdvance(s.Gen) {
						log.Printf("loop segment: restart at end")
						if err := eng.Reload(); err != nil {
							log.Printf("loop segment: reload: %v", err)
							mygo.RunOnMain(func() {
								a.status = "loop restart failed: " + err.Error()
								a.finishVideo(c)
							})
						} else {
							eng.SeekTo(loopStart)
						}
					}
				} else if c.shouldAdvance(s.Gen) {
					mygo.RunOnMain(func() { a.finishVideo(c) })
				}
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

// nextList is a viewer-requested step. Like upstream, manual Next wraps at
// the end regardless of the automatic list-end policy.
func (a *app) nextList(c *cell) { a.stepList(c, false) }

// advanceList is an automatic end/failure step, so its boundary applies the
// configured list-end policy.
func (a *app) advanceList(c *cell) { a.stepList(c, true) }

func (a *app) stepList(c *cell, isEnd bool) {
	if c == nil {
		return
	}
	c.skipGen = 0
	if c.advancing || len(c.list) == 0 {
		return
	}
	next := c.listIndex + 1
	if next >= len(c.list) {
		if isEnd && a.handleListEnd(c, true) {
			return
		}
		next = 0
	}
	clearExternalFiles(c)
	c.advancing = true
	defer func() { c.advancing = false }()
	c.listIndex = next
	c.path = c.list[next]
	c.setLoopSegment(0, 0, false)
	c.startAt = 0
	c.retries.Store(0)
	a.resetSponsor(c, c.path)
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
	clearExternalFiles(c)
	c.advancing = true
	defer func() { c.advancing = false }()
	c.listIndex = prev
	c.path = c.list[prev]
	c.setLoopSegment(0, 0, false)
	c.startAt = 0
	c.retries.Store(0)
	a.resetSponsor(c, c.path)
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
	mode = normalizeListEnd(mode)
	if mode == "" {
		return
	}
	c.listEnd = mode
	c.listEndRaw = ""
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
		a.pauseAtStart(c)
	}
}

// pauseAtStart is the frozen pause end action: back to the loop start, then
// paused. libVLC can drop a pause issued before the reopened input exists,
// so use the established paused-restore guard.
func (a *app) pauseAtStart(c *cell) {
	if c == nil || c.eng == nil {
		return
	}
	start, _ := c.loopBounds()
	if start < 0 {
		start = 0
	}
	log.Printf("pause at start: %dms", start)
	// A chapter jump pauses a playing input: seek and pause, as the frozen
	// source does. An ended input has to be reopened behind the
	// paused-restore guard instead.
	if c.eng.Snapshot().Phase != engine.Ended {
		a.seekCellTo(c, start)
		c.eng.Pause()
		return
	}
	c.startAt = int(start)
	c.startPaused = true
	c.skipGen = 0
	c.pauseOnPlay.Store(true)
	c.eng.SetStartPosition(start)
	if err := c.eng.Load(c.path); err != nil {
		a.status = err.Error()
		return
	}
	c.eng.Pause()
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
	if c.listIndex == 0 && a.handleListEnd(c, false) {
		return
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
	c.shuffleBag.sync(len(c.list), c.listIndex, randIntN)
	if c.shuffleBag.roundOver() && normalizeListEnd(c.listEnd) != listEndLoop &&
		a.handleListEnd(c, true) {
		return
	}
	index := c.shuffleBag.next(randIntN)
	if index >= 0 {
		a.loadListIndex(c, index)
	}
}

func randIntN(n int) int { return rand.IntN(n) }

func (a *app) loadListIndex(c *cell, index int) {
	if c == nil || index < 0 || index >= len(c.list) || c.advancing {
		return
	}
	clearExternalFiles(c)
	c.skipGen = 0
	c.advancing = true
	defer func() { c.advancing = false }()
	c.listIndex = index
	c.path = c.list[index]
	c.setLoopSegment(0, 0, false)
	c.startAt = 0
	c.retries.Store(0)
	a.resetSponsor(c, c.path)
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
	c.listFailRaw = ""
	c.skipGen = 0
	a.dirty = true
	a.refreshMenu()
}

func (a *app) ensureMosaic() {
	if len(a.mosaicTiles) != len(a.cells) {
		a.resetMosaic()
	}
	if a.mosaicLocked == nil {
		a.mosaicLocked = map[int]bool{}
	}
	a.mosaicOrder = grid.Restack(a.mosaicOrder, len(a.cells), a.mosaicLocked)
}

func (a *app) resetMosaic() {
	a.mosaicTiles = grid.Mosaic(len(a.cells))
	a.mosaicOrder = grid.DefaultOrder(len(a.cells))
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
		a.mosaicOrder = grid.Restack(a.mosaicOrder, len(a.cells), a.mosaicLocked)
	} else {
		a.mosaicLocked[a.active] = true
		// Locking changes the tier, not the front-to-back order: the tile
		// joins the locked tier and the locked tiles keep their order.
		a.mosaicOrder = grid.Restack(a.mosaicOrder, len(a.cells), a.mosaicLocked)
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
	a.mosaicOrder = grid.Restack(a.mosaicOrder, len(a.cells), a.mosaicLocked)
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
	a.screensaver.Close()
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

// fixedGrid reports a Fixed layout that is actually on show: Free Tiles,
// Mosaic and Single Mode keep cfg.Mode but replace the grid.
func (a *app) fixedGrid() bool {
	return a.cfg.Mode == grid.Fixed && !a.free && !a.mosaic && !a.single
}

// emptyCellRects returns the rects of the fixed grid places no cell covers.
// A template uses its own places, so its empty cells follow the big cell.
func (a *app) emptyCellRects(area grid.Rect) []grid.Rect {
	if !a.fixedGrid() || !a.cfg.Preallocate || len(a.cells) == 0 {
		return nil
	}
	if a.template != "" {
		template, ok := grid.FindTemplate(a.template)
		if !ok {
			return nil
		}
		places := template.Places()
		// Cells take the places in order; the frozen grid then draws one
		// empty cell per free position, so an unoccupied big place becomes
		// a block of 1x1 empties.
		covered := map[[2]int]bool{}
		for i, p := range places {
			if i >= len(a.cells) {
				break
			}
			for row := p[0]; row < p[0]+p[2]; row++ {
				for col := p[1]; col < p[1]+p[3]; col++ {
					covered[[2]int{row, col}] = true
				}
			}
		}
		var out []grid.Rect
		for row := 0; row < template.Rows; row++ {
			for col := 0; col < template.Cols; col++ {
				if covered[[2]int{row, col}] {
					continue
				}
				out = append(out, grid.CellRect(template, row, col, area, cellGap))
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	rows, cols := grid.Dims(len(a.cells), a.cfg)
	if rows*cols <= len(a.cells) {
		return nil
	}
	slots := grid.Layout(rows*cols, area, cellGap, a.cfg)
	if len(slots) != rows*cols {
		return nil
	}
	return slots[len(a.cells):]
}

func (a *app) view(c *ui.Context) {
	a.shortcuts(c)
	defer a.handleDropped()
	w, h := c.Size()
	area := grid.Rect{W: float64(w), H: float64(h)}
	a.freeArea = [2]float64{float64(w), float64(h)}
	// Preallocate belongs to a fixed grid on show: Single Mode with one
	// video (and Free/Mosaic) must not shrink a cell to a grid slot.
	layoutCfg := a.cfg
	if !a.fixedGrid() {
		layoutCfg.Preallocate = false
	}
	rects := grid.Layout(len(a.cells), area, cellGap, layoutCfg)
	if a.cfg.Mode == grid.Shapes && !a.free && !a.mosaic && !a.single {
		shapes := make([]float64, len(a.cells))
		for i, c := range a.cells {
			if c.eng != nil {
				shapes[i] = a.pictureShape(c, c.eng.Snapshot())
			}
		}
		rects = grid.FitShapes(len(a.cells), area, cellGap, shapes)
	}
	if a.template != "" {
		if template, ok := grid.FindTemplate(a.template); ok {
			if templateRects := grid.TemplateLayout(len(a.cells), area, cellGap, template); templateRects != nil {
				rects = templateRects
			}
		}
	}
	if a.mosaic {
		a.ensureMosaic()
		tiles := a.mosaicTiles
		rects = make([]grid.Rect, len(tiles))
		for i, t := range tiles {
			rects[i] = t.Rect(area, cellGap)
		}
	} else if a.free {
		if len(a.freeTiles) != len(a.cells) {
			a.resetFreeTiles()
		}
		tiles := a.freeTiles
		rects = make([]grid.Rect, len(tiles))
		for i, t := range tiles {
			rects[i] = t.Rect(area, cellGap)
		}
	}
	if a.single && len(a.cells) > 1 {
		rects, _ = grid.SingleLayout(len(a.cells), a.active, area, cellGap, a.singleSide, a.singleShare, 16.0/9.0)
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
	drawOrder := a.layoutDrawOrder()
	emptyCells := a.emptyCellRects(area)
	ui.Box(c).Fill().Background(ui.RGB(10, 10, 12)).Children(func() {
		for i, r := range emptyCells {
			ui.Box(c).Key(fmt.Sprintf("empty-cell-%d", i)).Absolute().
				Left(float32(r.X)).Top(float32(r.Y)).
				Width(float32(r.W)).Height(float32(r.H)).
				Radius(6).Background(ui.RGB(20, 20, 24)).
				Children(func() {
					// The frozen empty cell paints a plus in its middle.
					ui.Column(c).Fill().Center().Children(func() {
						ui.Text(c, "+").FontSize(28).TextColor(ui.RGB(96, 96, 108))
					})
				})
		}
		for _, i := range drawOrder {
			a.cellView(c, i, rects[i], theme)
		}
		a.syncNativeOrder(drawOrder)
		if a.free || a.mosaic {
			for _, i := range drawOrder {
				r := rects[i]
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
				if _, _, ended := h.DragEnded(); ended {
					a.refreshMenu()
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
	} else if a.showGridLines {
		border = ui.RGBA(1, 1, 1, 0.16)
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
	if a.showGridLines && !active && base.Hovered() {
		border = theme.Accent
	}
	if over, ok := ui.DragOver[int](base); ok && over != i {
		border = theme.Accent
	}
	el := base.Border(2, border).
		Draw(func(p *ui.Painter, box ui.Rect) {
			a.ensure(i, r)
			if box.W > 4 && box.H > 4 {
				inner := ui.Rect{X: box.X + 2, Y: box.Y + 2, W: box.W - 4, H: box.H - 4}
				p.Hole(inner, [4]float32{4, 4, 4, 4})
			}
		}).
		HandleInput(func(ev ui.InputEvent) bool {
			return a.zoomInput(i, ev, r, s)
		})
	el = a.allPlaybackCellShortcut(c, el)
	el = el.Children(func() {
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
		a.seekBar(c, i, cell, s)
		ui.Row(c).Absolute().Left(6).Bottom(6).Gap(4).Children(func() {
			label := "Play"
			if s.Phase == engine.Playing {
				label = "Pause"
			}
			if ui.Button(c.Key(fmt.Sprintf("play-%d", i)), label).Width(72).Clicked() && cell.eng != nil {
				cell.eng.Toggle()
			}
			if ui.Button(c.Key(fmt.Sprintf("back-%d", i)), "-5s").Width(60).Clicked() && cell.eng != nil {
				a.seekCellTo(cell, max(s.Time-5000, 0))
			}
			if ui.Button(c.Key(fmt.Sprintf("fwd-%d", i)), "+5s").Width(60).Clicked() && cell.eng != nil {
				a.seekCellTo(cell, s.Time+5000)
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
	if _, _, ended := el.DragEnded(); (a.free || a.mosaic) && ended {
		a.refreshMenu()
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
	if a.shortcut(c, "play_pause_all") && !c.ShortcutRepeated() {
		a.toggleAllPlayback()
	}
	if a.shortcut(c, "play_pause") && !c.ShortcutRepeated() && eng != nil {
		eng.Toggle()
	}
	if a.shortcut(c, "seek_back") && eng != nil {
		a.seekActive(max(eng.Snapshot().Time-5000, 0))
	}
	if a.shortcut(c, "seek_forward") && eng != nil {
		a.seekActive(eng.Snapshot().Time + 5000)
	}
	if a.shortcut(c, "seek_percent_back") {
		a.seekPercentOffsetActive(-1)
	}
	if a.shortcut(c, "seek_percent_forward") {
		a.seekPercentOffsetActive(1)
	}
	if a.shortcut(c, "seek_percent_back_large") {
		a.seekPercentOffsetActive(-5)
	}
	if a.shortcut(c, "seek_percent_forward_large") {
		a.seekPercentOffsetActive(5)
	}
	if a.shortcut(c, "seek_percent_back_huge") {
		a.seekPercentOffsetActive(-10)
	}
	if a.shortcut(c, "seek_percent_forward_huge") {
		a.seekPercentOffsetActive(10)
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
			if cell := a.activeCell(); cell != nil {
				if start, end, active := cell.loopState(); active {
					if l, h, ok := loopSeekBounds(start, end, hi, true); ok {
						lo, hi = l, h
					}
				}
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
	if a.shortcut(c, "loop_chapter") {
		a.loopChapter()
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
		if cell := a.activeCell(); cell != nil {
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
			a.nextList(cell)
		}
	}
	if a.shortcut(c, "previous_file") {
		if cell := a.activeCell(); cell != nil {
			a.previousList(cell)
		}
	}
	if a.shortcut(c, "next_file_all") {
		a.nextAllLists()
	}
	if a.shortcut(c, "previous_file_all") {
		a.previousAllLists()
	}
	if a.shortcut(c, "crop_off") {
		if cell := a.activeCell(); cell != nil {
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

func (a *app) seekCellTo(c *cell, ms int64) {
	if c == nil || c.eng == nil {
		return
	}
	start, end, active := c.loopState()
	c.eng.SeekTo(clampSegmentSeek(start, end, c.eng.Snapshot().Length, ms, active))
}

func (a *app) seekActive(ms int64) {
	a.seekCellTo(a.activeCell(), ms)
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
	c := a.activeCell()
	if c == nil {
		return
	}
	shape := videoShape{}
	if c.eng != nil {
		s := c.eng.Snapshot()
		shape = videoShape{W: float64(s.Width), H: float64(s.Height)}
	}
	c.cropMargins = clampCropMargins(cropStep(c.cropMargins, side, delta), shape)
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
	start, end := c.loopBounds()
	start, end, ok := setLoopPoint(start, end, at, isStart)
	if !ok {
		return
	}
	c.setLoopSegment(start, end, true)
	a.dirty = true
	a.status = fmt.Sprintf("loop %d-%dms", start, end)
}

func (a *app) resetLoopPoints() {
	if c := a.activeCell(); c != nil {
		c.setLoopSegment(0, 0, false)
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
					a.fetchSponsorFirst(raw)
				} else {
					a.status = err.Error()
				}
				return
			}
			a.status = ""
			a.setPlaylist([]string{direct})
			a.fetchSponsorFirst(raw)
		})
	}()
}

func (a *app) fetchSponsorFirst(raw string) {
	if len(a.cells) > 0 {
		a.fetchSponsor(a.cells[0], raw)
	}
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

// togglePreallocate shows every configured cell of a fixed grid, empty ones
// included, as the frozen "Show All Cells" action does. Other modes ignore it.
func (a *app) togglePreallocate() {
	if !a.fixedGrid() {
		return
	}
	a.cfg.Preallocate = !a.cfg.Preallocate
	a.dirty = true
	a.saveSettings()
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

// fixedCapacity is how many videos a fixed grid holds.
func fixedCapacity(cfg grid.Config) int {
	return max(cfg.Rows, 1) * max(cfg.Cols, 1)
}

// overflowCount is how many trailing cells a fixed config cannot hold.
func (a *app) overflowCount(cfg grid.Config) int {
	if cfg.Mode != grid.Fixed {
		return 0
	}
	return a.overflowCountFor(fixedCapacity(cfg))
}

// overflowCountFor is how many cells past capacity the app holds.
func (a *app) overflowCountFor(capacity int) int {
	return max(len(a.cells)-capacity, 0)
}

// confirmOverflow asks before a fixed grid loses the videos it cannot hold.
func (a *app) confirmOverflow(cfg grid.Config) bool {
	if cfg.Mode != grid.Fixed {
		return true
	}
	return a.confirmClose(fixedCapacity(cfg))
}

// confirmClose asks, as the frozen grid does, before capacity drops below
// the open videos. The frozen dialog makes Cancel the default button for a
// destructive confirm, so this does too.
func (a *app) confirmClose(capacity int) bool {
	over := a.overflowCountFor(capacity)
	if over <= 0 {
		return true
	}
	if a.askOverflow != nil {
		return a.askOverflow(over, capacity)
	}
	noun := "videos"
	if over == 1 {
		noun = "video"
	}
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:             a.win,
		Type:               mygo.MessageWarning,
		Title:              "Set grid size",
		Message:            fmt.Sprintf("Close %d %s?", over, noun),
		Detail:             fmt.Sprintf("This grid holds %d videos.", capacity),
		Buttons:            []string{"Close Videos", "Cancel"},
		DefaultButton:      1,
		CancelButton:       1,
		DestructiveButtons: []int{0},
	})
	if err != nil {
		return false
	}
	return res.Button == 0
}

// closeOverflow drops the cells a fixed config cannot hold.
func (a *app) closeOverflow(cfg grid.Config) {
	if cfg.Mode != grid.Fixed {
		return
	}
	a.leaveTiledLayout()
	a.closeBeyond(a.liveOrder(), fixedCapacity(cfg))
}

// leaveTiledLayout puts the cells in the order they are read and clears the
// free layout, as the frozen _leave_free does on the way out of Free Tiles or
// Mosaic, and as _apply_fixed does with pack_flow(keep_ids) on a fixed entry:
// the top-left place then keeps the top-left video.
func (a *app) leaveTiledLayout() {
	n := len(a.cells)
	if n == 0 || (!a.free && !a.mosaic) {
		return
	}
	order := a.liveOrder()
	if len(order) != n {
		return
	}
	identity := true
	for i, from := range order {
		if from != i {
			identity = false
			break
		}
	}
	active := a.activeCell()
	if !identity {
		next := make([]*cell, n)
		for slot, from := range order {
			next[slot] = a.cells[from]
		}
		a.cells = next
		if active != nil {
			for i, c := range a.cells {
				if c == active {
					a.active = i
					break
				}
			}
		}
	}
	// The frozen _apply_fixed clears the free layout on the way in:
	// _free.clear() drops the tiles and the locked set, whether or not the
	// order changes. Keeping them would leave index-keyed locks and boxes
	// pointing at other videos after a permutation.
	a.freeTiles = nil
	a.mosaicTiles = nil
	a.freeOrder = nil
	a.mosaicOrder = nil
	a.mosaicLocked = nil
}

// liveOrder is the order the videos are laid out in: playlist order for the
// grid modes, and the tiles read row by row for Free Tiles and Mosaic, as
// the frozen _live_order does through read_in_rows.
func (a *app) liveOrder() []int {
	n := len(a.cells)
	if a.free && len(a.freeTiles) == n {
		return grid.ReadInRows(a.freeTiles)
	}
	if a.mosaic && len(a.mosaicTiles) == n {
		return grid.ReadInRows(a.mosaicTiles)
	}
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// closeBeyond closes every cell not among the first keep of order, through
// closeCell, the same path that closes a cell at teardown; the rest keep
// their playlist order.
func (a *app) closeBeyond(order []int, keep int) {
	if keep >= len(a.cells) {
		return
	}
	kept := make(map[int]bool, keep)
	for i := 0; i < keep && i < len(order); i++ {
		kept[order[i]] = true
	}
	for i := len(a.cells) - 1; i >= 0; i-- {
		if kept[i] {
			continue
		}
		// Closing a cell before the active one shifts it down; leaving the
		// index alone would switch the active video.
		if i < a.active {
			a.active--
		}
		a.closeCell(a.cells[i])
		a.cells = append(a.cells[:i], a.cells[i+1:]...)
	}
	if a.active >= len(a.cells) {
		a.active = len(a.cells) - 1
	}
	if a.active < 0 {
		a.active = 0
	}
	a.dirty = true
	a.resetFreeTiles()
	a.refreshKeepAwake()
}

func (a *app) setGrid(cfg grid.Config) {
	// The preallocate flag belongs to the layout, not to one mode: a preset
	// or template switch carries it, as the frozen grid keeps the setting.
	cfg.Preallocate = a.cfg.Preallocate
	if cfg.Mode == grid.Fixed {
		// A fixed grid that cannot hold every video asks before closing the
		// extras, as the frozen _apply_fixed does; only after that does the
		// free layout go.
		if !a.confirmOverflow(cfg) {
			return
		}
		a.closeOverflow(cfg)
	} else {
		// Leaving Free Tiles or Mosaic for a plain grid re-reads the tiles,
		// as the frozen _leave_free does.
		a.leaveTiledLayout()
	}
	a.cfg = cfg
	a.template = ""
	a.free, a.mosaic, a.single = false, false, false
	a.dirty = true
	a.saveSettings()
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) setTemplate(name string) {
	template, ok := grid.FindTemplate(name)
	if !ok {
		return
	}
	// A template has one place per cell it can show; the frozen action asks
	// before it closes the videos that do not fit.
	capacity := len(template.Places())
	if !a.confirmClose(capacity) {
		return
	}
	a.leaveTiledLayout()
	a.closeBeyond(a.liveOrder(), capacity)
	a.cfg = grid.Config{Mode: grid.Fixed, Rows: template.Rows, Cols: template.Cols, Preallocate: a.cfg.Preallocate}
	a.template = name
	a.free, a.mosaic, a.single = false, false, false
	a.dirty = true
	a.saveSettings()
	a.refreshMenu()
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

// pictureShape is the shape of the picture as shown: the decoded frame turned
// by the cell transform, cropped by margins, then overridden by crop_ratio or
// aspect_ratio (crop first, as upstream's picture_shape does). Zero means the
// shape is not known yet and Fit Shapes takes 16:9.
func (a *app) pictureShape(c *cell, s engine.Snapshot) float64 {
	if c == nil || s.Width <= 0 || s.Height <= 0 {
		return 0
	}
	shape, crop := a.cellShape(c, s)
	if ratio := ratioValue(c.crop); ratio > 0 {
		return ratio
	}
	if ratio := ratioValue(c.aspect); ratio > 0 {
		return ratio
	}
	w := shape.W - crop.Left - crop.Right
	h := shape.H - crop.Top - crop.Bottom
	if w <= 0 || h <= 0 {
		return 0
	}
	return w / h
}

// ratioValue parses "16:9" into 16/9. An empty or malformed value is 0.
func ratioValue(ratio string) float64 {
	if i := indexByteColon(ratio); i > 0 {
		w, err1 := strconv.ParseFloat(ratio[:i], 64)
		h, err2 := strconv.ParseFloat(ratio[i+1:], 64)
		if err1 == nil && err2 == nil && w > 0 && h > 0 {
			return w / h
		}
	}
	return 0
}

func modeName(m grid.Mode) string {
	switch m {
	case grid.AutoCols:
		return "auto_cols"
	case grid.Fixed:
		return "fixed"
	case grid.Shapes:
		return "fit_shapes"
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
	case "fit_shapes":
		return grid.Shapes
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
		c.id = v.ID
		c.title = v.Title
		c.extra = cloneRawMessages(v.Extra)
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
		if v.AspectRatio != nil {
			c.aspect = normalizeAspectRatio(*v.AspectRatio)
		} else {
			c.aspect = a.aspect
		}
		if v.Scale != nil {
			c.scale = *v.Scale
			if c.scale < 0 {
				c.scale = 0
			}
		} else {
			c.scale = a.scale
		}
		if v.CropRatio != nil {
			c.crop = normalizeCropRatio(*v.CropRatio)
		} else {
			c.crop = a.crop
		}
		c.startPaused = strings.EqualFold(v.PlaybackState, "paused") ||
			strings.EqualFold(v.PlaybackState, "stopped")
		oldPath := c.path
		oldTransform := normalizeTransform(c.transform)
		if len(c.list) > 0 {
			if v.ListIndex >= 0 && v.ListIndex < len(c.list) {
				c.listIndex = v.ListIndex
				c.path = c.list[c.listIndex]
			}
		}
		c.listEndRaw = ""
		if action := normalizeListEnd(v.ListEnd); action != "" {
			c.listEnd = action
		} else if v.ListEnd != "" {
			// Preserve a future/unknown enum rather than replacing it with
			// this build's default on the next save.
			c.listEnd = ""
			c.listEndRaw = v.ListEnd
		} else if len(c.list) > 0 {
			c.listEnd = listEndLoop
		} else {
			c.listEnd = ""
		}
		c.listFailRaw = ""
		switch v.ListFail {
		case listFailPlayNext, listFailShowError:
			c.listFail = v.ListFail
		case "":
			c.listFail = ""
		default:
			c.listFail = ""
			c.listFailRaw = v.ListFail
		}
		if v.ListFailWait != nil {
			wait := *v.ListFailWait
			c.listFailWait = &wait
		}
		if action := normalizeEndAction(v.EndAction); action != "" {
			c.endAction = action
			c.loop = action == endLoop
		}
		c.audioDelay = engine.ClampAudioDelay(v.AudioDelay)
		c.subDelay = engine.ClampSubtitleDelay(v.SubtitleDelay)
		c.setLoopSegment(v.LoopStart, v.LoopEnd, loopActiveFromVideo(v))
		if v.Anchor != "" {
			c.anchorX, c.anchorY = parseAnchor(v.Anchor)
		} else {
			c.anchorX, c.anchorY = parseAnchor(a.defaultAnchor)
		}
		if len(v.Crop) == 4 {
			c.cropMargins = cropMarginsFromList(v.Crop)
		} else {
			c.cropMargins = cropMarginsFromList(v.CropMargins)
		}
		c.transform = normalizeTransform(v.Transform)
		c.externalAudio = cloneStrings(v.ExternalAudio)
		c.externalSubtitles = cloneStrings(v.ExternalSubtitles)
		c.setSponsorMode(v.SponsorMode)
		a.setSponsorOverrides(c, normalizeSponsorOverrides(v.SponsorModes))
		c.sponsorSource = v.SponsorURL
		a.refetchSponsor(c)
		if c.eng != nil {
			// The transform is an instance option, so a changed transform
			// needs a new libVLC instance even when the path is unchanged.
			reload := c.path != oldPath || c.transform != oldTransform
			if c.eng.SetExternalFilesForLoad(c.externalAudio, c.externalSubtitles) {
				reload = true
			}
			if reload {
				c.eng.SetTransform(c.transform)
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
	if mode, rows, cols, preallocate := d.GridState(); mode != "" {
		// Clamp sizes from a document so a hand-edited grid cannot ask the
		// view to build tens of thousands of cells each frame.
		a.cfg = grid.Config{Mode: parseMode(mode), Rows: clampInt(rows, 1, maxFixedSize), Cols: clampInt(cols, 1, maxFixedSize), Preallocate: preallocate}
	}
	a.template = ""
	var templateName string
	if getRawSetting(d, "grid_template", &templateName) {
		if _, ok := grid.FindTemplate(templateName); ok && a.cfg.Mode == grid.Fixed {
			a.template = templateName
		}
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
			a.resetFreeTiles()
		}
		var order []int
		if getRawSetting(d, "grid_free_order", &order) {
			a.freeOrder = grid.NormalizeOrder(order, len(a.cells))
		} else {
			a.freeOrder = grid.DefaultOrder(len(a.cells))
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
		var order []int
		if !getRawSetting(d, "grid_mosaic_order", &order) {
			order = grid.DefaultOrder(len(a.cells))
		}
		a.mosaicOrder = grid.Restack(order, len(a.cells), a.mosaicLocked)
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
		aspectRatio := c.aspect
		cropRatio := c.crop
		// Upstream's model requires scale >= 1.0, so 0 is only written when
		// it carries information (an explicit Fit against a non-zero
		// default). A plain 0 default is left absent.
		var scale *float64
		if c.scale != 0 || a.scale != 0 {
			s := c.scale
			scale = &s
		}
		listEnd := c.listEnd
		if listEnd == "" {
			listEnd = c.listEndRaw
		}
		listFail := c.listFail
		if listFail == "" {
			listFail = c.listFailRaw
		}
		loopStart, loopEnd, loopActive := c.loopState()
		var loopActivePtr *bool
		if loopActive && loopStart == 0 && loopEnd == 0 {
			active := true
			loopActivePtr = &active
		}
		v := playlist.Video{
			ID:                c.id,
			Title:             c.title,
			Extra:             cloneRawMessages(c.extra),
			URI:               source,
			CurrentPosition:   int(s.Time),
			IsMuted:           s.Muted,
			Volume:            float64(s.Volume) / 100,
			Rate:              s.Rate,
			PlaybackState:     s.Phase.String(),
			CellViewMode:      a.cellViewMode(c),
			AspectRatio:       &aspectRatio,
			Scale:             scale,
			CropRatio:         &cropRatio,
			ListIndex:         c.listIndex,
			ListEnd:           listEnd,
			EndAction:         upstreamEndAction(c.endAction),
			ListFail:          listFail,
			ListFailWait:      cloneWait(c.listFailWait),
			ExternalAudio:     cloneStrings(c.externalAudio),
			ExternalSubtitles: cloneStrings(c.externalSubtitles),
			AudioDelay:        c.audioDelay,
			SubtitleDelay:     c.subDelay,
			LoopStart:         loopStart,
			LoopEnd:           loopEnd,
			LoopActive:        loopActivePtr,
			Transform:         upstreamTransform(c.transform),
			SponsorMode:       c.sponsorMode(),
			SponsorModes:      a.sponsorOverridesCopy(c),
			SponsorURL:        c.sponsorSource,
			Anchor:            anchorName(c.anchorX, c.anchorY),
			Crop:              cropMarginsList(c.cropMargins),
			CropMargins:       cropMarginsList(c.cropMargins),
		}
		d.Videos[i] = v
	}
	d.SetGridState(modeName(a.cfg.Mode), a.cfg.Rows, a.cfg.Cols, a.cfg.Preallocate && a.cfg.Mode == grid.Fixed)
	if a.template != "" {
		setRawSetting(d, "grid_template", a.template)
	} else {
		delete(d.Settings, "grid_template")
	}
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
		a.freeOrder = grid.NormalizeOrder(a.freeOrder, len(a.cells))
		setRawSetting(d, "grid_free_tiles", a.freeTiles)
		setRawSetting(d, "grid_free_order", a.freeOrder)
	} else {
		delete(d.Settings, "grid_free_tiles")
		delete(d.Settings, "grid_free_order")
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
		setRawSetting(d, "grid_mosaic_order", a.mosaicOrder)
	} else {
		delete(d.Settings, "grid_mosaic_tiles")
		delete(d.Settings, "grid_mosaic_locked")
		delete(d.Settings, "grid_mosaic_order")
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

// applyMinimized follows the frozen window_state away handling: entering
// the minimized state pauses every playing cell when the setting is on, and
// leaving it resumes only those cells the minimize paused.
func (a *app) applyMinimized(minimized bool) {
	if a.closed.Load() || minimized == a.minimized {
		return
	}
	a.minimized = minimized
	if minimized {
		a.minimizePaused = nil
		if !a.pauseMinimized {
			return
		}
		a.minimizePaused = map[*cell]bool{}
		for _, c := range a.cells {
			if c == nil || c.eng == nil || c.eng.Snapshot().Phase != engine.Playing {
				continue
			}
			c.eng.Pause()
			a.minimizePaused[c] = true
		}
		return
	}
	for c := range a.minimizePaused {
		if c.eng != nil {
			c.eng.Play()
		}
	}
	a.minimizePaused = nil
}

// playingCells counts cells in the Playing phase, the frozen source's
// playing_videos_count for the keep-awake decision.
func (a *app) playingCells() int {
	n := 0
	for _, c := range a.cells {
		if c != nil && c.eng != nil && c.eng.Snapshot().Phase == engine.Playing {
			n++
		}
	}
	return n
}

// keepAwakeMode mirrors the frozen ScreensaverManager._mode: nothing while
// no video plays, the system alone while the window is minimized (sound
// without a picture), and the display while a video is on screen.
func (a *app) keepAwakeMode() keepAwakeMode {
	if !a.inhibitScreensaver || a.playingCells() == 0 {
		return keepAwakeNone
	}
	if a.minimized {
		return keepAwakeSystem
	}
	return keepAwakeDisplay
}

func (a *app) refreshKeepAwake() {
	// The window is the source of truth, so a launch that starts minimized
	// does not wait for an event that already happened. Tests without a
	// window set the field directly.
	if a.win != nil {
		a.applyMinimized(a.win.IsMinimized())
	}
	a.screensaver.SetMode(a.keepAwakeMode())
}

// pollKeepAwake re-reads the live window state once a second. Minimize and
// restore events are enough on their own; the poll also covers a launch that
// starts minimized and a window state change the event path does not report.
func (a *app) pollKeepAwake() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if a.closed.Load() {
			return
		}
		mygo.RunOnMain(func() {
			if a.closed.Load() {
				return
			}
			a.refreshKeepAwake()
		})
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
	c := a.activeCell()
	if c == nil {
		return
	}
	c.aspect = normalizeAspectRatio(aspect)
	if c.eng != nil {
		if _, stretch := viewModeFlags(a.cellViewMode(c)); !stretch {
			c.eng.SetAspectRatio(c.aspect)
		}
	}
	a.dirty = true
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
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
	c := a.activeCell()
	if c == nil {
		return
	}
	c.anchorX, c.anchorY = parseAnchor(name)
	a.dirty = true
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
	c := a.activeCell()
	if c == nil {
		return
	}
	if scale < 0 {
		scale = 0
	}
	c.scale = scale
	if c.eng != nil {
		c.eng.SetScale(scale)
	}
	a.dirty = true
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) setCrop(crop string) {
	c := a.activeCell()
	if c == nil {
		return
	}
	c.crop = normalizeCropRatio(crop)
	a.dirty = true
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) setTransform(transform string) {
	a.defaultTransform = normalizeTransform(transform)
	for _, c := range a.cells {
		c.transform = a.defaultTransform
		if c.eng != nil {
			c.eng.SetTransform(c.transform)
			_ = c.eng.Reload()
		}
	}
	a.dirty = true
	a.saveSettings()
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) setViewMode(fill bool, stretch bool) {
	c := a.activeCell()
	if c == nil {
		return
	}
	c.viewMode = viewModeName(fill, stretch)
	a.dirty = true
	if a.win != nil {
		a.win.Invalidate()
	}
}

// normalizeAspectRatio maps upstream's explicit own-shape sentinel to this
// build's empty string and trims the value.
func normalizeAspectRatio(aspect string) string {
	aspect = strings.TrimSpace(aspect)
	if aspect == "own" {
		return ""
	}
	return aspect
}

// normalizeCropRatio maps upstream's explicit no-crop sentinel to this
// build's empty string.
func normalizeCropRatio(crop string) string {
	crop = strings.TrimSpace(crop)
	if crop == "none" {
		return ""
	}
	return crop
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
	c := a.activeCell()
	eng := a.activeEngine()
	if c == nil || eng == nil {
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
	changed := false
	for _, p := range paths {
		var attached, added bool
		if kind == "subtitle" {
			attached = eng.AddSubtitle(p)
			if attached {
				c.externalSubtitles, added = rememberExternalFile(c.externalSubtitles, p)
			}
		} else {
			attached = eng.AddAudio(p)
			if attached {
				c.externalAudio, added = rememberExternalFile(c.externalAudio, p)
			}
		}
		if !attached {
			a.status = "Could not attach " + p
		}
		changed = changed || added
	}
	if changed {
		a.dirty = true
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
	a.chapterSeek(step)
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
	a.seekActive(pos)
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
	aspect, scale, crop := a.aspect, a.scale, a.crop
	listEnd := "loop"
	listFail := listFailPlayNext
	endAction := endLoop
	hasList := false
	if c := a.activeCell(); c != nil {
		aspect, scale, crop = c.aspect, c.scale, c.crop
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
	templateItems := make([]*mygo.MenuItem, 0, len(grid.Templates))
	for _, template := range grid.Templates {
		name := template.Name
		templateItems = append(templateItems, &mygo.MenuItem{
			Label:   name,
			Checked: a.template == name,
			Type:    mygo.MenuItemRadio,
			Click:   func(*mygo.MenuItem, *mygo.Window) { a.setTemplate(name) },
		})
	}
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
			{Label: a.tr("Actions", "Play/Pause All"), Accelerator: a.keys.Get("play_pause_all").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				a.toggleAllPlayback()
			}},
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
					a.nextList(a.cells[a.active])
				}
			}},
			{Label: a.tr("Actions", "Previous in List (All)"), Accelerator: a.keys.Get("previous_file_all").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				a.previousAllLists()
			}},
			{Label: a.tr("Actions", "Next in List (All)"), Accelerator: a.keys.Get("next_file_all").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				a.nextAllLists()
			}},
			{Label: a.tr("Actions", "End of List"), Submenu: []*mygo.MenuItem{
				{Label: a.tr("Actions", "Start Over"), Type: mygo.MenuItemRadio, Checked: listEnd == listEndLoop, Disabled: !hasList,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setListEnd(listEndLoop) }},
				{Label: a.tr("Actions", "Pause At Start"), Type: mygo.MenuItemRadio, Checked: listEnd == listEndPause, Disabled: !hasList,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setListEnd(listEndPause) }},
				{Label: a.tr("Actions", "Stop"), Type: mygo.MenuItemRadio, Checked: listEnd == listEndStop, Disabled: !hasList,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setListEnd(listEndStop) }},
				{Label: a.tr("Actions", "Close"), Type: mygo.MenuItemRadio, Checked: listEnd == listEndClose, Disabled: !hasList,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setListEnd(listEndClose) }},
				{Label: a.tr("Actions", "Next Cell"), Type: mygo.MenuItemRadio, Checked: listEnd == listEndNext, Disabled: !hasList,
					Click: func(*mygo.MenuItem, *mygo.Window) { a.setListEnd(listEndNext) }},
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
					a.seekActive(max(eng.Snapshot().Time-5000, 0))
				}
			}},
			{Label: a.tr("Actions", "Seek +5s"), Accelerator: a.keys.Get("seek_forward").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) {
				if eng := a.activeEngine(); eng != nil {
					a.seekActive(eng.Snapshot().Time + 5000)
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
			{Label: a.tr("Actions", "Set Loop Start"), Accelerator: a.keys.Get("loop_start").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) { a.setLoopPoint(true) }},
			{Label: a.tr("Actions", "Set Loop End"), Accelerator: a.keys.Get("loop_end").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) { a.setLoopPoint(false) }},
			{Label: a.tr("Actions", "Loop Chapter"), Accelerator: a.keys.Get("loop_chapter").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) { a.loopChapter() }},
			{Label: a.tr("Actions", "Loop Reset"), Accelerator: a.keys.Get("loop_reset").Accelerator(), Click: func(*mygo.MenuItem, *mygo.Window) { a.resetLoopPoints() }},
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
			{Label: "Fit Shapes", Click: func(*mygo.MenuItem, *mygo.Window) { a.setGrid(grid.Config{Mode: grid.Shapes}) }},
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
			{Label: "Show All Cells", Type: mygo.MenuItemCheckbox, Checked: a.fixedGrid() && a.cfg.Preallocate, Disabled: !a.fixedGrid(), Click: func(*mygo.MenuItem, *mygo.Window) {
				a.togglePreallocate()
			}},
			{Label: "Templates", Submenu: templateItems},
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
			{Label: "Bring to Front", Disabled: !a.canStackActive(true), Click: func(*mygo.MenuItem, *mygo.Window) {
				a.stackActive(true)
			}},
			{Label: "Send to Back", Disabled: !a.canStackActive(false), Click: func(*mygo.MenuItem, *mygo.Window) {
				a.stackActive(false)
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
				{Label: "Auto", Type: mygo.MenuItemCheckbox, Checked: aspect == "", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAspect("") }},
				{Label: "16:9", Type: mygo.MenuItemCheckbox, Checked: aspect == "16:9", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAspect("16:9") }},
				{Label: "4:3", Type: mygo.MenuItemCheckbox, Checked: aspect == "4:3", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAspect("4:3") }},
				{Label: "1:1", Type: mygo.MenuItemCheckbox, Checked: aspect == "1:1", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAspect("1:1") }},
				{Label: "2.35:1", Type: mygo.MenuItemCheckbox, Checked: aspect == "2.35:1", Click: func(*mygo.MenuItem, *mygo.Window) { a.setAspect("2.35:1") }},
			}},
			{Label: a.tr("Actions", "Scale"), Submenu: []*mygo.MenuItem{
				{Label: "Fit", Type: mygo.MenuItemCheckbox, Checked: scale == 0, Click: func(*mygo.MenuItem, *mygo.Window) { a.setScale(0) }},
				{Label: "1x", Type: mygo.MenuItemCheckbox, Checked: scale == 1, Click: func(*mygo.MenuItem, *mygo.Window) { a.setScale(1) }},
				{Label: "1.5x", Type: mygo.MenuItemCheckbox, Checked: scale == 1.5, Click: func(*mygo.MenuItem, *mygo.Window) { a.setScale(1.5) }},
				{Label: "2x", Type: mygo.MenuItemCheckbox, Checked: scale == 2, Click: func(*mygo.MenuItem, *mygo.Window) { a.setScale(2) }},
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
				{Label: "None", Type: mygo.MenuItemCheckbox, Checked: crop == "", Click: func(*mygo.MenuItem, *mygo.Window) { a.setCrop("") }},
				{Label: "16:9", Type: mygo.MenuItemCheckbox, Checked: crop == "16:9", Click: func(*mygo.MenuItem, *mygo.Window) { a.setCrop("16:9") }},
				{Label: "4:3", Type: mygo.MenuItemCheckbox, Checked: crop == "4:3", Click: func(*mygo.MenuItem, *mygo.Window) { a.setCrop("4:3") }},
				{Label: "2.35:1", Type: mygo.MenuItemCheckbox, Checked: crop == "2.35:1", Click: func(*mygo.MenuItem, *mygo.Window) { a.setCrop("2.35:1") }},
			}},
			{Label: "Transform", Submenu: []*mygo.MenuItem{
				{Label: "None", Click: func(*mygo.MenuItem, *mygo.Window) { a.setTransform("none") }},
				{Label: "Rotate 90", Click: func(*mygo.MenuItem, *mygo.Window) { a.setTransform("90") }},
				{Label: "Rotate 180", Click: func(*mygo.MenuItem, *mygo.Window) { a.setTransform("180") }},
				{Label: "Rotate 270", Click: func(*mygo.MenuItem, *mygo.Window) { a.setTransform("270") }},
				{Label: "Flip horizontal", Click: func(*mygo.MenuItem, *mygo.Window) { a.setTransform("hflip") }},
				{Label: "Flip vertical", Click: func(*mygo.MenuItem, *mygo.Window) { a.setTransform("vflip") }},
				{Label: "Transpose", Click: func(*mygo.MenuItem, *mygo.Window) { a.setTransform("transpose") }},
				{Label: "Anti-transpose", Click: func(*mygo.MenuItem, *mygo.Window) { a.setTransform("antitranspose") }},
			}},
		}},
		{Label: a.tr("Actions", "Bookmarks"), Submenu: append([]*mygo.MenuItem{
			{Label: "Add Bookmark", Click: func(*mygo.MenuItem, *mygo.Window) { a.addBookmark() }},
			{Label: "Manage Bookmarks…", Click: func(*mygo.MenuItem, *mygo.Window) { a.openBookmarkManager() }},
			{Label: "Clear This File's Bookmarks", Click: func(*mygo.MenuItem, *mygo.Window) { a.clearBookmarks() }},
			{Type: mygo.MenuItemSeparator},
		}, bookmarkItems...)},
		{Label: a.tr("Actions", "Snapshots"), Submenu: snapshotMenu},
		{Label: a.tr("Actions", "SponsorBlock"), Submenu: a.sponsorMenuItems()},
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
	saveOnQuit := flag.Bool("save-on-quit", false, "save --save-playlist immediately before autoquit instead of after 3s")
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
			AlwaysOnTop:    a.stayOnTop,
			Maximized:      a.startMaximized && !a.startFullscreen,
			FullScreen:     a.startFullscreen,
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
		w.OnMinimize(func() { a.refreshKeepAwake() })
		w.OnRestore(func() { a.refreshKeepAwake() })
		go a.pollKeepAwake()
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
		if *savePath != "" && !*saveOnQuit {
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
					if *saveOnQuit && *savePath != "" {
						if err := a.savePlaylist(*savePath); err != nil {
							log.Printf("save playlist on quit: %v", err)
						}
					}
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
