package grid

import (
	"math"
	"testing"
)

func TestDims(t *testing.T) {
	cases := []struct {
		n          int
		cfg        Config
		rows, cols int
	}{
		{1, Config{Mode: AutoRows}, 1, 1},
		{2, Config{Mode: AutoRows}, 1, 2},
		{4, Config{Mode: AutoRows}, 2, 2},
		{5, Config{Mode: AutoRows}, 2, 3},
		{9, Config{Mode: AutoRows}, 3, 3},
		{5, Config{Mode: AutoCols}, 3, 2},
		{4, Config{Mode: Fixed, Rows: 2, Cols: 2}, 2, 2},
		{5, Config{Mode: Fixed, Rows: 2, Cols: 2}, 2, 3},
		{0, Config{Mode: AutoRows}, 0, 0},
	}
	for _, c := range cases {
		rows, cols := Dims(c.n, c.cfg)
		if rows != c.rows || cols != c.cols {
			t.Errorf("Dims(%d,%+v)=(%d,%d), want (%d,%d)", c.n, c.cfg, rows, cols, c.rows, c.cols)
		}
	}
}

func TestLayoutCoversAreaWithGaps(t *testing.T) {
	area := Rect{X: 10, Y: 20, W: 1000, H: 600}
	rects := Layout(4, area, 4, Config{Mode: AutoRows})
	if len(rects) != 4 {
		t.Fatalf("len=%d", len(rects))
	}
	if rects[0].X != 10 || rects[0].Y != 20 {
		t.Errorf("first rect %+v", rects[0])
	}
	if math.Abs(rects[1].W-rects[0].W) > 1e-9 || math.Abs(rects[2].H-rects[0].H) > 1e-9 {
		t.Errorf("cells differ: %+v", rects)
	}
	if got := rects[1].X + rects[1].W; math.Abs(got-(area.X+area.W)) > 1e-9 {
		t.Errorf("right edge %v", got)
	}
	if got := rects[2].Y + rects[2].H; math.Abs(got-(area.Y+area.H)) > 1e-9 {
		t.Errorf("bottom edge %v", got)
	}
	if rects[0].X+rects[0].W+4 != rects[1].X {
		t.Errorf("gap %v", rects[1].X-rects[0].X-rects[0].W)
	}
}

func TestLayoutPartialLastRow(t *testing.T) {
	rects := Layout(5, Rect{W: 300, H: 300}, 0, Config{Mode: AutoRows})
	if len(rects) != 5 {
		t.Fatalf("len=%d", len(rects))
	}
	// 5 cells -> 2 rows x 3 cols; the last row has two cells, both inside.
	for i, r := range rects {
		if r.X < 0 || r.Y < 0 || r.X+r.W > 300.0001 || r.Y+r.H > 300.0001 {
			t.Errorf("cell %d outside: %+v", i, r)
		}
	}
}

func TestLayoutRejectsEmpty(t *testing.T) {
	if Layout(4, Rect{}, 2, Config{Mode: AutoRows}) != nil {
		t.Fatal("empty area should produce no cells")
	}
	if Layout(0, Rect{W: 10, H: 10}, 0, Config{Mode: AutoRows}) != nil {
		t.Fatal("zero cells")
	}
}

func TestFixedCapacityGrows(t *testing.T) {
	c := Config{Mode: Fixed, Rows: 2, Cols: 2}
	if got := Capacity(5, c); got != 6 {
		t.Fatalf("capacity=%d", got)
	}
}
