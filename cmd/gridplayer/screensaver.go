package main

import (
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

type screensaverGuard struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	active  bool
	command func() *exec.Cmd
}

func (g *screensaverGuard) Enable(enabled bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if enabled == g.active {
		return
	}
	if !enabled {
		if g.cmd != nil && g.cmd.Process != nil {
			_ = g.cmd.Process.Kill()
		}
		g.cmd = nil
		g.active = false
		return
	}
	cmd := (*exec.Cmd)(nil)
	if g.command != nil {
		cmd = g.command()
	} else {
		path, err := exec.LookPath("caffeinate")
		if err != nil {
			path = "/usr/bin/caffeinate"
		}
		cmd = exec.Command(path, "-d", "-i", "-w", strconv.Itoa(os.Getpid()))
	}
	if err := cmd.Start(); err != nil {
		log.Printf("screensaver: %v", err)
		g.cmd = nil
		g.active = false
		return
	}
	g.cmd = cmd
	g.active = true
	go func() {
		_ = cmd.Wait()
	}()
}

func (g *screensaverGuard) Close() {
	g.Enable(false)
}
