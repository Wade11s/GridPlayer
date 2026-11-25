package main

import (
	"testing"

	"gridplayer-go/internal/grid"
)

func freeApp(n int) *app {
	cells := make([]*cell, n)
	for i := range cells {
		cells[i] = &cell{path: "v.mp4"}
	}
	return &app{
		cells:     cells,
		free:      true,
		freeTiles: grid.FreeLayout(n),
		freeOrder: grid.DefaultOrder(n),
		cfg:       grid.Config{Mode: grid.AutoRows},
	}
}

func TestGridUndoRestoresTheFreeLayout(t *testing.T) {
	a := freeApp(3)
	before := a.captureGrid()
	a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2})
	if a.free || a.cfg.Mode != grid.Fixed {
		t.Fatalf("setGrid did not apply: free=%v cfg=%+v", a.free, a.cfg)
	}
	if len(a.gridUndo) != 1 {
		t.Fatalf("undo entries = %d, want 1", len(a.gridUndo))
	}
	a.undoGrid()
	if !a.free || a.cfg.Mode != grid.AutoRows {
		t.Fatalf("undo left free=%v cfg=%+v", a.free, a.cfg)
	}
	if len(a.freeTiles) != 3 || a.freeTiles[0].Box != before.freeTiles[0].Box {
		t.Fatalf("undo tiles = %v", a.freeTiles)
	}
	if len(a.gridUndo) != 0 || len(a.gridRedo) != 1 {
		t.Fatalf("stacks after undo: undo=%d redo=%d", len(a.gridUndo), len(a.gridRedo))
	}
}

func TestGridRedoReappliesTheChange(t *testing.T) {
	a := freeApp(3)
	a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2})
	a.undoGrid()
	a.redoGrid()
	if a.free || a.cfg.Mode != grid.Fixed {
		t.Fatalf("redo left free=%v cfg=%+v", a.free, a.cfg)
	}
	if len(a.gridRedo) != 0 || len(a.gridUndo) != 1 {
		t.Fatalf("stacks after redo: undo=%d redo=%d", len(a.gridUndo), len(a.gridRedo))
	}
}

func TestGridChangeDropsTheRedoTrail(t *testing.T) {
	a := freeApp(3)
	a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2})
	a.undoGrid()
	if len(a.gridRedo) != 1 {
		t.Fatalf("redo entries = %d", len(a.gridRedo))
	}
	a.setGrid(grid.Config{Mode: grid.AutoCols})
	if len(a.gridRedo) != 0 {
		t.Fatalf("a new change kept %d redo entries", len(a.gridRedo))
	}
}

func TestGridUndoSnapshotIsDeep(t *testing.T) {
	a := freeApp(2)
	snap := a.captureGrid()
	original := snap.freeTiles[0].Box
	a.freeTiles[0].Box = [4]float64{0.5, 0.5, 0.6, 0.6}
	locked := map[int]bool{0: true}
	a.mosaicLocked = locked
	locked[1] = true
	if snap.freeTiles[0].Box != original {
		t.Fatalf("the snapshot shares the tile slice: %v", snap.freeTiles[0].Box)
	}
	if len(snap.mosaicLocked) != 0 {
		t.Fatalf("the snapshot shares the lock map: %v", snap.mosaicLocked)
	}
}

func TestGridUndoLimit(t *testing.T) {
	a := freeApp(3)
	for i := 0; i < gridUndoLimit+8; i++ {
		a.pushGridUndo()
	}
	if len(a.gridUndo) != gridUndoLimit {
		t.Fatalf("undo entries = %d, want %d", len(a.gridUndo), gridUndoLimit)
	}
}

func TestGridUndoResetsTilesWhenCellsClosed(t *testing.T) {
	a := freeApp(3)
	snap := a.captureGrid()
	a.cells = a.cells[:2]
	a.restoreGrid(snap)
	if len(a.freeTiles) != 2 {
		t.Fatalf("tiles = %d for %d cells", len(a.freeTiles), len(a.cells))
	}
	if len(a.mosaicTiles) != 2 || len(a.mosaicLocked) != 0 {
		t.Fatalf("mosaic state = %d tiles, %d locks", len(a.mosaicTiles), len(a.mosaicLocked))
	}
}

func TestGridUndoIgnoresAnEmptyStack(t *testing.T) {
	a := freeApp(2)
	a.undoGrid()
	a.redoGrid()
	if a.cfg.Mode != grid.AutoRows || !a.free {
		t.Fatalf("empty stacks changed the layout: cfg=%+v free=%v", a.cfg, a.free)
	}
}
