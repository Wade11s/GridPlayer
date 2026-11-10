package main

import (
	"log"
	"os/exec"
	"sync"
)

type screensaverGuard struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	active bool
}

func (g *screensaverGuard) Enable(enabled bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if enabled == g.active {
		return
	}
	g.active = enabled
	if !enabled {
		if g.cmd != nil && g.cmd.Process != nil {
			_ = g.cmd.Process.Kill()
		}
		g.cmd = nil
		return
	}
	cmd := exec.Command("caffeinate", "-d", "-i")
	if err := cmd.Start(); err != nil {
		log.Printf("screensaver: %v", err)
		return
	}
	g.cmd = cmd
	go func() {
		_ = cmd.Wait()
	}()
}

func (g *screensaverGuard) Close() {
	g.Enable(false)
}
