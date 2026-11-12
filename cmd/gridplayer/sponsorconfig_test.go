package main

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"gridplayer-go/internal/sponsorblock"
)

func TestSponsorModeOverride(t *testing.T) {
	c := &cell{}
	if c.sponsorMode() != "" {
		t.Fatalf("zero cell mode %q", c.sponsorMode())
	}
	c.setSponsorMode("on")
	if c.sponsorMode() != "on" {
		t.Fatalf("on override %q", c.sponsorMode())
	}
	c.setSponsorMode("off")
	if c.sponsorMode() != "off" {
		t.Fatalf("off override %q", c.sponsorMode())
	}
	c.setSponsorMode("")
	if c.sponsorMode() != "" {
		t.Fatalf("cleared override %q", c.sponsorMode())
	}
}

func TestSponsorCategoryOverride(t *testing.T) {
	c := &cell{}
	a := &app{sponsorModes: map[string]string{}}
	a.setSponsorCategoryOverride(c, "intro", "skip")
	if c.sponsorOverrideModes["intro"] != "skip" {
		t.Fatalf("modes %+v", c.sponsorOverrideModes)
	}
	a.setSponsorCategoryOverride(c, "intro", "show")
	if c.sponsorOverrideModes["intro"] != "show" {
		t.Fatalf("modes %+v", c.sponsorOverrideModes)
	}
	a.setSponsorCategoryOverride(c, "intro", "")
	if c.sponsorOverrideModes != nil {
		t.Fatalf("modes should be nil: %+v", c.sponsorOverrideModes)
	}
}

func TestSponsorLookupPrefersSource(t *testing.T) {
	c := &cell{source: "list.m3u", path: "https://youtu.be/dQw4w9WgXcQ"}
	if got := c.sponsorLookup(); got != c.path {
		t.Fatalf("lookup %q", got)
	}
	c.sponsorSource = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
	if got := c.sponsorLookup(); got != c.sponsorSource {
		t.Fatalf("lookup %q", got)
	}
}

func TestNormalizeSponsorOverrides(t *testing.T) {
	got := normalizeSponsorOverrides(map[string]string{
		"intro":  "skip",
		"bogus":  "skip",
		"outro":  "nonsense",
		"filler": "off",
	})
	if len(got) != 2 || got["intro"] != "skip" || got["filler"] != "off" {
		t.Fatalf("normalized %+v", got)
	}
	if got := normalizeSponsorOverrides(map[string]string{"bogus": "skip"}); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestSponsorModesForMergesCellOverrides(t *testing.T) {
	a := &app{sponsorModes: map[string]string{"sponsor": "skip", "intro": "off"}}
	c := &cell{sponsorOverrideModes: map[string]string{"intro": "skip", "outro": "show"}}
	modes := a.sponsorModesFor(c)
	if modes["sponsor"] != "skip" || modes["intro"] != "skip" || modes["outro"] != "show" {
		t.Fatalf("merged %+v", modes)
	}
	if a.sponsorModes["intro"] != "off" {
		t.Fatalf("global modes mutated: %+v", a.sponsorModes)
	}
}

func TestSponsorEnabledFor(t *testing.T) {
	a := &app{}
	a.sponsorEnabled.Store(true)
	if !a.sponsorEnabledFor(nil) || !a.sponsorEnabledFor(&cell{}) {
		t.Fatal("global default should apply")
	}
	offCell := &cell{}
	offCell.setSponsorMode("off")
	if a.sponsorEnabledFor(offCell) {
		t.Fatal("cell off should win")
	}
	a.sponsorEnabled.Store(false)
	onCell := &cell{}
	onCell.setSponsorMode("on")
	if !a.sponsorEnabledFor(onCell) {
		t.Fatal("cell on should win")
	}
}

func TestSponsorCategoriesFrom(t *testing.T) {
	modes := map[string]string{
		"sponsor": "skip",
		"intro":   "off",
		"outro":   "show",
	}
	categories := sponsorCategoriesFrom(modes)
	found := map[string]bool{}
	for _, name := range categories {
		found[name] = true
	}
	if found["intro"] {
		t.Fatalf("off category requested: %v", categories)
	}
	if !found["sponsor"] || !found["outro"] {
		t.Fatalf("missing categories: %v", categories)
	}
}

func TestSponsorMenuItemsFollowActiveCell(t *testing.T) {
	c := &cell{}
	c.setSponsorMode("off")
	a := &app{cells: []*cell{c}, active: 0}
	a.setSponsorCategoryOverride(c, "intro", "skip")
	items := a.sponsorMenuItems()
	if len(items) < 4 {
		t.Fatalf("items %+v", items)
	}
	if items[0].Checked {
		t.Fatal("global radio should be off")
	}
	if !items[2].Checked {
		t.Fatal("disabled radio should be on")
	}
	var intro *mygo.MenuItem
	for _, item := range items {
		if item.Label == "intro" {
			intro = item
		}
	}
	if intro == nil || len(intro.Submenu) != 4 {
		t.Fatalf("intro submenu %+v", intro)
	}
	if intro.Submenu[0].Checked {
		t.Fatal("use-global radio should be off")
	}
	if !intro.Submenu[3].Checked {
		t.Fatal("skip radio should be on")
	}
}

func TestSponsorMenuWithoutActiveCell(t *testing.T) {
	a := &app{}
	items := a.sponsorMenuItems()
	if len(items) != 1 || !items[0].Disabled {
		t.Fatalf("items %+v", items)
	}
}

func TestRefetchSponsorUsesOriginalPageURL(t *testing.T) {
	var gotID atomic.Value
	client := &sponsorblock.Client{
		HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotID.Store(r.URL.Query().Get("videoID"))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`[{"segments":[{"segment":[1,2],"category":"sponsor"}]}]`)),
				Header: http.Header{},
			}, nil
		})},
		BaseURL: "https://sponsor.example/api/skipSegments",
	}
	a := &app{
		sponsorClient:   client,
		sponsorModes:    sponsorblock.DefaultModes(),
		sponsorSpans:    map[*cell][]sponsorblock.Span{},
		sponsorSegments: map[*cell][]sponsorblock.Segment{},
		sponsorLoading:  map[*cell]bool{},
	}
	a.sponsorEnabled.Store(true)
	c := &cell{
		path:          "https://cdn.example/video.mp4",
		sponsorSource: "https://youtu.be/dQw4w9WgXcQ",
	}
	a.refetchSponsor(c)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		a.sponsorMu.Lock()
		segments := a.sponsorSegments[c]
		a.sponsorMu.Unlock()
		if len(segments) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got, _ := gotID.Load().(string); got != "dQw4w9WgXcQ" {
		t.Fatalf("video id sent %q, want the page URL's id", got)
	}
	a.sponsorMu.Lock()
	defer a.sponsorMu.Unlock()
	if len(a.sponsorSpans[c]) != 1 {
		t.Fatalf("spans %+v", a.sponsorSpans[c])
	}
}

func TestResetSponsorTracksPageURL(t *testing.T) {
	c := &cell{path: "https://cdn.example/video.mp4"}
	a := &app{
		sponsorSpans:    map[*cell][]sponsorblock.Span{},
		sponsorSegments: map[*cell][]sponsorblock.Segment{},
		sponsorLoading:  map[*cell]bool{},
	}
	a.resetSponsor(c, "https://youtu.be/dQw4w9WgXcQ")
	if c.sponsorSource != "https://youtu.be/dQw4w9WgXcQ" {
		t.Fatalf("sponsor source %q", c.sponsorSource)
	}
	a.resetSponsor(c, "/tmp/other.mp4")
	if c.sponsorSource != "" {
		t.Fatalf("sponsor source should clear, got %q", c.sponsorSource)
	}
}

func TestFetchSponsorRemembersURLWhenDisabled(t *testing.T) {
	a := &app{
		sponsorModes:    sponsorblock.DefaultModes(),
		sponsorSpans:    map[*cell][]sponsorblock.Span{},
		sponsorSegments: map[*cell][]sponsorblock.Segment{},
		sponsorLoading:  map[*cell]bool{},
	}
	c := &cell{}
	a.fetchSponsor(c, "https://youtu.be/dQw4w9WgXcQ")
	if c.sponsorSource != "https://youtu.be/dQw4w9WgXcQ" {
		t.Fatalf("sponsor source %q", c.sponsorSource)
	}
	if len(a.sponsorSegments) != 0 || len(a.sponsorLoading) != 0 {
		t.Fatalf("disabled fetch touched state: %+v %+v", a.sponsorSegments, a.sponsorLoading)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
