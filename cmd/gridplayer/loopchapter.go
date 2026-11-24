package main

import (
	"fmt"
	"sort"

	"gridplayer-go/internal/engine"
	"gridplayer-go/internal/playlist"
)

// chapterToleranceMS is the frozen tolerance: VLC reports a time a hair
// short of where a seek landed, so anything this close to a chapter's start
// counts as being in it.
const chapterToleranceMS = 250

// cleanChapters drops what a broken file says: negative, past-the-end,
// out-of-order and duplicate starts. A start within the tolerance snaps to
// zero, and a file whose only chapter starts at zero has no chapter at all.
func cleanChapters(chapters []engine.Chapter, length int64) []engine.Chapter {
	sorted := append([]engine.Chapter(nil), chapters...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Time < sorted[j].Time })
	seen := map[int64]bool{}
	out := make([]engine.Chapter, 0, len(sorted))
	for _, ch := range sorted {
		start := ch.Time
		if start < 0 || (length > 0 && start >= length) {
			continue
		}
		if start < chapterToleranceMS {
			start = 0
		}
		if seen[start] {
			continue
		}
		seen[start] = true
		ch.Time = start
		out = append(out, ch)
	}
	for _, ch := range out {
		if ch.Time > 0 {
			return out
		}
	}
	return nil
}

// chapterSpan returns the chapter that contains at, and where the next one
// starts. The last chapter runs to length. Before the first chapter the
// stretch from the file start is the span, as in the frozen span_at.
func chapterSpan(chapters []engine.Chapter, at, length int64) (start, end int64, ok bool) {
	cleaned := cleanChapters(chapters, length)
	if len(cleaned) == 0 {
		return 0, 0, false
	}
	idx := -1
	for i := range cleaned {
		if cleaned[i].Time <= at+chapterToleranceMS {
			idx = i
		}
	}
	if idx < 0 {
		start, end = 0, cleaned[0].Time
	} else {
		start = cleaned[idx].Time
		if idx+1 < len(cleaned) {
			end = cleaned[idx+1].Time
		} else {
			// The last chapter runs to the file end. Frozen leaves the end
			// unset for it, so an unknown length still gives a start-only
			// segment rather than no chapter.
			end = 0
		}
	}
	if end == 0 {
		if start <= 0 {
			return 0, 0, false
		}
		return start, 0, true
	}
	if end <= start {
		return 0, 0, false
	}
	return start, end, true
}

// loopSeekBounds is the segment a seek must stay inside. A zero end means
// EOF, as the frozen loop_end property does.
func loopSeekBounds(start, end, length int64, active bool) (lo, hi int64, ok bool) {
	if !active {
		return 0, 0, false
	}
	lo = start
	if lo < 0 {
		lo = 0
	}
	hi = end
	if hi <= 0 {
		hi = length
	}
	if hi <= lo {
		return 0, 0, false
	}
	return lo, hi, true
}

// loopActiveFromVideo reads the persisted segment, including the Go-only
// whole-file marker.
func loopActiveFromVideo(v playlist.Video) bool {
	return (v.LoopActive != nil && *v.LoopActive) || v.LoopStart > 0 || v.LoopEnd > 0
}

// chapterSeek is the frozen chapter jump: a stop inside the segment, the
// segment start when none is ahead, or the end action past the last chapter.
func (a *app) chapterSeek(step int) {
	eng := a.activeEngine()
	if eng == nil {
		return
	}
	eng.RefreshTracks()
	s := eng.Snapshot()
	chapters := cleanChapters(s.Chapters, s.Length)
	if len(chapters) == 0 {
		return
	}
	start, end := int64(0), s.Length
	active := false
	if c := a.activeCell(); c != nil {
		loopStart, loopEnd, isActive := c.loopState()
		start, active = loopStart, isActive
		if loopStart < 0 {
			start = 0
		}
		if loopEnd > 0 {
			end = loopEnd
		}
	}
	if step > 0 {
		if stop, ok := nextStop(chapters, s.Time, start, end); ok {
			a.seekActive(stop)
			return
		}
		if active {
			a.seekActive(start)
			return
		}
		c := a.activeCell()
		if c != nil && c.finishAction() == endLoop {
			a.seekActive(0)
			return
		}
		a.finishVideo(c)
		return
	}
	a.seekActive(previousStop(chapters, s.Time, start, end))
}

// clampSegmentSeek keeps ms inside the segment; without a length only the
// lower bound is knowable.
func clampSegmentSeek(start, end, length, ms int64, active bool) int64 {
	if !active {
		return ms
	}
	if lo, hi, ok := loopSeekBounds(start, end, length, true); ok {
		return seekableTime(ms, lo, hi)
	}
	if start < 0 {
		start = 0
	}
	return seekableTime(ms, start, 0)
}

// chapterRestartMS is the frozen cut: going back from further into a
// chapter than this starts the same chapter over.
const chapterRestartMS = 3000

// chapterStops lists where a jump can land: the segment start and the
// chapter starts strictly inside it, in order and once each.
func chapterStops(chapters []engine.Chapter, start, end int64) []int64 {
	stops := []int64{start}
	for _, ch := range chapters {
		if ch.Time > start && (end <= 0 || ch.Time < end) {
			stops = append(stops, ch.Time)
		}
	}
	sort.Slice(stops, func(i, j int) bool { return stops[i] < stops[j] })
	out := stops[:0]
	for _, stop := range stops {
		if len(out) == 0 || out[len(out)-1] != stop {
			out = append(out, stop)
		}
	}
	return out
}

// nextStop is the next stop past the frozen tolerance, or none at the last.
func nextStop(chapters []engine.Chapter, at, start, end int64) (int64, bool) {
	for _, stop := range chapterStops(chapters, start, end) {
		if stop > at+chapterToleranceMS {
			return stop, true
		}
	}
	return 0, false
}

// previousStop is this chapter's start, or the one before near its start.
func previousStop(chapters []engine.Chapter, at, start, end int64) int64 {
	stops := chapterStops(chapters, start, end)
	index := sort.Search(len(stops), func(i int) bool { return stops[i] > at+chapterToleranceMS }) - 1
	if index < 0 {
		index = 0
	}
	if at-stops[index] > chapterRestartMS {
		return stops[index]
	}
	if index > 0 {
		return stops[index-1]
	}
	return stops[0]
}

// loopSeekTarget is the frozen segment rule: a time past the end, or more
// than 100 ms before the start, belongs back at the start.
func loopSeekTarget(at, start, end int64) (int64, bool) {
	if start < 0 {
		start = 0
	}
	if end <= start {
		return 0, false
	}
	if at < start-100 || at > end {
		return start, true
	}
	return 0, false
}

// loopChapter turns the chapter under the playhead into the cell's loop
// segment. The last chapter ends at the file's end.
func (a *app) loopChapter() {
	c := a.activeCell()
	if c == nil || c.eng == nil {
		return
	}
	c.eng.RefreshTracks()
	s := c.eng.Snapshot()
	start, end, ok := chapterSpan(s.Chapters, s.Time, s.Length)
	if !ok {
		a.status = "no chapter to loop"
		return
	}
	c.setLoopSegment(start, end, true)
	a.dirty = true
	if end == 0 {
		a.status = fmt.Sprintf("loop chapter from %dms", start)
	} else {
		a.status = fmt.Sprintf("loop chapter %d-%dms", start, end)
	}
}
