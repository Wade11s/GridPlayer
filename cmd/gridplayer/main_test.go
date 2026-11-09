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
	if got := (subtitleStyle{Opacity: 255}).args(); len(got) != 0 {
		t.Fatalf("empty style args: %v", got)
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
