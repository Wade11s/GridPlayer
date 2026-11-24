package main

import (
	"math"
	"testing"

	"gridplayer-go/internal/grid"
)

// The frozen CellLayout sets spacing 0 and every Box shares its edge with
// its neighbour, so each Go layout must hand back rects that meet exactly.
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
