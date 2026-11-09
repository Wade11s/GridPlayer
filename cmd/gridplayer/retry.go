package main

import (
	"net/url"
	"strings"
	"time"
)

const maxRetries = 3

func retryable(path string) bool {
	u, err := url.Parse(path)
	if err != nil {
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
	return time.Duration(1<<(attempt-1)) * time.Second
}
