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
		// A fixed grid keeps its columns and takes the rows the cells need;
		// the old Go-only rule grew the longer side.
		{5, Config{Mode: Fixed, Rows: 2, Cols: 2}, 3, 2},
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

func TestFixedShowsOnlyUsedCells(t *testing.T) {
	c := Config{Mode: Fixed, Rows: 3, Cols: 3}
	if rows, cols := Dims(2, c); rows != 1 || cols != 2 {
		t.Fatalf("two cells in a 3x3 grid = %dx%d, want 1x2", rows, cols)
	}
	if rows, cols := Dims(4, c); rows != 2 || cols != 3 {
		t.Fatalf("four cells in a 3x3 grid = %dx%d, want 2x3", rows, cols)
	}
	if rows, cols := Dims(1, c); rows != 1 || cols != 1 {
		t.Fatalf("one cell in a 3x3 grid = %dx%d, want 1x1", rows, cols)
	}
	rects := Layout(2, Rect{W: 300, H: 300}, 0, c)
	if len(rects) != 2 || rects[0].W != 150 || rects[0].H != 300 || rects[1].X != 150 {
		t.Fatalf("two cells in a 3x3 grid = %+v", rects)
	}
}

func TestFixedCapacityGrowsOnOverflow(t *testing.T) {
	c := Config{Mode: Fixed, Rows: 2, Cols: 2}
	if got := Capacity(5, c); got != 6 {
		t.Fatalf("capacity=%d", got)
	}
	if rows, cols := Dims(5, c); rows != 3 || cols != 2 {
		t.Fatalf("overflow dims = %dx%d, want 3x2", rows, cols)
	}
}
