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
	got := SnapMove(tiles, 0, 1000, 1000, 8)
	if math.Abs(got[0].Box[2]-0.5) > 1e-9 || math.Abs(got[0].Box[0]-0.3) > 1e-9 {
		t.Fatalf("edge snap: %+v", got[0].Box)
	}
}

func TestSnapMoveToPageMiddle(t *testing.T) {
	tiles := []Tile{{Box: [4]float64{0.4, 0.1, 0.59, 0.3}}}
	got := SnapMove(tiles, 0, 1000, 1000, 8)
	if math.Abs((got[0].Box[0]+got[0].Box[2])/2-0.5) > 1e-9 {
		t.Fatalf("middle snap: %+v", got[0].Box)
	}
}

func TestSnapResizeToTileEdge(t *testing.T) {
	tiles := []Tile{
		{Box: [4]float64{0.1, 0.1, 0.495, 0.3}},
		{Box: [4]float64{0.5, 0, 0.75, 0.5}},
	}
	got := SnapResize(tiles, 0, 1000, 1000, 8)
	if math.Abs(got[0].Box[2]-0.5) > 1e-9 {
		t.Fatalf("resize snap: %+v", got[0].Box)
	}
	if got[0].Box[0] != 0.1 || got[0].Box[3] != 0.3 {
		t.Fatalf("resize moved other edges: %+v", got[0].Box)
	}
}

func TestSnapResizeKeepsMinimum(t *testing.T) {
	tiles := []Tile{{Box: [4]float64{0.5, 0.5, 0.6, 0.6}}}
	got := SnapResize(tiles, 0, 1000, 1000, 1000)
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
