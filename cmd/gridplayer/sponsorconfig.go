package main

import (
	"strings"

	"gridplayer-go/internal/sponsorblock"
)

const (
	sponsorInherit int32 = iota
	sponsorForceOn
	sponsorForceOff
)

// sponsorMode is the cell's override of the global SponsorBlock toggle.
// "" means inherit.
func (c *cell) sponsorMode() string {
	if c == nil {
		return ""
	}
	switch c.sponsorOverrideState.Load() {
	case sponsorForceOn:
		return "on"
	case sponsorForceOff:
		return "off"
	}
	return ""
}

func (c *cell) setSponsorMode(mode string) {
	if c == nil {
		return
	}
	switch mode {
	case "on":
		c.sponsorOverrideState.Store(sponsorForceOn)
	case "off":
		c.sponsorOverrideState.Store(sponsorForceOff)
	default:
		c.sponsorOverrideState.Store(sponsorInherit)
	}
}

// sponsorLookup returns the URL SponsorBlock should key on: the original page
// URL when the cell was opened from one, else the current source.
func (c *cell) sponsorLookup() string {
	if c == nil {
		return ""
	}
	if c.sponsorSource != "" {
		return c.sponsorSource
	}
	if c.path != "" {
		return c.path
	}
	return c.source
}

func (a *app) setSponsorCategoryOverride(c *cell, name, mode string) {
	a.sponsorMu.Lock()
	defer a.sponsorMu.Unlock()
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

func (a *app) sponsorOverrideFor(c *cell, name string) string {
	a.sponsorMu.Lock()
	defer a.sponsorMu.Unlock()
	return c.sponsorOverrideModes[name]
}

func (a *app) setSponsorOverrides(c *cell, modes map[string]string) {
	a.sponsorMu.Lock()
	defer a.sponsorMu.Unlock()
	c.sponsorOverrideModes = modes
}

func (a *app) sponsorOverridesCopy(c *cell) map[string]string {
	a.sponsorMu.Lock()
	defer a.sponsorMu.Unlock()
	return cloneSponsorOverrides(c.sponsorOverrideModes)
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
	if c != nil {
		switch c.sponsorOverrideState.Load() {
		case sponsorForceOn:
			return true
		case sponsorForceOff:
			return false
		}
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
	a.refetchSponsor(c)
}

// refetchSponsor clears this cell's SponsorBlock state and fetches again using
// the original page URL when one is known.
func (a *app) refetchSponsor(c *cell) {
	if c == nil {
		return
	}
	c.sponsorFetchGen.Add(1)
	a.sponsorMu.Lock()
	delete(a.sponsorSpans, c)
	delete(a.sponsorSegments, c)
	delete(a.sponsorLoading, c)
	a.sponsorMu.Unlock()
	c.sponsorSkipEnd.Store(0)
	a.fetchSponsor(c, c.sponsorLookup())
}

func sponsorSpanIsSkip(modes map[string]string, span sponsorblock.Span) bool {
	return strings.EqualFold(modes[span.Category], "skip")
}
