package main

import (
	"testing"

	"gridplayer-go/internal/engine"
)

func TestChapterSpan(t *testing.T) {
	chapters := []engine.Chapter{{Index: 0, Time: 0}, {Index: 1, Time: 10000}, {Index: 2, Time: 30000}}
	for _, tt := range []struct {
		name       string
		at, length int64
		wantStart  int64
		wantEnd    int64
		wantOK     bool
	}{
		{name: "first", at: 0, length: 60000, wantStart: 0, wantEnd: 10000, wantOK: true},
		{name: "middle", at: 15000, length: 60000, wantStart: 10000, wantEnd: 30000, wantOK: true},
		{name: "last", at: 45000, length: 60000, wantStart: 30000, wantEnd: 60000, wantOK: true},
		{name: "at boundary", at: 10000, length: 60000, wantStart: 10000, wantEnd: 30000, wantOK: true},
		{name: "before first", at: -1, length: 60000, wantStart: 0, wantEnd: 10000, wantOK: true},
	} {
		start, end, ok := chapterSpan(chapters, tt.at, tt.length)
		if ok != tt.wantOK || start != tt.wantStart || end != tt.wantEnd {
			t.Fatalf("%s: span=%d-%d ok=%v, want %d-%d %v", tt.name, start, end, ok, tt.wantStart, tt.wantEnd, tt.wantOK)
		}
	}
	if _, _, ok := chapterSpan(nil, 0, 1000); ok {
		t.Fatal("empty chapters produced a span")
	}
	if _, _, ok := chapterSpan([]engine.Chapter{{Time: 10000}}, 10000, 10000); ok {
		t.Fatal("zero-length last chapter produced a span")
	}
}

func TestLoopSeekTarget(t *testing.T) {
	for _, tt := range []struct {
		name       string
		at, start  int64
		end        int64
		wantTarget int64
		wantSeek   bool
	}{
		{name: "inside", at: 15000, start: 10000, end: 20000},
		{name: "at start", at: 10000, start: 10000, end: 20000},
		{name: "just before start", at: 9901, start: 10000, end: 20000},
		{name: "past slack", at: 9800, start: 10000, end: 20000, wantTarget: 10000, wantSeek: true},
		{name: "at end", at: 20000, start: 10000, end: 20000},
		{name: "past end", at: 20001, start: 10000, end: 20000, wantTarget: 10000, wantSeek: true},
		{name: "wrapped", at: 0, start: 10000, end: 20000, wantTarget: 10000, wantSeek: true},
		{name: "empty segment", at: 10000, start: 10000, end: 10000},
		{name: "negative start", at: 0, start: -5, end: 1000},
	} {
		target, seek := loopSeekTarget(tt.at, tt.start, tt.end)
		if seek != tt.wantSeek || target != tt.wantTarget {
			t.Fatalf("%s: target=%d seek=%v, want %d %v", tt.name, target, seek, tt.wantTarget, tt.wantSeek)
		}
	}
}
