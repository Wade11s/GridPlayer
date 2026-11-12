package main

import (
	"github.com/egoist/mygo"

	"gridplayer-go/internal/sponsorblock"
)

// sponsorMenuItems builds the per-cell SponsorBlock overrides. The menu is
// rebuilt after each change so the radio marks follow the active cell.
func (a *app) sponsorMenuItems() []*mygo.MenuItem {
	c := a.activeCell()
	if c == nil {
		return []*mygo.MenuItem{{Label: "No Active Cell", Disabled: true}}
	}
	mode := c.sponsorMode()
	items := []*mygo.MenuItem{
		{Label: "Use Global Setting", Type: mygo.MenuItemRadio, Checked: mode == "",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				a.setSponsorOverride(c, func() { c.setSponsorMode("") })
			}},
		{Label: "Enabled", Type: mygo.MenuItemRadio, Checked: mode == "on",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				a.setSponsorOverride(c, func() { c.setSponsorMode("on") })
			}},
		{Label: "Disabled", Type: mygo.MenuItemRadio, Checked: mode == "off",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				a.setSponsorOverride(c, func() { c.setSponsorMode("off") })
			}},
		{Type: mygo.MenuItemSeparator},
	}
	for _, category := range sponsorblock.Categories {
		name := category.Name
		override := c.sponsorOverrideModes[name]
		items = append(items, &mygo.MenuItem{
			Label: name,
			Submenu: []*mygo.MenuItem{
				{Label: "Use Global", Type: mygo.MenuItemRadio, Checked: override == "",
					Click: func(*mygo.MenuItem, *mygo.Window) {
						a.setSponsorOverride(c, func() { c.setSponsorCategoryMode(name, "") })
					}},
				{Label: "Off", Type: mygo.MenuItemRadio, Checked: override == "off",
					Click: func(*mygo.MenuItem, *mygo.Window) {
						a.setSponsorOverride(c, func() { c.setSponsorCategoryMode(name, "off") })
					}},
				{Label: "Show", Type: mygo.MenuItemRadio, Checked: override == "show",
					Click: func(*mygo.MenuItem, *mygo.Window) {
						a.setSponsorOverride(c, func() { c.setSponsorCategoryMode(name, "show") })
					}},
				{Label: "Skip", Type: mygo.MenuItemRadio, Checked: override == "skip",
					Click: func(*mygo.MenuItem, *mygo.Window) {
						a.setSponsorOverride(c, func() { c.setSponsorCategoryMode(name, "skip") })
					}},
			},
		})
	}
	return items
}
