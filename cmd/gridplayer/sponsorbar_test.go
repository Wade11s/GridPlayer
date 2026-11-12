package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"gridplayer-go/internal/sponsorblock"
)

func TestVisibleSponsorMarksFiltersOffCategories(t *testing.T) {
	segments := []sponsorblock.Segment{
		{Category: "sponsor", Start: 1000, End: 2000},
		{Category: "selfpromo", Start: 3000, End: 4000},
		{Category: "intro", Start: 5000, End: 6000},
	}
	modes := map[string]string{"sponsor": "skip", "selfpromo": "show", "intro": "off"}
	marks := visibleSponsorMarks(segments, modes)
	if len(marks) != 2 {
		t.Fatalf("marks %+v", marks)
	}
	if marks[0].Category != "sponsor" || marks[1].Category != "selfpromo" {
		t.Fatalf("marks %+v", marks)
	}
}

func TestVisibleSponsorMarksEmpty(t *testing.T) {
	if marks := visibleSponsorMarks(nil, map[string]string{"sponsor": "skip"}); marks != nil {
		t.Fatalf("marks %+v", marks)
	}
	segments := []sponsorblock.Segment{{Category: "intro", Start: 0, End: 1}}
	if marks := visibleSponsorMarks(segments, map[string]string{"intro": "off"}); len(marks) != 0 {
		t.Fatalf("marks %+v", marks)
	}
}

func TestClampFraction(t *testing.T) {
	cases := map[float32]float32{-1: 0, 0: 0, 0.5: 0.5, 1: 1, 2: 1}
	for in, want := range cases {
		if got := clampFraction(in); got != want {
			t.Fatalf("clampFraction(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestSeekTargetForX(t *testing.T) {
	cases := []struct {
		x, width float32
		want     int64
	}{
		{0, 500, 0},
		{250, 500, 5000},
		{500, 500, 10000},
		{600, 500, 10000},
		{-20, 500, 0},
	}
	for _, tc := range cases {
		if got := seekTargetForX(tc.x, tc.width, 10000); got != tc.want {
			t.Fatalf("seekTargetForX(%v, %v) = %v, want %v", tc.x, tc.width, got, tc.want)
		}
	}
	if got := seekTargetForX(100, 0, 10000); got != 0 {
		t.Fatalf("zero width = %v", got)
	}
	if got := seekTargetForX(100, 500, 0); got != 0 {
		t.Fatalf("zero length = %v", got)
	}
}

func TestSponsorMarkRectsMapsTimeToBar(t *testing.T) {
	box := ui.Rect{X: 100, Y: 50, W: 1000, H: 10}
	marks := []sponsorblock.Segment{
		{Category: "sponsor", Start: 0, End: 10000},
		{Category: "selfpromo", Start: 25000, End: 50000},
	}
	rects := sponsorMarkRects(box, 100000, marks)
	if len(rects) != 2 {
		t.Fatalf("rects %+v", rects)
	}
	if rects[0].rect != (ui.Rect{X: 100, Y: 50, W: 100, H: 10}) {
		t.Fatalf("first rect %+v", rects[0].rect)
	}
	if rects[1].rect != (ui.Rect{X: 350, Y: 50, W: 250, H: 10}) {
		t.Fatalf("second rect %+v", rects[1].rect)
	}
	if rects[1].category != "selfpromo" {
		t.Fatalf("category %q", rects[1].category)
	}
}

func TestSponsorMarkRectsWidensShortSegments(t *testing.T) {
	box := ui.Rect{X: 0, Y: 0, W: 1000, H: 10}
	rects := sponsorMarkRects(box, 1_000_000, []sponsorblock.Segment{
		{Category: "poi_highlight", Start: 500000, End: 500001},
	})
	if len(rects) != 1 {
		t.Fatalf("rects %+v", rects)
	}
	if rects[0].rect.W < 2 {
		t.Fatalf("short mark width %v", rects[0].rect.W)
	}
}

func TestSponsorMarkRectsClampsAndRejectsInvalidInput(t *testing.T) {
	box := ui.Rect{X: 0, Y: 0, W: 100, H: 10}
	rects := sponsorMarkRects(box, 1000, []sponsorblock.Segment{
		{Category: "sponsor", Start: -500, End: 2000},
	})
	if len(rects) != 1 || rects[0].rect.X != 0 || rects[0].rect.W != 100 {
		t.Fatalf("clamped rects %+v", rects)
	}
	if got := sponsorMarkRects(ui.Rect{W: 0, H: 10}, 1000, []sponsorblock.Segment{{Start: 0, End: 1}}); got != nil {
		t.Fatalf("zero-width rects %+v", got)
	}
	if got := sponsorMarkRects(box, 0, []sponsorblock.Segment{{Start: 0, End: 1}}); got != nil {
		t.Fatalf("zero-length rects %+v", got)
	}
}
