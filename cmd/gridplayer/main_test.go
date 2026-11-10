package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gridplayer-go/internal/bookmarks"
	"gridplayer-go/internal/engine"
	"gridplayer-go/internal/grid"
	"gridplayer-go/internal/keymap"
	"gridplayer-go/internal/playlist"
	"gridplayer-go/internal/resolve"
	"gridplayer-go/internal/settings"
	"gridplayer-go/internal/snapshots"
)

func TestSwapCells(t *testing.T) {
	a := &app{cells: []*cell{{label: "a"}, {label: "b"}, {label: "c"}}, active: 0}
	a.swapCells(0, 2)
	if a.cells[0].label != "c" || a.cells[2].label != "a" {
		t.Fatalf("cells %q %q %q", a.cells[0].label, a.cells[1].label, a.cells[2].label)
	}
	if a.active != 2 || !a.dirty {
		t.Fatalf("active=%d dirty=%v", a.active, a.dirty)
	}
	a.swapCells(-1, 0)
	a.swapCells(0, 9)
	if a.cells[0].label != "c" {
		t.Fatal("invalid swap changed cells")
	}
}

func TestInsertCells(t *testing.T) {
	a := &app{defaultVolume: 80}
	a.setPlaylist([]string{"a.mp4", "b.mp4"})
	keepA, keepB := a.cells[0], a.cells[1]
	a.insertCells(1, []string{"x.mp4", "y.mp4"})
	if len(a.cells) != 4 {
		t.Fatalf("len=%d", len(a.cells))
	}
	if a.cells[0] != keepA || a.cells[3] != keepB {
		t.Fatal("insert did not preserve existing cells")
	}
	if a.cells[1].path != "x.mp4" || a.cells[2].path != "y.mp4" || a.active != 1 {
		t.Fatalf("cells=%v active=%d", a.paths(), a.active)
	}
	if !a.dirty {
		t.Fatal("dirty not set")
	}
	a.insertCells(99, []string{"z.mp4"})
	if a.cells[len(a.cells)-1].path != "z.mp4" {
		t.Fatal("append failed")
	}
}

func TestListAdvanceAndWrap(t *testing.T) {
	c := &cell{list: []string{"a.mp4", "b.mp4"}, listIndex: 0, listEnd: "loop", path: "a.mp4"}
	a := &app{cells: []*cell{c}}
	a.advanceList(c)
	if c.listIndex != 1 || c.path != "b.mp4" {
		t.Fatalf("advance %d %q", c.listIndex, c.path)
	}
	a.advanceList(c)
	if c.listIndex != 0 || c.path != "a.mp4" {
		t.Fatalf("wrap %d %q", c.listIndex, c.path)
	}
	a.previousList(c)
	if c.listIndex != 1 || c.path != "b.mp4" {
		t.Fatalf("previous %d %q", c.listIndex, c.path)
	}
}

func TestListEndStopAndNext(t *testing.T) {
	stop := &cell{list: []string{"a.mp4", "b.mp4"}, listIndex: 1, listEnd: "stop", path: "b.mp4"}
	a := &app{cells: []*cell{stop}}
	a.advanceList(stop)
	if stop.listIndex != 1 || stop.path != "b.mp4" {
		t.Fatalf("stop advanced: %d %q", stop.listIndex, stop.path)
	}

	next := &cell{list: []string{"a.mp4", "b.mp4"}, listIndex: 1, listEnd: "next", path: "b.mp4"}
	other := &cell{path: "other.mp4"}
	a = &app{cells: []*cell{next, other}, active: 0}
	a.advanceList(next)
	if a.active != 1 {
		t.Fatalf("next cell active=%d", a.active)
	}
	last := &cell{list: []string{"a.mp4"}, listIndex: 0, listEnd: "next", path: "a.mp4"}
	a = &app{cells: []*cell{other, last}, active: 1}
	a.advanceList(last)
	if a.active != 1 {
		t.Fatalf("last cell wrapped active=%d", a.active)
	}
}

func TestShouldAdvanceOncePerGeneration(t *testing.T) {
	c := &cell{}
	if !c.shouldAdvance(1) {
		t.Fatal("first end should advance")
	}
	if c.shouldAdvance(1) {
		t.Fatal("Stopped after EndReached must not advance again")
	}
	if !c.shouldAdvance(2) {
		t.Fatal("new generation should advance")
	}
}

func TestNewCellReportsEmptyList(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.m3u")
	if err := os.WriteFile(p, []byte("#EXTM3U\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &app{defaultVolume: 80}
	c := a.newCell(0, p)
	if c.listErr == "" {
		t.Fatal("empty m3u should record an error")
	}
}

func TestMosaicArrangeKeepsLock(t *testing.T) {
	a := &app{
		cells:  []*cell{{label: "a"}, {label: "b"}, {label: "c"}, {label: "d"}},
		mosaic: true,
	}
	a.ensureMosaic()
	if len(a.mosaicTiles) != 4 {
		t.Fatalf("tiles=%d", len(a.mosaicTiles))
	}
	a.mosaicLocked[0] = true
	locked := a.mosaicTiles[0].Box
	a.arrangeMosaic()
	if a.mosaicTiles[0].Box != locked {
		t.Fatalf("locked tile moved: %+v != %+v", a.mosaicTiles[0].Box, locked)
	}
	if len(a.mosaicTiles) != 4 || !a.dirty {
		t.Fatalf("tiles=%d dirty=%v", len(a.mosaicTiles), a.dirty)
	}
}

func TestMosaicLockToggleAndReset(t *testing.T) {
	a := &app{cells: []*cell{{label: "a"}, {label: "b"}}, mosaic: true, active: 1}
	a.ensureMosaic()
	a.toggleMosaicLock()
	if !a.mosaicLocked[1] {
		t.Fatal("lock not set")
	}
	a.toggleMosaicLock()
	if a.mosaicLocked[1] {
		t.Fatal("lock not cleared")
	}
	a.mosaicLocked[0] = true
	a.resetMosaic()
	if len(a.mosaicLocked) != 0 || len(a.mosaicTiles) != 2 {
		t.Fatalf("reset: locked=%v tiles=%d", a.mosaicLocked, len(a.mosaicTiles))
	}
}

func TestMoveTileSnapAndOverlap(t *testing.T) {
	a := &app{snap: true, overlap: true, freeArea: [2]float64{1000, 1000}}
	tiles := []grid.Tile{
		{Box: [4]float64{0.295, 0.1, 0.495, 0.3}},
		{Box: [4]float64{0.5, 0, 0.75, 0.5}},
	}
	got := a.moveTile(tiles, 0, 0, 0, false)
	if got[0].Box[2] != 0.5 {
		t.Fatalf("snap not applied: %+v", got[0].Box)
	}

	a.overlap = false
	tiles = []grid.Tile{
		{Box: [4]float64{0, 0, 0.5, 0.5}},
		{Box: [4]float64{0.5, 0, 1, 0.5}},
	}
	got = a.moveTile(tiles, 0, 10, 0, false)
	if got[0].Box != tiles[0].Box {
		t.Fatalf("overlap accepted: %+v", got[0].Box)
	}
	got = a.moveTile(tiles, 0, 10, 0, true)
	if got[0].Box == tiles[0].Box {
		t.Fatal("mosaic overlap should be allowed")
	}
}

func TestResizeTileSnapAndOverlap(t *testing.T) {
	a := &app{snap: true, overlap: false, freeArea: [2]float64{1000, 1000}}
	tiles := []grid.Tile{
		{Box: [4]float64{0.1, 0.1, 0.495, 0.3}},
		{Box: [4]float64{0.5, 0, 0.75, 0.5}},
	}
	got := a.resizeTile(tiles, 0, 0, 0, false)
	if got[0].Box[2] != 0.5 {
		t.Fatalf("resize snap not applied: %+v", got[0].Box)
	}
	a.snap = false
	got = a.resizeTile(tiles, 0, 200, 0, false)
	if got[0].Box != tiles[0].Box {
		t.Fatalf("overlap resize accepted: %+v", got[0].Box)
	}
}

func TestSettingsDraftRoundTrip(t *testing.T) {
	a := &app{
		cfg:           grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 4},
		fill:          true,
		snap:          true,
		overlap:       true,
		defaultVolume: 80,
		defaultRate:   1,
		defaultLoop:   true,
		cells:         []*cell{{}, {}},
	}
	d := a.currentSettingsDraft()
	if d.layout != "Fixed" || d.rows != "2" || d.cols != "4" {
		t.Fatalf("draft layout=%q rows=%q cols=%q", d.layout, d.rows, d.cols)
	}
	d.layout = "Mosaic"
	d.volume = 55
	d.rate = 1.5
	d.loop = false
	d.pauseBackground = false
	d.snap = false
	d.overlap = false
	d.view = "Fit (contain)"
	d.aspect = "4:3"
	d.scale = "1.5x"
	d.crop = "16:9"
	d.singleSide = "Left"
	d.proxy = "http://proxy:8080"
	d.cookies = "/tmp/cookies.txt"
	d.ytdlp = "/usr/local/bin/yt-dlp"
	d.streamlink = "/usr/local/bin/streamlink"
	d.format = "720p"
	a.applySettingsDraft(d)
	if !a.mosaic || a.free || a.single {
		t.Fatalf("layout free=%v mosaic=%v single=%v", a.free, a.mosaic, a.single)
	}
	if a.defaultVolume != 55 || a.defaultRate != 1.5 || a.defaultLoop {
		t.Fatalf("defaults volume=%d rate=%v loop=%v", a.defaultVolume, a.defaultRate, a.defaultLoop)
	}
	if a.pauseBackground {
		t.Fatal("pause background not applied")
	}
	if a.fill || a.stretch {
		t.Fatalf("view fill=%v stretch=%v", a.fill, a.stretch)
	}
	if a.aspect != "4:3" || a.scale != 1.5 || a.crop != "16:9" {
		t.Fatalf("video aspect=%q scale=%v crop=%q", a.aspect, a.scale, a.crop)
	}
	if a.singleSide != grid.StripLeft {
		t.Fatalf("side=%v", a.singleSide)
	}
	if a.resolver == nil || a.resolver.Proxy != "http://proxy:8080" ||
		a.resolver.Cookies != "/tmp/cookies.txt" ||
		a.resolver.Path != "/usr/local/bin/yt-dlp" ||
		a.resolver.Streamlink != "/usr/local/bin/streamlink" ||
		a.resolver.Format != "best[height<=720]" {
		t.Fatalf("resolver: %+v", a.resolver)
	}
	if len(a.mosaicTiles) != 2 {
		t.Fatalf("mosaic tiles=%d", len(a.mosaicTiles))
	}
}

func TestSettingsDraftLayouts(t *testing.T) {
	cases := []struct {
		name string
		a    *app
		want string
	}{
		{"auto rows", &app{cfg: grid.Config{Mode: grid.AutoRows}}, "Auto Rows"},
		{"auto cols", &app{cfg: grid.Config{Mode: grid.AutoCols}}, "Auto Columns"},
		{"free", &app{free: true}, "Free Tiles"},
		{"mosaic", &app{mosaic: true}, "Mosaic"},
		{"single", &app{single: true}, "Single Mode"},
	}
	for _, tc := range cases {
		if got := tc.a.currentSettingsDraft().layout; got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestBookmarkKeyUsesCurrentItem(t *testing.T) {
	a := &app{active: 0, cells: []*cell{{source: "list.m3u", path: "item2.mp4"}}}
	if got := a.bookmarkKey(); got != "item2.mp4" {
		t.Fatalf("bookmark key %q", got)
	}
	a.cells[0].path = ""
	if got := a.bookmarkKey(); got != "list.m3u" {
		t.Fatalf("fallback key %q", got)
	}
	a.active = 2
	if got := a.bookmarkKey(); got != "" {
		t.Fatalf("inactive key %q", got)
	}
}

func TestBookmarkNameSkipsExisting(t *testing.T) {
	s, err := bookmarks.Load(filepath.Join(t.TempDir(), "bookmarks.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.Add("a.mp4", "Bookmark 1", 1000)
	s.Add("a.mp4", "Bookmark 2", 2000)
	a := &app{bookmarks: s}
	if got := a.bookmarkName("a.mp4"); got != "Bookmark 3" {
		t.Fatalf("name %q", got)
	}
	s.Add("a.mp4", "Bookmark 3", 3000)
	if got := a.bookmarkName("a.mp4"); got != "Bookmark 4" {
		t.Fatalf("collision name %q", got)
	}
}

func TestLoadNetworkSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.ini")
	body := "[network]\nproxy=http://proxy:8080\ncookies=/tmp/cookies.txt\nformat=720p\nytdlp_path=\nstreamlink_path=/tmp/streamlink\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{resolver: resolve.New()}
	a.loadNetworkSettings(st)
	if a.resolver.Proxy != "http://proxy:8080" ||
		a.resolver.Cookies != "/tmp/cookies.txt" ||
		a.resolver.Format != "720p" {
		t.Fatalf("resolver: %+v", a.resolver)
	}
	if a.resolver.Path != "" {
		t.Fatalf("yt-dlp path should be cleared: %q", a.resolver.Path)
	}
	if a.resolver.Streamlink != "/tmp/streamlink" {
		t.Fatalf("streamlink path %q", a.resolver.Streamlink)
	}
}

func TestLoadPlaylistItemSettings(t *testing.T) {
	dir := t.TempDir()
	list := filepath.Join(dir, "list.m3u")
	body := "#EXTM3U\n" + filepath.Join(dir, "a.mp4") + "\n" + filepath.Join(dir, "b.mp4") + "\n"
	if err := os.WriteFile(list, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "p.gpls")
	doc := playlist.Document{
		Format:  playlist.FormatID,
		Version: 1,
		Videos: []playlist.Video{{
			URI:             list,
			CurrentPosition: 1234,
			IsMuted:         true,
			Volume:          0.5,
			Rate:            1.5,
			CellViewMode:    "stretch",
			PlaybackState:   "paused",
			ListIndex:       1,
			ListEnd:         "next",
		}},
	}
	if err := doc.Save(path); err != nil {
		t.Fatal(err)
	}
	a := &app{defaultVolume: 80, defaultRate: 1, defaultLoop: true, resolver: resolve.New()}
	if err := a.loadPlaylist(path); err != nil {
		t.Fatal(err)
	}
	c := a.cells[0]
	if c.viewMode != "stretch" || !c.startPaused || c.listEnd != "next" ||
		c.listIndex != 1 || c.path != filepath.Join(dir, "b.mp4") {
		t.Fatalf("cell state: %+v", c)
	}
	if c.startAt != 1234 || !c.muted || c.volume != 50 || c.rate != 1.5 {
		t.Fatalf("playback state: %+v", c)
	}
}

func TestDocumentViewMode(t *testing.T) {
	a := &app{cells: []*cell{{
		path:      "a.mp4",
		viewMode:  "stretch",
		listIndex: 2,
		listEnd:   "stop",
	}}}
	d := a.document()
	if len(d.Videos) != 1 {
		t.Fatalf("videos=%d", len(d.Videos))
	}
	v := d.Videos[0]
	if v.CellViewMode != "stretch" || v.ListIndex != 2 || v.ListEnd != "stop" {
		t.Fatalf("video %+v", v)
	}
}

func TestDocumentPersistsFreeGrid(t *testing.T) {
	a := &app{
		cells: []*cell{{path: "a.mp4"}, {path: "b.mp4"}},
		free:  true,
		freeTiles: []grid.Tile{
			{Box: [4]float64{0, 0, 0.6, 1}},
			{Box: [4]float64{0.6, 0, 1, 1}},
		},
	}
	d := a.document()
	var layout string
	if !getRawSetting(d, "grid_layout", &layout) || layout != "free" {
		t.Fatalf("layout %q", layout)
	}
	var tiles []grid.Tile
	if !getRawSetting(d, "grid_free_tiles", &tiles) || len(tiles) != 2 ||
		tiles[0].Box != a.freeTiles[0].Box {
		t.Fatalf("tiles %+v", tiles)
	}
	b := &app{defaultVolume: 80, defaultRate: 1, defaultLoop: true, resolver: resolve.New()}
	b.applyDocument(d, "")
	if !b.free || b.mosaic || b.single || len(b.freeTiles) != 2 || b.freeTiles[0].Box != a.freeTiles[0].Box {
		t.Fatalf("restored free=%v mosaic=%v single=%v tiles=%+v", b.free, b.mosaic, b.single, b.freeTiles)
	}
}

func TestDocumentPersistsMosaicGrid(t *testing.T) {
	a := &app{
		cells:  []*cell{{path: "a.mp4"}, {path: "b.mp4"}, {path: "c.mp4"}},
		mosaic: true,
	}
	a.ensureMosaic()
	a.mosaicTiles[1].Box = [4]float64{0.4, 0.1, 0.8, 0.9}
	a.mosaicLocked = map[int]bool{1: true}
	d := a.document()
	var layout string
	if !getRawSetting(d, "grid_layout", &layout) || layout != "mosaic" {
		t.Fatalf("layout %q", layout)
	}
	var locked []int
	if !getRawSetting(d, "grid_mosaic_locked", &locked) || len(locked) != 1 || locked[0] != 1 {
		t.Fatalf("locked %+v", locked)
	}
	var tiles []grid.Tile
	if !getRawSetting(d, "grid_mosaic_tiles", &tiles) || len(tiles) != 3 ||
		tiles[1].Box != a.mosaicTiles[1].Box {
		t.Fatalf("tiles %+v", tiles)
	}
	b := &app{defaultVolume: 80, defaultRate: 1, defaultLoop: true, resolver: resolve.New()}
	b.applyDocument(d, "")
	if !b.mosaic || b.free || b.single || !b.mosaicLocked[1] || len(b.mosaicTiles) != 3 ||
		b.mosaicTiles[1].Box != a.mosaicTiles[1].Box {
		t.Fatalf("restored free=%v mosaic=%v single=%v locked=%v tiles=%+v", b.free, b.mosaic, b.single, b.mosaicLocked, b.mosaicTiles)
	}
}

func TestDocumentPersistsSingleMode(t *testing.T) {
	share := 0.25
	a := &app{
		cells:       []*cell{{path: "a.mp4"}, {path: "b.mp4"}},
		single:      true,
		singleSide:  grid.StripRight,
		singleShare: &share,
	}
	d := a.document()
	var layout, side string
	var gotShare float64
	if !getRawSetting(d, "grid_layout", &layout) || layout != "single" {
		t.Fatalf("layout %q", layout)
	}
	if !getRawSetting(d, "single_side", &side) || side != "right" {
		t.Fatalf("side %q", side)
	}
	if !getRawSetting(d, "single_share", &gotShare) || gotShare != 0.25 {
		t.Fatalf("share %v", gotShare)
	}
	b := &app{defaultVolume: 80, defaultRate: 1, defaultLoop: true, resolver: resolve.New()}
	b.applyDocument(d, "")
	if !b.single || b.free || b.mosaic || b.singleSide != grid.StripRight ||
		b.singleShare == nil || *b.singleShare != 0.25 {
		t.Fatalf("restored free=%v mosaic=%v single=%v side=%v share=%v", b.free, b.mosaic, b.single, b.singleSide, b.singleShare)
	}
}

func TestApplyDocumentClearsStaleLayouts(t *testing.T) {
	a := &app{
		cells:  []*cell{{path: "a.mp4"}},
		free:   true,
		mosaic: true,
		single: true,
	}
	d := &playlist.Document{
		Format:  playlist.FormatID,
		Version: 1,
		Videos:  []playlist.Video{{URI: "a.mp4"}},
	}
	d.SetGridState("auto_rows", 0, 0)
	a.applyDocument(d, "")
	if a.free || a.mosaic || a.single {
		t.Fatalf("stale layout free=%v mosaic=%v single=%v", a.free, a.mosaic, a.single)
	}
	if a.cfg.Mode != grid.AutoRows {
		t.Fatalf("grid %+v", a.cfg)
	}
}

func TestApplyDocumentClampsSingleShare(t *testing.T) {
	a := &app{cells: []*cell{{path: "a.mp4"}}}
	d := &playlist.Document{
		Format:  playlist.FormatID,
		Version: 1,
		Videos:  []playlist.Video{{URI: "a.mp4"}},
	}
	setRawSetting(d, "grid_layout", "single")
	setRawSetting(d, "single_share", 5.0)
	a.applyDocument(d, "")
	if a.singleShare == nil || *a.singleShare != 0.4 {
		t.Fatalf("high share %v", a.singleShare)
	}
	setRawSetting(d, "single_share", 0.01)
	a.applyDocument(d, "")
	if a.singleShare == nil || *a.singleShare != 0.05 {
		t.Fatalf("low share %v", a.singleShare)
	}
}

func TestBindingUI(t *testing.T) {
	b, err := keymap.Parse("cmd+shift+p")
	if err != nil {
		t.Fatal(err)
	}
	mods, key, ok := bindingUI(b)
	if !ok || key != ui.KeyP || mods != ui.Cmd|ui.Shift {
		t.Fatalf("cmd+shift+p -> %v %v %v", mods, key, ok)
	}
	b, err = keymap.Parse("space")
	if err != nil {
		t.Fatal(err)
	}
	mods, key, ok = bindingUI(b)
	if !ok || key != ui.KeySpace || mods != 0 {
		t.Fatalf("space -> %v %v %v", mods, key, ok)
	}
	b, _ = keymap.Parse("none")
	if _, _, ok := bindingUI(b); ok {
		t.Fatal("empty binding accepted")
	}
}

func TestSettingsDraftKeymap(t *testing.T) {
	a := &app{keys: keymap.Default()}
	d := a.currentSettingsDraft()
	if len(d.keys) != len(keymap.Actions) {
		t.Fatalf("keys=%d", len(d.keys))
	}
	for i := range d.keys {
		switch d.keys[i].Action {
		case "play_pause":
			d.keys[i].Value = "p"
		case "mute":
			d.keys[i].Value = "none"
		}
	}
	if err := d.validateKeymap(); err != nil {
		t.Fatal(err)
	}
	a.applySettingsDraft(d)
	if got := a.keys.Get("play_pause").String(); got != "p" {
		t.Fatalf("play %q", got)
	}
	if !a.keys.Get("mute").Empty() {
		t.Fatalf("mute %q", a.keys.Get("mute").String())
	}
	d.keys[0].Value = "p"
	d.keys[1].Value = "p"
	if err := d.validateKeymap(); err == nil {
		t.Fatal("duplicate keymap accepted")
	}
}

func TestSubtitleArgs(t *testing.T) {
	s := subtitleStyle{
		Font:         "Arial",
		Size:         48,
		Color:        "#FFFF00",
		OutlineColor: "#000000",
		Outline:      4,
		Bold:         true,
		Opacity:      128,
		Encoding:     "UTF-8",
		Autodetect:   true,
	}
	args := strings.Join(s.args(), " ")
	for _, want := range []string{
		"--freetype-font=Arial",
		"--freetype-fontsize=48",
		"--freetype-color=16776960",
		"--freetype-outline-color=0",
		"--freetype-outline-thickness=4",
		"--freetype-bold",
		"--freetype-opacity=128",
		"--subsdec-encoding=UTF-8",
		"--sub-autodetect-file",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("args missing %q: %s", want, args)
		}
	}
	empty := strings.Join((subtitleStyle{Opacity: 255}).args(), " ")
	for _, want := range []string{"--freetype-outline-thickness=0", "--no-sub-autodetect-file"} {
		if !strings.Contains(empty, want) {
			t.Fatalf("empty style args missing %q: %s", want, empty)
		}
	}
}

func TestHexColor(t *testing.T) {
	for in, want := range map[string]string{
		"#FFFFFF": "16777215",
		"FFFFFF":  "16777215",
		"#000000": "0",
		"#FFFF00": "16776960",
		"#fff":    "",
		"zzzzzz":  "",
	} {
		if got := hexColor(in); got != want {
			t.Fatalf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestSubtitleSize(t *testing.T) {
	for in, want := range map[string]int{
		"0":     0,
		"12":    12,
		"48":    48,
		"48abc": 0,
		"-5":    0,
		"200":   96,
	} {
		if got := subtitleSize(in); got != want {
			t.Fatalf("%q -> %d, want %d", in, got, want)
		}
	}
}

func TestSubtitleOpacity(t *testing.T) {
	for in, want := range map[string]int{
		"255":    255,
		"128":    128,
		"128abc": 255,
		"500":    255,
		"-5":     0,
	} {
		if got := subtitleOpacity(in); got != want {
			t.Fatalf("%q -> %d, want %d", in, got, want)
		}
	}
}

func TestRetryable(t *testing.T) {
	for _, path := range []string{
		"http://example/a.m3u8",
		"https://example/a.mp4",
		"rtsp://example/live",
		"rtmp://example/live",
	} {
		if !retryable(path) {
			t.Fatalf("%q not retryable", path)
		}
	}
	for _, path := range []string{"a.mp4", "file:///a.mp4", ""} {
		if retryable(path) {
			t.Fatalf("%q retryable", path)
		}
	}
}

func TestRetryDelay(t *testing.T) {
	want := []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second, 30 * time.Second}
	for i, delay := range want {
		attempt := int32(i + 1)
		if got := retryDelay(attempt); got != delay {
			t.Fatalf("attempt %d -> %s, want %s", attempt, got, delay)
		}
	}
	if retryDelay(0) != time.Second {
		t.Fatal("attempt 0 should clamp to the first delay")
	}
}

func TestNetworkRetryLimit(t *testing.T) {
	a := &app{}
	a.setRetryPolicy("times", 3)
	if a.networkRetryLimit() != 3 {
		t.Fatalf("times limit=%d", a.networkRetryLimit())
	}
	a.setRetryPolicy("off", 3)
	if a.networkRetryLimit() != 0 {
		t.Fatal("off should not retry")
	}
	a.setRetryPolicy("infinite", 3)
	if a.networkRetryLimit() != -1 {
		t.Fatal("infinite should not cap")
	}
	a.setRetryPolicy("times", 0)
	if got := a.networkRetryLimit(); got != 1 {
		t.Fatalf("times clamped to %d", got)
	}
	a.setRetryPolicy("times", 5000)
	if got := a.networkRetryLimit(); got != maxRetryTimes {
		t.Fatalf("times capped to %d", got)
	}
}

func TestSettingsDraftSubtitles(t *testing.T) {
	a := &app{}
	d := defaultSettingsDraft()
	d.subFont = "Arial"
	d.subSize = "48"
	d.subColor = "#FFFF00"
	d.subOutlineColor = "#000000"
	d.subOutline = "Normal"
	d.subBold = true
	d.subOpacity = 50
	d.subEncoding = "UTF-8"
	d.subAutodetect = true
	a.applySettingsDraft(d)
	s := a.subtitle
	if s.Font != "Arial" || s.Size != 48 || s.Color != "#FFFF00" ||
		s.OutlineColor != "#000000" || s.Outline != 4 || !s.Bold ||
		s.Opacity != 128 || s.Encoding != "UTF-8" || !s.Autodetect {
		t.Fatalf("subtitle %+v", s)
	}
}

func TestApplyDocumentRestoresSnapshot(t *testing.T) {
	d := &playlist.Document{
		Format:  playlist.FormatID,
		Version: 1,
		Videos: []playlist.Video{
			{
				URI:             "a.mp4",
				CurrentPosition: 1234,
				IsMuted:         true,
				Volume:          0.5,
				Rate:            1.5,
				CellViewMode:    "stretch",
				PlaybackState:   "paused",
				ListEnd:         "next",
			},
			{URI: "b.mp4", CellViewMode: "fit"},
		},
	}
	d.SetGridState("fixed", 2, 3)
	a := &app{defaultVolume: 80, defaultRate: 1, defaultLoop: true, resolver: resolve.New()}
	a.applyDocument(d, "")
	if len(a.cells) != 2 {
		t.Fatalf("cells=%d", len(a.cells))
	}
	c := a.cells[0]
	if c.viewMode != "stretch" || !c.startPaused || c.startAt != 1234 ||
		!c.muted || c.volume != 50 || c.rate != 1.5 {
		t.Fatalf("cell %+v", c)
	}
	if a.cells[1].viewMode != "fit" {
		t.Fatalf("second view %q", a.cells[1].viewMode)
	}
	if a.cfg.Mode != grid.Fixed || a.cfg.Rows != 2 || a.cfg.Cols != 3 {
		t.Fatalf("grid %+v", a.cfg)
	}
	if a.playlistPath != "" || a.dirty {
		t.Fatalf("path=%q dirty=%v", a.playlistPath, a.dirty)
	}
}

func TestSnapshotNameAndRestore(t *testing.T) {
	store := snapshots.New(filepath.Join(t.TempDir(), "snapshots.json"))
	doc := &playlist.Document{
		Format:  playlist.FormatID,
		Version: 1,
		Videos:  []playlist.Video{{URI: "a.mp4", CellViewMode: "stretch"}},
	}
	if err := store.Save("Snapshot 1", doc); err != nil {
		t.Fatal(err)
	}
	if err := store.Save("Snapshot 2", doc); err != nil {
		t.Fatal(err)
	}
	a := &app{snapshots: store, defaultVolume: 80, defaultRate: 1, defaultLoop: true, resolver: resolve.New()}
	if got := a.snapshotName(); got != "Snapshot 3" {
		t.Fatalf("name %q", got)
	}
	a.restoreSnapshot("Snapshot 1")
	if len(a.cells) != 1 || a.cells[0].viewMode != "stretch" || !a.dirty {
		t.Fatalf("restore cells=%d dirty=%v", len(a.cells), a.dirty)
	}
}

func TestApplyDocumentRestoresListIndexOnMatchingCell(t *testing.T) {
	dir := t.TempDir()
	list := filepath.Join(dir, "list.m3u")
	item0 := filepath.Join(dir, "a.mp4")
	item1 := filepath.Join(dir, "b.mp4")
	body := "#EXTM3U\n" + item0 + "\n" + item1 + "\n"
	if err := os.WriteFile(list, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &app{defaultVolume: 80, defaultRate: 1, defaultLoop: true, resolver: resolve.New()}
	a.setPlaylist([]string{list})
	c := a.cells[0]
	a.advanceList(c)
	if c.listIndex != 1 || c.path != item1 {
		t.Fatalf("advance %d %q", c.listIndex, c.path)
	}
	d := &playlist.Document{
		Format:  playlist.FormatID,
		Version: 1,
		Videos:  []playlist.Video{{URI: item1, ListIndex: 0, ListEnd: "loop"}},
	}
	a.applyDocument(d, "")
	if c.listIndex != 0 || c.path != item0 {
		t.Fatalf("restore %d %q", c.listIndex, c.path)
	}
}

func TestApplyDocumentUpdatesLiveEngine(t *testing.T) {
	fixture, err := filepath.Abs("../../testdata/fixtures/plane-a.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("fixture missing")
	}
	a := &app{defaultVolume: 80, defaultRate: 1, defaultLoop: true, resolver: resolve.New()}
	a.setPlaylist([]string{fixture})
	e := engine.New(engine.NewVLCFactory(engine.VLCOptions{
		Args: []string{"--vout=dummy", "--aout=dummy"},
	}))
	defer e.Close()
	if err := e.Load(fixture); err != nil {
		t.Skipf("libVLC unavailable: %v", err)
	}
	a.cells[0].eng = e
	d := &playlist.Document{
		Format:  playlist.FormatID,
		Version: 1,
		Videos: []playlist.Video{{
			URI:             fixture,
			CurrentPosition: 3000,
			IsMuted:         true,
			Volume:          0.5,
			Rate:            1.5,
			PlaybackState:   "paused",
		}},
	}
	a.applyDocument(d, "")
	deadline := time.Now().Add(10 * time.Second)
	for {
		s := e.Snapshot()
		if s.Phase == engine.Paused && s.Time >= 2500 && s.Muted && s.Volume == 50 && s.Rate == 1.5 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("engine not restored: %+v", s)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestDecideListFail(t *testing.T) {
	skip, wait, status, failures, network := decideListFail("clip.mp4", 3, "", 2, 0, 0)
	if !skip || wait != 2*time.Second || status != "Next file in 2s" || failures != 1 || network != 0 {
		t.Fatalf("local skip=%v wait=%s status=%q failures=%d network=%d", skip, wait, status, failures, network)
	}
	skip, _, _, failures, network = decideListFail("http://example/a.mp4", 3, listFailPlayNext, 2, 0, 0)
	if !skip || failures != 1 || network != 1 {
		t.Fatalf("network skip=%v failures=%d network=%d", skip, failures, network)
	}
	skip, _, status, _, _ = decideListFail("clip.mp4", 3, listFailShowError, 2, 0, 0)
	if skip || status != "" {
		t.Fatalf("show error skip=%v status=%q", skip, status)
	}
	skip, wait, status, _, _ = decideListFail("clip.mp4", 3, listFailPlayNext, 0, 0, 0)
	if !skip || wait != 0 || status != "" {
		t.Fatalf("at once skip=%v wait=%s status=%q", skip, wait, status)
	}
	_, _, status, failures, network = decideListFail("http://example/a", 20, listFailPlayNext, 2, 10, 10)
	if status != "Stopped: 11 failed in a row" || failures != 11 || network != 11 {
		t.Fatalf("row stop status=%q failures=%d network=%d", status, failures, network)
	}
	skip, _, status, failures, _ = decideListFail("gone.mp4", 2, listFailPlayNext, 2, 2, 0)
	if skip || status != "Stopped: nothing on the list plays" || failures != 3 {
		t.Fatalf("list stop skip=%v status=%q failures=%d", skip, status, failures)
	}
}

func TestNoteListFailureUsesCellOverride(t *testing.T) {
	a := &app{}
	a.setListFailPolicy(listFailPlayNext, 2)
	wait := 0
	c := &cell{list: []string{"a.mp4", "b.mp4"}, listFail: listFailShowError, listFailWait: &wait}
	skip, _, status := a.noteListFailure(c, "a.mp4", 2)
	if skip || status != "" || c.failStreak != 1 {
		t.Fatalf("override skip=%v status=%q streak=%d", skip, status, c.failStreak)
	}
	c.listFail = listFailPlayNext
	skip, got, status := a.noteListFailure(c, "a.mp4", 2)
	if !skip || got != 0 || status != "" || c.failStreak != 2 {
		t.Fatalf("cell wait skip=%v got=%s status=%q streak=%d", skip, got, status, c.failStreak)
	}
}

func TestDocumentPersistsListFail(t *testing.T) {
	wait := 0
	a := &app{cells: []*cell{{
		path:         "list.m3u",
		listIndex:    1,
		listEnd:      "loop",
		listFail:     listFailShowError,
		listFailWait: &wait,
	}}}
	v := a.document().Videos[0]
	if v.ListFail != listFailShowError || v.ListFailWait == nil || *v.ListFailWait != 0 {
		t.Fatalf("video %+v", v)
	}
}

func TestDocumentPersistsGridTemplate(t *testing.T) {
	a := &app{
		cells:    []*cell{{path: "a.mp4"}},
		cfg:      grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 3},
		template: "1 + 2",
	}
	document := a.document()
	var name string
	if !getRawSetting(document, "grid_template", &name) || name != "1 + 2" {
		t.Fatalf("template %q", name)
	}
	b := &app{defaultVolume: 80, defaultRate: 1, defaultLoop: true, resolver: resolve.New()}
	b.setPlaylist([]string{"a.mp4"})
	b.applyDocument(document, "")
	if b.template != "1 + 2" || b.cfg.Mode != grid.Fixed || b.cfg.Rows != 2 || b.cfg.Cols != 3 {
		t.Fatalf("restored template %q cfg %+v", b.template, b.cfg)
	}
}

func TestFinishVideoSteps(t *testing.T) {
	next := &cell{list: []string{"a.mp4", "b.mp4"}, listIndex: 0, listEnd: "loop", endAction: endNext, path: "a.mp4"}
	a := &app{cells: []*cell{next}}
	a.finishVideo(next)
	if next.listIndex != 1 || next.path != "b.mp4" {
		t.Fatalf("next %d %q", next.listIndex, next.path)
	}
	prev := &cell{list: []string{"a.mp4", "b.mp4", "c.mp4"}, listIndex: 0, listEnd: "loop", endAction: endPrevious, path: "a.mp4"}
	a = &app{cells: []*cell{prev}}
	a.finishVideo(prev)
	if prev.listIndex != 2 || prev.path != "c.mp4" {
		t.Fatalf("previous wrap %d %q", prev.listIndex, prev.path)
	}
	stopped := &cell{list: []string{"a.mp4", "b.mp4"}, listIndex: 0, listEnd: "stop", endAction: endPrevious, path: "a.mp4"}
	a = &app{cells: []*cell{stopped}}
	a.stepPrevious(stopped)
	if stopped.listIndex != 0 || stopped.path != "a.mp4" {
		t.Fatalf("previous stop %d %q", stopped.listIndex, stopped.path)
	}
	looped := &cell{list: []string{"a.mp4", "b.mp4"}, listIndex: 0, endAction: endLoop, loop: true, path: "a.mp4"}
	a = &app{cells: []*cell{looped}}
	a.finishVideo(looped)
	if looped.listIndex != 0 || looped.path != "a.mp4" {
		t.Fatalf("loop stepped %d %q", looped.listIndex, looped.path)
	}
}

func TestPickShuffleSkipsCurrent(t *testing.T) {
	calls := 0
	got := pickShuffle(3, 1, func(n int) int {
		calls++
		if calls == 1 {
			return 1
		}
		return 2
	})
	if got != 2 || calls != 2 {
		t.Fatalf("pick=%d calls=%d", got, calls)
	}
	if pickShuffle(1, 0, func(int) int { return 0 }) != 0 {
		t.Fatal("single list should stay")
	}
}

func TestShuffleListChangesItem(t *testing.T) {
	c := &cell{list: []string{"a.mp4", "b.mp4", "c.mp4"}, listIndex: 0, listEnd: "loop", endAction: endShuffle, path: "a.mp4"}
	a := &app{cells: []*cell{c}}
	a.shuffleList(c)
	if c.listIndex == 0 || c.path == "a.mp4" {
		t.Fatalf("shuffle stayed on %d %q", c.listIndex, c.path)
	}
}

func TestAudioDelayPersistsPerVideo(t *testing.T) {
	a := &app{cells: []*cell{{path: "a.mp4", audioDelay: -450}}}
	v := a.document().Videos[0]
	if v.AudioDelay != -450 {
		t.Fatalf("saved %d", v.AudioDelay)
	}
	b := &app{defaultVolume: 80, defaultRate: 1, defaultLoop: true, resolver: resolve.New()}
	b.setPlaylist([]string{"a.mp4"})
	b.applyDocument(&playlist.Document{Format: playlist.FormatID, Version: 1, Videos: []playlist.Video{{URI: "a.mp4", AudioDelay: -450}}}, "")
	if b.cells[0].audioDelay != -450 {
		t.Fatalf("restored %d", b.cells[0].audioDelay)
	}
	if len(b.cells[0].list) != 0 {
		t.Fatalf("plain file became a list: %v", b.cells[0].list)

	}
}

func TestAutoReloadInterval(t *testing.T) {
	if autoReloadInterval(0) != 0 {
		t.Fatal("0 minutes should disable")
	}
	if autoReloadInterval(-3) != 0 {
		t.Fatal("negative should disable")
	}
	if autoReloadInterval(1) != time.Minute {
		t.Fatal("1 minute")
	}
	if autoReloadInterval(5) != 5*time.Minute {
		t.Fatal("5 minutes")
	}
	if autoReloadInterval(100000) != 1000*time.Minute {
		t.Fatal("should clamp to 1000 minutes")
	}
}

func TestPictureRectGeometry(t *testing.T) {
	centre := viewZoom{Scale: 1, AnchorX: 0.5, AnchorY: 0.5}
	// Square video in a wide cell, fit: the picture is the cell height.
	r := placePicture(videoShape{W: 100, H: 100}, 200, 100, false, false, centre)
	if r.W != 100 || r.H != 100 || r.X != 50 || r.Y != 0 {
		t.Fatalf("fit: %+v", r)
	}
	// Same, fill: the picture covers the cell width.
	r = placePicture(videoShape{W: 100, H: 100}, 200, 100, true, false, centre)
	if r.W != 200 || r.H != 200 || r.X != 0 || r.Y != -50 {
		t.Fatalf("fill: %+v", r)
	}
	// Stretch ignores the video shape.
	r = placePicture(videoShape{W: 100, H: 100}, 200, 100, false, true, centre)
	if r.W != 200 || r.H != 100 {
		t.Fatalf("stretch: %+v", r)
	}
	// Zoom doubles the picture about the centre.
	r = placePicture(videoShape{W: 100, H: 100}, 200, 100, false, false, viewZoom{Scale: 2, AnchorX: 0.5, AnchorY: 0.5})
	if r.W != 200 || r.H != 200 || r.X != 0 || r.Y != -50 {
		t.Fatalf("zoom: %+v", r)
	}
	// Zoom about the top-left keeps that corner fixed.
	r = placePicture(videoShape{W: 100, H: 100}, 200, 100, false, false, viewZoom{Scale: 2, AnchorX: 0, AnchorY: 0})
	if r.X != 0 || r.Y != 0 {
		t.Fatalf("top-left anchor: %+v", r)
	}
	// A shift moves the picture but cannot push it off the pane.
	// A quarter-cell shift of a right-anchored fit moves by the room at
	// the anchor: half the 100 spare pixels plus a quarter cell of shift.
	if x := placePicture(videoShape{W: 100, H: 100}, 200, 100, false, false, viewZoom{Scale: 1, AnchorX: 0.5, AnchorY: 0.5, ShiftX: 0.25}).X; x != 100 {
		t.Fatalf("shift: %g", x)
	}
	if clampZoom(0.1) != 1 || clampZoom(20) != 10 || clampZoom(1.5) != 1.5 {
		t.Fatal("clampZoom")
	}
	// A picture larger than the pane may slide the overshoot either way up
	// to half the overshoot, so black still does not show.
	if clampShift(999, 100, 200) != 50 || clampShift(-999, 100, 200) != -50 {
		t.Fatalf("clampShift bigger picture: %g %g", clampShift(999, 100, 200), clampShift(-999, 100, 200))
	}
	// A picture smaller than the pane may move up to half the spare room.
	if clampShift(999, 200, 100) != 50 || clampShift(-999, 200, 100) != -50 {
		t.Fatalf("clampShift smaller picture: %g %g", clampShift(999, 200, 100), clampShift(-999, 200, 100))
	}
	// A picture at the pane size cannot move at all.
	if clampShift(5, 100, 100) != 0 {
		t.Fatal("clampShift equal picture")
	}
}

func TestParseAnchor(t *testing.T) {
	if x, y := parseAnchor("top-left"); x != 0 || y != 0 {
		t.Fatalf("top-left %v %v", x, y)
	}
	if x, y := parseAnchor("wat"); x != 0.5 || y != 0.5 {
		t.Fatalf("unknown %v %v", x, y)
	}
	if anchorName(0, 1) != "bottom-left" {
		t.Fatal("name")
	}
}

func TestCropMargins(t *testing.T) {
	// Square frame, cut to 16:9: the crop comes off the top and bottom.
	m := parseCropRatio("16:9", videoShape{W: 1920, H: 1920})
	if m.Left != 0 || m.Right != 0 {
		t.Fatalf("16:9 sides %+v", m)
	}
	if m.Top != 420 || m.Bottom != 420 {
		t.Fatalf("16:9 top/bottom %+v", m)
	}
	// Wide frame, cut to 4:3: the crop comes off the left and right.
	m = parseCropRatio("4:3", videoShape{W: 1920, H: 1080})
	if m.Left != 240 || m.Right != 240 {
		t.Fatalf("4:3 sides %+v", m)
	}
	if m.Top != 0 || m.Bottom != 0 {
		t.Fatalf("4:3 top/bottom %+v", m)
	}
	if !parseCropRatio("", videoShape{W: 10, H: 10}).empty() {
		t.Fatal("empty ratio should crop nothing")
	}
	if !parseCropRatio("wat", videoShape{W: 10, H: 10}).empty() {
		t.Fatal("bad ratio should crop nothing")
	}
	// Decimal ratios must parse: 2.35:1 on 1920x1080 keeps 817 px, so the
	// margins are 131/132, not a 1 px sliver.
	m = parseCropRatio("2.35:1", videoShape{W: 1920, H: 1080})
	if m.Top != 131 || m.Bottom != 132 {
		t.Fatalf("2.35:1 top/bottom %+v", m)
	}
	m = parseCropRatio("1.5:1", videoShape{W: 1920, H: 1080})
	if m.Left != 150 || m.Right != 150 {
		t.Fatalf("1.5:1 sides %+v", m)
	}
	x, y, w, h := cropRegion(videoShape{W: 1920, H: 1080}, cropMargins{Left: 240, Right: 240})
	if x != 240 || y != 0 || w != 1440 || h != 1080 {
		t.Fatalf("region %v %v %v %v", x, y, w, h)
	}
	x, y, w, h = cropRegion(videoShape{W: 10, H: 10}, cropMargins{Left: 9, Right: 9, Top: 9, Bottom: 9})
	if w != 1 || h != 1 {
		t.Fatalf("clamped region %v %v", w, h)
	}
}

func TestCropIsFoldedIntoPlacement(t *testing.T) {
	video := videoShape{W: 1920, H: 1080}
	crop := parseCropRatio("4:3", video)
	// The kept 1440x1080 region fits a 200x100 cell as 133.3x100, and the
	// whole frame is scaled by the same factor and moved left.
	r := placePicture(video, 200, 100, false, false, viewZoom{Scale: 1, Crop: crop, AnchorX: 0.5, AnchorY: 0.5})
	if !near(r.W, 177.78) || !near(r.H, 100) {
		t.Fatalf("frame size %v %v", r.W, r.H)
	}
	if !near(r.X, 11.11) || !near(r.Y, 0) {
		t.Fatalf("frame position %v %v", r.X, r.Y)
	}
	// The kept region lands centred in the cell: 133.3 wide at x=33.3.
	if !near(r.X+240*(r.W/1920), 33.33) {
		t.Fatalf("kept region not centred: %+v", r)
	}
}

func TestFillLimitFallsBackToFit(t *testing.T) {
	video := videoShape{W: 100, H: 100}
	fill := placePicture(video, 200, 100, true, false, viewZoom{Scale: 1, FillLimit: 100, AnchorX: 0.5, AnchorY: 0.5})
	if fill.W != 200 || fill.H != 200 {
		t.Fatalf("unlimited fill %+v", fill)
	}
	limited := placePicture(video, 200, 100, true, false, viewZoom{Scale: 1, FillLimit: 30, AnchorX: 0.5, AnchorY: 0.5})
	if limited.W != 100 || limited.H != 100 || limited.X != 50 {
		t.Fatalf("limited fill %+v", limited)
	}
	if fillLimitName(100) != "Off" || fillLimitName(30) != "30%" || fillLimitValue("20%") != 20 || fillLimitValue("Off") != 100 {
		t.Fatal("fill limit names")
	}
}

func near(a, b float64) bool {
	d := a - b
	return d < 0.05 && d > -0.05
}

func TestFrozenStepConstants(t *testing.T) {
	if clampPlaybackRate(0.1) != 0.2 || clampPlaybackRate(20) != 12 || clampPlaybackRate(1.5) != 1.5 {
		t.Fatal("rate clamp")
	}
	if got := zoomRung(1, 1, 0.1, false); got != 1.1 {
		t.Fatalf("zoom in %v", got)
	}
	if got := zoomRung(1.1, -1, 0.1, false); got != 1 {
		t.Fatalf("zoom out %v", got)
	}
	if got := zoomRung(10, -1, 0.1, false); got != 9.9 {
		t.Fatalf("zoom down from cap %v", got)
	}
	if got := zoomRung(10, 1, 0.1, false); got != 10 {
		t.Fatalf("zoom at cap %v", got)
	}
	if clampSubtitleDelay(-400000) != -300000 || clampSubtitleDelay(400000) != 300000 {
		t.Fatal("subtitle delay clamp")
	}
	if subtitleDelayNudge(299950, engine.SubtitleDelayStepMS) != 300000 {
		t.Fatal("subtitle delay nudge")
	}
	if seekPercentTarget(10000, 0.5) != 5000 {
		t.Fatal("percent")
	}
	if randomSeekTarget(1000, 2000, func(n int64) int64 { return 0 }) != 1000 {
		t.Fatal("random lower")
	}
	if randomSeekTarget(1000, 2000, func(n int64) int64 { return 1000 }) != 2000 {
		t.Fatal("random upper")
	}
	var start, end int64
	if !setLoopPoint(&start, &end, 3000, true) || start != 3000 {
		t.Fatal("loop start")
	}
	if !setLoopPoint(&start, &end, 5000, false) || end != 5000 {
		t.Fatal("loop end")
	}
	if setLoopPoint(&start, &end, 6000, true) {
		t.Fatal("start after end should fail")
	}
	if seekableTime(7000, start, end) != start {
		t.Fatal("outside loop seeks to start")
	}
	if ms, ok := parseTimecode("1:02.5"); !ok || ms != 62500 {
		t.Fatalf("timecode %d %v", ms, ok)
	}
	m := cropStep(cropMargins{}, "left", 10)
	if m.Left != 10 {
		t.Fatal("crop step")
	}
}
