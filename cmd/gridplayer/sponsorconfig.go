package main

import (
	"gridplayer-go/internal/sponsorblock"
)

// sponsorMode is the cell's override of the global SponsorBlock toggle.
// "" means inherit.
func (c *cell) sponsorMode() string {
	if c == nil || c.sponsorOverrideEnabled == nil {
		return ""
	}
	if *c.sponsorOverrideEnabled {
		return "on"
	}
	return "off"
}

func (c *cell) setSponsorMode(mode string) {
	switch mode {
	case "on":
		enabled := true
		c.sponsorOverrideEnabled = &enabled
	case "off":
		enabled := false
		c.sponsorOverrideEnabled = &enabled
	default:
		c.sponsorOverrideEnabled = nil
	}
}

func (c *cell) setSponsorCategoryMode(name, mode string) {
	if mode == "" {
		delete(c.sponsorOverrideModes, name)
		if len(c.sponsorOverrideModes) == 0 {
			c.sponsorOverrideModes = nil
		}
		return
	}
	if c.sponsorOverrideModes == nil {
		c.sponsorOverrideModes = map[string]string{}
	}
	c.sponsorOverrideModes[name] = mode
}

func sponsorModesValid(mode string) bool {
	switch mode {
	case "off", "show", "skip":
		return true
	}
	return false
}

// normalizeSponsorOverrides keeps only known categories and modes so a
// hand-edited playlist cannot inject junk into the API request.
func normalizeSponsorOverrides(modes map[string]string) map[string]string {
	if len(modes) == 0 {
		return nil
	}
	known := map[string]bool{}
	for _, category := range sponsorblock.Categories {
		known[category.Name] = true
	}
	out := map[string]string{}
	for name, mode := range modes {
		if known[name] && sponsorModesValid(mode) {
			out[name] = mode
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneSponsorOverrides(modes map[string]string) map[string]string {
	if len(modes) == 0 {
		return nil
	}
	out := make(map[string]string, len(modes))
	for name, mode := range modes {
		out[name] = mode
	}
	return out
}

// sponsorEnabledFor reports whether SponsorBlock applies to this cell.
func (a *app) sponsorEnabledFor(c *cell) bool {
	if c != nil && c.sponsorOverrideEnabled != nil {
		return *c.sponsorOverrideEnabled
	}
	return a.sponsorEnabled.Load()
}

// sponsorModesFor merges this cell's per-category overrides over the global
// modes.
func (a *app) sponsorModesFor(c *cell) map[string]string {
	a.sponsorMu.Lock()
	defer a.sponsorMu.Unlock()
	modes := make(map[string]string, len(a.sponsorModes))
	for name, mode := range a.sponsorModes {
		modes[name] = mode
	}
	if c != nil {
		for name, mode := range c.sponsorOverrideModes {
			modes[name] = mode
		}
	}
	return modes
}

// sponsorCategoriesFrom lists the categories a fetch needs: everything not off.
func sponsorCategoriesFrom(modes map[string]string) []string {
	var categories []string
	for _, category := range sponsorblock.Categories {
		if modes[category.Name] != "off" {
			categories = append(categories, category.Name)
		}
	}
	return categories
}

// setSponsorOverride updates one cell override and refetches that cell.
func (a *app) setSponsorOverride(c *cell, apply func()) {
	if c == nil {
		return
	}
	apply()
	a.dirty = true
	a.refreshMenu()
	a.resetSponsor(c, c.source)
}
