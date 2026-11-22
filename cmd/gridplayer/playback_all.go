package main

import "gridplayer-go/internal/engine"

// toggleAllPlayback mirrors upstream Play / Pause [ALL]: if at least one
// cell is unpaused, pause all; otherwise play all.
func (a *app) toggleAllPlayback() {
	pause := false
	for _, c := range a.cells {
		if c == nil || c.eng == nil {
			continue
		}
		phase := c.eng.Snapshot().Phase
		if phase == engine.Playing || phase == engine.Loading {
			pause = true
			break
		}
	}
	for _, c := range a.cells {
		if c == nil || c.eng == nil {
			continue
		}
		if pause {
			c.eng.Pause()
		} else {
			c.eng.Play()
		}
	}
}

func (a *app) nextAllLists() {
	for _, c := range a.cells {
		if c != nil && len(c.list) > 0 {
			a.nextList(c)
		}
	}
}

func (a *app) previousAllLists() {
	for _, c := range a.cells {
		if c != nil && len(c.list) > 0 {
			a.previousList(c)
		}
	}
}

func seekPercentOffsetTarget(now, length int64, percent float64) int64 {
	if length <= 0 {
		return now
	}
	return now + int64(float64(length)*percent/100)
}

func (a *app) seekPercentOffsetActive(percent float64) {
	s := a.activeSnapshot()
	a.seekActive(seekPercentOffsetTarget(s.Time, s.Length, percent))
}
