package grid

import (
	"math"
	"testing"
)

func checkFitRects(t *testing.T, rects []Rect, area Rect, shapes []float64) {
	t.Helper()
	if shapes != nil && len(rects) != len(shapes) {
		t.Fatalf("rects=%d shapes=%d", len(rects), len(shapes))
	}
	for i, r := range rects {
		if r.W <= 0 || r.H <= 0 {
			t.Fatalf("rect %d empty: %+v", i, r)
		}
		if r.X < area.X-1e-6 || r.Y < area.Y-1e-6 ||
			r.X+r.W > area.X+area.W+1e-6 || r.Y+r.H > area.Y+area.H+1e-6 {
			t.Fatalf("rect %d outside area: %+v in %+v", i, r, area)
		}
		if shapes != nil && shapes[i] > 0 {
			got := r.W / r.H
			if math.Abs(got-shapes[i]) > 1e-6 {
				t.Fatalf("rect %d aspect %g, want %g (%+v)", i, got, shapes[i], r)
			}
		}
		for j := 0; j < i; j++ {
			o := rects[j]
			if r.X < o.X+o.W-1e-6 && o.X < r.X+r.W-1e-6 &&
				r.Y < o.Y+o.H-1e-6 && o.Y < r.Y+r.H-1e-6 {
				t.Fatalf("rects %d and %d overlap: %+v %+v", j, i, o, r)
			}
		}
	}
}

func TestFitShapesOneFillsArea(t *testing.T) {
	area := Rect{W: 800, H: 450}
	rects := FitShapes(1, area, 0, []float64{16.0 / 9.0})
	if len(rects) != 1 {
		t.Fatalf("rects %d", len(rects))
	}
	r := rects[0]
	if math.Abs(r.W-800) > 1e-6 || math.Abs(r.H-450) > 1e-6 {
		t.Fatalf("rect %+v", r)
	}
}

func TestFitShapesTallVideosSideBySide(t *testing.T) {
	area := Rect{W: 1920, H: 1080}
	shapes := []float64{9.0 / 16.0, 9.0 / 16.0, 9.0 / 16.0}
	rects := FitShapes(3, area, 0, shapes)
	checkFitRects(t, rects, area, shapes)
	for i := range rects {
		if math.Abs(rects[i].Y-rects[0].Y) > 1e-6 || math.Abs(rects[i].H-rects[0].H) > 1e-6 {
			t.Fatalf("tall videos are not in one row: %+v", rects)
		}
		if i > 0 && rects[i].X <= rects[i-1].X {
			t.Fatalf("tall videos not left to right: %+v", rects)
		}
	}
	// centred: the space left over at the sides is even
	if math.Abs(rects[0].X-(area.W-rects[2].X-rects[2].W)) > 1e-6 {
		t.Fatalf("not centred: %+v", rects)
	}
}

func TestFitShapesWideVideosStackInTallArea(t *testing.T) {
	area := Rect{W: 1080, H: 1920}
	shapes := []float64{16.0 / 9.0, 16.0 / 9.0, 16.0 / 9.0}
	rects := FitShapes(3, area, 0, shapes)
	checkFitRects(t, rects, area, shapes)
	for i := range rects {
		if math.Abs(rects[i].X-rects[0].X) > 1e-6 || math.Abs(rects[i].W-rects[0].W) > 1e-6 {
			t.Fatalf("wide videos are not in one column: %+v", rects)
		}
		if i > 0 && rects[i].Y <= rects[i-1].Y {
			t.Fatalf("wide videos not top to bottom: %+v", rects)
		}
	}
}

func TestFitShapesMixedKeepsEveryShape(t *testing.T) {
	area := Rect{W: 1920, H: 1080}
	shapes := []float64{16.0 / 9.0, 9.0 / 16.0, 16.0 / 9.0, 1, 9.0 / 16.0, 16.0 / 9.0, 4.0 / 3.0, 2.35}
	rects := FitShapes(len(shapes), area, 0, shapes)
	checkFitRects(t, rects, area, shapes)
	// Cells keep playlist order: cell 0 is the first rect.
	if math.Abs(rects[0].W/rects[0].H-16.0/9.0) > 1e-6 {
		t.Fatalf("first rect does not carry the first shape: %+v", rects[0])
	}
}

func TestFitShapesChoosesColumnsWhenTheyShowMore(t *testing.T) {
	area := Rect{W: 1920, H: 1080}
	mixed := []float64{9.0 / 16.0, 16.0 / 9.0, 9.0 / 16.0, 16.0 / 9.0, 16.0 / 9.0, 9.0 / 16.0, 16.0 / 9.0, 9.0 / 16.0}
	rects := FitShapes(len(mixed), area, 0, mixed)
	checkFitRects(t, rects, area, mixed)
	if math.Abs(rects[0].X-rects[1].X) > 1e-6 || math.Abs(rects[0].W-rects[1].W) > 1e-6 {
		t.Fatalf("expected a column layout, got %+v", rects)
	}
}

func TestFitShapesDefaultsUnknownShapes(t *testing.T) {
	area := Rect{W: 1280, H: 720}
	rects := FitShapes(4, area, 0, nil)
	if len(rects) != 4 {
		t.Fatalf("rects %d", len(rects))
	}
	for i, r := range rects {
		if math.Abs(r.W/r.H-16.0/9.0) > 1e-6 {
			t.Fatalf("rect %d not 16:9: %+v", i, r)
		}
	}
	// A zero/negative/NaN shape falls back too.
	rects = FitShapes(3, area, 0, []float64{0, -2, math.NaN()})
	for i, r := range rects {
		if math.Abs(r.W/r.H-16.0/9.0) > 1e-6 {
			t.Fatalf("fallback rect %d: %+v", i, r)
		}
	}
}

func TestFitShapesInsetByGap(t *testing.T) {
	area := Rect{W: 1920, H: 1080}
	shapes := []float64{16.0 / 9.0, 16.0 / 9.0}
	rects := FitShapes(2, area, 8, shapes)
	checkFitRects(t, rects, area, nil)
	for _, r := range rects {
		if r.X < 4-1e-6 || r.Y < 4-1e-6 || r.X+r.W > area.W-4+1e-6 || r.Y+r.H > area.H-4+1e-6 {
			t.Fatalf("gap not inset: %+v", r)
		}
	}
}
