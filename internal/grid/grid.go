// Package grid lays out equal cells for a row/column grid.
//
// Modes mirror GridPlayer's AUTO_ROWS, AUTO_COLS and FIXED. Shapes, free
// tiles and mosaic are later increments; the model is kept separate from
// the UI so each mode can add math and tests without touching AppKit.
package grid

import "math"

type Mode uint8

const (
	AutoRows Mode = iota
	AutoCols
	Fixed
)

// Config is the requested grid.
type Config struct {
	Mode Mode
	Rows int
	Cols int
}

// Rect is a DIP rectangle with a top-left origin.
type Rect struct{ X, Y, W, H float64 }

// Dims returns the rows and columns for n cells.
func Dims(n int, c Config) (rows, cols int) {
	if n <= 0 {
		return 0, 0
	}
	switch c.Mode {
	case Fixed:
		rows, cols = max(c.Rows, 1), max(c.Cols, 1)
		for rows*cols < n {
			// A fixed grid grows along its longer side, as the upstream
			// fixed layout does when videos exceed its cells.
			if cols <= rows {
				cols++
			} else {
				rows++
			}
		}
		return rows, cols
	case AutoCols:
		rows = int(math.Ceil(math.Sqrt(float64(n))))
		cols = int(math.Ceil(float64(n) / float64(rows)))
		return rows, cols
	default:
		cols = int(math.Ceil(math.Sqrt(float64(n))))
		rows = int(math.Ceil(float64(n) / float64(cols)))
		return rows, cols
	}
}

// Layout returns one rect per cell, row-major, inside area with gap between
// cells. Cells share the available space equally; the last row/column may
// be partly empty when n does not fill the grid.
func Layout(n int, area Rect, gap float64, c Config) []Rect {
	if n <= 0 || area.W <= 0 || area.H <= 0 {
		return nil
	}
	rows, cols := Dims(n, c)
	if rows == 0 || cols == 0 {
		return nil
	}
	gap = max(gap, 0)
	cellW := (area.W - gap*float64(cols-1)) / float64(cols)
	cellH := (area.H - gap*float64(rows-1)) / float64(rows)
	if cellW <= 0 || cellH <= 0 {
		return nil
	}
	out := make([]Rect, n)
	for i := range out {
		r, c := i/cols, i%cols
		out[i] = Rect{
			X: area.X + float64(c)*(cellW+gap),
			Y: area.Y + float64(r)*(cellH+gap),
			W: cellW,
			H: cellH,
		}
	}
	return out
}

// Capacity returns rows*cols for a fixed grid, or n for the automatic ones.
func Capacity(n int, c Config) int {
	rows, cols := Dims(n, c)
	return rows * cols
}
