// Command gridplayer is the Go + MyGo migration PoC (macOS/arm64).
//
// One MyGo native-UI window, 4 VLC hardware panes (libVLC via set_nsobject
// under Painter.Hole). Mirrors upstream macOS HW_SP: one libVLC instance
// per pane, set_nsobject before SetMedia, fixed teardown
// stop -> unbind -> detach -> release.
//
// Usage: scripts/build.sh gridplayer && ./bin/gridplayer [--out=DIR] [--autoquit=SEC]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"gridplayer-go/internal/nativehost"
	"gridplayer-go/internal/vlcmac"
)

type cell struct {
	label string
	path  string
	inst  *vlcmac.Instance
	pl    *vlcmac.Player
	view  uintptr
	mute  bool
}

type app struct {
	win     *mygo.Window
	fixDir  string
	resolve func() (cv, sv uintptr, h float64, ok bool)
	cells   [4]*cell
	closed  atomic.Bool
	status  string
}

func newApp(fixDir string) *app {
	a := &app{fixDir: fixDir, status: "starting"}
	labels := []string{"A", "B", "C", "D"}
	names := []string{"plane-a.mp4", "plane-b.mp4", "plane-c.mp4", "plane-d.mp4"}
	for i := range a.cells {
		a.cells[i] = &cell{
			label: labels[i],
			path:  filepath.Join(fixDir, names[i]),
			mute:  i != 0, // cell A audible, others muted (upstream default muted=true)
		}
	}
	return a
}

// ensure creates the native view + libVLC player for cell i and sizes it to
// r (DIPs, top-left). Main thread only (Draw callback).
func (a *app) ensure(i int, r ui.Rect) {
	if a.win == nil || a.closed.Load() {
		return
	}
	cv, sv, h, ok := a.resolve()
	if !ok {
		a.status = "nativehost: surface not found yet"
		return
	}
	c := a.cells[i]
	if c.view == 0 {
		inst, err := vlcmac.NewInstance(0)
		if err != nil {
			a.status = "vlc: " + err.Error()
			return
		}
		pl, err := vlcmac.NewPlayer(inst)
		if err != nil {
			inst.Release()
			a.status = "vlc: " + err.Error()
			return
		}
		view := nativehost.NewView()
		if view == 0 {
			pl.Release()
			inst.Release()
			a.status = "nativehost: no view"
			return
		}
		nativehost.AddBelow(cv, sv, view)
		// Size BEFORE binding: the macosx vout latches the drawable size
		// when output opens (PoC-A sized views before SetNSObject).
		nativehost.SetFrame(view, nativehost.ToMac(
			nativehost.DIP{X: float64(r.X), Y: float64(r.Y), W: float64(r.W), H: float64(r.H)}, h))
		pl.SetNSObject(view)
		if err := pl.SetMedia(c.path); err != nil {
			pl.Release()
			nativehost.Remove(view)
			nativehost.Release(view)
			inst.Release()
			a.status = "vlc: " + err.Error()
			return
		}
		c.inst, c.pl, c.view = inst, pl, view
		if c.mute {
			pl.SetMute(true)
		} else {
			pl.SetVolume(128)
		}
		pl.Play()
	}
	nativehost.SetFrame(c.view, nativehost.ToMac(
		nativehost.DIP{X: float64(r.X), Y: float64(r.Y), W: float64(r.W), H: float64(r.H)}, h))
}

func (a *app) teardown() {
	if !a.closed.CompareAndSwap(false, true) {
		return
	}
	fmt.Println("gridplayer: teardown (stop->unbind->detach->release)...")
	for i, c := range a.cells {
		if c == nil {
			continue
		}
		if c.pl != nil {
			fmt.Printf("gridplayer: cell %d time=%dms state=%d\n", i, c.pl.Time(), c.pl.State())
			c.pl.Stop()
			time.Sleep(50 * time.Millisecond)
			c.pl.Release() // unbinds NSView
			c.pl = nil
		}
		if c.view != 0 {
			nativehost.Remove(c.view) // detach
			nativehost.Release(c.view)
			c.view = 0
		}
		if c.inst != nil {
			c.inst.Release()
			c.inst = nil
		}
	}
	fmt.Println("gridplayer: teardown done")
}

func (a *app) view(c *ui.Context) {
	// global shortcuts: Space = pause/resume all, arrows = seek cell A.
	if c.Shortcut(0, ui.KeySpace) && !c.ShortcutRepeated() {
		for _, cell := range a.cells {
			if cell.pl != nil {
				if cell.pl.State() == 3 { // playing? state values per libvlc_state_t
					cell.pl.Pause()
				} else {
					cell.pl.Play()
				}
			}
		}
	}
	if c.Shortcut(0, ui.KeyLeft) {
		if a.cells[0].pl != nil {
			t := a.cells[0].pl.Time() - 5000
			if t < 0 {
				t = 0
			}
			a.cells[0].pl.SetTime(t)
		}
	}
	if c.Shortcut(0, ui.KeyRight) {
		if a.cells[0].pl != nil {
			a.cells[0].pl.SetTime(a.cells[0].pl.Time() + 5000)
		}
	}
	ui.Box(c).Fill().Background(ui.RGB(10, 10, 12)).Children(func() {
		ui.Column(c).Fill().Gap(2).Padding(2).Children(func() {
			for row := 0; row < 2; row++ {
				ui.Row(c).Gap(2).HeightPercent(50).FillWidth().Children(func() {
					for col := 0; col < 2; col++ {
						i := row*2 + col
						a.cellView(c, i)
					}
				})
			}
		})
	})
}

func (a *app) cellView(c *ui.Context, i int) {
	cell := a.cells[i]
	ui.Box(c.Key(fmt.Sprintf("cell-%d", i))).Grow(1).FillHeight().
		Radius(6).Background(ui.RGB(20, 20, 24)).
		Draw(func(p *ui.Painter, r ui.Rect) {
			a.ensure(i, r)
			// Hole reveals the VLC NSView; what paints after (children)
			// composites above it.
			p.Hole(r, [4]float32{0, 0, 0, 0})
		}).
		Children(func() {
			// status badge, painted after the Hole so it shows over video
			badge := fmt.Sprintf(" %s ", cell.label)
			if cell.pl != nil {
				badge = fmt.Sprintf(" %s %dms ", cell.label, cell.pl.Time())
			}
			ui.Text(c, badge).Absolute().Top(8).Left(8).
				Background(ui.RGBA(0, 0, 0, 170)).TextColor(ui.RGB(255, 255, 255)).FontSize(20).Bold()
		})
	if ui.Box(c.Key(fmt.Sprintf("cell-click-%d", i))).Clicked() {
		// handled above via shortcut for now
	}
}

func main() {
	fixDir := flag.String("out", "testdata/fixtures", "fixture dir")
	autoquit := flag.Int("autoquit", 0, "quit after N seconds (0 = manual)")
	logFile := flag.String("log", "testdata/gridplayer-vlc.log", "libVLC log file")
	flag.Parse()

	if lf, err := os.Create(*logFile); err == nil {
		vlcmac.LogSink = func(level int, msg string) {
			fmt.Fprintf(lf, "[vlc %d] %s\n", level, msg)
		}
	}
	a := newApp(*fixDir)
	mygo.App.WhenReady(func() {
		w := mygo.NewWindow(mygo.WindowOptions{
			Title:          "GridPlayer",
			Width:          1280,
			Height:         720,
			UseContentSize: true,
			Content:        ui.View(a.view),
		})
		a.win = w
		// Resolve lazily inside ensure (needs NativeHandle after creation).
		a.resolve = func() (uintptr, uintptr, float64, bool) {
			return nativehost.Resolve(w.NativeHandle())
		}
		w.OnClose(func(*mygo.CloseEvent) {
			a.teardown()
		})
		// status ticker: refresh time badges ~10fps
		go func() {
			for {
				time.Sleep(100 * time.Millisecond)
				if a.closed.Load() {
					return
				}
				if a.win != nil {
					a.win.Invalidate()
				}
			}
		}()
		if *autoquit > 0 {
			go func() {
				time.Sleep(time.Duration(*autoquit) * time.Second)
				mygo.RunOnMain(func() {
					if a.win != nil {
						a.win.Close()
					}
				})
			}()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	a.teardown()
	fmt.Println("gridplayer: bye")
}
