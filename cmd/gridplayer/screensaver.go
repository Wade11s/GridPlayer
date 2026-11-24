package main

import (
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

// keepAwakeMode is the frozen 0.5.5 KeepAwakeMode: nothing held, the
// system kept awake while the screen may go off, or both kept awake.
type keepAwakeMode int

const (
	keepAwakeNone keepAwakeMode = iota
	keepAwakeSystem
	keepAwakeDisplay
)

func (m keepAwakeMode) String() string {
	switch m {
	case keepAwakeSystem:
		return "system"
	case keepAwakeDisplay:
		return "display"
	default:
		return "none"
	}
}

// screensaverGuard holds at most one keep-awake process, started for the
// requested mode. Every mode change starts the new process before the old
// one is killed, so there is no gap in the assertion.
type screensaverGuard struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	mode    keepAwakeMode
	command func(keepAwakeMode) *exec.Cmd
}

func (g *screensaverGuard) SetMode(mode keepAwakeMode) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if mode == g.mode {
		return
	}
	old := g.cmd
	g.cmd = nil
	if mode == keepAwakeNone {
		if old != nil && old.Process != nil {
			_ = old.Process.Kill()
		}
		g.mode = keepAwakeNone
		log.Printf("keep awake: %s", mode)
		return
	}
	cmd := (*exec.Cmd)(nil)
	if g.command != nil {
		cmd = g.command(mode)
	} else {
		cmd = caffeinateCommand(mode)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("screensaver: %v", err)
		if old != nil && old.Process != nil {
			// The previous mode still holds; keep it rather than drop to
			// none when the new assertion could not start.
			g.cmd = old
			return
		}
		g.mode = keepAwakeNone
		return
	}
	g.cmd = cmd
	g.mode = mode
	log.Printf("keep awake: %s", mode)
	go func() {
		_ = cmd.Wait()
	}()
	if old != nil && old.Process != nil {
		_ = old.Process.Kill()
	}
}

func (g *screensaverGuard) Close() {
	g.SetMode(keepAwakeNone)
}

// caffeinateCommand mirrors the frozen macOS assertions: -i keeps the
// system awake, -d also keeps the display awake, and -w bounds lifetime
// to this process.
func caffeinateCommand(mode keepAwakeMode) *exec.Cmd {
	path, err := exec.LookPath("caffeinate")
	if err != nil {
		path = "/usr/bin/caffeinate"
	}
	args := make([]string, 0, 4)
	if mode == keepAwakeDisplay {
		args = append(args, "-d")
	}
	args = append(args, "-i", "-w", strconv.Itoa(os.Getpid()))
	return exec.Command(path, args...)
}
