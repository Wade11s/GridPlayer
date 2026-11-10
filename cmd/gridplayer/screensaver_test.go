package main

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestScreensaverGuardDisableKillsChild(t *testing.T) {
	guard := &screensaverGuard{
		command: func() *exec.Cmd {
			return exec.Command("sh", "-c", "exec sleep 30")
		},
	}
	defer guard.Close()
	guard.Enable(true)
	if !guard.active || guard.cmd == nil || guard.cmd.Process == nil {
		t.Fatal("guard did not start")
	}
	pid := guard.cmd.Process.Pid
	guard.Enable(false)
	if guard.active || guard.cmd != nil {
		t.Fatal("guard did not stop")
	}
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
	guard := &screensaverGuard{
		command: func() *exec.Cmd {
			return exec.Command("/definitely/not/a/command")
		},
	}
	guard.Enable(true)
	if guard.active || guard.cmd != nil {
		t.Fatal("failed start left guard active")
	}
	guard.Enable(true)
	if guard.active || guard.cmd != nil {
		t.Fatal("retry left guard active")
	}
}
