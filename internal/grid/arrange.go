package grid

import "math"

const arrangeEpsilon = 1e-9

// Arrange lays out n tiles inside the unit page while keeping locked tiles
// fixed. Locked boxes act as walls: the remaining area is cut around them and
// the unlocked tiles fill the free pieces. If the walls leave no usable free
// area, Arrange falls back to Mosaic(n), matching the upstream "arrange
// everything" fallback instead of producing an empty page.
func Arrange(n int, tiles []Tile, locked map[int]bool) []Tile {
	if n <= 0 {
		return nil
	}
	if len(tiles) != n {
		tiles = Mosaic(n)
	}
	if len(locked) == 0 {
		return Mosaic(n)
	}
	out := append([]Tile(nil), tiles...)
	walls := make([]Rect, 0, len(locked))
	for i := 0; i < n; i++ {
		if locked[i] {
			walls = append(walls, tileRect(out[i]))
		}
	}
	if len(walls) == n {
		return out
	}
	free := mergeRects(subtractWalls([]Rect{{W: 1, H: 1}}, walls))
	free = usableRects(free)
	if len(free) == 0 {
		return Mosaic(n)
	}
	unlocked := make([]int, 0, n-len(walls))
	for i := 0; i < n; i++ {
		if !locked[i] {
			unlocked = append(unlocked, i)
		}
	}
	counts := distribute(len(unlocked), free)
	at := 0
	for i, count := range counts {
		if count == 0 {
			continue
		}
		boxes := flowBoxes(count, free[i])
		for j := 0; j < count && at < len(unlocked); j++ {
			out[unlocked[at]] = Tile{Box: boxes[j]}
			at++
		}
	}
	return out
}

func tileRect(t Tile) Rect {
	return Rect{
		X: t.Box[0],
		Y: t.Box[1],
		W: t.Box[2] - t.Box[0],
		H: t.Box[3] - t.Box[1],
	}
}

func subtractWalls(rects []Rect, walls []Rect) []Rect {
	for _, w := range walls {
		next := make([]Rect, 0, len(rects)+3)
		for _, r := range rects {
			next = append(next, subtractRect(r, w)...)
		}
		rects = next
	}
	return rects
}

// subtractRect partitions r \ w into up to four disjoint rectangles.
func subtractRect(r, w Rect) []Rect {
	if w.X >= r.X+r.W-arrangeEpsilon || w.X+w.W <= r.X+arrangeEpsilon ||
		w.Y >= r.Y+r.H-arrangeEpsilon || w.Y+w.H <= r.Y+arrangeEpsilon {
		return []Rect{r}
	}
	out := make([]Rect, 0, 4)
	if w.X > r.X+arrangeEpsilon {
		out = append(out, Rect{X: r.X, Y: r.Y, W: w.X - r.X, H: r.H})
	}
	if w.X+w.W < r.X+r.W-arrangeEpsilon {
		out = append(out, Rect{X: w.X + w.W, Y: r.Y, W: r.X + r.W - (w.X + w.W), H: r.H})
	}
	x0 := math.Max(r.X, w.X)
	x1 := math.Min(r.X+r.W, w.X+w.W)
	if w.Y > r.Y+arrangeEpsilon {
		out = append(out, Rect{X: x0, Y: r.Y, W: x1 - x0, H: w.Y - r.Y})
	}
	if w.Y+w.H < r.Y+r.H-arrangeEpsilon {
		out = append(out, Rect{X: x0, Y: w.Y + w.H, W: x1 - x0, H: r.Y + r.H - (w.Y + w.H)})
	}
	return out
}

func mergeRects(rects []Rect) []Rect {
	changed := true
	for changed {
		changed = false
		for i := 0; i < len(rects); i++ {
			for j := i + 1; j < len(rects); j++ {
				if m, ok := mergePair(rects[i], rects[j]); ok {
					rects[i] = m
					rects = append(rects[:j], rects[j+1:]...)
					changed = true
					j--
				}
			}
		}
	}
	return rects
}

func mergePair(a, b Rect) (Rect, bool) {
	if near(a.Y, b.Y) && near(a.H, b.H) {
		if near(a.X+a.W, b.X) {
			return Rect{X: a.X, Y: a.Y, W: a.W + b.W, H: a.H}, true
		}
		if near(b.X+b.W, a.X) {
			return Rect{X: b.X, Y: b.Y, W: a.W + b.W, H: a.H}, true
		}
	}
	if near(a.X, b.X) && near(a.W, b.W) {
		if near(a.Y+a.H, b.Y) {
			return Rect{X: a.X, Y: a.Y, W: a.W, H: a.H + b.H}, true
		}
		if near(b.Y+b.H, a.Y) {
			return Rect{X: b.X, Y: b.Y, W: a.W, H: a.H + b.H}, true
		}
	}
	return Rect{}, false
}

func near(a, b float64) bool {
	return math.Abs(a-b) <= arrangeEpsilon
}

func usableRects(rects []Rect) []Rect {
	out := rects[:0]
	for _, r := range rects {
		if r.W > 0.01 && r.H > 0.01 {
			out = append(out, r)
		}
	}
	return out
}

// distribute splits m tiles across free rects, one per rect first, then by
// area with largest remainders. When m is smaller than the rect count, only
// the largest rects receive a tile.
func distribute(m int, rects []Rect) []int {
	counts := make([]int, len(rects))
	if m <= 0 || len(rects) == 0 {
		return counts
	}
	if m <= len(rects) {
		order := make([]int, len(rects))
		for i := range order {
			order[i] = i
		}
		sortByArea(order, rects)
		for i := 0; i < m; i++ {
			counts[order[i]] = 1
		}
		return counts
	}
	total := 0.0
	for _, r := range rects {
		total += r.W * r.H
	}
	for i := range counts {
		counts[i] = 1
	}
	remaining := m - len(rects)
	type share struct {
		index int
		frac  float64
	}
	shares := make([]share, len(rects))
	used := 0
	for i, r := range rects {
		exact := float64(remaining) * (r.W * r.H) / total
		whole := int(math.Floor(exact))
		counts[i] += whole
		used += whole
		shares[i] = share{index: i, frac: exact - float64(whole)}
	}
	left := remaining - used
	for i := 0; i < left; i++ {
		best := i
		for j := i + 1; j < len(shares); j++ {
			if shares[j].frac > shares[best].frac ||
				(shares[j].frac == shares[best].frac && shares[j].index < shares[best].index) {
				best = j
			}
		}
		shares[i], shares[best] = shares[best], shares[i]
		counts[shares[i].index]++
	}
	return counts
}

func sortByArea(order []int, rects []Rect) {
	for i := 0; i < len(order); i++ {
		best := i
		for j := i + 1; j < len(order); j++ {
			aj := rects[order[j]].W * rects[order[j]].H
			ab := rects[order[best]].W * rects[order[best]].H
			if aj > ab || (aj == ab && order[j] < order[best]) {
				best = j
			}
		}
		order[i], order[best] = order[best], order[i]
	}
}

// flowBoxes fills box with k justified rows: each row spans the full width,
// and the row count follows the box aspect ratio.
func flowBoxes(k int, box Rect) [][4]float64 {
	if k <= 0 {
		return nil
	}
	if k == 1 {
		return [][4]float64{{box.X, box.Y, box.X + box.W, box.Y + box.H}}
	}
	aspect := 1.0
	if box.H > 0 {
		aspect = box.W / box.H
	}
	cols := int(math.Round(math.Sqrt(float64(k) * aspect)))
	cols = max(1, min(cols, k))
	rows := (k + cols - 1) / cols
	cellH := box.H / float64(rows)
	out := make([][4]float64, 0, k)
	for r := 0; r < rows; r++ {
		count := min(cols, k-r*cols)
		cellW := box.W / float64(count)
		y0 := box.Y + float64(r)*cellH
		y1 := y0 + cellH
		for c := 0; c < count; c++ {
			x0 := box.X + float64(c)*cellW
			x1 := x0 + cellW
			out = append(out, [4]float64{x0, y0, x1, y1})
		}
	}
	return out
}
