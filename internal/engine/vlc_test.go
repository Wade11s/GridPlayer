//go:build darwin

package engine

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"gridplayer-go/internal/vlcmac"
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

// TestRealVLCRTSP serves the fixture from VLC's own RTSP server and lets a
// second libVLC instance open it. It skips when VLC.app is unavailable or the
// sandbox denies the listener.
func TestRealVLCRTSP(t *testing.T) {
	layout, err := vlcmac.Discover()
	if err != nil {
		t.Skipf("VLC.app not found: %v", err)
	}
	vlcBin := filepath.Join(layout.AppDir, "Contents", "MacOS", "VLC")
	if _, err := os.Stat(vlcBin); err != nil {
		t.Skipf("VLC binary missing: %v", err)
	}
	fixture, err := filepath.Abs("../../testdata/fixtures/plane-a.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("fixture missing")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot reserve RTSP port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	url := fmt.Sprintf("rtsp://127.0.0.1:%d/live", port)
	logFile, err := os.CreateTemp("", "gridplayer-rtsp-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(logFile.Name())
	defer logFile.Close()

	cmd := exec.Command(vlcBin,
		"-I", "dummy", "--repeat", "--loop", "--no-video-title-show",
		fixture, "--sout", "#rtp{sdp="+url+"}", "--sout-keep",
	)
	cmd.Stderr = logFile
	cmd.Stdout = logFile
	if err := cmd.Start(); err != nil {
		t.Skipf("VLC server start: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			data, _ := os.ReadFile(logFile.Name())
			t.Skipf("RTSP listener did not start: %s", data)
		}
		time.Sleep(50 * time.Millisecond)
	}

	e := New(NewVLCFactory(VLCOptions{Args: []string{
		"--vout=dummy", "--aout=dummy", "--network-caching=100",
	}}))
	defer e.Close()
	if err := e.Load(url); err != nil {
		t.Skipf("libVLC unavailable: %v", err)
	}
	deadline = time.Now().Add(15 * time.Second)
	for e.Snapshot().Phase != Playing && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if s := e.Snapshot(); s.Phase != Playing {
		data, _ := os.ReadFile(logFile.Name())
		t.Fatalf("never played RTSP: %+v (%s)", s, data)
	}
}
