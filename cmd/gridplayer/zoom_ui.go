package main

import (
	"math"

	"github.com/egoist/mygo/ui"

	"gridplayer-go/internal/engine"
	"gridplayer-go/internal/grid"
)

// cellShape returns the transposed video shape and the effective crop for the
// cell's current transform.
func (a *app) cellShape(c *cell, s engine.Snapshot) (videoShape, cropMargins) {
	shape := videoShape{W: float64(s.Width), H: float64(s.Height)}
	crop := c.cropMargins.combined(parseCropRatio(a.crop, shape))
	crop = transformCrop(crop, c.transform)
	if isSideTransform(c.transform) {
		shape.W, shape.H = shape.H, shape.W
	}
	return shape, crop
}

// zoomCell applies a multiplicative zoom. With zoom-to-cursor it moves the
// picture so the point under the cursor stays there; otherwise the existing
// anchor and shift are kept.
//
// The anchor is deliberately left alone. Retargeting it to the cursor and then
// clamping the shift is not the same problem: the clamp depends on the anchor,
// so a retargeted anchor can push a legal shift out of range and drift.
func (a *app) zoomCell(c *cell, r grid.Rect, s engine.Snapshot, factor, cursorX, cursorY float64) {
	if c == nil || s.Width <= 0 || s.Height <= 0 || r.W <= 0 || r.H <= 0 || factor <= 0 {
		return
	}
	current := c.zoom
	if current <= 0 {
		current = 1
	}
	next := clampZoom(current * factor)
	if math.Abs(next-current) < 1e-9 {
		return
	}
	if a.zoomToCursor {
		shape, crop := a.cellShape(c, s)
		_, _, rw, rh := cropRegion(shape, crop)
		if rw > 0 && rh > 0 {
			fill, stretch := viewModeFlags(a.cellViewMode(c))
			baseW, baseH := keptSize(rw, rh, r.W, r.H, fill, stretch, a.fillLimit, 1)
			oldW, oldH := baseW*current, baseH*current
			newW, newH := baseW*next, baseH*next
			oldX := (r.W-oldW)*c.anchorX + clampShift(c.shiftX*r.W, r.W, oldW)
			oldY := (r.H-oldH)*c.anchorY + clampShift(c.shiftY*r.H, r.H, oldH)
			u := (cursorX - oldX) / oldW
			v := (cursorY - oldY) / oldH
			c.shiftX = clampShift(cursorX-u*newW-(r.W-newW)*c.anchorX, r.W, newW) / r.W
			c.shiftY = clampShift(cursorY-v*newH-(r.H-newH)*c.anchorY, r.H, newH) / r.H
		}
	}
	c.zoom = next
	if a.win != nil {
		a.win.Invalidate()
	}
}

// zoomInput takes wheel and pinch zoom over a cell. It reports whether the
// event was used.
func (a *app) zoomInput(i int, ev ui.InputEvent, r grid.Rect, s engine.Snapshot) bool {
	if i < 0 || i >= len(a.cells) {
		return false
	}
	switch ev.Kind {
	case ui.InputZoom:
		if ev.Scale <= 0 || math.Abs(float64(ev.Scale)-1) < 1e-6 {
			return false
		}
		a.zoomCell(a.cells[i], r, s, float64(ev.Scale), float64(ev.X), float64(ev.Y))
		return true
	case ui.InputScroll:
		if ev.Mods&(ui.Ctrl|ui.Super) == 0 || ev.DY == 0 {
			return false
		}
		a.zoomCell(a.cells[i], r, s, math.Exp(-float64(ev.DY)*0.02), float64(ev.X), float64(ev.Y))
		return true
	}
	return false
}
