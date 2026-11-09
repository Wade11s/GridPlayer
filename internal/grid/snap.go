package grid

import "math"

// Valid reports whether t is a non-empty box inside the unit page.
func (t Tile) Valid() bool {
	b := t.Box
	return b[0] >= 0 && b[1] >= 0 && b[2] <= 1 && b[3] <= 1 &&
		b[2] > b[0] && b[3] > b[1]
}

// Overlaps reports whether tile i overlaps any other tile.
func Overlaps(tiles []Tile, i int) bool {
	return OverlapArea(tiles, i) > 1e-12
}

// OverlapArea returns the total area tile i shares with the other tiles.
func OverlapArea(tiles []Tile, i int) float64 {
	if i < 0 || i >= len(tiles) {
		return 0
	}
	total := 0.0
	for j, t := range tiles {
		if j == i {
			continue
		}
		total += boxesOverlapArea(tiles[i].Box, t.Box)
	}
	return total
}

func boxesOverlap(a, b [4]float64) bool {
	return boxesOverlapArea(a, b) > 1e-12
}

func boxesOverlapArea(a, b [4]float64) float64 {
	ox := math.Max(0, math.Min(a[2], b[2])-math.Max(a[0], b[0]))
	oy := math.Max(0, math.Min(a[3], b[3])-math.Max(a[1], b[1]))
	return ox * oy
}

// MoveTileNoOverlap moves tile i, trying the full delta then each axis, and
// rejects any step that increases its overlap with the other tiles.
func MoveTileNoOverlap(tiles []Tile, i int, dx, dy, areaW, areaH float64) []Tile {
	if i < 0 || i >= len(tiles) {
		return tiles
	}
	before := OverlapArea(tiles, i)
	for _, d := range [][2]float64{{dx, dy}, {dx, 0}, {0, dy}} {
		if d[0] == 0 && d[1] == 0 {
			continue
		}
		next := MoveTile(tiles, i, d[0], d[1], areaW, areaH)
		if OverlapArea(next, i) <= before+1e-12 {
			return next
		}
	}
	return tiles
}

// ResizeTileNoOverlap resizes tile i unless the result increases overlap.
func ResizeTileNoOverlap(tiles []Tile, i int, dx, dy, areaW, areaH float64) []Tile {
	if i < 0 || i >= len(tiles) {
		return tiles
	}
	before := OverlapArea(tiles, i)
	next := ResizeTile(tiles, i, dx, dy, areaW, areaH)
	if OverlapArea(next, i) <= before+1e-12 {
		return next
	}
	return tiles
}

// SnapMove moves tile i to the nearest page or tile edge/middle within
// threshold DIPs, preserving its size.
func SnapMove(tiles []Tile, i int, areaW, areaH, threshold float64) []Tile {
	if i < 0 || i >= len(tiles) || areaW <= 0 || areaH <= 0 || threshold <= 0 {
		return tiles
	}
	out := append([]Tile(nil), tiles...)
	b := out[i].Box
	xs, ys := snapTargets(tiles, i)
	if d, ok := nearestDelta([]float64{b[0], (b[0] + b[2]) / 2, b[2]}, xs, threshold/areaW); ok {
		b[0] += d
		b[2] += d
	}
	if d, ok := nearestDelta([]float64{b[1], (b[1] + b[3]) / 2, b[3]}, ys, threshold/areaH); ok {
		b[1] += d
		b[3] += d
	}
	clampBox(&b)
	out[i].Box = b
	return out
}

// SnapResize snaps only tile i's right and bottom edges within threshold DIPs.
func SnapResize(tiles []Tile, i int, areaW, areaH, threshold float64) []Tile {
	if i < 0 || i >= len(tiles) || areaW <= 0 || areaH <= 0 || threshold <= 0 {
		return tiles
	}
	out := append([]Tile(nil), tiles...)
	b := out[i].Box
	xs, ys := snapTargets(tiles, i)
	if d, ok := nearestDelta([]float64{b[2]}, xs, threshold/areaW); ok {
		x1 := b[2] + d
		if x1 > b[0]+64/areaW && x1 <= 1 {
			b[2] = x1
		}
	}
	if d, ok := nearestDelta([]float64{b[3]}, ys, threshold/areaH); ok {
		y1 := b[3] + d
		if y1 > b[1]+48/areaH && y1 <= 1 {
			b[3] = y1
		}
	}
	out[i].Box = b
	return out
}

func snapTargets(tiles []Tile, i int) (xs, ys []float64) {
	xs = []float64{0, 0.5, 1}
	ys = []float64{0, 0.5, 1}
	for j, t := range tiles {
		if j == i {
			continue
		}
		b := t.Box
		xs = append(xs, b[0], b[2], (b[0]+b[2])/2)
		ys = append(ys, b[1], b[3], (b[1]+b[3])/2)
	}
	return xs, ys
}

func nearestDelta(anchors, targets []float64, threshold float64) (float64, bool) {
	best := 0.0
	bestAbs := threshold + 1
	found := false
	for _, a := range anchors {
		for _, t := range targets {
			d := t - a
			if abs := math.Abs(d); abs <= threshold && abs < bestAbs {
				best, bestAbs, found = d, abs, true
			}
		}
	}
	return best, found
}

func clampBox(b *[4]float64) {
	if b[0] < 0 {
		b[2] -= b[0]
		b[0] = 0
	}
	if b[1] < 0 {
		b[3] -= b[1]
		b[1] = 0
	}
	if b[2] > 1 {
		b[0] -= b[2] - 1
		b[2] = 1
	}
	if b[3] > 1 {
		b[1] -= b[3] - 1
		b[3] = 1
	}
}
