package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSwapCells(t *testing.T) {
	a := &app{cells: []*cell{{label: "a"}, {label: "b"}, {label: "c"}}, active: 0}
	a.swapCells(0, 2)
	if a.cells[0].label != "c" || a.cells[2].label != "a" {
		t.Fatalf("cells %q %q %q", a.cells[0].label, a.cells[1].label, a.cells[2].label)
	}
	if a.active != 2 || !a.dirty {
		t.Fatalf("active=%d dirty=%v", a.active, a.dirty)
	}
	a.swapCells(-1, 0)
	a.swapCells(0, 9)
	if a.cells[0].label != "c" {
		t.Fatal("invalid swap changed cells")
	}
}

func TestInsertCells(t *testing.T) {
	a := &app{defaultVolume: 80}
	a.setPlaylist([]string{"a.mp4", "b.mp4"})
	keepA, keepB := a.cells[0], a.cells[1]
	a.insertCells(1, []string{"x.mp4", "y.mp4"})
	if len(a.cells) != 4 {
		t.Fatalf("len=%d", len(a.cells))
	}
	if a.cells[0] != keepA || a.cells[3] != keepB {
		t.Fatal("insert did not preserve existing cells")
	}
	if a.cells[1].path != "x.mp4" || a.cells[2].path != "y.mp4" || a.active != 1 {
		t.Fatalf("cells=%v active=%d", a.paths(), a.active)
	}
	if !a.dirty {
		t.Fatal("dirty not set")
	}
	a.insertCells(99, []string{"z.mp4"})
	if a.cells[len(a.cells)-1].path != "z.mp4" {
		t.Fatal("append failed")
	}
}

func TestListAdvanceAndWrap(t *testing.T) {
	c := &cell{list: []string{"a.mp4", "b.mp4"}, listIndex: 0, listLoop: true, path: "a.mp4"}
	a := &app{cells: []*cell{c}}
	a.advanceList(c)
	if c.listIndex != 1 || c.path != "b.mp4" {
		t.Fatalf("advance %d %q", c.listIndex, c.path)
	}
	a.advanceList(c)
	if c.listIndex != 0 || c.path != "a.mp4" {
		t.Fatalf("wrap %d %q", c.listIndex, c.path)
	}
	a.previousList(c)
	if c.listIndex != 1 || c.path != "b.mp4" {
		t.Fatalf("previous %d %q", c.listIndex, c.path)
	}
}

func TestShouldAdvanceOncePerGeneration(t *testing.T) {
	c := &cell{}
	if !c.shouldAdvance(1) {
		t.Fatal("first end should advance")
	}
	if c.shouldAdvance(1) {
		t.Fatal("Stopped after EndReached must not advance again")
	}
	if !c.shouldAdvance(2) {
		t.Fatal("new generation should advance")
	}
}

func TestNewCellReportsEmptyList(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.m3u")
	if err := os.WriteFile(p, []byte("#EXTM3U\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &app{defaultVolume: 80}
	c := a.newCell(0, p)
	if c.listErr == "" {
		t.Fatal("empty m3u should record an error")
	}
}

func TestMosaicArrangeKeepsLock(t *testing.T) {
	a := &app{
		cells:  []*cell{{label: "a"}, {label: "b"}, {label: "c"}, {label: "d"}},
		mosaic: true,
	}
	a.ensureMosaic()
	if len(a.mosaicTiles) != 4 {
		t.Fatalf("tiles=%d", len(a.mosaicTiles))
	}
	a.mosaicLocked[0] = true
	locked := a.mosaicTiles[0].Box
	a.arrangeMosaic()
	if a.mosaicTiles[0].Box != locked {
		t.Fatalf("locked tile moved: %+v != %+v", a.mosaicTiles[0].Box, locked)
	}
	if len(a.mosaicTiles) != 4 || !a.dirty {
		t.Fatalf("tiles=%d dirty=%v", len(a.mosaicTiles), a.dirty)
	}
}

func TestMosaicLockToggleAndReset(t *testing.T) {
	a := &app{cells: []*cell{{label: "a"}, {label: "b"}}, mosaic: true, active: 1}
	a.ensureMosaic()
	a.toggleMosaicLock()
	if !a.mosaicLocked[1] {
		t.Fatal("lock not set")
	}
	a.toggleMosaicLock()
	if a.mosaicLocked[1] {
		t.Fatal("lock not cleared")
	}
	a.mosaicLocked[0] = true
	a.resetMosaic()
	if len(a.mosaicLocked) != 0 || len(a.mosaicTiles) != 2 {
		t.Fatalf("reset: locked=%v tiles=%d", a.mosaicLocked, len(a.mosaicTiles))
	}
}
