package main

import (
	"slices"

	"gridplayer-go/internal/grid"
	"gridplayer-go/internal/nativehost"
)

func (a *app) resetFreeTiles() {
	a.freeTiles = grid.FreeLayout(len(a.cells))
	a.freeOrder = grid.DefaultOrder(len(a.cells))
	a.forgetDragOrigin()
}

// forgetDragOrigin drops the old-place target: a drag that did not see its
// end pass (a mode switch, a reset, a restore) must not leave it behind.
func (a *app) forgetDragOrigin() {
	a.moveIdx = 0
	a.snapOrigin = [4]float64{}
}

// layoutDrawOrder returns a validated back-to-front permutation. Geometry
// remains in cell order; only drawing/hit-testing order changes.
func (a *app) layoutDrawOrder() []int {
	n := len(a.cells)
	if a.mosaic {
		a.ensureMosaic()
		a.mosaicOrder = grid.Restack(a.mosaicOrder, n, a.mosaicLocked)
		return a.mosaicOrder
	}
	if a.free {
		a.freeOrder = grid.NormalizeOrder(a.freeOrder, n)
		return a.freeOrder
	}
	return grid.DefaultOrder(n)
}

func (a *app) activeStackState() (order []int, tiles []grid.Tile, locked map[int]bool, ok bool) {
	if a.active < 0 || a.active >= len(a.cells) {
		return nil, nil, nil, false
	}
	if a.mosaic {
		a.ensureMosaic()
		a.mosaicOrder = grid.Restack(a.mosaicOrder, len(a.cells), a.mosaicLocked)
		return a.mosaicOrder, a.mosaicTiles, a.mosaicLocked, true
	}
	if a.free {
		if len(a.freeTiles) != len(a.cells) {
			a.resetFreeTiles()
		}
		a.freeOrder = grid.NormalizeOrder(a.freeOrder, len(a.cells))
		return a.freeOrder, a.freeTiles, nil, true
	}
	return nil, nil, nil, false
}

func (a *app) canStackActive(front bool) bool {
	order, tiles, locked, ok := a.activeStackState()
	return ok && grid.CanStack(order, tiles, a.active, front, locked)
}

// syncNativeOrder makes the actual NSView clip stack match MyGo's draw and
// hit-test order. The clip-pointer signature also catches cell swaps and
// recreated native views, while avoiding AppKit reordering every frame.
func (a *app) syncNativeOrder(order []int) {
	if len(order) != len(a.cells) {
		return
	}
	clips := make([]uintptr, len(order))
	for at, index := range order {
		if index < 0 || index >= len(a.cells) || a.cells[index] == nil || a.cells[index].clip == 0 {
			return
		}
		clips[at] = a.cells[index].clip
	}
	if slices.Equal(clips, a.nativeClips) {
		return
	}
	if a.resolve == nil {
		return
	}
	cv, sv, _, ok := a.resolve()
	if !ok {
		return
	}
	nativehost.ReorderBelow(cv, sv, clips)
	a.nativeClips = clips
}

func (a *app) stackActive(front bool) {
	order, tiles, locked, ok := a.activeStackState()
	if !ok || !grid.CanStack(order, tiles, a.active, front, locked) {
		return
	}
	a.pushGridUndo()
	order = grid.Stack(order, a.active, front, locked)
	if a.mosaic {
		a.mosaicOrder = order
		a.saveSettings()
	} else {
		a.freeOrder = order
	}
	a.dirty = true
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}
