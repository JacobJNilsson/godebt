package duplication

import (
	"math/rand"
	"slices"
	"testing"
)

// neighbours returns the largest member of the sorted slice below x and the smallest above x, with -1 for none.
func neighbours(sorted []int32, x int32) (int32, int32) {
	below, above := int32(-1), int32(-1)
	for _, v := range sorted {
		if v < x {
			below = v
		}
		if v > x && above < 0 {
			above = v
		}
	}
	return below, above
}

// TestPosSetMatchesSortedSlice applies random inserts and removes to a set and a sorted slice, and compares every neighbour query.
func TestPosSetMatchesSortedSlice(t *testing.T) {
	for _, size := range []int{1, 63, 64, 65, 4096, 4097, 20000, 262144, 262145} {
		rng := rand.New(rand.NewSource(int64(size)))
		set := newPosSet(size)
		var members []int32
		for step := range 600 {
			x := int32(rng.Intn(size))
			if at, found := slices.BinarySearch(members, x); found {
				members = slices.Delete(members, at, at+1)
				set.remove(x)
			} else {
				members = slices.Insert(members, at, x)
				set.insert(x)
			}
			q := int32(rng.Intn(size))
			below, above := neighbours(members, q)
			if got := set.prev(q); got != below {
				t.Fatalf("size %d step %d: prev(%d) = %d, want %d", size, step, q, got, below)
			}
			if got := set.next(q); got != above {
				t.Fatalf("size %d step %d: next(%d) = %d, want %d", size, step, q, got, above)
			}
		}
	}
}

func TestPosSetEmptyHasNoNeighbours(t *testing.T) {
	set := newPosSet(100)
	if set.next(0) != -1 || set.prev(99) != -1 {
		t.Error("an empty set returned a neighbour")
	}
}
