package main

import (
	"math"
	"path/filepath"
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
