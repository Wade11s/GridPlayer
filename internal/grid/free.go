package grid

import (
	"math"
	"sort"
)

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

// tileHair is the frozen _HAIR: how far a tile may overlap a row's band
// and still count as part of it.
const tileHair = 1e-6

// ReadInRows returns the tile indices as the tiles are read, row by row: a
// tile joins the row of the one before it when it starts above that row's
// bottom, and each row reads left to right. It is the frozen read_in_rows,
// which decides the order a tiled grid closes videos in.
func ReadInRows(tiles []Tile) []int {
	if len(tiles) == 0 {
		return nil
	}
	order := make([]int, len(tiles))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := tiles[order[i]].Box, tiles[order[j]].Box
		if a[1] != b[1] {
			return a[1] < b[1]
		}
		return a[0] < b[0]
	})
	var rows [][]int
	rowBottom := 0.0
	for _, index := range order {
		box := tiles[index].Box
		if len(rows) > 0 && box[1] < rowBottom-tileHair {
			rows[len(rows)-1] = append(rows[len(rows)-1], index)
			rowBottom = math.Min(rowBottom, box[3])
			continue
		}
		rows = append(rows, []int{index})
		rowBottom = box[3]
	}
	out := make([]int, 0, len(tiles))
	for _, row := range rows {
		sort.SliceStable(row, func(i, j int) bool {
			return tiles[row[i]].Box[0] < tiles[row[j]].Box[0]
		})
		out = append(out, row...)
	}
	return out
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
