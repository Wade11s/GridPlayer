package main

import (
	"math"
	"strconv"
)

// Aspect of a source video, in the same arbitrary units as a cell size.
type videoShape struct {
	W, H float64
}

// picturePlace is a placed rectangle in cell coordinates.
type picturePlace struct {
	X, Y, W, H float64
}

// viewZoom is a picture zoom with the anchor and shift it is about.
type viewZoom struct {
	Scale float64
	Crop  cropMargins
	// FillLimit is the most of the picture Fill may cut, in percent.
	// 100 means no limit.
	FillLimit int
	// AnchorX and AnchorY locate the anchor in the cell: 0..1 fraction.
	AnchorX, AnchorY float64
	// ShiftX and ShiftY move the picture in cell fractions.
	ShiftX, ShiftY float64
}

var (
	anchorNames = map[string][2]float64{
		"center":       {0.5, 0.5},
		"top":          {0.5, 0},
		"bottom":       {0.5, 1},
		"left":         {0, 0.5},
		"right":        {1, 0.5},
		"top-left":     {0, 0},
		"top-right":    {1, 0},
		"bottom-left":  {0, 1},
		"bottom-right": {1, 1},
	}
	anchorList = []string{
		"center", "top", "bottom", "left", "right",
		"top-left", "top-right", "bottom-left", "bottom-right",
	}
)

var transforms = map[string]bool{
	"none": true, "90": true, "180": true, "270": true,
	"hflip": true, "vflip": true, "transpose": true, "antitranspose": true,
}

func normalizeTransform(name string) string {
	if transforms[name] {
		return name
	}
	return "none"
}

func isSideTransform(name string) bool {
	switch name {
	case "90", "270", "transpose", "antitranspose":
		return true
	default:
		return false
	}
}

func transformCrop(m cropMargins, transform string) cropMargins {
	switch transform {
	case "90":
		return cropMargins{Left: m.Bottom, Top: m.Left, Right: m.Top, Bottom: m.Right}
	case "270":
		return cropMargins{Left: m.Top, Top: m.Right, Right: m.Bottom, Bottom: m.Left}
	case "transpose":
		return cropMargins{Left: m.Top, Top: m.Left, Right: m.Bottom, Bottom: m.Right}
	case "antitranspose":
		return cropMargins{Left: m.Bottom, Top: m.Right, Right: m.Top, Bottom: m.Left}
	case "hflip":
		return cropMargins{Left: m.Right, Top: m.Top, Right: m.Left, Bottom: m.Bottom}
	case "vflip":
		return cropMargins{Left: m.Left, Top: m.Bottom, Right: m.Right, Bottom: m.Top}
	default:
		return m
	}
}

// parseAnchor turns a stored anchor name into its cell fraction.
func parseAnchor(name string) (float64, float64) {
	if p, ok := anchorNames[name]; ok {
		return p[0], p[1]
	}
	return 0.5, 0.5
}

// anchorName renders a cell fraction back to a name.
func anchorName(x, y float64) string {
	for name, p := range anchorNames {
		if p[0] == x && p[1] == y {
			return name
		}
	}
	return "center"
}

// cropMargins cuts the picture by margins, in frame pixels, clamped to
// the video bounds.
type cropMargins struct {
	Left, Top, Right, Bottom float64
}

func (m cropMargins) empty() bool {
	return m.Left <= 0 && m.Top <= 0 && m.Right <= 0 && m.Bottom <= 0
}

func (m cropMargins) combined(other cropMargins) cropMargins {
	if !m.empty() {
		return m
	}
	return other
}

// minCropKeep is the fraction of a frame side that cropping always leaves
// visible, so a tile can never be cropped away entirely.
const minCropKeep = 0.05

// clampCropMargins keeps a cell's crop inside its own frame. Each axis must
// keep at least minCropKeep of the picture; when the requested margins exceed
// that, both sides are scaled back proportionally instead of one side winning.
func clampCropMargins(m cropMargins, video videoShape) cropMargins {
	m.Left = max(m.Left, 0)
	m.Top = max(m.Top, 0)
	m.Right = max(m.Right, 0)
	m.Bottom = max(m.Bottom, 0)
	if video.W <= 0 || video.H <= 0 {
		return m
	}
	if room := video.W - max(1, video.W*minCropKeep); room <= 0 {
		m.Left, m.Right = 0, 0
	} else if sum := m.Left + m.Right; sum > room {
		scale := room / sum
		m.Left *= scale
		m.Right *= scale
	}
	if room := video.H - max(1, video.H*minCropKeep); room <= 0 {
		m.Top, m.Bottom = 0, 0
	} else if sum := m.Top + m.Bottom; sum > room {
		scale := room / sum
		m.Top *= scale
		m.Bottom *= scale
	}
	return m
}

// cropMarginsList stores margins in .gpls order: left, top, right, bottom.
func cropMarginsList(m cropMargins) []float64 {
	if m.empty() {
		return nil
	}
	return []float64{m.Left, m.Top, m.Right, m.Bottom}
}

func cropMarginsFromList(values []float64) cropMargins {
	if len(values) != 4 {
		return cropMargins{}
	}
	return cropMargins{
		Left:   max(values[0], 0),
		Top:    max(values[1], 0),
		Right:  max(values[2], 0),
		Bottom: max(values[3], 0),
	}
}

// parseCropRatio turns a crop shape such as "2.35:1" into margins in
// frame pixels, taken evenly off the two sides it is too long between.
func parseCropRatio(ratio string, video videoShape) cropMargins {
	if ratio == "" || video.W <= 0 || video.H <= 0 {
		return cropMargins{}
	}
	want := 0.0
	if i := indexByteColon(ratio); i > 0 {
		w, err1 := strconv.ParseFloat(ratio[:i], 64)
		h, err2 := strconv.ParseFloat(ratio[i+1:], 64)
		if err1 == nil && err2 == nil && h > 0 {
			want = w / h
		}
	}
	if want <= 0 {
		return cropMargins{}
	}
	if video.W/video.H > want {
		kept := clamped(round(video.H*want), 1, video.W)
		left := math.Floor((video.W - kept) / 2)
		return cropMargins{Left: left, Right: video.W - kept - left}
	}
	kept := clamped(round(video.W/want), 1, video.H)
	top := math.Floor((video.H - kept) / 2)
	return cropMargins{Top: top, Bottom: video.H - kept - top}
}

func indexByteColon(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return i
		}
	}
	return -1
}

func round(v float64) float64 {
	if v < 0 {
		return -float64(int(-v + 0.5))
	}
	return float64(int(v + 0.5))
}

// cropRegion is the part of the frame that stays on show: the crop
// clamped to the video bounds, as calc_crop_region computes it.
func cropRegion(video videoShape, m cropMargins) (x, y, w, h float64) {
	if video.W <= 0 || video.H <= 0 {
		return 0, 0, video.W, video.H
	}
	x = clamped(m.Left, 0, max(video.W-1, 0))
	y = clamped(m.Top, 0, max(video.H-1, 0))
	w = max(video.W-m.Left-m.Right, 1)
	h = max(video.H-m.Top-m.Bottom, 1)
	return x, y, w, h
}

// picturePlace is the placed rectangle with where its corners sit in the
// cell. Scale zooms about the anchor, and the shift moves it on from there,
// clamped so the picture cannot be pushed entirely off the cell.
//
// The rectangle is the whole frame, not the cropped part: the crop is
// folded in by moving the frame so the kept region lands where it belongs.
// The cell clips the rest, which is how the hardware path avoids asking
// libVLC to crop as well.
func placePicture(video videoShape, cellW, cellH float64, fill, stretch bool, v viewZoom) picturePlace {
	rx, ry, rw, rh := cropRegion(video, v.Crop)
	if rw <= 0 || rh <= 0 || cellW <= 0 || cellH <= 0 {
		return picturePlace{W: cellW, H: cellH}
	}
	vw, vh := keptSize(rw, rh, cellW, cellH, fill, stretch, v.FillLimit, v.Scale)
	ax, ay := v.AnchorX, v.AnchorY
	if v.Scale <= 0 {
		ax, ay = 0.5, 0.5
	}
	if ax != 0.5 || ay != 0.5 {
		if ax < 0 || ax > 1 {
			ax = clampUnit(ax)
		}
		if ay < 0 || ay > 1 {
			ay = clampUnit(ay)
		}
	}
	x := (cellW-vw)*ax + clampShift(v.ShiftX*cellW, cellW, vw)
	y := (cellH-vh)*ay + clampShift(v.ShiftY*cellH, cellH, vh)
	// Scale the whole frame so the kept region is vw wide, then move the
	// frame so the kept region's top-left sits at (x, y).
	sx := vw / rw
	sy := vh / rh
	return picturePlace{
		X: x - rx*sx,
		Y: y - ry*sy,
		W: video.W * sx,
		H: video.H * sy,
	}
}

// keptSize sizes the cropped region inside the cell before the anchor and
// shift place it.
func keptSize(rw, rh, cellW, cellH float64, fill, stretch bool, fillLimit int, scale float64) (float64, float64) {
	// Size the kept region inside the cell.
	vw, vh := cellW, cellH
	if !stretch {
		picAspect := rw / rh
		cellAspect := cellW / cellH
		if fill && fillLimit < 100 {
			fitX := cellW / rw
			fitY := cellH / rh
			cut := 1 - math.Min(fitX, fitY)/math.Max(fitX, fitY)
			if cut*100 > float64(fillLimit)+1e-9 {
				fill = false
			}
		}
		if fill {
			if cellAspect > picAspect {
				vw, vh = cellW, cellW/picAspect
			} else {
				vh, vw = cellH, cellH*picAspect
			}
		} else {
			if cellAspect > picAspect {
				vh, vw = cellH, cellH*picAspect
			} else {
				vw, vh = cellW, cellW/picAspect
			}
		}
		if scale > 0 {
			vw *= scale
			vh *= scale
		}
	}
	return vw, vh
}

// clampZoom clamps a zoom to the range the app offers: 1x to 10x.
func clampZoom(scale float64) float64 {
	return clampFrozenZoom(scale)
}

// clampShift limits a shift to the room the picture has inside its cell.
// room = cell - picture. At the anchor place the picture sits between the
// pane edges; moving it one way fills the room on that side, and the other
// way the room on the other side. The shift is measured from the centre,
// so the allowed range is [-room/2, room/2].
func clampShift(shift, cell, picture float64) float64 {
	room := cell - picture
	lo, hi := -0.5*room, 0.5*room
	if lo > hi {
		lo, hi = hi, lo
	}
	return clampFloat(shift, lo, hi)
}

// clampUnit clamps a 0..1 fraction.
func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// clamped limits v to [lo, hi].
func clamped(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
