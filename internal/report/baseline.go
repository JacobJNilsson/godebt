package report

import "strconv"

// Baseline is what the renderers print about a comparable baseline, since this package cannot import the comparison.
type Baseline struct {
	Index    int
	Delta    int
	New      int
	Resolved int
	Changed  int
}

// Signed prints a delta with its sign, so a rise reads as +3 and no change as 0.
func Signed(delta int) string {
	if delta > 0 {
		return "+" + strconv.Itoa(delta)
	}
	return strconv.Itoa(delta)
}
