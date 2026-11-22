package main

// shuffleBag tracks the part of upstream's ShuffleBag needed for automatic
// list completion: every current list index is played once before the next
// round or list-end action. The UI list itself is immutable between edits.
type shuffleBag struct {
	initialized bool
	listLen     int
	current     int
	pending     []int
}

func (b *shuffleBag) sync(n, current int, random func(int) int) {
	if n <= 0 || current < 0 || current >= n {
		b.initialized = false
		b.pending = nil
		return
	}
	if !b.initialized || b.listLen != n {
		b.initialized = true
		b.listLen = n
		b.current = current
		b.refill(random)
		return
	}
	if b.current != current {
		b.current = current
		kept := b.pending[:0]
		for _, index := range b.pending {
			if index != current {
				kept = append(kept, index)
			}
		}
		b.pending = kept
	}
}

func (b *shuffleBag) roundOver() bool {
	return b.initialized && len(b.pending) == 0
}

func (b *shuffleBag) next(random func(int) int) int {
	if !b.initialized || b.current < 0 || b.current >= b.listLen {
		return -1
	}
	if len(b.pending) == 0 {
		b.refill(random)
	}
	if len(b.pending) == 0 {
		return b.current
	}
	index := b.pending[0]
	b.pending = b.pending[1:]
	b.current = index
	return index
}

func (b *shuffleBag) refill(random func(int) int) {
	b.pending = b.pending[:0]
	for i := 0; i < b.listLen; i++ {
		if i != b.current {
			b.pending = append(b.pending, i)
		}
	}
	if random == nil {
		return
	}
	for i := len(b.pending) - 1; i > 0; i-- {
		j := random(i + 1)
		if j >= 0 && j <= i {
			b.pending[i], b.pending[j] = b.pending[j], b.pending[i]
		}
	}
}
