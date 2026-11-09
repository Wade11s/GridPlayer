package grid

// Tile is a video's box as shares of the page (left, top, right, bottom).
type Tile struct {
	Box [4]float64 `json:"box"`
}

// FreeLayout starts with an even auto-rows arrangement recorded as tiles.
// Tiles are in cell order; the last tile is on top (as upstream).
func FreeLayout(n int) []Tile {
	if n <= 0 {
		return nil
	}
	rects := Layout(n, Rect{W: 1, H: 1}, 0, Config{Mode: AutoRows})
	out := make([]Tile, len(rects))
	for i, r := range rects {
		out[i] = Tile{Box: [4]float64{r.X, r.Y, r.X + r.W, r.Y + r.H}}
	}
	return out
}

// Rect converts a tile's fractions into absolute DIPs, inset by the gap.
func (t Tile) Rect(area Rect, gap float64) Rect {
	r := Rect{
		X: area.X + t.Box[0]*area.W,
		Y: area.Y + t.Box[1]*area.H,
		W: (t.Box[2] - t.Box[0]) * area.W,
		H: (t.Box[3] - t.Box[1]) * area.H,
	}
	if gap > 0 {
		r = inset(r, gap/2)
	}
	return r
}

// MoveTile shifts tile i by dx/dy DIPs, keeping it inside the unit area.
func MoveTile(tiles []Tile, i int, dx, dy, areaW, areaH float64) []Tile {
	if i < 0 || i >= len(tiles) || areaW <= 0 || areaH <= 0 {
		return tiles
	}
	out := append([]Tile(nil), tiles...)
	b := out[i].Box
	b[0] += dx / areaW
	b[2] += dx / areaW
	b[1] += dy / areaH
	b[3] += dy / areaH
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
	out[i].Box = b
	return out
}

// ResizeTile resizes tile i's bottom-right corner by dx/dy DIPs, keeping a
// minimum 64x48 point size and staying inside the unit area.
func ResizeTile(tiles []Tile, i int, dx, dy, areaW, areaH float64) []Tile {
	if i < 0 || i >= len(tiles) || areaW <= 0 || areaH <= 0 {
		return tiles
	}
	out := append([]Tile(nil), tiles...)
	b := out[i].Box
	b[2] = min(max(b[2]+dx/areaW, b[0]+64/areaW), 1)
	b[3] = min(max(b[3]+dy/areaH, b[1]+48/areaH), 1)
	out[i].Box = b
	return out
}
