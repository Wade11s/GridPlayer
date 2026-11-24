package main

import (
	"reflect"
	"testing"

	"gridplayer-go/internal/grid"
)

func TestFreeStackActionsAndDocumentRoundTrip(t *testing.T) {
	a := &app{
		cells: []*cell{{path: "a.mp4"}, {path: "b.mp4"}},
		free:  true, active: 0,
		freeTiles: []grid.Tile{
			{Box: [4]float64{0, 0, 0.7, 0.7}},
			{Box: [4]float64{0.3, 0.3, 1, 1}},
		},
		freeOrder: []int{0, 1},
	}
	if !a.canStackActive(true) || a.canStackActive(false) {
		t.Fatal("initial free stack actions are wrong")
	}
	a.stackActive(true)
	if !reflect.DeepEqual(a.freeOrder, []int{1, 0}) || !a.dirty {
		t.Fatalf("front order=%v dirty=%v", a.freeOrder, a.dirty)
	}
	d := a.document()
	var order []int
	if !getRawSetting(d, "grid_free_order", &order) || !reflect.DeepEqual(order, []int{1, 0}) {
		t.Fatalf("saved order %v", order)
	}
	b := &app{defaultVolume: 80, defaultRate: 1}
	b.applyDocument(d, "")
	if !b.free || !reflect.DeepEqual(b.layoutDrawOrder(), []int{1, 0}) {
		t.Fatalf("restored free=%v order=%v", b.free, b.freeOrder)
	}
}

func TestMosaicLockRestacksAndPersistsOrder(t *testing.T) {
	a := &app{cells: []*cell{{path: "a.mp4"}, {path: "b.mp4"}, {path: "c.mp4"}}, mosaic: true}
	a.ensureMosaic()
	a.toggleMosaicLock()
	if !a.mosaicLocked[0] || !reflect.DeepEqual(a.mosaicOrder, []int{1, 2, 0}) {
		t.Fatalf("locked=%v order=%v", a.mosaicLocked, a.mosaicOrder)
	}
	d := a.document()
	b := &app{defaultVolume: 80, defaultRate: 1}
	b.applyDocument(d, "")
	if !b.mosaicLocked[0] || !reflect.DeepEqual(b.layoutDrawOrder(), []int{1, 2, 0}) {
		t.Fatalf("restored locked=%v order=%v", b.mosaicLocked, b.mosaicOrder)
	}
	b.toggleMosaicLock()
	if b.mosaicLocked[0] || !reflect.DeepEqual(b.layoutDrawOrder(), []int{1, 2, 0}) {
		t.Fatalf("unlock changed relative order: locked=%v order=%v", b.mosaicLocked, b.mosaicOrder)
	}
}

func TestMosaicLockKeepsLockedTierOrder(t *testing.T) {
	a := &app{cells: []*cell{{path: "a.mp4"}, {path: "b.mp4"}, {path: "c.mp4"}}, mosaic: true}
	a.ensureMosaic()
	a.toggleMosaicLock()
	a.active = 1
	a.toggleMosaicLock()
	if !a.mosaicLocked[0] || !a.mosaicLocked[1] || !reflect.DeepEqual(a.mosaicOrder, []int{2, 1, 0}) {
		t.Fatalf("locking reordered the locked tier: locked=%v order=%v", a.mosaicLocked, a.mosaicOrder)
	}
}
