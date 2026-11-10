// Package sponsorblock fetches and merges SponsorBlock video segments.
package sponsorblock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

const (
	SkipJoinMS    int64 = 1000
	SkipMinMS     int64 = 1000
	SkipEndMargin int64 = 1000
)

// Category is one SponsorBlock category and its default mode.
type Category struct {
	Name  string
	Mode  string
	Point bool
}

var Categories = []Category{
	{Name: "sponsor", Mode: "skip"},
	{Name: "selfpromo", Mode: "show"},
	{Name: "interaction", Mode: "show"},
	{Name: "intro", Mode: "off"},
	{Name: "outro", Mode: "off"},
	{Name: "preview", Mode: "off"},
	{Name: "hook", Mode: "off"},
	{Name: "filler", Mode: "off"},
	{Name: "music_offtopic", Mode: "off"},
	{Name: "poi_highlight", Mode: "show", Point: true},
}

// DefaultModes returns the frozen source's category modes.
func DefaultModes() map[string]string {
	modes := map[string]string{}
	for _, category := range Categories {
		modes[category.Name] = category.Mode
	}
	return modes
}

// Segment is one fetched segment in milliseconds.
type Segment struct {
	Category string
	Start    int64
	End      int64
}

// Span is a merged skip range.
type Span struct {
	Start    int64
	End      int64
	Category string
}

// Client fetches from the SponsorBlock API.
type Client struct {
	HTTP      *http.Client
	BaseURL   string
	UserAgent string
}

// New returns a client for the public SponsorBlock API.
func New() *Client {
	return &Client{
		HTTP:      http.DefaultClient,
		BaseURL:   "https://sponsor.ajay.app/api/skipSegments",
		UserAgent: "GridPlayer-Go",
	}
}

// Fetch asks for segments for a YouTube video ID.
func (c *Client) Fetch(ctx context.Context, videoID string, categories []string) ([]Segment, error) {
	if c == nil || videoID == "" {
		return nil, nil
	}
	categoriesJSON, err := json.Marshal(categories)
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("videoID", videoID)
	query.Set("categories", string(categoriesJSON))
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.UserAgent != "" {
		request.Header.Set("User-Agent", c.UserAgent)
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil, fmt.Errorf("sponsorblock: status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

type apiResponse []struct {
	Segments []struct {
		Segment  [2]float64 `json:"segment"`
		Category string     `json:"category"`
	} `json:"segments"`
}

// Parse turns the API response into sanitized segments.
func Parse(data []byte) ([]Segment, error) {
	var response apiResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, category := range Categories {
		known[category.Name] = true
	}
	var segments []Segment
	for _, result := range response {
		for _, item := range result.Segments {
			if !known[item.Category] {
				continue
			}
			start := int64(item.Segment[0] * 1000)
			end := int64(item.Segment[1] * 1000)
			if start < 0 || end <= start {
				continue
			}
			segments = append(segments, Segment{Category: item.Category, Start: start, End: end})
		}
	}
	sort.Slice(segments, func(i, j int) bool {
		if segments[i].Start != segments[j].Start {
			return segments[i].Start < segments[j].Start
		}
		return segments[i].End < segments[j].End
	})
	return segments, nil
}

// SkipSpans merges SKIP-mode segments that are within one second.
func SkipSpans(segments []Segment, modes map[string]string) []Span {
	var spans []Span
	for _, segment := range segments {
		if !strings.EqualFold(modes[segment.Category], "skip") {
			continue
		}
		if len(spans) > 0 && segment.Start <= spans[len(spans)-1].End+SkipJoinMS {
			if segment.End > spans[len(spans)-1].End {
				spans[len(spans)-1].End = segment.End
			}
			continue
		}
		spans = append(spans, Span{Start: segment.Start, End: segment.End, Category: segment.Category})
	}
	return spans
}

// SpanAt returns the span containing ms.
func SpanAt(spans []Span, ms int64) (Span, bool) {
	for _, span := range spans {
		if ms >= span.Start && ms < span.End {
			return span, true
		}
	}
	return Span{}, false
}

// YouTubeID extracts a video ID from common YouTube URLs.
func YouTubeID(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	switch host {
	case "youtu.be", "www.youtu.be":
		id := strings.Trim(parsed.Path, "/")
		return validID(id)
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com":
		if id := parsed.Query().Get("v"); id != "" {
			return validID(id)
		}
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) >= 2 && (parts[0] == "shorts" || parts[0] == "embed" || parts[0] == "v") {
			return validID(parts[1])
		}
	}
	return "", false
}

func validID(id string) (string, bool) {
	id = strings.TrimSpace(id)
	if len(id) != 11 {
		return "", false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') && r != '-' && r != '_' {
			return "", false
		}
	}
	return id, true
}
