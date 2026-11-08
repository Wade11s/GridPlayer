package grid

import (
	"math"
	"testing"
)

func TestMosaic(t *testing.T) {
	if Mosaic(0) != nil {
		t.Fatal("zero")
	}
	for _, n := range []int{1, 2, 3, 4, 5, 8, 12} {
		tiles := Mosaic(n)
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
		// big tile overlaps none of the rest
		big := tiles[0].Box
		for j := 1; j < len(tiles); j++ {
			o := tiles[j].Box
			ox := math.Max(0, math.Min(big[2], o[2])-math.Max(big[0], o[0]))
			oy := math.Max(0, math.Min(big[3], o[3])-math.Max(big[1], o[1]))
			if ox > 1e-9 && oy > 1e-9 {
				t.Fatalf("n=%d big overlaps %d", n, j)
			}
		}
	}
}

func TestMosaicBigSizes(t *testing.T) {
	if got := Mosaic(4)[0].Box[2]; got != 0.5 {
		t.Fatalf("n=4 big width %v", got)
	}
	if got := Mosaic(5)[0].Box[2]; got != 1.0/3.0 {
		t.Fatalf("n=5 big width %v", got)
	}
}

func TestMosaicRectGap(t *testing.T) {
	tiles := Mosaic(4)
	r := tiles[0].Rect(Rect{W: 1280, H: 720}, 4)
	if r.X < 1.9 || r.X > 2.1 || r.W <= 0 {
		t.Fatalf("rect %+v", r)
	}
}
