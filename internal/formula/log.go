package formula

import "math"

// logTerms is the number of odd powers of s that naturalLog sums.
// It gives full double accuracy on [sqrt(0.5), sqrt(2)).
const logTerms = 14

// naturalLog returns ln(x) for finite x above zero.
// It uses only separately rounded operations, because math.Log differs between platforms.
func naturalLog(x float64) float64 {
	m, k := math.Frexp(x)
	if m < math.Sqrt2/2 {
		m *= 2
		k--
	}
	s := (m - 1) / (m + 1)
	s2 := float64(s * s)
	series := 1 / float64(2*logTerms-1)
	for n := logTerms - 1; n >= 1; n-- {
		series = float64(series*s2) + 1/float64(2*n-1)
	}
	return float64(float64(k)*math.Ln2) + float64(2*float64(s*series))
}
