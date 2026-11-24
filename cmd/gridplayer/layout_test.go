package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"gridplayer-go/internal/grid"
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
		cells:  cells(5),
		active: 2,
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
	a.closeOverflow(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2})
	if len(a.cells) != 4 {
		t.Fatalf("cells = %d, want 4", len(a.cells))
	}
	want := []string{"1.mp4", "2.mp4", "3.mp4", "4.mp4"}
	for i, w := range want {
		if a.cells[i].path != w {
			t.Fatalf("kept %v, want %v", cellPaths(a.cells), want)
		}
	}
	// The active video was index 2 and is still itself after index 0 closed.
	if got := a.cells[a.active].path; got != "2.mp4" {
		t.Fatalf("active is %q, want 2.mp4", got)
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
