package main

import "testing"

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
