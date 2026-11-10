package sponsorblock

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseAndSkipSpans(t *testing.T) {
	data := []byte(`[
	  {"segments":[
	    {"segment":[10.0,20.0],"category":"sponsor"},
	    {"segment":[20.5,30.0],"category":"sponsor"},
	    {"segment":[40.0,50.0],"category":"selfpromo"},
	    {"segment":[60.0,70.0],"category":"unknown"},
	    {"segment":[80.0,80.0],"category":"intro"}
	  ]}
	]`)
	segments, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 3 {
		t.Fatalf("segments %d", len(segments))
	}
	spans := SkipSpans(segments, map[string]string{"sponsor": "skip", "selfpromo": "show"})
	if len(spans) != 1 || spans[0].Start != 10000 || spans[0].End != 30000 {
		t.Fatalf("spans %+v", spans)
	}
	if span, ok := SpanAt(spans, 15000); !ok || span.Category != "sponsor" {
		t.Fatalf("span at %+v %v", span, ok)
	}
	if _, ok := SpanAt(spans, 35000); ok {
		t.Fatal("span outside")
	}
}

func TestYouTubeID(t *testing.T) {
	cases := map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ":                "dQw4w9WgXcQ",
		"https://youtube.com/shorts/dQw4w9WgXcQ":      "dQw4w9WgXcQ",
	}
	for raw, want := range cases {
		if got, ok := YouTubeID(raw); !ok || got != want {
			t.Fatalf("%s -> %q %v", raw, got, ok)
		}
	}
	if _, ok := YouTubeID("https://example.com/a"); ok {
		t.Fatal("wrong host")
	}
}

func TestFetch(t *testing.T) {
	var calls atomic.Int32
	client := &Client{
		HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			if r.URL.Query().Get("videoID") != "dQw4w9WgXcQ" {
				t.Errorf("video id %q", r.URL.Query().Get("videoID"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[{"segments":[{"segment":[1,2],"category":"sponsor"}]}]`)),
				Header:     http.Header{},
			}, nil
		})},
		BaseURL: "https://sponsor.example/api/skipSegments",
	}
	segments, err := client.Fetch(context.Background(), "dQw4w9WgXcQ", []string{"sponsor"})
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 || segments[0].Start != 1000 || segments[0].End != 2000 {
		t.Fatalf("segments %+v", segments)
	}
	if _, err := client.Fetch(context.Background(), "dQw4w9WgXcQ", []string{"sponsor"}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("cache miss, calls %d", calls.Load())
	}
}

func TestFetchDedupesConcurrentRequests(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	client := &Client{
		HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			select {
			case entered <- struct{}{}:
			default:
			}
			<-release
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[{"segments":[{"segment":[1,2],"category":"sponsor"}]}]`)),
				Header:     http.Header{},
			}, nil
		})},
		BaseURL: "https://sponsor.example/api/skipSegments",
	}

	const workers = 8
	var wait sync.WaitGroup
	wait.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wait.Done()
			got, err := client.Fetch(context.Background(), "dQw4w9WgXcQ", []string{"sponsor"})
			if err != nil || len(got) != 1 {
				t.Errorf("concurrent fetch %v %v", got, err)
			}
		}()
	}
	<-entered
	time.Sleep(50 * time.Millisecond)
	close(release)
	wait.Wait()

	if calls.Load() != 1 {
		t.Fatalf("http calls %d, want 1", calls.Load())
	}
}

func TestFetchCacheKeyIgnoresCategoryOrder(t *testing.T) {
	var calls atomic.Int32
	client := &Client{
		HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[]`)),
				Header:     http.Header{},
			}, nil
		})},
		BaseURL: "https://sponsor.example/api/skipSegments",
	}
	if _, err := client.Fetch(context.Background(), "dQw4w9WgXcQ", []string{"sponsor", "intro"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Fetch(context.Background(), "dQw4w9WgXcQ", []string{"intro", "sponsor"}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("http calls %d, want 1", calls.Load())
	}
}

func TestFetchExpiresCache(t *testing.T) {
	var calls atomic.Int32
	client := &Client{
		HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[]`)),
				Header:     http.Header{},
			}, nil
		})},
		BaseURL:  "https://sponsor.example/api/skipSegments",
		CacheTTL: 20 * time.Millisecond,
	}
	if _, err := client.Fetch(context.Background(), "dQw4w9WgXcQ", []string{"sponsor"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := client.Fetch(context.Background(), "dQw4w9WgXcQ", []string{"sponsor"}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("http calls %d, want 2", calls.Load())
	}
}

func TestFetchDoesNotCacheErrors(t *testing.T) {
	var calls atomic.Int32
	client := &Client{
		HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if calls.Add(1) == 1 {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(strings.NewReader(`boom`)),
					Header:     http.Header{},
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[{"segments":[{"segment":[1,2],"category":"sponsor"}]}]`)),
				Header:     http.Header{},
			}, nil
		})},
		BaseURL: "https://sponsor.example/api/skipSegments",
	}
	if _, err := client.Fetch(context.Background(), "dQw4w9WgXcQ", []string{"sponsor"}); err == nil {
		t.Fatal("expected error")
	}
	got, err := client.Fetch(context.Background(), "dQw4w9WgXcQ", []string{"sponsor"})
	if err != nil || len(got) != 1 {
		t.Fatalf("retry %v %v", got, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("http calls %d, want 2", calls.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
