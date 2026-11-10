package main

import (
	"math"
	"strconv"
	"strings"

	"gridplayer-go/internal/engine"
)

const (
	minPlaybackRate = 0.2
	maxPlaybackRate = 12.0
	minFrozenZoom   = 1.0
	maxFrozenZoom   = 10.0
	defaultZoomStep = 0.1
	shiftStepShare  = 0.05
)

// zoomRung returns the next frozen zoom rung. Rungs are 10% of the whole
// picture by default; proportional mode multiplies by 1+step instead.
func zoomRung(scale float64, direction int, step float64, proportional bool) float64 {
	if step <= 0 {
		step = defaultZoomStep
	}
	var rung float64
	var rungScale func(int) float64
	if proportional {
		ratio := 1 + step
		rung = math.Log(scale) / math.Log(ratio)
		rungScale = func(n int) float64 {
			return math.Pow(ratio, float64(n))
		}
	} else {
		rung = scale / step
		rungScale = func(n int) float64 {
			return float64(n) * step
		}
	}
	index := int(roundFloat(rung)) + direction
	if direction < 0 && scale >= maxFrozenZoom {
		index = int(math.Floor(rung - 1e-6))
	}
	next := clampFrozenZoom(roundScale(rungScale(index)))
	for next == scale && minFrozenZoom < next && next < maxFrozenZoom {
		index += direction
		next = clampFrozenZoom(roundScale(rungScale(index)))
	}
	return next
}

func clampFrozenZoom(scale float64) float64 {
	if scale < minFrozenZoom {
		return minFrozenZoom
	}
	if scale > maxFrozenZoom {
		return maxFrozenZoom
	}
	return scale
}

func clampPlaybackRate(rate float64) float64 {
	if rate < minPlaybackRate {
		return minPlaybackRate
	}
	if rate > maxPlaybackRate {
		return maxPlaybackRate
	}
	return rate
}

func clampSubtitleDelay(delay int64) int64 {
	return engine.ClampSubtitleDelay(delay)
}

// seekPercentTarget is the millisecond target for a 0..1 seek shortcut.
func seekPercentTarget(length int64, percent float64) int64 {
	if length <= 0 || percent <= 0 {
		return 0
	}
	return int64(float64(length) * percent)
}

// randomSeekTarget picks a time between loopStart and loopEnd.
func randomSeekTarget(loopStart, loopEnd int64, next func(int64) int64) int64 {
	if loopEnd <= 0 {
		return 0
	}
	if loopStart < 0 {
		loopStart = 0
	}
	if loopEnd <= loopStart {
		return loopStart
	}
	return loopStart + next(loopEnd-loopStart+1)
}

// setLoopPoint validates the frozen fragment-loop bounds.
func setLoopPoint(start, end *int64, at int64, isStart bool) bool {
	if isStart {
		if end != nil && *end != 0 && at >= *end {
			return false
		}
		*start = at
		return true
	}
	if start != nil && *start != 0 && at <= *start {
		return false
	}
	*end = at
	return true
}

// seekableTime clamps a shortcut seek into the fragment loop.
func seekableTime(at, loopStart, loopEnd int64) int64 {
	if at < loopStart {
		return loopStart
	}
	if loopEnd > 0 && at > loopEnd {
		return loopStart
	}
	return at
}

// parseTimecode accepts hh:mm:ss.mmm, mm:ss.mmm, or ss.mmm.
func parseTimecode(text string) (int64, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	parts := strings.Split(text, ":")
	if len(parts) > 3 {
		return 0, false
	}
	var total float64
	for _, part := range parts {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil || value < 0 {
			return 0, false
		}
		total = total*60 + value
	}
	return int64(total * 1000), true
}

// cropStep changes one of the four frame-pixel margins by a step.
func cropStep(m cropMargins, side string, delta float64) cropMargins {
	switch side {
	case "left":
		m.Left += delta
	case "top":
		m.Top += delta
	case "right":
		m.Right += delta
	case "bottom":
		m.Bottom += delta
	}
	return m
}

// subtitleDelayNudge moves the persisted subtitle delay by ±100ms.
func subtitleDelayNudge(current int64, delta int64) int64 {
	return engine.ClampSubtitleDelay(current + delta)
}

func roundFloat(v float64) float64 {
	if v < 0 {
		return -math.Floor(-v + 0.5)
	}
	return math.Floor(v + 0.5)
}

func roundScale(v float64) float64 {
	return math.Round(v*100) / 100
}
