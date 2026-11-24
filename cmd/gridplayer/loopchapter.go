package main

import (
	"fmt"

	"gridplayer-go/internal/engine"
)

// chapterSpan returns the chapter that contains at, and where the next one
// starts. The last chapter runs to length. A chapter before the first one
// or a live video has no span.
func chapterSpan(chapters []engine.Chapter, at, length int64) (start, end int64, ok bool) {
	if len(chapters) == 0 {
		return 0, 0, false
	}
	idx := 0
	for i := range chapters {
		if chapters[i].Time <= at {
			idx = i
		}
	}
	start = chapters[idx].Time
	if start < 0 {
		start = 0
	}
	if idx+1 < len(chapters) {
		end = chapters[idx+1].Time
	} else {
		end = length
	}
	if end <= start {
		return 0, 0, false
	}
	return start, end, true
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
	s := c.eng.Snapshot()
	start, end, ok := chapterSpan(s.Chapters, s.Time, s.Length)
	if !ok {
		a.status = "no chapter to loop"
		return
	}
	c.loopStart, c.loopEnd = start, end
	a.dirty = true
	a.status = fmt.Sprintf("loop chapter %d-%dms", start, end)
}
