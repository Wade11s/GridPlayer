package main

import (
	"time"

	"gridplayer-go/internal/engine"
)

const (
	minAutoReloadMin = 0
	maxAutoReloadMin = 1000
)

// autoReloadInterval is how often a live cell reloads itself.
// 0 minutes disables it, matching upstream.
func autoReloadInterval(minutes int) time.Duration {
	if minutes <= minAutoReloadMin {
		return 0
	}
	if minutes > maxAutoReloadMin {
		minutes = maxAutoReloadMin
	}
	return time.Duration(minutes) * time.Minute
}

// autoReloadReloadsCell reloads the current media at its position,
// keeping the paused state across the reload.
func (a *app) autoReloadReloadsCell(c *cell) {
	if c == nil || c.eng == nil {
		return
	}
	s := c.eng.Snapshot()
	if s.Phase != engine.Loading {
		c.startAt = int(s.Time)
		c.startPaused = s.Phase != engine.Playing
	}
	if c.startPaused {
		c.pauseOnPlay.Store(true)
	}
	c.eng.SetStartPosition(int64(c.startAt))
	_ = c.eng.Reload()
}
