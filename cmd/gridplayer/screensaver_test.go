package main

import (
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"gridplayer-go/internal/dummy"
	"gridplayer-go/internal/engine"
)

func sleepCommand(string) *exec.Cmd {
	return exec.Command("sh", "-c", "exec sleep 30")
}

func TestScreensaverGuardSetModeSwitchesChild(t *testing.T) {
	var started []keepAwakeMode
	guard := &screensaverGuard{
		command: func(mode keepAwakeMode) *exec.Cmd {
			started = append(started, mode)
			return sleepCommand("")
		},
	}
	defer guard.Close()
	if guard.mode != keepAwakeNone || guard.cmd != nil {
		t.Fatal("guard did not start idle")
	}
	guard.SetMode(keepAwakeDisplay)
	if guard.mode != keepAwakeDisplay || guard.cmd == nil || guard.cmd.Process == nil {
		t.Fatal("display mode did not start")
	}
	displayPID := guard.cmd.Process.Pid
	guard.SetMode(keepAwakeDisplay)
	if guard.cmd.Process.Pid != displayPID {
		t.Fatal("same mode restarted the child")
	}
	guard.SetMode(keepAwakeSystem)
	if guard.mode != keepAwakeSystem || guard.cmd.Process.Pid == displayPID {
		t.Fatal("system mode did not replace the child")
	}
	systemPID := guard.cmd.Process.Pid
	waitDead(t, displayPID)
	guard.SetMode(keepAwakeNone)
	if guard.mode != keepAwakeNone || guard.cmd != nil {
		t.Fatal("none did not stop the child")
	}
	waitDead(t, systemPID)
	if got := strings.TrimSpace(strings.Join(modeStrings(started), ",")); got != "display,system" {
		t.Fatalf("modes started = %s", got)
	}
}

func modeStrings(modes []keepAwakeMode) []string {
	out := make([]string, len(modes))
	for i, m := range modes {
		out[i] = m.String()
	}
	return out
}

func waitDead(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child %d still alive", pid)
}

func TestScreensaverGuardStartFailureResets(t *testing.T) {
	attempts := 0
	guard := &screensaverGuard{
		command: func(keepAwakeMode) *exec.Cmd {
			attempts++
			return exec.Command("/definitely/not/a/command")
		},
	}
	guard.SetMode(keepAwakeDisplay)
	if guard.mode != keepAwakeNone || guard.cmd != nil {
		t.Fatal("failed start left guard active")
	}
	guard.SetMode(keepAwakeDisplay)
	if attempts != 2 {
		t.Fatalf("retry attempts = %d, want 2", attempts)
	}
	if guard.mode != keepAwakeNone || guard.cmd != nil {
		t.Fatal("retry left guard active")
	}
}

func TestScreensaverGuardFailedSwitchKeepsPreviousMode(t *testing.T) {
	guard := &screensaverGuard{
		command: func(mode keepAwakeMode) *exec.Cmd {
			if mode == keepAwakeSystem {
				return exec.Command("/definitely/not/a/command")
			}
			return sleepCommand("")
		},
	}
	defer guard.Close()
	guard.SetMode(keepAwakeDisplay)
	if guard.mode != keepAwakeDisplay || guard.cmd == nil {
		t.Fatal("display mode did not start")
	}
	pid := guard.cmd.Process.Pid
	guard.SetMode(keepAwakeSystem)
	if guard.mode != keepAwakeDisplay || guard.cmd == nil || guard.cmd.Process.Pid != pid {
		t.Fatal("failed switch dropped the running assertion")
	}
}

func TestCaffeinateCommandModes(t *testing.T) {
	for _, tt := range []struct {
		mode     keepAwakeMode
		display  bool
		wantArgs []string
	}{
		{mode: keepAwakeDisplay, display: true, wantArgs: []string{"-d", "-i", "-w"}},
		{mode: keepAwakeSystem, wantArgs: []string{"-i", "-w"}},
	} {
		args := caffeinateCommand(tt.mode).Args
		if len(args) < 2 {
			t.Fatalf("%s args = %v", tt.mode, args)
		}
		got := args[1 : len(args)-1]
		if strings.Join(got, " ") != strings.Join(tt.wantArgs, " ") {
			t.Fatalf("%s flags = %v, want %v", tt.mode, got, tt.wantArgs)
		}
		if display := args[1] == "-d"; display != tt.display {
			t.Fatalf("%s display = %v", tt.mode, display)
		}
	}
}

func TestKeepAwakeModeFollowsPlaybackAndMinimize(t *testing.T) {
	newEngine := func() *engine.Engine {
		e := engine.New(func(string, bool, string) (engine.Backend, error) {
			return dummy.New(), nil
		})
		if err := e.Load("movie.mp4"); err != nil {
			t.Fatal(err)
		}
		// The dummy backend reports Playing on its next Tick; the engine's
		// optimistic Play sets the phase without one.
		e.Play()
		return e
	}
	playing := newEngine()
	paused := newEngine()
	paused.Pause()
	defer playing.Close()
	defer paused.Close()
	if got := playing.Snapshot().Phase; got != engine.Playing {
		t.Fatalf("playing phase = %s", got)
	}
	if got := paused.Snapshot().Phase; got != engine.Paused {
		t.Fatalf("paused phase = %s", got)
	}
	var modes []keepAwakeMode
	a := &app{
		inhibitScreensaver: true,
		cells:              []*cell{{eng: playing}, {eng: paused}, {}},
	}
	a.screensaver.command = func(mode keepAwakeMode) *exec.Cmd {
		modes = append(modes, mode)
		return sleepCommand("")
	}
	defer a.screensaver.Close()
	a.refreshKeepAwake()
	if a.screensaver.mode != keepAwakeDisplay {
		t.Fatalf("playing mode = %s, want display", a.screensaver.mode)
	}
	a.minimized = true
	a.refreshKeepAwake()
	if a.screensaver.mode != keepAwakeSystem {
		t.Fatalf("minimized mode = %s, want system", a.screensaver.mode)
	}
	a.minimized = false
	playing.Pause()
	a.refreshKeepAwake()
	if a.screensaver.mode != keepAwakeNone {
		t.Fatalf("paused mode = %s, want none", a.screensaver.mode)
	}
	a.inhibitScreensaver = false
	playing.Play()
	a.refreshKeepAwake()
	if a.screensaver.mode != keepAwakeNone {
		t.Fatalf("disabled mode = %s, want none", a.screensaver.mode)
	}
	if len(modes) != 2 || modes[0] != keepAwakeDisplay || modes[1] != keepAwakeSystem {
		t.Fatalf("started modes = %v", modeStrings(modes))
	}
}
