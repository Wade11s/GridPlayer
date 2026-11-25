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

// SnapMove moves tile i to the nearest enabled snap target within threshold
// DIPs, preserving its size. origin is the tile's box when the drag started,
// used by the old-place snap; zeros mean no origin.
func SnapMove(tiles []Tile, i int, areaW, areaH, threshold float64, kinds SnapKinds, origin [4]float64) []Tile {
	if i < 0 || i >= len(tiles) || areaW <= 0 || areaH <= 0 || threshold <= 0 || kinds == 0 {
		return tiles
	}
	out := append([]Tile(nil), tiles...)
	b := out[i].Box
	edgesX, edgesY, midsX, midsY := snapTargets(tiles, i, kinds, origin)
	// Edges snap to lines, the middle to middles: the frozen editor keeps
	// the two apart, so a neighbour's edge is not a middle target.
	if d, ok := nearestOf([]float64{b[0], b[2]}, edgesX, (b[0]+b[2])/2, midsX, threshold/areaW); ok {
		b[0] += d
		b[2] += d
	}
	if d, ok := nearestOf([]float64{b[1], b[3]}, edgesY, (b[1]+b[3])/2, midsY, threshold/areaH); ok {
		b[1] += d
		b[3] += d
	}
	clampBox(&b)
	out[i].Box = b
	return out
}

// SnapResize snaps only tile i's right and bottom edges within threshold
// DIPs. shape is the video's width/height, used by the video-shape snap; a
// non-positive shape leaves that snap out.
func SnapResize(tiles []Tile, i int, areaW, areaH, threshold float64, kinds SnapKinds, shape float64) []Tile {
	if i < 0 || i >= len(tiles) || areaW <= 0 || areaH <= 0 || threshold <= 0 || kinds == 0 {
		return tiles
	}
	out := append([]Tile(nil), tiles...)
	b := out[i].Box
	edgesX, edgesY, _, _ := snapTargets(tiles, i, kinds, [4]float64{})
	if d, ok := nearestDelta([]float64{b[2]}, edgesX, threshold/areaW); ok {
		x1 := b[2] + d
		if x1 > b[0]+64/areaW && x1 <= 1 {
			b[2] = x1
		}
	}
	if d, ok := nearestDelta([]float64{b[3]}, edgesY, threshold/areaH); ok {
		y1 := b[3] + d
		if y1 > b[1]+48/areaH && y1 <= 1 {
			b[3] = y1
		}
	}
	if kinds.Has(SnapVideoShape) && shape > 0 && areaW > 0 && areaH > 0 {
		// The width that gives the box the video's shape in pixels.
		want := (b[3] - b[1]) * areaH * shape / areaW
		if want > 64/areaW && b[0]+want <= 1 && absf(b[0]+want-b[2]) <= threshold/areaW {
			b[2] = b[0] + want
		}
	}
	out[i].Box = b
	return out
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// snapTargets is where an edge or middle can snap to, for the kinds that are
// on. Lines take the page's edges and middle, the neighbours' edges and the
// old place's edges; middles take the page's and neighbours' middles.
func snapTargets(tiles []Tile, i int, kinds SnapKinds, origin [4]float64) (edgeX, edgeY, midX, midY []float64) {
	if kinds.Has(SnapWindowEdges) {
		edgeX = append(edgeX, 0, 1)
		edgeY = append(edgeY, 0, 1)
	}
	if kinds.Has(SnapWindowMiddle) {
		edgeX = append(edgeX, 0.5)
		edgeY = append(edgeY, 0.5)
		midX = append(midX, 0.5)
		midY = append(midY, 0.5)
	}
	for j, t := range tiles {
		if j == i {
			continue
		}
		b := t.Box
		if kinds.Has(SnapTileEdges) {
			edgeX = append(edgeX, b[0], b[2])
			edgeY = append(edgeY, b[1], b[3])
		}
		if kinds.Has(SnapTileMiddles) {
			midX = append(midX, (b[0]+b[2])/2)
			midY = append(midY, (b[1]+b[3])/2)
		}
	}
	if kinds.Has(SnapOldPlace) && origin != ([4]float64{}) {
		edgeX = append(edgeX, origin[0], origin[2])
		edgeY = append(edgeY, origin[1], origin[3])
		midX = append(midX, (origin[0]+origin[2])/2)
		midY = append(midY, (origin[1]+origin[3])/2)
	}
	return edgeX, edgeY, midX, midY
}

// nearestOf picks the closer of an edge snap against lines and a middle snap
// against middles, so a kind never acts as the other's target.
func nearestOf(edges, lines []float64, middle float64, middles []float64, threshold float64) (float64, bool) {
	dEdge, okEdge := nearestDelta(edges, lines, threshold)
	dMid, okMid := nearestDelta([]float64{middle}, middles, threshold)
	switch {
	case okEdge && okMid:
		if absf(dMid) < absf(dEdge) {
			return dMid, true
		}
		return dEdge, true
	case okEdge:
		return dEdge, true
	case okMid:
		return dMid, true
	}
	return 0, false
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
