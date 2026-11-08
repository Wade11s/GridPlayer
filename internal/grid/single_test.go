package grid

import (
	"math"
	"testing"
)

func TestSingleOneCell(t *testing.T) {
	rects, big := SingleLayout(1, 0, Rect{W: 100, H: 100}, 0, StripAuto, nil, 16.0/9.0)
	if big != 0 || len(rects) != 1 || rects[0].W != 100 {
		t.Fatalf("big=%d rects=%v", big, rects)
	}
}

func TestSingleWideAndTall(t *testing.T) {
	wide, big := SingleLayout(4, 1, Rect{W: 1280, H: 720}, 4, StripAuto, nil, 16.0/9.0)
	if big != 1 || len(wide) != 4 {
		t.Fatalf("big=%d len=%d", big, len(wide))
	}
	if wide[1].W < 700 { // big takes most of the width on a wide window
		t.Fatalf("wide big %+v", wide[1])
	}
	tall, _ := SingleLayout(4, 0, Rect{W: 480, H: 800}, 4, StripAuto, nil, 16.0/9.0)
	if tall[0].H < 400 { // big takes most of the height on a tall window
		t.Fatalf("tall big %+v", tall[0])
	}
}

func TestSingleSides(t *testing.T) {
	for _, side := range []StripSide{StripRight, StripLeft, StripBottom, StripTop} {
		rects, big := SingleLayout(3, 2, Rect{W: 800, H: 600}, 0, side, nil, 16.0/9.0)
		if big != 2 || len(rects) != 3 {
			t.Fatalf("side %v", side)
		}
		for i, r := range rects {
			if r.X < -1e-9 || r.Y < -1e-9 || r.X+r.W > 800+1e-6 || r.Y+r.H > 600+1e-6 {
				t.Fatalf("side %v rect %d %+v", side, i, r)
			}
		}
		// strip strips do not overlap the big rect
		b := rects[big]
		for i, r := range rects {
			if i == big {
				continue
			}
			ox := math.Max(0, math.Min(r.X+r.W, b.X+b.W)-math.Max(r.X, b.X))
			oy := math.Max(0, math.Min(r.Y+r.H, b.Y+b.H)-math.Max(r.Y, b.Y))
			if ox > 1 && oy > 1 {
				t.Fatalf("side %v strip overlaps big: %+v vs %+v", side, r, b)
			}
		}
	}
}

func TestSingleShareClamps(t *testing.T) {
	small := 0.001
	rects, _ := SingleLayout(9, 0, Rect{W: 1000, H: 500}, 0, StripAuto, &small, 16.0/9.0)
	// least share keeps strips non-empty but big stays dominant
	if rects[0].W < 550 {
		t.Fatalf("big too small: %+v", rects[0])
	}
}

func TestSingleInvalid(t *testing.T) {
	if got, big := SingleLayout(0, 0, Rect{W: 100, H: 100}, 0, StripAuto, nil, 0); got != nil || big != -1 {
		t.Fatalf("got=%v big=%d", got, big)
	}
}
