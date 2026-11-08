package grid

import "math"

// Mosaic returns a tiered arrangement: the first tiles form a larger row /
// column tier of similar boxes, like Single Mode's strip flattened. The
// first tile is the largest; the rest are equal tiles in a right-hand
// "tier" column. This is the initial arrange step; locked tiles snap to
// their saved boxes in later increments.
func Mosaic(n int) []Tile {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []Tile{{Box: [4]float64{0, 0, 1, 1}}}
	}
	if n == 2 {
		return []Tile{
			{Box: [4]float64{0, 0, 0.5, 1}},
			{Box: [4]float64{0.5, 0, 1, 1}},
		}
	}
	out := make([]Tile, n)
	big := halfOrThird(n)
	out[0] = Tile{Box: [4]float64{0, 0, big, 1}}
	rest := n - 1
	rows := int(math.Ceil(math.Sqrt(float64(rest))))
	cols := int(math.Ceil(float64(rest) / float64(rows)))
	for i := 0; i < rest; i++ {
		r := i / cols
		c := i % cols
		x0 := big + float64(c)*(1-big)/float64(cols)
		x1 := big + float64(c+1)*(1-big)/float64(cols)
		y0 := float64(r) / float64(rows)
		y1 := float64(r+1) / float64(rows)
		out[i+1] = Tile{Box: [4]float64{x0, y0, x1, y1}}
	}
	return out
}

// halfOrThird sizes the big tile at half the page for few tiles, a third
// for many, keeping rest tiles usable.
func halfOrThird(n int) float64 {
	if n <= 4 {
		return 0.5
	}
	return 1.0 / 3.0
}
