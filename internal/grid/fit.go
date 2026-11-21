package grid

import (
	"math"
	"sort"
)

// Fit Shapes (upstream AUTO_SHAPES): every video gets a tile of its own
// shape, lines end flush, and the whole group is centred in the area. The
// split into lines and the choice of rows or columns are the one that shows
// the most picture, following upstream's justified layout (splits that no
// other beats on both line height and shown area, scored by tile area).

const (
	fitHeightStep    = 0.01
	fitDefaultAspect = 16.0 / 9.0
	fitMinAspect     = 0.05
	fitMaxAspect     = 20.0
)

type fitSplit struct {
	lengths    []int
	height     float64 // sum of 1/lineWeight
	logHeights float64 // sum over lines of length*log(lineWeight)
}

type fitState struct {
	height    float64
	lessLog   float64 // negative of logHeights
	start     int
	prevIndex int
}

// bestFitSplits returns the splits of weights into flush lines that are not
// dominated on both total height and shown line area, mirroring upstream's
// best_splits without the size-worth adjustments this build does not use.
func bestFitSplits(weights []float64) []fitSplit {
	n := len(weights)
	if n == 0 {
		return nil
	}
	prefix := make([]float64, n+1)
	for i, w := range weights {
		prefix[i+1] = prefix[i] + w
	}

	splits := make([][]fitState, n+1)
	splits[0] = []fitState{{}}
	for end := 1; end <= n; end++ {
		var found []fitState
		for start := 0; start < end; start++ {
			lineWeight := prefix[end] - prefix[start]
			height := 1 / lineWeight
			lessLog := float64(end-start) * math.Log(lineWeight)
			for index, prev := range splits[start] {
				found = append(found, fitState{
					height:    prev.height + height,
					lessLog:   prev.lessLog + lessLog,
					start:     start,
					prevIndex: index,
				})
			}
		}
		sort.SliceStable(found, func(i, j int) bool {
			if found[i].height != found[j].height {
				return found[i].height < found[j].height
			}
			if found[i].lessLog != found[j].lessLog {
				return found[i].lessLog < found[j].lessLog
			}
			if found[i].start != found[j].start {
				return found[i].start < found[j].start
			}
			return found[i].prevIndex < found[j].prevIndex
		})
		var kept []fitState
		stepHeight := 0.0
		for _, candidate := range found {
			if len(kept) > 0 && candidate.lessLog >= kept[len(kept)-1].lessLog {
				continue
			}
			if len(kept) > 0 && candidate.height <= stepHeight {
				kept[len(kept)-1] = candidate
				continue
			}
			kept = append(kept, candidate)
			stepHeight = candidate.height * (1 + fitHeightStep)
		}
		splits[end] = kept
	}

	out := make([]fitSplit, 0, len(splits[n]))
	for index, state := range splits[n] {
		lengths := make([]int, 0, n)
		end, at := n, index
		for end > 0 {
			prev := splits[end][at]
			lengths = append(lengths, end-prev.start)
			end, at = prev.start, prev.prevIndex
		}
		for i, j := 0, len(lengths)-1; i < j; i, j = i+1, j-1 {
			lengths[i], lengths[j] = lengths[j], lengths[i]
		}
		out = append(out, fitSplit{lengths: lengths, height: state.height, logHeights: -state.lessLog})
	}
	return out
}

func measureFitSplit(weights []float64, lengths []int) fitSplit {
	height, logHeights := 0.0, 0.0
	start := 0
	for _, length := range lengths {
		lineWeight := 0.0
		for _, w := range weights[start : start+length] {
			lineWeight += w
		}
		height += 1 / lineWeight
		logHeights += float64(length) * math.Log(lineWeight)
		start += length
	}
	return fitSplit{lengths: lengths, height: height, logHeights: logHeights}
}

// fitScore is upstream split_score without crop or size worth: the sum of
// the logs of the line heights, plus the area the lines get in this area.
func fitScore(split fitSplit, weights []float64, width, height float64) float64 {
	logWeights := 0.0
	for _, w := range weights {
		logWeights += math.Log(w)
	}
	count := float64(len(weights))
	logWidth, logHeight := math.Log(width), math.Log(height)
	logLinesHeight := math.Log(split.height)
	shownWidth := math.Min(logWidth, logHeight-logLinesHeight)
	// With no crop, upstream's stretch term is min(abs(leftover), 0) = 0.
	return logWeights + 2*split.logHeights + 2*count*shownWidth
}

type fitChoice struct {
	split   fitSplit
	weights []float64
	columns bool
	score   float64
}

func bestFit(weights []float64, area Rect) fitChoice {
	best := fitChoice{score: math.Inf(-1)}
	for _, columns := range []bool{false, true} {
		lineWeights := append([]float64(nil), weights...)
		width, height := area.W, area.H
		if columns {
			for i, w := range lineWeights {
				lineWeights[i] = 1 / w
			}
			width, height = height, width
		}
		for _, split := range bestFitSplits(lineWeights) {
			score := fitScore(split, lineWeights, width, height)
			if score > best.score {
				best = fitChoice{split: split, weights: lineWeights, columns: columns, score: score}
			}
		}
	}
	return best
}

// fitBoxes lays weights out in rows of the split and returns fractions of the
// logical width/height. With no crop, every tile keeps its weight's shape and
// the rows are centred in the leftover space.
func fitBoxes(weights []float64, lengths []int, width, height float64) [][4]float64 {
	linesHeight := measureFitSplit(weights, lengths).height
	linesWidth := math.Min(width, height/linesHeight)
	shownHeight := linesWidth * linesHeight
	left := (width - linesWidth) / 2
	top := (height - shownHeight) / 2
	boxes := make([][4]float64, 0, len(weights))
	start := 0
	y := top
	for _, length := range lengths {
		row := weights[start : start+length]
		start += length
		lineWeight := 0.0
		for _, w := range row {
			lineWeight += w
		}
		unit := linesWidth / lineWeight
		x := left
		for place, weight := range row {
			right := left + linesWidth
			if place != length-1 {
				right = x + unit*weight
			}
			boxes = append(boxes, [4]float64{x / width, y / height, right / width, (y + unit) / height})
			x = right
		}
		y += unit
	}
	return boxes
}

// FitShapes returns one rect per cell in a justified layout that gives each
// cell the shape of its video. shapes holds width/height per cell; a missing,
// zero or negative shape is taken as 16:9. The group is centred in area and
// every cell is inset by half the gap.
func FitShapes(n int, area Rect, gap float64, shapes []float64) []Rect {
	if n <= 0 || area.W <= 0 || area.H <= 0 {
		return nil
	}
	weights := make([]float64, n)
	for i := range weights {
		weights[i] = fitDefaultAspect
		if i < len(shapes) && shapes[i] > 0 && !math.IsInf(shapes[i], 0) && !math.IsNaN(shapes[i]) {
			weights[i] = shapes[i]
		}
		if weights[i] < fitMinAspect {
			weights[i] = fitMinAspect
		}
		if weights[i] > fitMaxAspect {
			weights[i] = fitMaxAspect
		}
	}
	choice := bestFit(weights, area)
	width, height := area.W, area.H
	if choice.columns {
		width, height = height, width
	}
	boxes := fitBoxes(choice.weights, choice.split.lengths, width, height)
	if choice.columns {
		for i, b := range boxes {
			boxes[i] = [4]float64{b[1], b[0], b[3], b[2]}
		}
	}
	out := make([]Rect, len(boxes))
	for i, b := range boxes {
		r := Rect{
			X: area.X + b[0]*area.W,
			Y: area.Y + b[1]*area.H,
			W: (b[2] - b[0]) * area.W,
			H: (b[3] - b[1]) * area.H,
		}
		if gap > 0 {
			r = inset(r, gap/2)
		}
		out[i] = r
	}
	return out
}
