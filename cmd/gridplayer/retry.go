package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	retryOff int32 = iota
	retryTimes
	retryInfinite

	defaultRetryTimes = 3
	maxRetryTimes     = 1000
)

// NETWORK_RETRY_DELAYS_S from the frozen source.
var retryDelays = []time.Duration{
	time.Second,
	2 * time.Second,
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
}

func retryable(path string) bool {
	u, err := url.Parse(path)
	if err != nil || u.Scheme == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "rtsp", "rtmp", "udp", "mms", "srt":
		return true
	default:
		return false
	}
}

func retryDelay(attempt int32) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	idx := int(attempt - 1)
	if idx >= len(retryDelays) {
		idx = len(retryDelays) - 1
	}
	return retryDelays[idx]
}

func (a *app) setRetryPolicy(mode string, times int) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "infinite", "keep", "keep trying":
		a.retryMode.Store(retryInfinite)
	case "off", "none":
		a.retryMode.Store(retryOff)
	default:
		a.retryMode.Store(retryTimes)
	}
	if times < 1 {
		times = 1
	}
	if times > maxRetryTimes {
		times = maxRetryTimes
	}
	a.retryTimes.Store(int32(times))
}

func (a *app) networkRetryLimit() int {
	switch a.retryMode.Load() {
	case retryInfinite:
		return -1
	case retryOff:
		return 0
	default:
		n := int(a.retryTimes.Load())
		if n < 1 {
			return defaultRetryTimes
		}
		if n > maxRetryTimes {
			return maxRetryTimes
		}
		return n
	}
}

func retryModeName(mode int32) string {
	switch mode {
	case retryInfinite:
		return "Keep trying"
	case retryOff:
		return "Off"
	default:
		return "Fixed attempts"
	}
}

func retryModeValue(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "off", "none":
		return "off"
	case "keep trying", "infinite", "keep":
		return "infinite"
	default:
		return "times"
	}
}

func retryStatus(name string, delay time.Duration, attempt int32, limit int) string {
	if limit < 0 {
		return fmt.Sprintf("retrying %s in %s (try %d)", name, delay, attempt)
	}
	return fmt.Sprintf("retrying %s in %s (%d/%d)", name, delay, attempt, limit)
}
