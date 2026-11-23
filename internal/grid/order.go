package grid

// DefaultOrder returns cell indices in back-to-front draw order.
func DefaultOrder(n int) []int {
	if n <= 0 {
		return nil
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	return order
}

// NormalizeOrder returns a valid permutation, rejecting missing, duplicate,
// or out-of-range persisted indices rather than dropping cells from drawing.
func NormalizeOrder(order []int, n int) []int {
	if len(order) != n {
		return DefaultOrder(n)
	}
	seen := make([]bool, n)
	for _, index := range order {
		if index < 0 || index >= n || seen[index] {
			return DefaultOrder(n)
		}
		seen[index] = true
	}
	return append([]int(nil), order...)
}

// Restack keeps unlocked tiles behind locked tiles without changing relative
// order inside either tier.
func Restack(order []int, n int, locked map[int]bool) []int {
	order = NormalizeOrder(order, n)
	out := make([]int, 0, n)
	for _, index := range order {
		if !locked[index] {
			out = append(out, index)
		}
	}
	for _, index := range order {
		if locked[index] {
			out = append(out, index)
		}
	}
	return out
}

// Stack moves index within its locked/unlocked tier. Locked tiles always
// remain in front of unlocked tiles, matching upstream _restack.
func Stack(order []int, index int, front bool, locked map[int]bool) []int {
	n := len(order)
	order = Restack(order, n, locked)
	if index < 0 || index >= n {
		return order
	}
	tier := locked[index]
	kept := make([]int, 0, n-1)
	for _, current := range order {
		if current != index {
			kept = append(kept, current)
		}
	}
	position := 0
	switch {
	case front && tier:
		// A front locked tile follows every other tile.
		position = len(kept)
	case front:
		// A front unlocked tile is immediately before the locked tier.
		position = len(kept)
		for i, current := range kept {
			if locked[current] {
				position = i
				break
			}
		}
	case tier:
		// A back locked tile is the first locked tile, behind its peers
		// but still in front of every unlocked tile.
		position = len(kept)
		for i, current := range kept {
			if locked[current] {
				position = i
				break
			}
		}
	default:
		// A back unlocked tile is the first tile on the page.
		position = 0
	}
	out := make([]int, 0, n)
	out = append(out, kept[:position]...)
	out = append(out, index)
	out = append(out, kept[position:]...)
	return out
}

// CanStack reports a visible front/back action: an overlapping tile of the
// same lock tier is respectively in front of or behind index.
func CanStack(order []int, tiles []Tile, index int, front bool, locked map[int]bool) bool {
	if len(order) != len(tiles) || index < 0 || index >= len(tiles) {
		return false
	}
	position := -1
	for i, current := range order {
		if current == index {
			position = i
			break
		}
	}
	if position < 0 {
		return false
	}
	for i, current := range order {
		if current == index || locked[current] != locked[index] {
			continue
		}
		if front && i > position || !front && i < position {
			if boxesOverlap(tiles[index].Box, tiles[current].Box) {
				return true
			}
		}
	}
	return false
}

// Topmost returns the last back-to-front tile containing a unit-page point.
func Topmost(order []int, tiles []Tile, x, y float64) int {
	if len(order) != len(tiles) {
		return -1
	}
	found := -1
	for _, index := range order {
		if index < 0 || index >= len(tiles) {
			return -1
		}
		b := tiles[index].Box
		if x >= b[0] && x <= b[2] && y >= b[1] && y <= b[3] {
			found = index
		}
	}
	return found
}
