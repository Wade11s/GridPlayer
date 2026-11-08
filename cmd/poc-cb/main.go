// Command poc-cb is Attempt B: libVLC video-callback frames (RV32) in a
// headless drain, to measure decode path + throughput before wiring
// MyGo textures. No window.
//
// Usage: scripts/build.sh cb && ./bin/poc-cb
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gridplayer-go/internal/vlcmac"
)

func main() {
	out := flag.String("out", "testdata/fixtures", "fixture dir")
	flag.Parse()
	var logBuf strings.Builder
	vlcmac.LogSink = func(level int, msg string) { logBuf.WriteString(msg + "\n") }
	inst, err := vlcmac.NewInstance(2)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer inst.Release()
	pl, err := vlcmac.NewPlayer(inst)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	pl.EnableCallbackVideo()
	path := filepath.Join(*out, "plane-a.mp4")
	if err := pl.SetMedia(path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	pl.Play()
	fmt.Println("poc-cb: playing 5s, draining frames...")
	deadline := time.After(5 * time.Second)
	frames := 0
	var first, last vlcmac.Frame
	t0 := time.Now()
	for {
		select {
		case f := <-vlcmac.FrameCh:
			if frames == 0 {
				first = f
			}
			last = f
			frames++
		case <-deadline:
			goto done
		}
	}
done:
	fmt.Printf("poc-cb: frames=%d in %v state=%d time=%dms %dx%d\n",
		frames, time.Since(t0), pl.State(), pl.Time(), last.W, last.H)
	log := logBuf.String()
	fmt.Println("--- decoder evidence ---")
	lines := strings.Split(log, "\n")
	for i, line := range lines {
		s := strings.TrimSpace(line)
		if i < 40 || strings.Contains(s, "vout") || strings.Contains(s, "Video Toolbox") || strings.Contains(s, "videotoolbox") || strings.Contains(s, "avcodec") || strings.Contains(s, "h264") {
			fmt.Println(" ", s)
		}
	}
	fmt.Printf("log lines: %d\n", len(lines))
	_ = first
	if frames < 20 {
		fmt.Println("poc-cb: FAIL (too few frames)")
		os.Exit(1)
	}
	fmt.Println("poc-cb: PASS (frames flow; decode mode per log above)")
}
