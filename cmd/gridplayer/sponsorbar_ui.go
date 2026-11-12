package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"gridplayer-go/internal/engine"
	"gridplayer-go/internal/sponsorblock"
)

// sponsorColors mirrors the category colors upstream GridPlayer paints on its
// seek bar so a segment is recognizable without reading a legend.
var sponsorColors = map[string]ui.Color{
	"sponsor":        ui.RGB(0, 212, 0),
	"selfpromo":      ui.RGB(255, 255, 0),
	"interaction":    ui.RGB(204, 0, 255),
	"intro":          ui.RGB(0, 255, 255),
	"outro":          ui.RGB(2, 2, 237),
	"preview":        ui.RGB(0, 143, 214),
	"music_offtopic": ui.RGB(255, 153, 0),
	"filler":         ui.RGB(115, 0, 255),
	"poi_highlight":  ui.RGB(255, 22, 132),
	"hook":           ui.RGB(255, 0, 0),
}

func sponsorColor(category string) ui.Color {
	if color, ok := sponsorColors[category]; ok {
		return color
	}
	return ui.RGB(150, 150, 150)
}

// visibleSponsorMarks keeps the segments of categories that are not off, which
// is what the seek bar shows. Skip-mode categories appear here too: they are
// both marked and skipped.
func visibleSponsorMarks(segments []sponsorblock.Segment, modes map[string]string) []sponsorblock.Segment {
	if len(segments) == 0 {
		return nil
	}
	marks := make([]sponsorblock.Segment, 0, len(segments))
	for _, segment := range segments {
		if modes[segment.Category] == "off" {
			continue
		}
		marks = append(marks, segment)
	}
	return marks
}

func clampFraction(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// seekTargetForX turns a click at x into a position in milliseconds.
func seekTargetForX(x, width float32, length int64) int64 {
	if width <= 0 || length <= 0 {
		return 0
	}
	return int64(clampFraction(x/width) * float32(length))
}

type sponsorMarkRect struct {
	rect     ui.Rect
	category string
}

// sponsorMarkRects maps segments onto the bar. Marks shorter than two points
// are widened so a brief segment stays visible.
func sponsorMarkRects(box ui.Rect, length int64, marks []sponsorblock.Segment) []sponsorMarkRect {
	if box.W <= 0 || box.H <= 0 || length <= 0 || len(marks) == 0 {
		return nil
	}
	scale := float32(length)
	rects := make([]sponsorMarkRect, 0, len(marks))
	for _, mark := range marks {
		start := box.X + box.W*clampFraction(float32(mark.Start)/scale)
		end := box.X + box.W*clampFraction(float32(mark.End)/scale)
		if end-start < 2 {
			end = start + 2
		}
		rects = append(rects, sponsorMarkRect{
			rect:     ui.Rect{X: start, Y: box.Y, W: end - start, H: box.H},
			category: mark.Category,
		})
	}
	return rects
}

// sponsorMarks snapshots the fetched segments for a cell, filtered by the
// current category modes.
func (a *app) sponsorMarks(c *cell) []sponsorblock.Segment {
	a.sponsorMu.Lock()
	segments := a.sponsorSegments[c]
	a.sponsorMu.Unlock()
	return visibleSponsorMarks(segments, a.sponsorModesFor(c))
}

// seekBar draws the cell scrubber with SponsorBlock marks and seeks on click.
// It is a no-op until libVLC reports a length.
func (a *app) seekBar(c *ui.Context, i int, cell *cell, s engine.Snapshot) {
	if s.Length <= 0 {
		return
	}
	var marks []sponsorblock.Segment
	if a.sponsorEnabledFor(cell) {
		marks = a.sponsorMarks(cell)
	}
	length := float32(s.Length)
	played := clampFraction(float32(s.Time) / length)
	el := ui.Box(c.Key(fmt.Sprintf("seek-%d", i))).
		Absolute().Left(6).Right(6).Bottom(40).Height(10).
		Radius(5).Background(ui.RGBA(255, 255, 255, 0.16)).
		Cursor(ui.CursorPointer).
		Draw(func(p *ui.Painter, box ui.Rect) {
			if box.W <= 0 || box.H <= 0 {
				return
			}
			p.Fill(ui.Rect{X: box.X, Y: box.Y, W: box.W * played, H: box.H},
				ui.RGBA(255, 255, 255, 0.55), 5)
			for _, mark := range sponsorMarkRects(box, s.Length, marks) {
				p.Fill(mark.rect, sponsorColor(mark.category), 2)
			}
		})
	if !el.Clicked() || cell.eng == nil {
		return
	}
	x, _, ok := el.PointerPosition()
	if !ok {
		return
	}
	width := el.Bounds().W
	if width <= 0 {
		return
	}
	cell.eng.SeekTo(seekTargetForX(x, width, s.Length))
	a.setActive(i)
}
