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
