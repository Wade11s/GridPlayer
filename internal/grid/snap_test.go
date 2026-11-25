package grid

import (
	"math"
	"testing"
)

func TestTileValid(t *testing.T) {
	valid := Tile{Box: [4]float64{0, 0, 0.5, 1}}
	if !valid.Valid() {
		t.Fatal("valid tile rejected")
	}
	for _, b := range [][4]float64{
		{-0.1, 0, 0.5, 1},
		{0, 0, 1.1, 1},
		{0.5, 0, 0.5, 1},
		{0, 0.6, 1, 0.4},
	} {
		if (Tile{Box: b}).Valid() {
			t.Fatalf("invalid tile accepted: %+v", b)
		}
	}
}

func TestSnapMoveToTileEdge(t *testing.T) {
	tiles := []Tile{
		{Box: [4]float64{0.295, 0.1, 0.495, 0.3}},
		{Box: [4]float64{0.5, 0, 0.75, 0.5}},
	}
	got := SnapMove(tiles, 0, 1000, 1000, 8, DefaultSnaps, [4]float64{})
	if math.Abs(got[0].Box[2]-0.5) > 1e-9 || math.Abs(got[0].Box[0]-0.3) > 1e-9 {
		t.Fatalf("edge snap: %+v", got[0].Box)
	}
}

func TestSnapMoveToPageMiddle(t *testing.T) {
	tiles := []Tile{{Box: [4]float64{0.4, 0.1, 0.59, 0.3}}}
	got := SnapMove(tiles, 0, 1000, 1000, 8, DefaultSnaps, [4]float64{})
	if math.Abs((got[0].Box[0]+got[0].Box[2])/2-0.5) > 1e-9 {
		t.Fatalf("middle snap: %+v", got[0].Box)
	}
}

func TestSnapResizeToTileEdge(t *testing.T) {
	tiles := []Tile{
		{Box: [4]float64{0.1, 0.1, 0.495, 0.3}},
		{Box: [4]float64{0.5, 0, 0.75, 0.5}},
	}
	got := SnapResize(tiles, 0, 1000, 1000, 8, DefaultSnaps, 0)
	if math.Abs(got[0].Box[2]-0.5) > 1e-9 {
		t.Fatalf("resize snap: %+v", got[0].Box)
	}
	if got[0].Box[0] != 0.1 || got[0].Box[3] != 0.3 {
		t.Fatalf("resize moved other edges: %+v", got[0].Box)
	}
}

func TestSnapResizeKeepsMinimum(t *testing.T) {
	tiles := []Tile{{Box: [4]float64{0.5, 0.5, 0.6, 0.6}}}
	got := SnapResize(tiles, 0, 1000, 1000, 1000, DefaultSnaps, 0)
	if got[0].Box[2] <= got[0].Box[0] || got[0].Box[3] <= got[0].Box[1] {
		t.Fatalf("collapsed: %+v", got[0].Box)
	}
}

func TestMoveTileNoOverlapRejects(t *testing.T) {
	tiles := []Tile{
		{Box: [4]float64{0, 0, 0.5, 0.5}},
		{Box: [4]float64{0.5, 0, 1, 0.5}},
	}
	got := MoveTileNoOverlap(tiles, 0, 10, 0, 1000, 1000)
	if got[0].Box != tiles[0].Box {
		t.Fatalf("overlap accepted: %+v", got[0].Box)
	}
}

func TestMoveTileNoOverlapSlides(t *testing.T) {
	tiles := []Tile{
		{Box: [4]float64{0, 0, 0.5, 0.5}},
		{Box: [4]float64{0.5, 0.5, 1, 1}},
	}
	got := MoveTileNoOverlap(tiles, 0, 10, 10, 1000, 1000)
	if math.Abs(got[0].Box[0]-0.01) > 1e-9 {
		t.Fatalf("x slide: %+v", got[0].Box)
	}
	if got[0].Box[1] != 0 {
		t.Fatalf("y should not move: %+v", got[0].Box)
	}
}

func TestResizeTileNoOverlap(t *testing.T) {
	tiles := []Tile{
		{Box: [4]float64{0, 0, 0.4, 0.5}},
		{Box: [4]float64{0.5, 0, 1, 0.5}},
	}
	ok := ResizeTileNoOverlap(tiles, 0, 20, 0, 1000, 1000)
	if math.Abs(ok[0].Box[2]-0.42) > 1e-9 {
		t.Fatalf("safe resize: %+v", ok[0].Box)
	}
	bad := ResizeTileNoOverlap(tiles, 0, 200, 0, 1000, 1000)
	if bad[0].Box != tiles[0].Box {
		t.Fatalf("overlap resize accepted: %+v", bad[0].Box)
	}
}

func TestOverlapsEdgeTouch(t *testing.T) {
	tiles := []Tile{
		{Box: [4]float64{0, 0, 0.5, 0.5}},
		{Box: [4]float64{0.5, 0.5, 1, 1}},
	}
	if Overlaps(tiles, 0) {
		t.Fatal("edge touch counted as overlap")
	}
	tiles[1].Box = [4]float64{0.49, 0.49, 1, 1}
	if !Overlaps(tiles, 0) {
		t.Fatal("overlap not detected")
	}
	if got := OverlapArea(tiles, 0); math.Abs(got-0.0001) > 1e-9 {
		t.Fatalf("overlap area %v", got)
	}
}

func TestSnapKindsParseAndName(t *testing.T) {
	kinds := ParseSnapKinds([]string{"window_edges", "old_place", "bogus"})
	if kinds != SnapKinds(SnapWindowEdges|SnapOldPlace) {
		t.Fatalf("parsed kinds = %v", kinds.Names())
	}
	if got := kinds.Names(); len(got) != 2 || got[0] != "window_edges" || got[1] != "old_place" {
		t.Fatalf("names = %v", got)
	}
	if !DefaultSnaps.Has(SnapVideoShape) || DefaultSnaps.Has(SnapEqualGaps) {
		t.Fatal("default kinds are wrong")
	}
	if DefaultSnaps.With(SnapEqualGaps, true).Has(SnapEqualGaps) != true ||
		DefaultSnaps.With(SnapTileEdges, false).Has(SnapTileEdges) != false {
		t.Fatal("With is wrong")
	}
}

func TestSnapMoveOnlyUsesTheEnabledKinds(t *testing.T) {
	// The tile's right edge is 8 DIPs from the page middle, which is a
	// window-middle target but not a neighbour edge.
	tiles := []Tile{{Box: [4]float64{0.4, 0.1, 0.505, 0.3}}}
	with := SnapMove(tiles, 0, 1000, 1000, 8, SnapKinds(SnapWindowMiddle), [4]float64{})
	if math.Abs(with[0].Box[0]-0.395) > 1e-9 || math.Abs(with[0].Box[2]-0.5) > 1e-9 {
		t.Fatalf("window middle did not snap the right edge to 0.5: %+v", with[0].Box)
	}
	without := SnapMove(tiles, 0, 1000, 1000, 8, SnapKinds(SnapTileEdges|SnapTileMiddles), [4]float64{})
	if without[0].Box != tiles[0].Box {
		t.Fatalf("an unrelated kind snapped: %+v", without[0].Box)
	}
	if none := SnapMove(tiles, 0, 1000, 1000, 8, 0, [4]float64{}); none[0].Box != tiles[0].Box {
		t.Fatalf("no kinds snapped: %+v", none[0].Box)
	}
}

func TestSnapMoveOldPlace(t *testing.T) {
	// The tile was at x 0.2..0.5 and is now 9 DIPs right of it.
	tiles := []Tile{{Box: [4]float64{0.209, 0.1, 0.5, 0.3}}}
	origin := [4]float64{0.2, 0.1, 0.49, 0.3}
	got := SnapMove(tiles, 0, 1000, 1000, 10, SnapKinds(SnapOldPlace), origin)
	if math.Abs(got[0].Box[0]-0.2) > 1e-9 {
		t.Fatalf("old place did not snap: %+v", got[0].Box)
	}
	if other := SnapMove(tiles, 0, 1000, 1000, 10, SnapKinds(SnapTileEdges), origin); other[0].Box != tiles[0].Box {
		t.Fatalf("old place snapped without its kind: %+v", other[0].Box)
	}
}

func TestSnapResizeToVideoShape(t *testing.T) {
	// A 16:9 video in a 1000x1000 page: the box's width for a given height is
	// (h*1000*16/9)/1000. Starting from h 0.1..0.4 (0.3 high) the wanted
	// width is 0.5333; the box is 3 DIPs short of it.
	tiles := []Tile{{Box: [4]float64{0.1, 0.1, 0.63, 0.4}}}
	got := SnapResize(tiles, 0, 1000, 1000, 8, SnapKinds(SnapVideoShape), 16.0/9.0)
	if math.Abs(got[0].Box[2]-(0.1+0.3*16.0/9.0)) > 1e-9 {
		t.Fatalf("shape snap = %+v", got[0].Box)
	}
	if other := SnapResize(tiles, 0, 1000, 1000, 8, SnapKinds(SnapTileEdges), 16.0/9.0); other[0].Box != tiles[0].Box {
		t.Fatalf("shape snapped without its kind: %+v", other[0].Box)
	}
}

func TestSnapKindsKeepEdgesAndMiddlesApart(t *testing.T) {
	// A neighbour whose middle is 3 DIPs from tile 0's middle, its edges far
	// enough not to act: only the middle kind may snap.
	tiles := []Tile{
		{Box: [4]float64{0.52, 0.1, 0.674, 0.3}}, // middle 0.597
		{Box: [4]float64{0.5, 0.5, 0.7, 0.7}},    // middle 0.6, edges 0.5 and 0.7
	}
	mid := SnapMove(tiles, 0, 1000, 1000, 8, SnapKinds(SnapTileMiddles), [4]float64{})
	if math.Abs((mid[0].Box[0]+mid[0].Box[2])/2-0.6) > 1e-9 {
		t.Fatalf("middle kind did not snap to the neighbour middle: %+v", mid[0].Box)
	}
	edges := SnapMove(tiles, 0, 1000, 1000, 8, SnapKinds(SnapTileEdges), [4]float64{})
	if edges[0].Box != tiles[0].Box {
		t.Fatalf("the edge kind acted as a middle target: %+v", edges[0].Box)
	}
}
