package grid

import (
	"math"
	"testing"
)

func TestArrangeNoLocks(t *testing.T) {
	if Arrange(0, nil, nil) != nil {
		t.Fatal("zero")
	}
	for _, n := range []int{1, 2, 3, 4, 5, 8} {
		got := Arrange(n, nil, nil)
		want := Mosaic(n)
		if len(got) != len(want) {
			t.Fatalf("n=%d len=%d", n, len(got))
		}
		for i := range want {
			if got[i].Box != want[i].Box {
				t.Fatalf("n=%d tile %d: %+v != %+v", n, i, got[i].Box, want[i].Box)
			}
		}
	}
}

func TestArrangeKeepsLocked(t *testing.T) {
	for _, n := range []int{2, 3, 4, 5, 8} {
		tiles := Mosaic(n)
		got := Arrange(n, tiles, map[int]bool{0: true})
		if len(got) != n {
			t.Fatalf("n=%d len=%d", n, len(got))
		}
		if got[0].Box != tiles[0].Box {
			t.Fatalf("n=%d locked moved: %+v != %+v", n, got[0].Box, tiles[0].Box)
		}
		for i := 1; i < n; i++ {
			b := got[i].Box
			if b[0] < -1e-9 || b[1] < -1e-9 || b[2] > 1+1e-9 || b[3] > 1+1e-9 {
				t.Fatalf("n=%d tile %d outside %+v", n, i, b)
			}
			if b[2] <= b[0] || b[3] <= b[1] {
				t.Fatalf("n=%d tile %d empty %+v", n, i, b)
			}
			if boxesOverlap(b, tiles[0].Box) {
				t.Fatalf("n=%d tile %d overlaps locked: %+v", n, i, b)
			}
		}
	}
}

func TestArrangeAllLocked(t *testing.T) {
	tiles := Mosaic(4)
	locked := map[int]bool{0: true, 1: true, 2: true, 3: true}
	got := Arrange(4, tiles, locked)
	for i := range tiles {
		if got[i].Box != tiles[i].Box {
			t.Fatalf("tile %d moved: %+v != %+v", i, got[i].Box, tiles[i].Box)
		}
	}
}

func TestArrangeNoRoomFallsBack(t *testing.T) {
	tiles := Mosaic(3)
	tiles[0].Box = [4]float64{0, 0, 1, 1}
	got := Arrange(3, tiles, map[int]bool{0: true})
	want := Mosaic(3)
	for i := range want {
		if got[i].Box != want[i].Box {
			t.Fatalf("fallback tile %d: %+v != %+v", i, got[i].Box, want[i].Box)
		}
	}
}

func TestArrangeFillsAroundLeftWall(t *testing.T) {
	tiles := Mosaic(5)
	tiles[0].Box = [4]float64{0, 0, 0.5, 1}
	got := Arrange(5, tiles, map[int]bool{0: true})
	if got[0].Box != tiles[0].Box {
		t.Fatalf("locked moved: %+v", got[0].Box)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Box[0] < 0.5-1e-9 {
			t.Fatalf("tile %d crossed the wall: %+v", i, got[i].Box)
		}
	}
}

func TestArrangeNoUnlockedOverlap(t *testing.T) {
	for _, n := range []int{4, 5, 8} {
		for _, lock := range []map[int]bool{{1: true}, {0: true, 2: true}} {
			tiles := Mosaic(n)
			got := Arrange(n, tiles, lock)
			for i := 0; i < n; i++ {
				if lock[i] {
					continue
				}
				for j := 0; j < n; j++ {
					if lock[j] && boxesOverlap(got[i].Box, got[j].Box) {
						t.Fatalf("n=%d lock=%v tile %d overlaps locked %d: %+v %+v",
							n, lock, i, j, got[i].Box, got[j].Box)
					}
				}
				for j := i + 1; j < n; j++ {
					if lock[j] {
						continue
					}
					if boxesOverlap(got[i].Box, got[j].Box) {
						t.Fatalf("n=%d lock=%v tiles %d/%d overlap: %+v %+v",
							n, lock, i, j, got[i].Box, got[j].Box)
					}
				}
			}
		}
	}
}

func TestArrangeDeterministic(t *testing.T) {
	tiles := Mosaic(8)
	lock := map[int]bool{1: true, 4: true}
	a := Arrange(8, tiles, lock)
	b := Arrange(8, tiles, lock)
	for i := range a {
		if a[i].Box != b[i].Box {
			t.Fatalf("tile %d differs: %+v != %+v", i, a[i].Box, b[i].Box)
		}
	}
}

func TestArrangeEverySingleLock(t *testing.T) {
	for n := 2; n <= 12; n++ {
		for locked := 0; locked < n; locked++ {
			lock := map[int]bool{locked: true}
			tiles := Mosaic(n)
			got := Arrange(n, tiles, lock)
			checkArrange(t, n, tiles, got, lock)
		}
	}
}

func TestArrangeDoubleLocks(t *testing.T) {
	for n := 4; n <= 10; n++ {
		for a := 0; a < n; a++ {
			for b := a + 1; b < n; b++ {
				lock := map[int]bool{a: true, b: true}
				tiles := Mosaic(n)
				got := Arrange(n, tiles, lock)
				checkArrange(t, n, tiles, got, lock)
			}
		}
	}
}

func checkArrange(t *testing.T, n int, before, got []Tile, lock map[int]bool) {
	t.Helper()
	if len(got) != n {
		t.Fatalf("n=%d lock=%v len=%d", n, lock, len(got))
	}
	for i := 0; i < n; i++ {
		b := got[i].Box
		if b[0] < -1e-9 || b[1] < -1e-9 || b[2] > 1+1e-9 || b[3] > 1+1e-9 {
			t.Fatalf("n=%d lock=%v tile %d outside %+v", n, lock, i, b)
		}
		if b[2] <= b[0] || b[3] <= b[1] {
			t.Fatalf("n=%d lock=%v tile %d empty %+v", n, lock, i, b)
		}
		if lock[i] && b != before[i].Box {
			t.Fatalf("n=%d lock=%v locked tile %d moved: %+v != %+v", n, lock, i, b, before[i].Box)
		}
		for j := i + 1; j < n; j++ {
			if boxesOverlap(b, got[j].Box) {
				t.Fatalf("n=%d lock=%v tiles %d/%d overlap: %+v %+v", n, lock, i, j, b, got[j].Box)
			}
		}
	}
	again := Arrange(n, before, lock)
	for i := range got {
		if got[i].Box != again[i].Box {
			t.Fatalf("n=%d lock=%v tile %d not deterministic", n, lock, i)
		}
	}
}

func boxesOverlap(a, b [4]float64) bool {
	ox := math.Max(0, math.Min(a[2], b[2])-math.Max(a[0], b[0]))
	oy := math.Max(0, math.Min(a[3], b[3])-math.Max(a[1], b[1]))
	return ox > 1e-9 && oy > 1e-9
}
