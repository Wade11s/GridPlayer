package main

import (
	"slices"
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

func TestGridUndoRestoresTheCellOrderWithTheTiles(t *testing.T) {
	a := &app{
		cells: []*cell{{path: "a.mp4"}, {path: "b.mp4"}, {path: "c.mp4"}},
		free:  true,
		cfg:   grid.Config{Mode: grid.AutoRows},
		freeTiles: []grid.Tile{
			{Box: [4]float64{0.5, 0.02, 0.95, 0.3}}, // a, top right
			{Box: [4]float64{0.5, 0.6, 0.95, 0.95}}, // b, bottom right
			{Box: [4]float64{0.02, 0.02, 0.4, 0.3}}, // c, top left
		},
	}
	// Remember which box belongs to which cell before the switch.
	owners := make(map[*cell][4]float64, 3)
	for i, c := range a.cells {
		owners[c] = a.freeTiles[i].Box
	}
	a.setGrid(grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 2})
	if a.free {
		t.Fatal("setGrid did not leave the free layout")
	}
	a.undoGrid()
	if !a.free || len(a.freeTiles) != 3 {
		t.Fatalf("undo free=%v tiles=%d", a.free, len(a.freeTiles))
	}
	for i, c := range a.cells {
		if a.freeTiles[i].Box != owners[c] {
			t.Fatalf("tile %d belongs to another video after undo: %v vs %v", i, a.freeTiles[i].Box, owners[c])
		}
	}
}

func TestDeclinedSettingsGridDoesNotPush(t *testing.T) {
	a := freeApp(5)
	a.pushGridUndo()
	a.undoGrid()
	if len(a.gridRedo) != 1 {
		t.Fatalf("setup redo=%d", len(a.gridRedo))
	}
	a.askOverflow = func(int, int) bool { return false }
	if ok := a.applyGridDraft(settingsDraft{layout: "Fixed", rows: "2", cols: "2"}); ok {
		t.Fatal("a declined grid reported applied")
	}
	if len(a.gridUndo) != 0 || len(a.gridRedo) != 1 {
		t.Fatalf("declined confirm changed the stacks: undo=%d redo=%d", len(a.gridUndo), len(a.gridRedo))
	}
}

func TestNoOpGridActionsDoNotPush(t *testing.T) {
	a := &app{
		cells: []*cell{{path: "a.mp4"}, {path: "b.mp4"}},
		cfg:   grid.Config{Mode: grid.AutoRows},
	}
	a.setGrid(grid.Config{Mode: grid.AutoRows})
	if len(a.gridUndo) != 0 {
		t.Fatalf("a no-op setGrid pushed %d entries", len(a.gridUndo))
	}
	a.mosaic = true
	a.mosaicLocked = map[int]bool{}
	a.unlockMosaic()
	if len(a.gridUndo) != 0 {
		t.Fatalf("unlock with no locks pushed %d entries", len(a.gridUndo))
	}
}

func TestClosingAVideoForgetsTheGridTrail(t *testing.T) {
	a := freeApp(3)
	a.pushGridUndo()
	if len(a.gridUndo) != 1 {
		t.Fatal("setup")
	}
	a.closeCell(&cell{path: "gone.mp4"})
	if len(a.gridUndo) != 0 || len(a.gridRedo) != 0 {
		t.Fatalf("a closed video kept the trail: undo=%d redo=%d", len(a.gridUndo), len(a.gridRedo))
	}
}

func TestSameTemplateDoesNotPush(t *testing.T) {
	a := &app{
		cells:       []*cell{{path: "a.mp4"}, {path: "b.mp4"}, {path: "c.mp4"}},
		cfg:         grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 3},
		template:    "1 + 2",
		askOverflow: func(int, int) bool { return true },
	}
	a.setTemplate("1 + 2")
	if len(a.gridUndo) != 0 {
		t.Fatalf("re-applying a template pushed %d entries", len(a.gridUndo))
	}
}

func TestArrangingAnArrangedMosaicDoesNotPush(t *testing.T) {
	cells := []*cell{{path: "a.mp4"}, {path: "b.mp4"}, {path: "c.mp4"}}
	a := &app{cells: cells, mosaic: true}
	a.ensureMosaic()
	a.mosaicTiles = grid.Arrange(len(cells), a.mosaicTiles, a.mosaicLocked)
	before := slices.Clone(a.mosaicTiles)
	a.arrangeMosaic()
	if len(a.gridUndo) != 0 {
		t.Fatalf("arranging an arranged mosaic pushed %d entries", len(a.gridUndo))
	}
	if !slices.Equal(a.mosaicTiles, before) {
		t.Fatal("arranging an arranged mosaic changed the tiles")
	}
}

func TestRestoreWithAnotherVideoSetResetsTheTiles(t *testing.T) {
	a := freeApp(2)
	// Boxes that differ from the default layout, so keeping the snapshot's
	// tiles (the pre-fix behaviour) is visible rather than coincidentally
	// equal to a fresh FreeLayout.
	a.freeTiles = []grid.Tile{
		{Box: [4]float64{0.2, 0.2, 0.5, 0.5}},
		{Box: [4]float64{0.55, 0.6, 0.9, 0.9}},
	}
	snap := a.captureGrid()
	// Same count, different videos: the snapshot's boxes must not be reused.
	a.cells = []*cell{{path: "x.mp4"}, {path: "y.mp4"}}
	a.restoreGrid(snap)
	want := grid.FreeLayout(2)
	for i := range want {
		if a.freeTiles[i].Box != want[i].Box {
			t.Fatalf("stale tiles survived with another video set: %v", a.freeTiles)
		}
	}
}

func TestSettingsTiledModeChangeIsUndoable(t *testing.T) {
	a := &app{
		cells: []*cell{{path: "a.mp4"}, {path: "b.mp4"}},
		cfg:   grid.Config{Mode: grid.AutoRows},
	}
	if ok := a.applyGridDraft(settingsDraft{layout: "Free Tiles"}); !ok {
		t.Fatal("a tiled layout reported declined")
	}
	if len(a.gridUndo) != 1 {
		t.Fatalf("a tiled mode change pushed %d entries", len(a.gridUndo))
	}
	a.undoGrid()
	if a.free {
		t.Fatal("undo did not leave Free Tiles")
	}
}

func TestSettingsClearingATemplateIsUndoable(t *testing.T) {
	a := &app{
		cells:    []*cell{{path: "a.mp4"}},
		cfg:      grid.Config{Mode: grid.Fixed, Rows: 2, Cols: 3},
		template: "1 + 2",
	}
	if ok := a.applyGridDraft(settingsDraft{layout: "Fixed", rows: "2", cols: "3"}); !ok {
		t.Fatal("the same fixed grid reported declined")
	}
	if a.template != "" {
		t.Fatalf("template = %q, want cleared", a.template)
	}
	if len(a.gridUndo) != 1 {
		t.Fatalf("clearing the template pushed %d entries", len(a.gridUndo))
	}
	a.undoGrid()
	if a.template != "1 + 2" {
		t.Fatalf("undo did not restore the template: %q", a.template)
	}
}

func TestGrowingThePlaylistForgetsTheGridTrail(t *testing.T) {
	a := &app{
		cells: []*cell{{path: "a.mp4"}},
		cfg:   grid.Config{Mode: grid.AutoRows},
	}
	a.pushGridUndo()
	if len(a.gridUndo) != 1 {
		t.Fatal("setup")
	}
	// A set that only grows closes no cell, so the close path cannot forget.
	a.setPlaylist([]string{"a.mp4", "b.mp4"})
	if len(a.gridUndo) != 0 || len(a.gridRedo) != 0 {
		t.Fatalf("a grown playlist kept the trail: undo=%d redo=%d", len(a.gridUndo), len(a.gridRedo))
	}
}
