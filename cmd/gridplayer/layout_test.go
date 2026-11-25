package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gridplayer-go/internal/dummy"
	"gridplayer-go/internal/engine"
	"gridplayer-go/internal/grid"
	"gridplayer-go/internal/screenshots"
	"gridplayer-go/internal/settings"
)

// The frozen CellLayout sets spacing 0 and every Box shares its edge with
// its neighbour, so each Go layout must hand back rects that meet exactly.
func TestShowAllCellsTogglesInFixedMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.ini")
	st, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{settings: st, cfg: grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3}}
	a.togglePreallocate()
	if !a.cfg.Preallocate || !a.dirty {
		t.Fatalf("preallocate=%v dirty=%v", a.cfg.Preallocate, a.dirty)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Get("playlist/grid_preallocate", ""); got != "true" {
		t.Fatalf("saved preallocate = %q", got)
	}
	d := a.document()
	if _, _, _, preallocate := d.GridState(); !preallocate {
		t.Fatal("the document lost preallocate")
	}
	b := &app{defaultVolume: 80, defaultRate: 1}
	b.applyDocument(d, "")
	if !b.cfg.Preallocate || b.cfg.Mode != grid.Fixed {
		t.Fatalf("restored cfg=%+v", b.cfg)
	}

	// Presets and mode switches carry the flag instead of clearing it.
	a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2})
	if !a.cfg.Preallocate {
		t.Fatal("a size preset cleared preallocate")
	}
	a.setGrid(grid.Config{Mode: grid.AutoRows})
	if !a.cfg.Preallocate {
		t.Fatal("a mode switch cleared preallocate")
	}

	// Free Tiles, Mosaic and Single Mode are not a fixed grid even though
	// cfg.Mode still says Fixed.
	a.free = true
	if a.fixedGrid() {
		t.Fatal("free tiles counted as a fixed grid")
	}
	a.free = false
	a.mosaic = true
	if a.fixedGrid() {
		t.Fatal("mosaic counted as a fixed grid")
	}
	a.mosaic = false
	a.single = true
	if a.fixedGrid() {
		t.Fatal("single mode counted as a fixed grid")
	}
	a.single = false

	// Outside a fixed grid the action is a no-op, so a set flag stays set.
	a.cfg = grid.Config{Mode: grid.AutoRows, Rows: 3, Cols: 3, Preallocate: true}
	a.togglePreallocate()
	if !a.cfg.Preallocate {
		t.Fatal("preallocate toggled outside a fixed grid")
	}
}

func TestFixedOverflowAsksBeforeClosing(t *testing.T) {
	cells := func(n int) []*cell {
		out := make([]*cell, n)
		for i := range out {
			out[i] = &cell{path: fmt.Sprintf("%d.mp4", i)}
		}
		return out
	}

	asked := 0
	a := &app{
		cells:       cells(5),
		active:      4,
		cfg:         grid.Config{Mode: grid.AutoRows},
		askOverflow: func(over, capacity int) bool { asked++; return over == 1 && capacity == 4 },
	}
	a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2})
	if asked != 1 {
		t.Fatalf("asked %d times, want 1", asked)
	}
	if len(a.cells) != 4 || a.active != 3 {
		t.Fatalf("cells=%d active=%d, want 4/3", len(a.cells), a.active)
	}
	for i, c := range a.cells {
		if want := fmt.Sprintf("%d.mp4", i); c.path != want {
			t.Fatalf("kept cell %d is %q, want %q (the trailing cells close)", i, c.path, want)
		}
	}
	if a.cfg.Mode != grid.Fixed || a.cfg.Rows != 2 || a.cfg.Cols != 2 || !a.dirty {
		t.Fatalf("cfg=%+v dirty=%v", a.cfg, a.dirty)
	}

	// Cancelling keeps every cell and the old layout.
	a = &app{
		cells:       cells(5),
		cfg:         grid.Config{Mode: grid.AutoRows},
		askOverflow: func(int, int) bool { return false },
	}
	a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2})
	if len(a.cells) != 5 || a.cfg.Mode != grid.AutoRows {
		t.Fatalf("cancel kept cells=%d cfg=%+v", len(a.cells), a.cfg)
	}

	// A grid that holds every video asks nothing.
	a = &app{
		cells:       cells(4),
		cfg:         grid.Config{Mode: grid.AutoRows},
		askOverflow: func(int, int) bool { t.Fatal("asked for a grid that fits"); return false },
	}
	a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2})
	if len(a.cells) != 4 || a.cfg.Mode != grid.Fixed {
		t.Fatalf("fitted grid cells=%d cfg=%+v", len(a.cells), a.cfg)
	}
}

func TestOverflowCloseFollowsTheTiledReadingOrder(t *testing.T) {
	cells := func(n int) []*cell {
		out := make([]*cell, n)
		for i := range out {
			out[i] = &cell{path: fmt.Sprintf("%d.mp4", i)}
		}
		return out
	}
	// Playlist order 0..4, but read row by row the top-left tile is 4 and
	// the bottom-right is 0, so a 2x2 keeps 4,3,2,1 and closes 0.
	a := &app{
		cells: cells(5),
		// The reading-last video is active, so it must survive the re-pack
		// at a different index; index 2 would coincide before and after.
		active: 4,
		free:   true,
		freeTiles: []grid.Tile{
			{Box: [4]float64{0.5, 0.6, 0.95, 0.95}},
			{Box: [4]float64{0.02, 0.6, 0.4, 0.95}},
			{Box: [4]float64{0.5, 0.32, 0.95, 0.58}},
			{Box: [4]float64{0.55, 0.02, 0.95, 0.3}},
			{Box: [4]float64{0.02, 0.02, 0.4, 0.3}},
		},
	}
	if got := a.liveOrder(); len(got) != 5 || got[0] != 4 || got[1] != 3 || got[2] != 2 || got[3] != 1 || got[4] != 0 {
		t.Fatalf("live order = %v, want [4 3 2 1 0]", got)
	}
	activePtr := a.cells[a.active]
	a.closeOverflow(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2})
	if len(a.cells) != 4 {
		t.Fatalf("cells = %d, want 4", len(a.cells))
	}
	// Survivors are re-packed in reading order, so the grid reads
	// e,d,c,b top to bottom, as the frozen pack_flow does.
	want := []string{"4.mp4", "3.mp4", "2.mp4", "1.mp4"}
	for i, w := range want {
		if a.cells[i].path != w {
			t.Fatalf("kept %v, want %v", cellPaths(a.cells), want)
		}
	}
	// The active video is still the same cell, by identity, at a new index.
	if a.cells[a.active] != activePtr {
		t.Fatalf("active is %q, want %q", a.cells[a.active].path, activePtr.path)
	}
	if a.active == 4 {
		t.Fatalf("active index did not move with its video: %d", a.active)
	}

	// Closing the active cell itself leaves a valid active index on a
	// surviving video.
	b := &app{cells: cells(5), active: 4}
	b.closeOverflow(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2})
	if b.active < 0 || b.active >= len(b.cells) {
		t.Fatalf("active %d out of range for %d cells", b.active, len(b.cells))
	}
	if got := b.cells[b.active].path; got == "4.mp4" {
		t.Fatalf("active still points at the closed video")
	}
}

func TestFixedEntryPacksTiledOrderWithoutOverflow(t *testing.T) {
	cells := []*cell{{path: "a.mp4"}, {path: "b.mp4"}, {path: "c.mp4"}}
	a := &app{
		cells:  cells,
		active: 1,
		free:   true,
		freeTiles: []grid.Tile{
			{Box: [4]float64{0.5, 0.02, 0.95, 0.3}}, // b later in reading order
			{Box: [4]float64{0.5, 0.6, 0.95, 0.95}}, // c
			{Box: [4]float64{0.02, 0.02, 0.4, 0.3}}, // a first
		},
	}
	// Reading order is a, c, b: indices 2, 0, 1.
	if got := a.liveOrder(); len(got) != 3 || got[0] != 2 || got[1] != 0 || got[2] != 1 {
		t.Fatalf("live order = %v, want [2 0 1]", got)
	}
	activePtr := a.cells[a.active]
	a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3})
	want := []string{"c.mp4", "a.mp4", "b.mp4"}
	for i, w := range want {
		if a.cells[i].path != w {
			t.Fatalf("packed %v, want %v", cellPaths(a.cells), want)
		}
	}
	if a.cells[a.active] != activePtr {
		t.Fatalf("active moved to %q", a.cells[a.active].path)
	}
	if len(a.cells) != 3 {
		t.Fatalf("a grid that fits closed cells: %d", len(a.cells))
	}
}

func TestFixedEntryClearsTilesAndLocks(t *testing.T) {
	cells := []*cell{{path: "a.mp4"}, {path: "b.mp4"}, {path: "c.mp4"}}
	a := &app{
		cells:  cells,
		active: 1,
		mosaic: true,
		mosaicTiles: []grid.Tile{
			{Box: [4]float64{0.5, 0.02, 0.95, 0.3}}, // b
			{Box: [4]float64{0.5, 0.6, 0.95, 0.95}}, // c
			{Box: [4]float64{0.02, 0.02, 0.4, 0.3}}, // a
		},
		mosaicLocked: map[int]bool{1: true},
	}
	activePtr := a.cells[a.active]
	a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3})
	// The frozen _apply_fixed clears the free layout: no tiles, no locks.
	if a.mosaicLocked != nil {
		t.Fatalf("locks survived the fixed entry: %v", a.mosaicLocked)
	}
	if a.mosaicTiles != nil || a.freeTiles != nil {
		t.Fatal("tiles survived the fixed entry")
	}
	want := []string{"c.mp4", "a.mp4", "b.mp4"}
	for i, w := range want {
		if a.cells[i].path != w {
			t.Fatalf("packed %v, want %v", cellPaths(a.cells), want)
		}
	}
	if a.cells[a.active] != activePtr {
		t.Fatalf("active moved to %q", a.cells[a.active].path)
	}
}

func TestFixedEntryClearsDefaultTilesAndLocks(t *testing.T) {
	// A default Free/Mosaic layout reads in identity order, so the cells are
	// not permuted; the frozen _free.clear() still drops the tiles and locks.
	cells := []*cell{{path: "a.mp4"}, {path: "b.mp4"}, {path: "c.mp4"}}
	a := &app{
		cells:        cells,
		active:       1,
		mosaic:       true,
		mosaicTiles:  grid.Mosaic(3),
		mosaicOrder:  grid.DefaultOrder(3),
		mosaicLocked: map[int]bool{1: true},
	}
	if got := a.liveOrder(); len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 2 {
		t.Fatalf("default mosaic order = %v, want identity", got)
	}
	a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3})
	if a.mosaicLocked != nil || a.mosaicTiles != nil {
		t.Fatalf("default tiles or locks survived: tiles=%v locks=%v", a.mosaicTiles, a.mosaicLocked)
	}
	for i, want := range []string{"a.mp4", "b.mp4", "c.mp4"} {
		if a.cells[i].path != want {
			t.Fatalf("identity order changed: %v", cellPaths(a.cells))
		}
	}
}

func TestLeavingTiledForAPlainGridPacksAndClears(t *testing.T) {
	cells := []*cell{{path: "a.mp4"}, {path: "b.mp4"}, {path: "c.mp4"}}
	a := &app{
		cells:  cells,
		active: 0,
		free:   true,
		// Tiles are in cell order: cell a top right, b bottom right, c top
		// left, so the reading order is c, then a, then b.
		freeTiles: []grid.Tile{
			{Box: [4]float64{0.5, 0.02, 0.95, 0.3}}, // cell a
			{Box: [4]float64{0.5, 0.6, 0.95, 0.95}}, // cell b
			{Box: [4]float64{0.02, 0.02, 0.4, 0.3}}, // cell c
		},
	}
	activePtr := a.cells[a.active]
	a.setGrid(grid.Config{Mode: grid.AutoRows})
	if a.cfg.Mode != grid.AutoRows || a.free || a.mosaic || a.single {
		t.Fatalf("grid mode not plain: cfg=%+v free=%v", a.cfg, a.free)
	}
	if a.freeTiles != nil || a.freeOrder != nil || a.mosaicLocked != nil {
		t.Fatalf("tiles or locks survived: tiles=%v order=%v locks=%v", a.freeTiles, a.freeOrder, a.mosaicLocked)
	}
	want := []string{"c.mp4", "a.mp4", "b.mp4"}
	for i, w := range want {
		if a.cells[i].path != w {
			t.Fatalf("packed %v, want %v", cellPaths(a.cells), want)
		}
	}
	if a.cells[a.active] != activePtr {
		t.Fatalf("active moved to %q", a.cells[a.active].path)
	}
	if len(a.cells) != 3 {
		t.Fatalf("a plain grid closed cells: %d", len(a.cells))
	}
}

func TestDeclinedFixedEntryKeepsTheTiles(t *testing.T) {
	cells := func(n int) []*cell {
		out := make([]*cell, n)
		for i := range out {
			out[i] = &cell{path: fmt.Sprintf("%d.mp4", i)}
		}
		return out
	}
	tiles := []grid.Tile{
		{Box: [4]float64{0.5, 0.6, 0.95, 0.95}},
		{Box: [4]float64{0.02, 0.6, 0.4, 0.95}},
		{Box: [4]float64{0.5, 0.32, 0.95, 0.58}},
		{Box: [4]float64{0.55, 0.02, 0.95, 0.3}},
		{Box: [4]float64{0.02, 0.02, 0.4, 0.3}},
	}
	before := grid.Config{Mode: grid.AutoRows}
	a := &app{
		cells:       cells(5),
		free:        true,
		freeTiles:   tiles,
		cfg:         before,
		askOverflow: func(int, int) bool { return false },
	}
	a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2})
	if len(a.cells) != 5 || a.cfg != before {
		t.Fatalf("declined entry changed cells=%d cfg=%+v", len(a.cells), a.cfg)
	}
	if len(a.freeTiles) != 5 || !a.free || a.freeOrder != nil {
		t.Fatalf("declined entry cleared the tiles: free=%v tiles=%d", a.free, len(a.freeTiles))
	}
	if got := cellPaths(a.cells); got[0] != "0.mp4" || got[4] != "4.mp4" {
		t.Fatalf("declined entry re-packed: %v", got)
	}
}

func TestTemplateEntryPacksAndCloses(t *testing.T) {
	cells := []*cell{{path: "a.mp4"}, {path: "b.mp4"}, {path: "c.mp4"}, {path: "d.mp4"}}
	a := &app{
		cells:  cells,
		active: 1,
		free:   true,
		// Cell order a..d: a bottom right, b bottom left, c top right,
		// d top left, so the reading order is d, c, b, a.
		freeTiles: []grid.Tile{
			{Box: [4]float64{0.5, 0.6, 0.95, 0.95}},  // cell a
			{Box: [4]float64{0.02, 0.6, 0.4, 0.95}},  // cell b
			{Box: [4]float64{0.55, 0.02, 0.95, 0.3}}, // cell c
			{Box: [4]float64{0.02, 0.02, 0.4, 0.3}},  // cell d
		},
		askOverflow: func(over, capacity int) bool { return over == 1 && capacity == 3 },
	}
	activePtr := a.cells[a.active]
	// "1 + 2" has three places and the reading order is d,c,b,a, so three
	// survive and a, read last, closes.
	a.setTemplate("1 + 2")
	if a.template != "1 + 2" || a.cfg.Mode != grid.Fixed || a.cfg.Rows != 2 || a.cfg.Cols != 3 {
		t.Fatalf("template not applied: cfg=%+v name=%q", a.cfg, a.template)
	}
	want := []string{"d.mp4", "c.mp4", "b.mp4"}
	if len(a.cells) != len(want) {
		t.Fatalf("cells = %d, want %d (%v)", len(a.cells), len(want), cellPaths(a.cells))
	}
	for i, w := range want {
		if a.cells[i].path != w {
			t.Fatalf("packed %v, want %v", cellPaths(a.cells), want)
		}
	}
	if a.cells[a.active] != activePtr {
		t.Fatalf("active moved to %q", a.cells[a.active].path)
	}
}

func TestSettingsApplyKeepsATiledLayout(t *testing.T) {
	tiles := []grid.Tile{
		{Box: [4]float64{0.5, 0.02, 0.95, 0.3}},
		{Box: [4]float64{0.5, 0.6, 0.95, 0.95}},
		{Box: [4]float64{0.02, 0.02, 0.4, 0.3}},
	}
	for _, layout := range []string{"Free Tiles", "Mosaic"} {
		a := &app{
			cells:     []*cell{{path: "a.mp4"}, {path: "b.mp4"}, {path: "c.mp4"}},
			free:      layout == "Free Tiles",
			mosaic:    layout == "Mosaic",
			freeTiles: tiles,
			mosaicTiles: []grid.Tile{
				{Box: [4]float64{0.5, 0.02, 0.95, 0.3}},
				{Box: [4]float64{0.5, 0.6, 0.95, 0.95}},
				{Box: [4]float64{0.02, 0.02, 0.4, 0.3}},
			},
			mosaicLocked: map[int]bool{0: true},
		}
		if ok := a.applyGridDraft(settingsDraft{layout: layout, rows: "3", cols: "3"}); !ok {
			t.Fatalf("%s apply reported declined", layout)
		}
		// An Apply that keeps the tiled layout must not wipe the arrangement.
		if layout == "Free Tiles" && len(a.freeTiles) != 3 {
			t.Fatalf("free tiles were cleared: %d", len(a.freeTiles))
		}
		if layout == "Mosaic" {
			if len(a.mosaicTiles) != 3 || !a.mosaicLocked[0] {
				t.Fatalf("mosaic tiles or locks were cleared: %d %v", len(a.mosaicTiles), a.mosaicLocked)
			}
		}
		if got := cellPaths(a.cells); got[0] != "a.mp4" || got[1] != "b.mp4" || got[2] != "c.mp4" {
			t.Fatalf("%s apply reordered the cells: %v", layout, got)
		}
	}
}

func TestSettingsApplyLeavesTiledForPlain(t *testing.T) {
	a := &app{
		cells: []*cell{{path: "a.mp4"}, {path: "b.mp4"}, {path: "c.mp4"}},
		free:  true,
		// Tiles are in cell order: cell a top right, b bottom right, c top
		// left, so the reading order is c, then a, then b.
		freeTiles: []grid.Tile{
			{Box: [4]float64{0.5, 0.02, 0.95, 0.3}}, // cell a
			{Box: [4]float64{0.5, 0.6, 0.95, 0.95}}, // cell b
			{Box: [4]float64{0.02, 0.02, 0.4, 0.3}}, // cell c
		},
	}
	if ok := a.applyGridDraft(settingsDraft{layout: "Auto Rows"}); !ok {
		t.Fatal("plain apply reported declined")
	}
	if a.free || len(a.freeTiles) != 0 {
		t.Fatalf("free layout survived: free=%v tiles=%d", a.free, len(a.freeTiles))
	}
	// The tiles read c (top left), a (top right), b (bottom), so the flow
	// takes that order.
	want := []string{"c.mp4", "a.mp4", "b.mp4"}
	for i, w := range want {
		if a.cells[i].path != w {
			t.Fatalf("packed %v, want %v", cellPaths(a.cells), want)
		}
	}
	if a.cfg.Mode != grid.AutoRows {
		t.Fatalf("cfg = %+v", a.cfg)
	}
}

func TestLiveOrderUsesMosaicTilesAndFallsBack(t *testing.T) {
	mosaicTiles := []grid.Tile{
		{Box: [4]float64{0.5, 0.5, 0.95, 0.95}},
		{Box: [4]float64{0.02, 0.02, 0.4, 0.3}},
	}
	a := &app{cells: make([]*cell, 2), mosaic: true, mosaicTiles: mosaicTiles}
	if got := a.liveOrder(); len(got) != 2 || got[0] != 1 || got[1] != 0 {
		t.Fatalf("mosaic live order = %v, want [1 0]", got)
	}

	// A tile slice that does not match the cells falls back to playlist
	// order instead of dropping or duplicating videos.
	a = &app{cells: make([]*cell, 3), free: true, freeTiles: mosaicTiles}
	if got := a.liveOrder(); len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 2 {
		t.Fatalf("mismatched tiles live order = %v, want [0 1 2]", got)
	}
}

func cellPaths(cells []*cell) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = c.path
	}
	return out
}

func TestNonFixedLayoutsNeverAsk(t *testing.T) {
	a := &app{
		cells:       make([]*cell, 5),
		askOverflow: func(int, int) bool { t.Fatal("a non-fixed layout asked to close videos"); return false },
	}
	for _, cfg := range []grid.Config{
		{Mode: grid.AutoRows},
		{Mode: grid.AutoCols},
		{Mode: grid.Shapes},
		{Mode: grid.Fixed, Rows: 1, Cols: 1},
	} {
		if cfg.Mode == grid.Fixed {
			continue // a fixed grid is expected to ask
		}
		if !a.confirmOverflow(cfg) {
			t.Fatalf("%+v asked", cfg)
		}
	}
	if !a.confirmOverflow(grid.Config{Mode: grid.AutoRows}) || len(a.cells) != 5 {
		t.Fatal("a non-fixed config changed the cells")
	}

	// The Settings tab's own guard: cfg still says Fixed with less capacity
	// than the cells, but Free Tiles, Mosaic and Single Mode must not ask or
	// close anything.
	for _, layout := range []string{"Free Tiles", "Mosaic", "Single Mode"} {
		a := &app{
			cells:       make([]*cell, 5),
			cfg:         grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2},
			askOverflow: func(int, int) bool { t.Fatalf("%s asked to close videos", layout); return false },
		}
		if ok := a.applyGridDraft(settingsDraft{layout: layout, rows: "2", cols: "2"}); !ok {
			t.Fatalf("%s reported declined", layout)
		}
		if len(a.cells) != 5 {
			t.Fatalf("%s dropped cells: %d", layout, len(a.cells))
		}
	}
}

func TestOverflowCount(t *testing.T) {
	a := &app{cells: make([]*cell, 5)}
	if got := a.overflowCount(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2}); got != 1 {
		t.Fatalf("5 in 2x2 overflow = %d, want 1", got)
	}
	if got := a.overflowCount(grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3}); got != 0 {
		t.Fatalf("5 in 3x3 overflow = %d, want 0", got)
	}
	if got := a.overflowCount(grid.Config{Mode: grid.AutoRows}); got != 0 {
		t.Fatalf("auto overflow = %d, want 0", got)
	}
}

func TestGridDraftOverflowCancelKeepsLayout(t *testing.T) {
	a := &app{
		cells:       make([]*cell, 5),
		cfg:         grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3},
		askOverflow: func(int, int) bool { return false },
	}
	if ok := a.applyGridDraft(settingsDraft{layout: "Fixed", rows: "2", cols: "2"}); ok {
		t.Fatal("a declined smaller grid reported applied")
	}
	if len(a.cells) != 5 || a.cfg.Rows != 3 || a.cfg.Cols != 3 {
		t.Fatalf("cancel changed cells=%d cfg=%+v", len(a.cells), a.cfg)
	}
	// A declined Settings apply must not overwrite the stored grid keys,
	// even though the rest of the draft is saved.
	path := filepath.Join(t.TempDir(), "settings.ini")
	st, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a.settings = st
	a.saveSettings()
	a.applyGridDraft(settingsDraft{layout: "Fixed", rows: "2", cols: "2"})
	a.saveSettings()
	reloaded, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Get("playlist/grid_rows", ""); got != "3" {
		t.Fatalf("declined apply wrote grid_rows=%q, want 3", got)
	}

	a.askOverflow = func(int, int) bool { return true }
	if ok := a.applyGridDraft(settingsDraft{layout: "Fixed", rows: "2", cols: "2"}); !ok {
		t.Fatal("an accepted smaller grid reported declined")
	}
	if len(a.cells) != 4 || a.cfg.Rows != 2 || a.cfg.Cols != 2 {
		t.Fatalf("accept kept cells=%d cfg=%+v", len(a.cells), a.cfg)
	}
}

func TestSettingsLayoutSwitchKeepsShowAllCells(t *testing.T) {
	for _, layout := range []string{"Auto Rows", "Auto Columns", "Fit Shapes", "Fixed", "Free Tiles", "Mosaic", "Single Mode"} {
		a := &app{cfg: grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3, Preallocate: true}}
		a.applyGridDraft(settingsDraft{layout: layout, rows: "3", cols: "3", showAllCells: true})
		if !a.cfg.Preallocate {
			t.Fatalf("%s cleared preallocate", layout)
		}
	}
	a := &app{cfg: grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3, Preallocate: true}}
	a.applyGridDraft(settingsDraft{layout: "Fixed", rows: "3", cols: "3", showAllCells: false})
	if a.cfg.Preallocate {
		t.Fatal("a cleared checkbox kept preallocate on")
	}
}

func TestShowAllCellsTurnsOffCleanly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.ini")
	st, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{settings: st, cfg: grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3, Preallocate: true}}
	a.togglePreallocate()
	if a.cfg.Preallocate {
		t.Fatal("preallocate did not toggle off")
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Get("playlist/grid_preallocate", ""); got != "false" {
		t.Fatalf("saved preallocate = %q, want false", got)
	}
	d := a.document()
	if _, _, _, preallocate := d.GridState(); preallocate {
		t.Fatal("the document kept preallocate after turning it off")
	}
	if raw, ok := d.Settings["grid_state"]; ok && strings.Contains(string(raw), "preallocate") {
		t.Fatalf("grid_state kept the key: %s", raw)
	}
}

func TestEmptyCellsFollowTheLayout(t *testing.T) {
	area := grid.Rect{W: 900, H: 600}
	a := &app{
		cfg:   grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3, Preallocate: true},
		cells: []*cell{{path: "a.mp4"}, {path: "b.mp4"}},
	}
	empties := a.emptyCellRects(area)
	if len(empties) != 7 {
		t.Fatalf("empty cells = %d, want 7", len(empties))
	}
	live := grid.Layout(2, area, cellGap, a.cfg)
	for _, e := range empties {
		for _, l := range live {
			if e.X < l.X+l.W && l.X < e.X+e.W && e.Y < l.Y+l.H && l.Y < e.Y+e.H {
				t.Fatalf("empty cell %+v covers a live cell %+v", e, l)
			}
		}
	}

	// A template's empty cells follow its places, not uniform slots.
	a = &app{
		cfg:      grid.Config{Mode: grid.Fixed, Rows: 4, Cols: 4, Preallocate: true},
		template: "1 + 12",
		cells:    []*cell{{path: "a.mp4"}},
	}
	template, ok := grid.FindTemplate("1 + 12")
	if !ok {
		t.Fatal("missing template 1 + 12")
	}
	places := template.Places()
	empties = a.emptyCellRects(area)
	if len(empties) != len(places)-1 {
		t.Fatalf("template empty cells = %d, want %d", len(empties), len(places)-1)
	}
	live = grid.TemplateLayout(1, area, cellGap, template)
	big := [4]float64{live[0].X, live[0].Y, live[0].X + live[0].W, live[0].Y + live[0].H}
	for _, e := range empties {
		if e.X < big[2] && big[0] < e.X+e.W && e.Y < big[3] && big[1] < e.Y+e.H {
			t.Fatalf("template empty cell %+v covers the big cell", e)
		}
	}

	// A free big place becomes one empty per position, as the frozen grid
	// draws them; the occupied big place is covered and stays empty-free.
	a = &app{
		cfg:      grid.Config{Mode: grid.Fixed, Rows: 4, Cols: 4, Preallocate: true},
		template: "2 + 8",
		cells:    []*cell{{path: "a.mp4"}},
	}
	template, ok = grid.FindTemplate("2 + 8")
	if !ok {
		t.Fatal("missing template 2 + 8")
	}
	empties = a.emptyCellRects(area)
	if len(empties) != 12 {
		t.Fatalf("2 + 8 empty cells = %d, want 12", len(empties))
	}
	unit := grid.CellRect(template, 0, 0, area, cellGap)
	for _, e := range empties {
		if math.Abs(e.W-unit.W) > 1e-9 || math.Abs(e.H-unit.H) > 1e-9 {
			t.Fatalf("empty cell %+v is not one position", e)
		}
	}
	live = grid.TemplateLayout(1, area, cellGap, template)
	big = [4]float64{live[0].X, live[0].Y, live[0].X + live[0].W, live[0].Y + live[0].H}
	for _, e := range empties {
		if e.X < big[2] && big[0] < e.X+e.W && e.Y < big[3] && big[1] < e.Y+e.H {
			t.Fatalf("2 + 8 empty cell %+v covers the occupied big cell", e)
		}
	}

	// No preallocate, no fixed grid, or no cells means no empty places.
	a = &app{cfg: grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3}, cells: []*cell{{path: "a.mp4"}}}
	if got := a.emptyCellRects(area); got != nil {
		t.Fatalf("empty cells without preallocate = %d", len(got))
	}
	a = &app{cfg: grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3, Preallocate: true}, single: true, cells: []*cell{{path: "a.mp4"}}}
	if got := a.emptyCellRects(area); got != nil {
		t.Fatalf("empty cells in single mode = %d", len(got))
	}
	a = &app{cfg: grid.Config{Mode: grid.Fixed, Rows: 3, Cols: 3, Preallocate: true}}
	if got := a.emptyCellRects(area); got != nil {
		t.Fatalf("empty cells with no videos = %d", len(got))
	}
}

func TestCellsMeetWithoutGap(t *testing.T) {
	area := grid.Rect{X: 10, Y: 20, W: 300, H: 200}
	const eps = 1e-9

	cells := grid.Layout(4, area, cellGap, grid.Config{Mode: grid.AutoRows})
	if len(cells) != 4 {
		t.Fatalf("grid cells = %d", len(cells))
	}
	if cells[0].X+cells[0].W != cells[1].X || cells[0].Y != cells[1].Y {
		t.Fatalf("horizontal neighbours do not meet: %+v %+v", cells[0], cells[1])
	}
	if cells[0].Y+cells[0].H != cells[2].Y || cells[0].X != cells[2].X {
		t.Fatalf("vertical neighbours do not meet: %+v %+v", cells[0], cells[2])
	}
	if cells[0].X != area.X || cells[0].Y != area.Y ||
		cells[3].X+cells[3].W != area.X+area.W || cells[3].Y+cells[3].H != area.Y+area.H {
		t.Fatalf("cells do not fill the area: %+v", cells)
	}

	tile := grid.Tile{Box: [4]float64{0, 0, 0.5, 1}}
	if got := tile.Rect(area, cellGap); got != (grid.Rect{X: area.X, Y: area.Y, W: 150, H: 200}) {
		t.Fatalf("free tile = %+v", got)
	}

	// Single mode letterboxes the strip tiles inside their band, as the
	// frozen strip does, so only the main pane and the band meet flush.
	share := 0.2
	single, _ := grid.SingleLayout(2, 0, area, cellGap, grid.StripRight, &share, 16.0/9.0)
	if len(single) != 2 {
		t.Fatalf("single mode cells = %d", len(single))
	}
	if single[0].H != area.H || single[0].X+single[0].W != single[1].X {
		t.Fatalf("main pane and strip do not meet: %+v %+v", single[0], single[1])
	}
	if single[1].X+single[1].W > area.X+area.W+eps {
		t.Fatalf("strip leaves the area: %+v", single[1])
	}

	template, ok := grid.FindTemplate("1 + 2")
	if !ok {
		t.Fatal("missing template 1 + 2")
	}
	rects := grid.TemplateLayout(3, area, cellGap, template)
	if len(rects) != 3 {
		t.Fatalf("template cells = %d", len(rects))
	}
	total := 0.0
	for _, r := range rects {
		total += r.W * r.H
	}
	if math.Abs(total-area.W*area.H) > eps {
		t.Fatalf("template leaves a gap: %v vs %v", total, area.W*area.H)
	}
}

func TestScreenshotSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.ini")
	st, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{settings: st}
	a.loadShotSettings(st)
	if a.shotDir != "" || a.shotTemplate != screenshots.DefaultTemplate || a.shotFormat != screenshots.PNG || a.shotQuality != screenshots.DefaultJPGQuality {
		t.Fatalf("defaults: dir=%q template=%q format=%v quality=%d", a.shotDir, a.shotTemplate, a.shotFormat, a.shotQuality)
	}
	a.shotDir = "/tmp/shots"
	a.shotTemplate = "%F_%n"
	a.shotFormat = screenshots.JPG
	a.shotQuality = 55
	a.saveShotSettings()
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b := &app{}
	b.loadShotSettings(reloaded)
	if b.shotDir != "/tmp/shots" || b.shotTemplate != "%F_%n" || b.shotFormat != screenshots.JPG || b.shotQuality != 55 {
		t.Fatalf("reloaded: dir=%q template=%q format=%v quality=%d", b.shotDir, b.shotTemplate, b.shotFormat, b.shotQuality)
	}
}

func TestTakeScreenshotWritesTheTemplateName(t *testing.T) {
	data := t.TempDir()
	eng := engine.New(func(string, bool, string) (engine.Backend, error) { return dummy.New(), nil })
	defer eng.Close()
	if err := eng.Load("movie.mkv"); err != nil {
		t.Fatal(err)
	}
	a := &app{
		dataDir:      data,
		shotTemplate: "%F_%n",
		shotFormat:   screenshots.PNG,
		shotQuality:  screenshots.DefaultJPGQuality,
		shotDir:      filepath.Join(data, "shots"),
	}
	c := &cell{path: "movie.mkv", title: "", eng: eng}
	a.takeScreenshot(c)
	first := filepath.Join(data, "shots", "movie_0001.png")
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("first shot: %v", err)
	}
	a.takeScreenshot(c)
	second := filepath.Join(data, "shots", "movie_0002.png")
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("second shot: %v", err)
	}

	// JPEG goes through the temporary PNG and leaves no .png behind.
	a.shotDir = filepath.Join(data, "jpeg")
	a.shotFormat = screenshots.JPG
	a.shotQuality = 55
	a.shotTemplate = "%F"
	a.takeScreenshot(c)
	jpg := filepath.Join(data, "jpeg", "movie.jpg")
	raw, err := os.ReadFile(jpg)
	if err != nil {
		t.Fatalf("jpeg shot: %v", err)
	}
	if len(raw) < 2 || raw[0] != 0xFF || raw[1] != 0xD8 {
		t.Fatalf("not a JPEG: %x", raw[:2])
	}
	if _, err := os.Stat(jpg + ".png"); err == nil {
		t.Fatal("the temporary PNG was left behind")
	}
}

func TestScreenshotLengthLimitsCountCharacters(t *testing.T) {
	long := strings.Repeat("题", 200)
	name := screenshots.Sanitize(long)
	// The frozen limits are literals: 150 characters of stem, 100 of title.
	if got := []rune(name); len(got) != 150 {
		t.Fatalf("stem runes = %d, want 150", len(got))
	}
	if got := []rune(screenshots.Render("%f", long, 0, time.Now(), 1)); len(got) != 100 {
		t.Fatalf("title runes = %d, want 100", len(got))
	}
	if got := screenshots.Render("%f", long, 0, time.Now(), 1); []rune(got)[0] != '题' {
		t.Fatalf("title cut: %q", got)
	}
	if got := screenshots.Render("%F", "x.abcdef", 0, time.Now(), 1); got != "x.abcdef" {
		t.Fatalf("6-letter extension was cut: %q", got)
	}
	if got := screenshots.Render("%F", "x.abcde", 0, time.Now(), 1); got != "x" {
		t.Fatalf("5-letter extension was kept: %q", got)
	}
}

func TestScreenshotCallSitesUseTheAction(t *testing.T) {
	// No UI harness exists for a click test, so the two call sites are
	// guarded at the source: a revert to the save dialog would show here.
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if got := strings.Count(text, "a.takeScreenshot("); got < 3 {
		t.Fatalf("takeScreenshot call sites = %d, want the Shot button, the shortcut and Screenshot All", got)
	}
	if !strings.Contains(text, "func (a *app) takeScreenshotAll()") {
		t.Fatal("the all-videos screenshot action is gone")
	}
	if got := strings.Count(text, "a.saveFrame("); got != 1 {
		t.Fatalf("saveFrame call sites = %d, want only the Save Frame menu item", got)
	}
	if !strings.Contains(text, `"take_screenshot"`) {
		t.Fatal("the take_screenshot shortcut is gone")
	}
	ui, err := os.ReadFile("settings_ui.go")
	if err != nil {
		t.Fatal(err)
	}
	// The Settings draft must map the four fields through one helper, and
	// saveSettings must persist them.
	if got := strings.Count(string(ui), "a.applyShotDraft(d)"); got != 1 {
		t.Fatalf("applyShotDraft call sites = %d, want 1", got)
	}
	if got := strings.Count(text, "a.saveShotSettings()"); got != 1 {
		t.Fatalf("saveShotSettings call sites = %d, want saveSettings", got)
	}
}

func TestApplyShotDraftMapsTheFields(t *testing.T) {
	a := &app{}
	a.applyShotDraft(settingsDraft{shotDir: "  /tmp/x  ", shotTemplate: " ", shotFormat: "JPG", shotQuality: "150"})
	if a.shotDir != "/tmp/x" || a.shotTemplate != screenshots.DefaultTemplate || a.shotFormat != screenshots.JPG || a.shotQuality != 100 {
		t.Fatalf("mapped: dir=%q template=%q format=%v quality=%d", a.shotDir, a.shotTemplate, a.shotFormat, a.shotQuality)
	}
	a.applyShotDraft(settingsDraft{shotTemplate: "%F", shotFormat: "png", shotQuality: "0"})
	if a.shotTemplate != "%F" || a.shotFormat != screenshots.PNG || a.shotQuality != 1 {
		t.Fatalf("mapped: template=%q format=%v quality=%d", a.shotTemplate, a.shotFormat, a.shotQuality)
	}
}

func TestSaveSettingsPersistsTheShotKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.ini")
	st, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{settings: st, shotDir: "/tmp/shots", shotTemplate: "%F_%n", shotFormat: screenshots.JPG, shotQuality: 42}
	a.saveSettings()
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Get("screenshots/format", "") != "jpg" || reloaded.Get("screenshots/dir", "") != "/tmp/shots" {
		t.Fatalf("saved keys: format=%q dir=%q", reloaded.Get("screenshots/format", ""), reloaded.Get("screenshots/dir", ""))
	}
}

func TestTakeScreenshotWithoutADataFolderReportsIt(t *testing.T) {
	eng := engine.New(func(string, bool, string) (engine.Backend, error) { return dummy.New(), nil })
	defer eng.Close()
	if err := eng.Load("movie.mkv"); err != nil {
		t.Fatal(err)
	}
	a := &app{shotTemplate: "%F"}
	a.takeScreenshot(&cell{path: "movie.mkv", eng: eng})
	if a.status != "screenshots: no data folder" {
		t.Fatalf("status = %q", a.status)
	}
}

func TestTakeScreenshotAllWritesOnePerCell(t *testing.T) {
	data := t.TempDir()
	newEng := func() *engine.Engine {
		eng := engine.New(func(string, bool, string) (engine.Backend, error) { return dummy.New(), nil })
		if err := eng.Load("movie.mkv"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(eng.Close)
		return eng
	}
	a := &app{
		dataDir:      data,
		shotDir:      filepath.Join(data, "shots"),
		shotTemplate: "%F",
		shotFormat:   screenshots.JPG,
		shotQuality:  55,
		cells: []*cell{
			{path: "a.mkv", eng: newEng()},
			{path: "b.mkv", eng: newEng()},
			{path: "no-engine.mkv"},
		},
	}
	a.takeScreenshotAll()
	for _, name := range []string{"a.jpg", "b.jpg"} {
		path := filepath.Join(data, "shots", name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(raw) < 2 || raw[0] != 0xFF || raw[1] != 0xD8 {
			t.Fatalf("%s is not a JPEG", name)
		}
	}
	if a.status != "saved 2 screenshots" {
		t.Fatalf("status = %q", a.status)
	}
	if _, err := os.Stat(filepath.Join(data, "shots", "no-engine.jpg")); err == nil {
		t.Fatal("a cell without an engine produced a file")
	}
}

func TestTakeScreenshotAllWithoutAVideoReportsIt(t *testing.T) {
	a := &app{dataDir: t.TempDir(), shotTemplate: "%F", cells: []*cell{{path: "x.mkv"}}}
	a.takeScreenshotAll()
	if a.status != "screenshots: no video to save" {
		t.Fatalf("status = %q", a.status)
	}
}
