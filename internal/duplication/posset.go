package duplication

import "math/bits"

// posSet is a set of integers below a fixed size, with insert, remove, and neighbour queries in a constant number of word operations per level.
// Each bit of a level marks a non-empty word of the level below.
type posSet struct{ levels [][]uint64 }

func newPosSet(size int) *posSet {
	s := &posSet{}
	for n := size; ; n = (n + 63) / 64 {
		s.levels = append(s.levels, make([]uint64, (n+63)/64))
		if n <= 64 {
			return s
		}
	}
}

func (s *posSet) insert(x int32) {
	for _, level := range s.levels {
		level[x>>6] |= 1 << (x & 63)
		x >>= 6
	}
}

func (s *posSet) remove(x int32) {
	for _, level := range s.levels {
		level[x>>6] &^= 1 << (x & 63)
		if level[x>>6] != 0 {
			return
		}
		x >>= 6
	}
}

// next returns the smallest member above x, or -1.
func (s *posSet) next(x int32) int32 {
	for l, level := range s.levels {
		if w := level[x>>6] & (^uint64(0) << (x & 63) << 1); w != 0 {
			x = x&^63 + int32(bits.TrailingZeros64(w))
			for ; l > 0; l-- {
				x = x<<6 + int32(bits.TrailingZeros64(s.levels[l-1][x]))
			}
			return x
		}
		x >>= 6
	}
	return -1
}

// prev returns the largest member below x, or -1.
func (s *posSet) prev(x int32) int32 {
	for l, level := range s.levels {
		if w := level[x>>6] & (1<<(x&63) - 1); w != 0 {
			x = x&^63 + int32(63-bits.LeadingZeros64(w))
			for ; l > 0; l-- {
				x = x<<6 + int32(63-bits.LeadingZeros64(s.levels[l-1][x]))
			}
			return x
		}
		x >>= 6
	}
	return -1
}
