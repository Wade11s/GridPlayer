package grid

// SnapKind is one thing a tile snaps to, as the frozen TileSnap names them.
type SnapKind uint8

const (
	SnapTileEdges SnapKind = 1 << iota
	SnapTileMiddles
	SnapWindowEdges
	SnapWindowMiddle
	SnapOldPlace
	SnapVideoShape
	SnapEqualGaps
)

// SnapNames are the frozen names, in menu order.
var SnapNames = []string{
	"tile_edges",
	"tile_middles",
	"window_edges",
	"window_middle",
	"old_place",
	"video_shape",
	"equal_gaps",
}

// SnapKinds is the set of snaps a move or resize uses.
type SnapKinds uint8

// DefaultSnaps is the frozen default: everything but the equal gaps.
const DefaultSnaps = SnapKinds(SnapTileEdges | SnapTileMiddles | SnapWindowEdges |
	SnapWindowMiddle | SnapOldPlace | SnapVideoShape)

// Has reports whether the set holds a kind.
func (k SnapKinds) Has(kind SnapKind) bool {
	return SnapKinds(kind)&k != 0
}

// With returns the set with a kind added or removed.
func (k SnapKinds) With(kind SnapKind, on bool) SnapKinds {
	if on {
		return k | SnapKinds(kind)
	}
	return k &^ SnapKinds(kind)
}

// ParseSnapKinds reads the frozen setting's list of names. An unknown name is
// kept out rather than guessed at.
func ParseSnapKinds(names []string) SnapKinds {
	var out SnapKinds
	for _, name := range names {
		for i, known := range SnapNames {
			if known == name {
				out |= SnapKinds(1 << i)
			}
		}
	}
	return out
}

// Names returns the set's frozen names, in menu order. It is never nil, so
// the frozen JSON array of an empty set is [] and not null.
func (k SnapKinds) Names() []string {
	out := []string{}
	for i, name := range SnapNames {
		if k.Has(SnapKind(1 << i)) {
			out = append(out, name)
		}
	}
	return out
}
