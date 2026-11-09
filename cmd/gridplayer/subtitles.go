package main

import (
	"fmt"
	"strconv"
	"strings"
)

type subtitleStyle struct {
	Font         string
	Size         int
	Color        string
	OutlineColor string
	Outline      int
	Bold         bool
	Opacity      int
	Encoding     string
	Autodetect   bool
}

func (s subtitleStyle) args() []string {
	var args []string
	if s.Font != "" {
		args = append(args, "--freetype-font="+s.Font)
	}
	if s.Size > 0 {
		args = append(args, fmt.Sprintf("--freetype-fontsize=%d", s.Size))
	}
	if c := hexColor(s.Color); c != "" {
		args = append(args, "--freetype-color="+c)
	}
	if c := hexColor(s.OutlineColor); c != "" {
		args = append(args, "--freetype-outline-color="+c)
	}
	switch s.Outline {
	case 2, 4, 6:
		args = append(args, fmt.Sprintf("--freetype-outline-thickness=%d", s.Outline))
	}
	if s.Bold {
		args = append(args, "--freetype-bold")
	}
	if s.Opacity != 255 {
		args = append(args, fmt.Sprintf("--freetype-opacity=%d", s.Opacity))
	}
	if s.Encoding != "" {
		args = append(args, "--subsdec-encoding="+s.Encoding)
	}
	if s.Autodetect {
		args = append(args, "--sub-autodetect-file")
	}
	return args
}

func hexColor(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(s, "#"))
	if len(s) != 6 {
		return ""
	}
	v, err := strconv.ParseUint(s, 16, 24)
	if err != nil {
		return ""
	}
	return strconv.FormatUint(v, 10)
}

func subtitleOutlineValue(name string) int {
	switch name {
	case "Thin":
		return 2
	case "Normal":
		return 4
	case "Thick":
		return 6
	default:
		return 0
	}
}

func subtitleOutlineName(v int) string {
	switch v {
	case 2:
		return "Thin"
	case 4:
		return "Normal"
	case 6:
		return "Thick"
	default:
		return "None"
	}
}

func subtitleEncodingName(v string) string {
	if v == "" {
		return "Auto"
	}
	return v
}

func subtitleEncodingValue(name string) string {
	if name == "Auto" {
		return ""
	}
	return name
}
