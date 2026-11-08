package grid

import "math"

// StripSide is where Single Mode's strip goes.
type StripSide uint8

const (
	StripAuto StripSide = iota
	StripRight
	StripLeft
	StripBottom
	StripTop
)

const (
	stripPerLine = 4
	stripMost    = 0.4
	stripLeast   = 0.05
)

// SingleLayout lays out n cells with `active` big and the others in a strip.
// Tiles use shape (width for height); nil share uses the upstream default.
// Returned rects are in original cell order, inset by `gap` between neighbors.
func SingleLayout(n, active int, area Rect, gap float64, side StripSide, share *float64, shape float64) ([]Rect, int) {
	if n <= 0 || area.W <= 0 || area.H <= 0 {
		return nil, -1
	}
	if active < 0 || active >= n {
		active = 0
	}
	if n == 1 {
		return []Rect{{X: area.X, Y: area.Y, W: area.W, H: area.H}}, 0
	}
	if shape <= 0 {
		shape = 16.0 / 9.0
	}
	isSide, isBefore := place(side, area.W, area.H)
	along, across := area.H, area.W
	if !isSide {
		along, across = area.W, area.H
	}
	count := n - 1
	thicknessForLength := shape
	if !isSide {
		thicknessForLength = 1 / shape
	}
	s := shareOf(share, count, along, across, thicknessForLength)
	thickness := s * across
	lines := lineCount(count, thickness, along, thicknessForLength)
	perLine := int(math.Ceil(float64(count) / float64(lines)))
	tileAcross := thickness / float64(lines)
	tileAlong := math.Min(tileAcross/thicknessForLength, along/float64(perLine))

	out := make([]Rect, n)
	big := box(isSide, isBefore, 0, 1, 0, 1-s, area)
	out[active] = big
	idx := 0
	for i := range out {
		if i == active {
			continue
		}
		line := idx / perLine
		place := idx % perLine
		per := perLine
		if rem := count - line*perLine; rem < per {
			per = rem
		}
		start := (along-float64(per)*tileAlong)/2 + float64(place)*tileAlong
		low := across - thickness + float64(line)*tileAcross
		out[i] = box(isSide, isBefore, start/along, (start+tileAlong)/along, low/across, (low+tileAcross)/across, area)
		idx++
	}
	if gap > 0 {
		for i := range out {
			out[i] = inset(out[i], gap/2)
		}
	}
	return out, active
}

func place(side StripSide, w, h float64) (isSide, isBefore bool) {
	switch side {
	case StripRight:
		return true, false
	case StripLeft:
		return true, true
	case StripBottom:
		return false, false
	case StripTop:
		return false, true
	default:
		if w >= h {
			return true, false
		}
		return false, false
	}
}

func shareOf(share *float64, count int, along, across, thForLen float64) float64 {
	s := 0.2
	if share != nil {
		s = *share
	} else {
		lines := max(int(math.Ceil(float64(count)/stripPerLine)), 1)
		thick := float64(lines) * along / stripPerLine * thForLen
		s = thick / across
	}
	return min(max(s, stripLeast), stripMost)
}

func lineCount(count int, thickness, along, thForLen float64) int {
	for lines := 1; lines <= count; lines++ {
		length := math.Ceil(float64(count)/float64(lines)) * thickness / float64(lines) / thForLen
		if length <= along*(1+1e-9) {
			return lines
		}
	}
	return max(count, 1)
}

// box maps along/across fractions into an area, flipping across when before.
func box(isSide, isBefore bool, alongLow, alongHigh, acrossLow, acrossHigh float64, area Rect) Rect {
	if isBefore {
		acrossLow, acrossHigh = 1-acrossHigh, 1-acrossLow
	}
	if isSide {
		return Rect{
			X: area.X + acrossLow*area.W, Y: area.Y + alongLow*area.H,
			W: (acrossHigh - acrossLow) * area.W, H: (alongHigh - alongLow) * area.H,
		}
	}
	return Rect{
		X: area.X + alongLow*area.W, Y: area.Y + acrossLow*area.H,
		W: (alongHigh - alongLow) * area.W, H: (acrossHigh - acrossLow) * area.H,
	}
}

func inset(r Rect, d float64) Rect {
	r.X += d
	r.Y += d
	r.W = max(r.W-2*d, 0)
	r.H = max(r.H-2*d, 0)
	return r
}
