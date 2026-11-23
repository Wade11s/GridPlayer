package grid

import (
	"reflect"
	"testing"
)

func TestNormalizeOrderRejectsInvalidPermutation(t *testing.T) {
	if got := NormalizeOrder([]int{2, 0, 1}, 3); !reflect.DeepEqual(got, []int{2, 0, 1}) {
		t.Fatalf("valid order %v", got)
	}
	for _, invalid := range [][]int{{0, 0, 1}, {0, 1}, {0, 1, 3}} {
		if got := NormalizeOrder(invalid, 3); !reflect.DeepEqual(got, []int{0, 1, 2}) {
			t.Errorf("invalid %v normalized to %v", invalid, got)
		}
	}
}

func TestStackKeepsLockedTierInFront(t *testing.T) {
	locked := map[int]bool{2: true}
	if got := Stack([]int{0, 1, 2, 3}, 0, true, locked); !reflect.DeepEqual(got, []int{1, 3, 0, 2}) {
		t.Fatalf("front unlocked order %v", got)
	}
	if got := Stack([]int{0, 1, 2, 3}, 3, false, locked); !reflect.DeepEqual(got, []int{3, 0, 1, 2}) {
		t.Fatalf("back unlocked order %v", got)
	}
	locked = map[int]bool{1: true, 3: true}
	if got := Stack([]int{0, 1, 2, 3}, 3, false, locked); !reflect.DeepEqual(got, []int{0, 2, 3, 1}) {
		t.Fatalf("back locked order %v", got)
	}
}

func TestCanStackRequiresSameTierOverlap(t *testing.T) {
	tiles := []Tile{
		{Box: [4]float64{0, 0, 0.6, 0.6}},
		{Box: [4]float64{0.4, 0.4, 1, 1}},
		{Box: [4]float64{0.7, 0, 1, 0.2}},
	}
	order := []int{0, 1, 2}
	if !CanStack(order, tiles, 0, true, nil) || CanStack(order, tiles, 0, false, nil) {
		t.Fatal("tile 0 front/back availability is wrong")
	}
	if !CanStack(order, tiles, 1, false, nil) || CanStack(order, tiles, 1, true, nil) {
		t.Fatal("tile 1 front/back availability is wrong")
	}
	if CanStack(order, tiles, 0, true, map[int]bool{1: true}) {
		t.Fatal("locked tier overlap enabled an unlocked stack action")
	}
}

func TestTopmostUsesBackToFrontOrder(t *testing.T) {
	tiles := []Tile{
		{Box: [4]float64{0, 0, 0.7, 0.7}},
		{Box: [4]float64{0.3, 0.3, 1, 1}},
	}
	if got := Topmost([]int{0, 1}, tiles, 0.5, 0.5); got != 1 {
		t.Fatalf("topmost %d", got)
	}
	if got := Topmost([]int{1, 0}, tiles, 0.5, 0.5); got != 0 {
		t.Fatalf("restacked topmost %d", got)
	}
	if got := Topmost([]int{0, 1}, tiles, 2, 2); got != -1 {
		t.Fatalf("outside topmost %d", got)
	}
}
