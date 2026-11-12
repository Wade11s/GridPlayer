package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"

	"gridplayer-go/internal/engine"
	"gridplayer-go/internal/grid"
)

func TestZoomCellClampsToFrozenRange(t *testing.T) {
	a := &app{fillLimit: 100}
	c := &cell{viewMode: "fit", zoom: 1}
	r := grid.Rect{W: 800, H: 450}
	s := engine.Snapshot{Width: 1920, Height: 1080}

	a.zoomCell(c, r, s, 100, 0, 0)
	if c.zoom != maxFrozenZoom {
		t.Fatalf("zoom in %v, want %v", c.zoom, maxFrozenZoom)
	}
	a.zoomCell(c, r, s, 0.0001, 0, 0)
	if c.zoom != minFrozenZoom {
		t.Fatalf("zoom out %v, want %v", c.zoom, minFrozenZoom)
	}
}

func TestZoomCellToCursorKeepsPointUnderCursor(t *testing.T) {
	a := &app{fillLimit: 100, zoomToCursor: true}
	c := &cell{viewMode: "fit", zoom: 1, anchorX: 0.5, anchorY: 0.5}
	r := grid.Rect{W: 800, H: 450}
	s := engine.Snapshot{Width: 1920, Height: 1080}
	shape := videoShape{W: 1920, H: 1080}

	cursorX, cursorY := 600.0, 300.0
	before := placePicture(shape, r.W, r.H, false, false, viewZoom{
		Scale: c.zoom, AnchorX: c.anchorX, AnchorY: c.anchorY, ShiftX: c.shiftX, ShiftY: c.shiftY,
		FillLimit: a.fillLimit,
	})
	a.zoomCell(c, r, s, 2, cursorX, cursorY)
	after := placePicture(shape, r.W, r.H, false, false, viewZoom{
		Scale: c.zoom, AnchorX: c.anchorX, AnchorY: c.anchorY, ShiftX: c.shiftX, ShiftY: c.shiftY,
		FillLimit: a.fillLimit,
	})

	uBefore := (cursorX - before.X) / before.W
	uAfter := (cursorX - after.X) / after.W
	if math.Abs(uBefore-uAfter) > 1e-6 {
		t.Fatalf("x fraction moved from %v to %v", uBefore, uAfter)
	}
	vBefore := (cursorY - before.Y) / before.H
	vAfter := (cursorY - after.Y) / after.H
	if math.Abs(vBefore-vAfter) > 1e-6 {
		t.Fatalf("y fraction moved from %v to %v", vBefore, vAfter)
	}
	if c.zoom != 2 {
		t.Fatalf("zoom %v", c.zoom)
	}
}

func TestZoomCellToCursorKeepsPointAtClampBoundary(t *testing.T) {
	cases := []struct {
		name    string
		zoom    float64
		shiftX  float64
		shiftY  float64
		factor  float64
		cursorX float64
		cursorY float64
	}{
		{name: "corner out of a 1.5x fit", zoom: 1.5, factor: 0.8, cursorX: 0, cursorY: 0},
		{name: "off-centre at 4x", zoom: 4, factor: 1.1, cursorX: 20, cursorY: 225},
		{name: "panned at 2x", zoom: 2, shiftX: 0.1, shiftY: -0.1, factor: 0.9, cursorX: 700, cursorY: 40},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &app{fillLimit: 100, zoomToCursor: true}
			c := &cell{viewMode: "fit", zoom: tc.zoom, anchorX: 0.5, anchorY: 0.5,
				shiftX: tc.shiftX, shiftY: tc.shiftY}
			r := grid.Rect{W: 800, H: 450}
			s := engine.Snapshot{Width: 1920, Height: 1080}
			shape := videoShape{W: 1920, H: 1080}
			zoomOf := func() viewZoom {
				return viewZoom{Scale: c.zoom, AnchorX: c.anchorX, AnchorY: c.anchorY,
					ShiftX: c.shiftX, ShiftY: c.shiftY, FillLimit: a.fillLimit}
			}
			before := placePicture(shape, r.W, r.H, false, false, zoomOf())
			a.zoomCell(c, r, s, tc.factor, tc.cursorX, tc.cursorY)
			after := placePicture(shape, r.W, r.H, false, false, zoomOf())
			uBefore := (tc.cursorX - before.X) / before.W
			uAfter := (tc.cursorX - after.X) / after.W
			if math.Abs(uBefore-uAfter) > 1e-6 {
				t.Fatalf("x fraction moved from %v to %v (zoom %v -> %v)", uBefore, uAfter, before.W, after.W)
			}
			vBefore := (tc.cursorY - before.Y) / before.H
			vAfter := (tc.cursorY - after.Y) / after.H
			if math.Abs(vBefore-vAfter) > 1e-6 {
				t.Fatalf("y fraction moved from %v to %v", vBefore, vAfter)
			}
			if c.anchorX != 0.5 || c.anchorY != 0.5 {
				t.Fatalf("anchor moved to %v,%v", c.anchorX, c.anchorY)
			}
		})
	}
}

func TestZoomCellKeepsAnchorWhenCursorZoomOff(t *testing.T) {
	a := &app{fillLimit: 100}
	c := &cell{viewMode: "fit", zoom: 1, anchorX: 0.5, anchorY: 0.5}
	r := grid.Rect{W: 800, H: 450}
	s := engine.Snapshot{Width: 1920, Height: 1080}

	a.zoomCell(c, r, s, 2, 700, 400)
	if c.anchorX != 0.5 || c.anchorY != 0.5 {
		t.Fatalf("anchor moved to %v,%v", c.anchorX, c.anchorY)
	}
	if c.shiftX != 0 || c.shiftY != 0 {
		t.Fatalf("shift moved to %v,%v", c.shiftX, c.shiftY)
	}
}

func TestZoomInputIgnoresPlainScroll(t *testing.T) {
	a := &app{fillLimit: 100, cells: []*cell{{viewMode: "fit", zoom: 1}}}
	r := grid.Rect{W: 800, H: 450}
	s := engine.Snapshot{Width: 1920, Height: 1080}
	plain := ui.InputEvent{Kind: ui.InputScroll, DY: 40}
	if a.zoomInput(0, plain, r, s) {
		t.Fatal("plain scroll should not zoom")
	}
	if a.cells[0].zoom != 1 {
		t.Fatalf("zoom %v", a.cells[0].zoom)
	}
	zoomed := ui.InputEvent{Kind: ui.InputScroll, DY: -20, Mods: ui.Ctrl}
	if !a.zoomInput(0, zoomed, r, s) {
		t.Fatal("ctrl+scroll should zoom")
	}
	if a.cells[0].zoom <= 1 {
		t.Fatalf("zoom %v", a.cells[0].zoom)
	}
}
