package main

import "testing"

func TestShuffleListEndWaitsForACompleteRound(t *testing.T) {
	c := &cell{
		list:      []string{"a.mp4", "b.mp4", "c.mp4", "d.mp4"},
		listIndex: 0, listEnd: listEndStop, path: "a.mp4",
	}
	a := &app{cells: []*cell{c}}
	seen := map[int]bool{0: true}
	for i := 0; i < 3; i++ {
		a.shuffleList(c)
		if c.startPaused {
			t.Fatalf("list stopped before the round was complete at step %d", i+1)
		}
		if seen[c.listIndex] {
			t.Fatalf("index %d repeated before the round was complete", c.listIndex)
		}
		seen[c.listIndex] = true
	}
	if len(seen) != 4 {
		t.Fatalf("visited %d of 4 items", len(seen))
	}
	last := c.listIndex
	a.shuffleList(c)
	if !c.startPaused || c.listIndex != last {
		t.Fatalf("completed shuffle round did not stop in place: %+v", c)
	}
}

func TestShuffleListStartOverBeginsAnotherRound(t *testing.T) {
	c := &cell{
		list:      []string{"a.mp4", "b.mp4", "c.mp4"},
		listIndex: 0, listEnd: listEndLoop, path: "a.mp4",
	}
	a := &app{cells: []*cell{c}}
	for i := 0; i < 7; i++ {
		a.shuffleList(c)
		if c.startPaused || c.listIndex < 0 || c.listIndex >= len(c.list) {
			t.Fatalf("start-over shuffle stopped at step %d: %+v", i+1, c)
		}
	}
}
