package main

import (
	"fmt"
	"time"
)

const (
	listFailShowError = "show_error"
	listFailPlayNext  = "play_next"

	defaultListFailWait      = 2
	maxListFailWaitSec       = 300
	maxNetworkFailuresInARow = 10
)

const (
	listFailNext int32 = iota + 1
	listFailShow
)

// decideListFail applies the frozen list-fail policy. failures and network
// are the streaks before this failure. A local file is not a network failure.
// play_next is the default. The skip stops after the whole list has failed,
// or after more than 10 network failures in a row.
func decideListFail(path string, items int, action string, waitSec, failures, network int) (skip bool, wait time.Duration, status string, failuresOut, networkOut int) {
	if items <= 0 {
		return false, 0, "", failures, network
	}
	failures++
	if retryable(path) {
		network++
	}
	switch action {
	case listFailShowError, "show", "off":
		return false, 0, "", failures, network
	default:
		action = listFailPlayNext
	}
	if network > maxNetworkFailuresInARow {
		return false, 0, fmt.Sprintf("Stopped: %d failed in a row", failures), failures, network
	}
	if failures > items {
		return false, 0, "Stopped: nothing on the list plays", failures, network
	}
	if waitSec < 0 {
		waitSec = defaultListFailWait
	}
	if waitSec > maxListFailWaitSec {
		waitSec = maxListFailWaitSec
	}
	if waitSec > 0 {
		status = fmt.Sprintf("Next file in %ds", waitSec)
	}
	return true, time.Duration(waitSec) * time.Second, status, failures, network
}

func (a *app) setListFailPolicy(action string, waitSec int) {
	switch action {
	case listFailShowError, "show", "off":
		a.listFailMode.Store(listFailShow)
	default:
		a.listFailMode.Store(listFailNext)
	}
	if waitSec < 0 {
		waitSec = 0
	}
	if waitSec > maxListFailWaitSec {
		waitSec = maxListFailWaitSec
	}
	a.listFailWait.Store(int32(waitSec))
}

func (a *app) listFailAction(c *cell) string {
	if c != nil && c.listFail != "" {
		if c.listFail == listFailShowError {
			return listFailShowError
		}
		return listFailPlayNext
	}
	if a != nil && a.listFailMode.Load() == listFailShow {
		return listFailShowError
	}
	return listFailPlayNext
}

func (a *app) listFailWaitSec(c *cell) int {
	if c != nil && c.listFailWait != nil {
		wait := *c.listFailWait
		if wait < 0 {
			wait = 0
		}
		if wait > maxListFailWaitSec {
			wait = maxListFailWaitSec
		}
		return wait
	}
	if a == nil || a.listFailMode.Load() == 0 {
		return defaultListFailWait
	}
	return int(a.listFailWait.Load())
}

func listFailName(action string) string {
	if action == listFailShowError {
		return "Show error"
	}
	return "Play next"
}

func listFailValue(name string) string {
	switch name {
	case "Show error", listFailShowError, "show", "off":
		return listFailShowError
	default:
		return listFailPlayNext
	}
}

// noteListFailure records one failed list item and reports whether to move on.
// Main thread only.
func (a *app) noteListFailure(c *cell, path string, items int) (bool, time.Duration, string) {
	if c == nil {
		return false, 0, ""
	}
	skip, wait, status, failures, network := decideListFail(path, items, a.listFailAction(c), a.listFailWaitSec(c), c.failStreak, c.netFailStreak)
	c.failStreak = failures
	c.netFailStreak = network
	return skip, wait, status
}
