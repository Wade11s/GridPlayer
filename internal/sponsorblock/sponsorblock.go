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
	"sync"
	"time"
)

const (
	SkipJoinMS            int64 = 1000
	SkipMinMS             int64 = 1000
	SkipEndMargin         int64 = 1000
	DefaultCacheTTL             = time.Hour
	maxCacheEntries             = 256
	DefaultRequestTimeout       = 30 * time.Second
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
	CacheTTL  time.Duration
	// RequestTimeout bounds one shared lookup. It is independent of any single
	// caller's context so one canceled caller cannot fail the others.
	RequestTimeout time.Duration

	mu       sync.Mutex
	cache    map[string]cacheEntry
	inflight map[string]*fetchCall
}

type cacheEntry struct {
	segments []Segment
	at       time.Time
}

type fetchCall struct {
	done     chan struct{}
	segments []Segment
	err      error
}

// New returns a client for the public SponsorBlock API.
func New() *Client {
	return &Client{
		HTTP:           http.DefaultClient,
		BaseURL:        "https://sponsor.ajay.app/api/skipSegments",
		UserAgent:      "GridPlayer-Go",
		CacheTTL:       DefaultCacheTTL,
		RequestTimeout: DefaultRequestTimeout,
		cache:          map[string]cacheEntry{},
		inflight:       map[string]*fetchCall{},
	}
}

// Fetch asks for segments for a YouTube video ID.
func (c *Client) Fetch(ctx context.Context, videoID string, categories []string) ([]Segment, error) {
	if c == nil || videoID == "" {
		return nil, nil
	}
	key := cacheKey(videoID, categories)
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[string]cacheEntry{}
	}
	if c.inflight == nil {
		c.inflight = map[string]*fetchCall{}
	}
	ttl := c.CacheTTL
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	if entry, ok := c.cache[key]; ok && time.Since(entry.at) < ttl {
		segments := cloneSegments(entry.segments)
		c.mu.Unlock()
		return segments, nil
	}
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		select {
		case <-call.done:
			return cloneSegments(call.segments), call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &fetchCall{done: make(chan struct{})}
	c.inflight[key] = call
	c.mu.Unlock()

	go c.runFetch(ctx, key, videoID, categories, call)

	select {
	case <-call.done:
		return cloneSegments(call.segments), call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// runFetch performs the shared lookup. It runs on a context detached from the
// caller that happened to start it, so a canceled caller cannot fail the
// callers waiting on the same key.
func (c *Client) runFetch(ctx context.Context, key, videoID string, categories []string, call *fetchCall) {
	timeout := c.RequestTimeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	var segments []Segment
	var err error
	defer func() {
		if recovered := recover(); recovered != nil {
			segments = nil
			err = fmt.Errorf("sponsorblock: panic: %v", recovered)
		}
		c.mu.Lock()
		call.segments = cloneSegments(segments)
		call.err = err
		if err == nil {
			c.cache[key] = cacheEntry{segments: cloneSegments(segments), at: time.Now()}
			c.evictLocked()
		}
		delete(c.inflight, key)
		close(call.done)
		c.mu.Unlock()
	}()

	segments, err = c.fetchHTTP(shared, videoID, categories)
}

// evictLocked drops the oldest entry once the cache exceeds its bound.
// Callers must hold c.mu.
func (c *Client) evictLocked() {
	if len(c.cache) <= maxCacheEntries {
		return
	}
	oldestKey := ""
	var oldest time.Time
	for key, entry := range c.cache {
		if oldestKey == "" || entry.at.Before(oldest) {
			oldestKey = key
			oldest = entry.at
		}
	}
	delete(c.cache, oldestKey)
}

func (c *Client) fetchHTTP(ctx context.Context, videoID string, categories []string) ([]Segment, error) {
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
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
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

func cacheKey(videoID string, categories []string) string {
	sorted := append([]string(nil), categories...)
	sort.Strings(sorted)
	return videoID + "\x00" + strings.Join(sorted, ",")
}

func cloneSegments(segments []Segment) []Segment {
	return append([]Segment(nil), segments...)
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
