package main

const (
	endLoop     = "loop"
	endNext     = "next"
	endPrevious = "previous"
	endShuffle  = "shuffle"
	endPause    = "pause"
	endStop     = "stop"
)

func normalizeEndAction(action string) string {
	switch action {
	case endLoop, "loop_file":
		return endLoop
	case endPrevious, "previous_file":
		return endPrevious
	case endShuffle, "shuffle_file":
		return endShuffle
	case endPause:
		return endPause
	case endStop:
		return endStop
	case endNext, "next_file":
		return endNext
	default:
		return ""
	}
}

// finishAction is what this cell does when its current item ends.
// A list with no explicit action keeps the shipped next-item step.
func (c *cell) finishAction() string {
	if c == nil {
		return endStop
	}
	if action := normalizeEndAction(c.endAction); action != "" {
		return action
	}
	if len(c.list) > 0 && !c.loop {
		return endNext
	}
	if c.loop {
		return endLoop
	}
	return endStop
}

func endActionName(action string) string {
	switch normalizeEndAction(action) {
	case endNext:
		return "Next file"
	case endPrevious:
		return "Previous file"
	case endShuffle:
		return "Shuffle"
	case endPause:
		return "Pause"
	case endStop:
		return "Stop"
	default:
		return "Loop file"
	}
}

func endActionValue(name string) string {
	switch name {
	case "Next file":
		return endNext
	case "Previous file":
		return endPrevious
	case "Shuffle":
		return endShuffle
	case "Pause":
		return endPause
	case "Stop":
		return endStop
	default:
		return endLoop
	}
}

// pickShuffle returns an index other than current when the list has room.
func pickShuffle(n, current int, next func(int) int) int {
	if n <= 1 || next == nil {
		return current
	}
	for {
		i := next(n)
		if i != current && i >= 0 && i < n {
			return i
		}
	}
}
