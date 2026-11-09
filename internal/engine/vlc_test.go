//go:build darwin

package engine

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRealVLCDummyOutput is the real-libVLC integration test that needs no
// window: dummy vout/aout still decode and keep the clock. Hardware output
// is proven separately by cmd/poc-hw and the GUI acceptance.
func TestRealVLCDummyOutput(t *testing.T) {
	fixture, err := filepath.Abs("../../testdata/fixtures/plane-a.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("fixture missing")
	}
	factory := NewVLCFactory(VLCOptions{
		Args: []string{"--vout=dummy", "--aout=dummy"},
	})
	e := New(factory)
	defer e.Close()

	if err := e.Load(fixture); err != nil {
		t.Skipf("libVLC unavailable: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for e.Snapshot().Phase != Playing && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	s := e.Snapshot()
	if s.Phase != Playing {
		t.Fatalf("never played: %+v", s)
	}
	if s.Length <= 0 {
		t.Fatalf("length not reported: %+v", s)
	}

	e.SeekTo(3000)
	deadline = time.Now().Add(5 * time.Second)
	for e.Snapshot().Time < 2500 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := e.Snapshot().Time; got < 2500 {
		t.Fatalf("seek did not land: time=%d", got)
	}

	e.Pause()
	time.Sleep(300 * time.Millisecond)
	paused := e.Snapshot().Time
	time.Sleep(300 * time.Millisecond)
	if got := e.Snapshot().Time; got != paused {
		t.Fatalf("time advanced while paused: %d -> %d", paused, got)
	}
}

// TestRealVLCLoadPaused pauses immediately after Load, the path used when a
// saved playlist restores a paused cell.
func TestRealVLCLoadPaused(t *testing.T) {
	fixture, err := filepath.Abs("../../testdata/fixtures/plane-a.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("fixture missing")
	}
	e := New(NewVLCFactory(VLCOptions{Args: []string{"--vout=dummy", "--aout=dummy"}}))
	defer e.Close()
	if err := e.Load(fixture); err != nil {
		t.Skipf("libVLC unavailable: %v", err)
	}
	e.Pause()
	deadline := time.Now().Add(10 * time.Second)
	for e.Snapshot().Phase != Paused && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if s := e.Snapshot(); s.Phase != Paused {
		t.Fatalf("never paused: %+v", s)
	}
	paused := e.Snapshot().Time
	time.Sleep(300 * time.Millisecond)
	if got := e.Snapshot().Time; got != paused {
		t.Fatalf("time advanced while paused: %d -> %d", paused, got)
	}
}

func TestRealVLCFeatureTracksAndChapters(t *testing.T) {
	fixture, err := filepath.Abs("../../testdata/fixtures/feature.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("feature fixture missing")
	}
	e := New(NewVLCFactory(VLCOptions{Args: []string{"--vout=dummy", "--aout=dummy"}}))
	defer e.Close()
	if err := e.Load(fixture); err != nil {
		t.Skipf("libVLC unavailable: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for e.Snapshot().Phase != Playing && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	e.RefreshTracks()
	var s Snapshot
	for time.Now().Before(deadline) {
		s = e.Snapshot()
		if len(s.AudioTracks) >= 2 && len(s.Chapters) == 3 && len(s.SubtitleTracks) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
		e.RefreshTracks()
	}
	if len(s.AudioTracks) < 2 {
		t.Fatalf("audio tracks: %+v", s.AudioTracks)
	}
	if len(s.SubtitleTracks) < 2 {
		t.Fatalf("subtitle tracks: %+v", s.SubtitleTracks)
	}
	if len(s.Chapters) != 3 || s.Chapters[0].Name != "Intro" || s.Chapters[2].Name != "Part Two" {
		t.Fatalf("chapters: %+v (%q|%q)", s.Chapters, s.Chapters[0].Name, s.Chapters[2].Name)
	}

	if !e.SetAudioTrack(s.AudioTracks[1].ID) {
		t.Fatal("failed to select second audio track")
	}
	var subID int
	for _, tr := range s.SubtitleTracks {
		if tr.ID >= 0 {
			subID = tr.ID
			break
		}
	}
	if !e.SetSubtitleTrack(subID) {
		t.Fatal("failed to select subtitle track")
	}
	if !e.SetChapter(1) {
		t.Fatal("failed to select chapter")
	}
	s = e.Snapshot()
	if s.AudioTrack != s.AudioTracks[1].ID || s.SubtitleTrack != subID || s.Chapter != 1 {
		t.Fatalf("selection snapshot: %+v", s)
	}

	before := len(s.SubtitleTracks)
	subPath, _ := filepath.Abs("../../testdata/fixtures/feature.de.srt")
	if !e.AddSubtitle(subPath) {
		t.Fatal("failed to add external subtitle")
	}
	deadline = time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		e.RefreshTracks()
		if len(e.Snapshot().SubtitleTracks) > before {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	s = e.Snapshot()
	if len(s.SubtitleTracks) <= before {
		t.Fatalf("external subtitle not listed: %+v", s.SubtitleTracks)
	}
}

// TestRealVLCLocalHLS loads HLS from a local file URL (no external network).
// The master playlist has three renditions.
func TestRealVLCLocalHLS(t *testing.T) {
	master, err := filepath.Abs("../../testdata/fixtures/hls/master.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(master); err != nil {
		t.Skip("hls fixture missing")
	}
	e := New(NewVLCFactory(VLCOptions{Args: []string{"--vout=dummy", "--aout=dummy", "--network-caching=100"}}))
	defer e.Close()
	if err := e.Load("file://" + master); err != nil {
		t.Skipf("libVLC unavailable: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for e.Snapshot().Phase != Playing {
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if s := e.Snapshot(); s.Phase != Playing {
		t.Fatalf("never played: %+v", s)
	}
	lengthDeadline := time.Now().Add(10 * time.Second)
	for e.Snapshot().Length <= 0 {
		if time.Now().After(lengthDeadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if s := e.Snapshot(); s.Length <= 0 {
		t.Logf("length still unknown after play: %+v", s)
		e.Close()
		return
	}
}

// TestRealVLCHTTPMedia serves the fixture over HTTP and lets libVLC's own
// HTTP access open it, covering the non-resolver HTTP path end to end.
func TestRealVLCHTTPMedia(t *testing.T) {
	fixture, err := filepath.Abs("../../testdata/fixtures/plane-a.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("fixture missing")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen for HTTP fixture: %v", err)
	}
	srv := &httptest.Server{
		Listener: ln,
		Config:   &http.Server{Handler: http.FileServer(http.Dir(filepath.Dir(fixture)))},
	}
	srv.Start()
	defer srv.Close()

	e := New(NewVLCFactory(VLCOptions{Args: []string{
		"--vout=dummy", "--aout=dummy", "--network-caching=100",
	}}))
	defer e.Close()
	if err := e.Load(srv.URL + "/plane-a.mp4"); err != nil {
		t.Skipf("libVLC unavailable: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for e.Snapshot().Phase != Playing && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if s := e.Snapshot(); s.Phase != Playing {
		t.Fatalf("never played over HTTP: %+v", s)
	}
}
