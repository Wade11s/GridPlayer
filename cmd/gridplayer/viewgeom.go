package main

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

// picturePlace is the placed rectangle with where its corners sit in the
// cell. Scale zooms about the anchor, and the shift moves it on from there,
// clamped so the picture cannot be pushed entirely off the cell.
func placePicture(video videoShape, cellW, cellH float64, fill, stretch bool, v viewZoom) picturePlace {
	vw, vh := cellW, cellH
	if !stretch {
		if video.W > 0 && video.H > 0 {
			picAspect := video.W / video.H
			cellAspect := cellW / cellH
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
			if v.Scale > 0 {
				vw *= v.Scale
				vh *= v.Scale
			}
		}
	}
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
	return picturePlace{X: x, Y: y, W: vw, H: vh}
}

// clampZoom clamps a zoom to the range the app offers: 0.25x to 4x.
func clampZoom(scale float64) float64 {
	if scale < 0.25 {
		return 0.25
	}
	if scale > 4 {
		return 4
	}
	return scale
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
