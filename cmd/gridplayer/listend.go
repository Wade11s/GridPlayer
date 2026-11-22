package main

import "gridplayer-go/internal/grid"

const (
	listEndLoop  = "loop"
	listEndNext  = "next" // Go-only Next Cell extension.
	listEndPause = "pause"
	listEndStop  = "stop"
	listEndClose = "close"
)

// normalizeListEnd accepts both the frozen upstream AutoName values and the
// short names written by older Go builds. The returned value is internal.
func normalizeListEnd(action string) string {
	switch action {
	case listEndLoop, "start_over":
		return listEndLoop
	case listEndNext:
		return listEndNext
	case listEndPause:
		return listEndPause
	case listEndStop:
		return listEndStop
	case listEndClose:
		return listEndClose
	default:
		return ""
	}
}

// handleListEnd performs a terminal action and reports whether the caller
// should avoid wrapping. loop returns false so the caller performs the wrap.
func (a *app) handleListEnd(c *cell, forward bool) bool {
	switch normalizeListEnd(c.listEnd) {
	case listEndNext:
		if forward {
			a.nextCellFrom(c)
		} else {
			a.previousCellFrom(c)
		}
	case listEndPause:
		a.pauseListAtStart(c)
	case listEndStop:
		a.stopList(c)
	case listEndClose:
		a.closeListCell(c)
	default:
		return false
	}
	return true
}

func (a *app) pauseListAtStart(c *cell) {
	if c == nil {
		return
	}
	start := c.loopStart
	if start < 0 {
		start = 0
	}
	c.startAt = int(start)
	c.startPaused = true
	c.skipGen = 0
	if c.eng != nil {
		// Seeking libVLC's ended player can asynchronously resume or settle
		// as stopped. Reopen the same item and use the established paused
		// restore guard; the new generation also fences late Ended events.
		c.pauseOnPlay.Store(true)
		c.eng.SetStartPosition(start)
		if err := c.eng.Load(c.path); err != nil {
			a.status = err.Error()
			return
		}
		c.eng.Pause()
	}
}

func (a *app) stopList(c *cell) {
	if c == nil {
		return
	}
	c.startAt = 0
	c.startPaused = true
	c.skipGen = 0
	c.retries.Store(0)
	if c.eng != nil {
		c.eng.Stop()
	}
}

// closeListCell closes and removes one cell, matching upstream's Close end
// action rather than merely leaving a released player in an occupied tile.
func (a *app) closeListCell(c *cell) {
	if c == nil {
		return
	}
	index := -1
	for i, candidate := range a.cells {
		if candidate == c {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	a.closeCell(c)
	a.cells = append(a.cells[:index], a.cells[index+1:]...)
	if len(a.cells) == 0 {
		a.active = 0
	} else {
		if a.active > index {
			a.active--
		}
		if a.active >= len(a.cells) {
			a.active = len(a.cells) - 1
		}
		if a.active < 0 {
			a.active = 0
		}
	}
	a.freeTiles = grid.FreeLayout(len(a.cells))
	a.resetMosaic()
	a.dirty = true
	a.refreshMenu()
	if a.win != nil {
		a.win.Invalidate()
	}
}
