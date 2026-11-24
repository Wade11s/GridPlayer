package grid

import (
	"math"
	"testing"
)

func TestFreeLayout(t *testing.T) {
	if FreeLayout(0) != nil {
		t.Fatal("zero")
	}
	for _, n := range []int{1, 2, 3, 4, 5, 8} {
		tiles := FreeLayout(n)
		if len(tiles) != n {
			t.Fatalf("n=%d len=%d", n, len(tiles))
		}
		for i, tl := range tiles {
			b := tl.Box
			if b[0] < -1e-9 || b[1] < -1e-9 || b[2] > 1+1e-9 || b[3] > 1+1e-9 {
				t.Fatalf("n=%d tile %d outside %+v", n, i, b)
			}
			if b[2] <= b[0] || b[3] <= b[1] {
				t.Fatalf("n=%d tile %d empty %+v", n, i, b)
			}
		}
		// no overlap for n <= 4
		if n <= 4 {
			for i := range tiles {
				for j := i + 1; j < len(tiles); j++ {
					a, b := tiles[i].Box, tiles[j].Box
					ox := math.Max(0, math.Min(a[2], b[2])-math.Max(a[0], b[0]))
					oy := math.Max(0, math.Min(a[3], b[3])-math.Max(a[1], b[1]))
					if ox > 1e-9 && oy > 1e-9 {
						t.Fatalf("n=%d tiles %d/%d overlap", n, i, j)
					}
				}
			}
		}
	}
}

func TestFreeTileRectGap(t *testing.T) {
	tiles := FreeLayout(4)
	r := tiles[1].Rect(Rect{W: 1280, H: 720}, 4)
	if r.X < 638 || r.X > 644 {
		t.Fatalf("gap rect %+v", r)
	}
}

func TestFreeOneTile(t *testing.T) {
	tiles := FreeLayout(1)
	if tiles[0].Box != [4]float64{0, 0, 1, 1} {
		t.Fatalf("%v", tiles[0].Box)
	}
}

func TestMoveTileClamps(t *testing.T) {
	tiles := MoveTile(FreeLayout(2), 0, -500, -100, 1280, 720)
	b := tiles[0].Box
	if b[0] < 0 || b[1] < 0 || b[2]-b[0] <= 0 || b[3]-b[1] <= 0 {
		t.Fatalf("moved out: %+v", b)
	}
}

func TestResizeTileMinAndBound(t *testing.T) {
	tiles := ResizeTile(FreeLayout(1), 0, 5000, -5000, 1280, 720)
	if tiles[0].Box[2] != 1 || tiles[0].Box[3] < 48.0/720 {
		t.Fatalf("resize out of bounds: %+v", tiles[0].Box)
	}
	orig := FreeLayout(2)
	if got := ResizeTile(orig, 9, 10, 10, 1280, 720); got[0].Box != orig[0].Box {
		t.Fatal("invalid resize changed tiles")
	}
}

func TestReadInRowsReadsTilesRowByRow(t *testing.T) {
	// Three tiles: two sharing the top band, one lower. Reading order is
	// left to right inside a band, bands top to bottom.
	tiles := []Tile{
		{Box: [4]float64{0.5, 0.02, 0.95, 0.3}}, // top right
		{Box: [4]float64{0.02, 0.02, 0.4, 0.3}}, // top left
		{Box: [4]float64{0.02, 0.6, 0.4, 0.95}}, // bottom left
	}
	if got := ReadInRows(tiles); len(got) != 3 || got[0] != 1 || got[1] != 0 || got[2] != 2 {
		t.Fatalf("read order = %v, want [1 0 2]", got)
	}

	// A tile that starts above the band's bottom joins the band even when
	// its top is slightly lower; one starting below it starts a new band.
	tiles = []Tile{
		{Box: [4]float64{0.02, 0.02, 0.3, 0.5}},
		{Box: [4]float64{0.4, 0.1, 0.7, 0.2}},
		{Box: [4]float64{0.02, 0.6, 0.3, 0.9}},
	}
	if got := ReadInRows(tiles); len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 2 {
		t.Fatalf("overlapping bands read = %v, want [0 1 2]", got)
	}

	// The row band's bottom shrinks to the shallower tile: a tile starting
	// below that new bottom opens a row even though it is inside the first
	// tile's own height.
	tiles = []Tile{
		{Box: [4]float64{0.02, 0.1, 0.4, 0.9}},
		{Box: [4]float64{0.5, 0.2, 0.9, 0.3}},
		{Box: [4]float64{0.02, 0.35, 0.4, 0.5}},
	}
	if got := ReadInRows(tiles); len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 2 {
		t.Fatalf("shrunk band read = %v, want [0 1 2]", got)
	}

	// Exactly at the band's bottom minus the frozen hair joins; below it
	// does not.
	base := 0.3
	tiles = []Tile{
		{Box: [4]float64{0.02, 0.02, 0.4, base}},
		{Box: [4]float64{0.5, base - 2e-6, 0.9, 0.5}},
		{Box: [4]float64{0.02, base - 0.5e-6, 0.4, 0.6}},
	}
	if got := ReadInRows(tiles); len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 2 {
		t.Fatalf("hair boundary read = %v, want tile 1 in the band and 2 out", got)
	}

	// Identical boxes keep their index order; a single tile reads alone.
	tiles = []Tile{
		{Box: [4]float64{0.1, 0.1, 0.2, 0.2}},
		{Box: [4]float64{0.1, 0.1, 0.2, 0.2}},
	}
	if got := ReadInRows(tiles); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("identical boxes read = %v, want [0 1]", got)
	}
	if got := ReadInRows([]Tile{{Box: [4]float64{0, 0, 1, 1}}}); len(got) != 1 || got[0] != 0 {
		t.Fatalf("single tile read = %v", got)
	}
	if got := ReadInRows(nil); got != nil {
		t.Fatalf("no tiles read = %v", got)
	}

	// The order decides which tiles a smaller grid closes last.
	tiles = []Tile{
		{Box: [4]float64{0.5, 0.6, 0.95, 0.95}},  // 0 bottom right
		{Box: [4]float64{0.02, 0.6, 0.4, 0.95}},  // 1 bottom left
		{Box: [4]float64{0.5, 0.32, 0.95, 0.58}}, // 2 middle
		{Box: [4]float64{0.55, 0.02, 0.95, 0.3}}, // 3 top right
		{Box: [4]float64{0.02, 0.02, 0.4, 0.3}},  // 4 top left
	}
	got := ReadInRows(tiles)
	want := []int{4, 3, 2, 1, 0}
	if len(got) != len(want) {
		t.Fatalf("read order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("read order = %v, want %v", got, want)
		}
	}
}
