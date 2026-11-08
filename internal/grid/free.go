package grid

// Tile is a video's box as shares of the page (left, top, right, bottom).
type Tile struct {
	Box [4]float64
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
