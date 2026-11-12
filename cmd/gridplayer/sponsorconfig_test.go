package main

import (
	"testing"

	"github.com/egoist/mygo"
)

func TestSponsorModeOverride(t *testing.T) {
	c := &cell{}
	if c.sponsorMode() != "" {
		t.Fatalf("zero cell mode %q", c.sponsorMode())
	}
	c.setSponsorMode("on")
	if c.sponsorMode() != "on" || c.sponsorOverrideEnabled == nil || !*c.sponsorOverrideEnabled {
		t.Fatalf("on override %+v", c.sponsorOverrideEnabled)
	}
	c.setSponsorMode("off")
	if c.sponsorMode() != "off" || c.sponsorOverrideEnabled == nil || *c.sponsorOverrideEnabled {
		t.Fatalf("off override %+v", c.sponsorOverrideEnabled)
	}
	c.setSponsorMode("")
	if c.sponsorMode() != "" || c.sponsorOverrideEnabled != nil {
		t.Fatalf("cleared override %+v", c.sponsorOverrideEnabled)
	}
}

func TestSponsorCategoryOverride(t *testing.T) {
	c := &cell{}
	c.setSponsorCategoryMode("intro", "skip")
	if c.sponsorOverrideModes["intro"] != "skip" {
		t.Fatalf("modes %+v", c.sponsorOverrideModes)
	}
	c.setSponsorCategoryMode("intro", "show")
	if c.sponsorOverrideModes["intro"] != "show" {
		t.Fatalf("modes %+v", c.sponsorOverrideModes)
	}
	c.setSponsorCategoryMode("intro", "")
	if c.sponsorOverrideModes != nil {
		t.Fatalf("modes should be nil: %+v", c.sponsorOverrideModes)
	}
}

func TestNormalizeSponsorOverrides(t *testing.T) {
	got := normalizeSponsorOverrides(map[string]string{
		"intro":  "skip",
		"bogus":  "skip",
		"outro":  "nonsense",
		"filler": "off",
	})
	if len(got) != 2 || got["intro"] != "skip" || got["filler"] != "off" {
		t.Fatalf("normalized %+v", got)
	}
	if got := normalizeSponsorOverrides(map[string]string{"bogus": "skip"}); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestSponsorModesForMergesCellOverrides(t *testing.T) {
	a := &app{sponsorModes: map[string]string{"sponsor": "skip", "intro": "off"}}
	c := &cell{sponsorOverrideModes: map[string]string{"intro": "skip", "outro": "show"}}
	modes := a.sponsorModesFor(c)
	if modes["sponsor"] != "skip" || modes["intro"] != "skip" || modes["outro"] != "show" {
		t.Fatalf("merged %+v", modes)
	}
	if a.sponsorModes["intro"] != "off" {
		t.Fatalf("global modes mutated: %+v", a.sponsorModes)
	}
}

func TestSponsorEnabledFor(t *testing.T) {
	a := &app{}
	a.sponsorEnabled.Store(true)
	on, off := true, false
	if !a.sponsorEnabledFor(nil) || !a.sponsorEnabledFor(&cell{}) {
		t.Fatal("global default should apply")
	}
	if a.sponsorEnabledFor(&cell{sponsorOverrideEnabled: &off}) {
		t.Fatal("cell off should win")
	}
	a.sponsorEnabled.Store(false)
	if !a.sponsorEnabledFor(&cell{sponsorOverrideEnabled: &on}) {
		t.Fatal("cell on should win")
	}
}

func TestSponsorCategoriesFrom(t *testing.T) {
	modes := map[string]string{
		"sponsor": "skip",
		"intro":   "off",
		"outro":   "show",
	}
	categories := sponsorCategoriesFrom(modes)
	found := map[string]bool{}
	for _, name := range categories {
		found[name] = true
	}
	if found["intro"] {
		t.Fatalf("off category requested: %v", categories)
	}
	if !found["sponsor"] || !found["outro"] {
		t.Fatalf("missing categories: %v", categories)
	}
}

func TestSponsorMenuItemsFollowActiveCell(t *testing.T) {
	disabled := false
	c := &cell{
		sponsorOverrideEnabled: &disabled,
		sponsorOverrideModes:   map[string]string{"intro": "skip"},
	}
	a := &app{cells: []*cell{c}, active: 0}
	items := a.sponsorMenuItems()
	if len(items) < 4 {
		t.Fatalf("items %+v", items)
	}
	if items[0].Checked {
		t.Fatal("global radio should be off")
	}
	if !items[2].Checked {
		t.Fatal("disabled radio should be on")
	}
	var intro *mygo.MenuItem
	for _, item := range items {
		if item.Label == "intro" {
			intro = item
		}
	}
	if intro == nil || len(intro.Submenu) != 4 {
		t.Fatalf("intro submenu %+v", intro)
	}
	if intro.Submenu[0].Checked {
		t.Fatal("use-global radio should be off")
	}
	if !intro.Submenu[3].Checked {
		t.Fatal("skip radio should be on")
	}
}

func TestSponsorMenuWithoutActiveCell(t *testing.T) {
	a := &app{}
	items := a.sponsorMenuItems()
	if len(items) != 1 || !items[0].Disabled {
		t.Fatalf("items %+v", items)
	}
}
