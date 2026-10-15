package formula

import (
	"math"
	"testing"
)

// ulpDistance returns how many representable float64 values lie between a and b.
func ulpDistance(a, b float64) uint64 {
	x, y := math.Float64bits(a), math.Float64bits(b)
	if x > y {
		return x - y
	}
	return y - x
}

func TestNaturalLogAgreesWithMathLog(t *testing.T) {
	// math.Log has its own error, which adds to the error of naturalLog.
	const maxUlp = 3
	values := []float64{
		1, 1.5, 2, math.E, 1e6,
		1 + 1e-12, 1 + 1e-3,
		math.Sqrt2, math.Nextafter(math.Sqrt2, 0), math.Nextafter(math.Sqrt2, 2),
		1e300, math.MaxFloat64,
	}
	for i := range 10000 {
		values = append(values, math.Exp(700*float64(i)/9999))
	}
	for i := range 10000 {
		values = append(values, 1+float64(i)*(1e6-1)/9999)
	}
	for _, x := range values {
		if d := ulpDistance(naturalLog(x), math.Log(x)); d > maxUlp {
			t.Errorf("naturalLog(%v) = %v, math.Log = %v, %d ULP apart", x, naturalLog(x), math.Log(x), d)
		}
	}
}

func TestNaturalLogOfOneIsZero(t *testing.T) {
	if got := naturalLog(1); got != 0 {
		t.Errorf("naturalLog(1) = %v, want 0", got)
	}
}

// The expected bits come from this implementation. They hold on every architecture because CI runs amd64 and developers run arm64.
func TestNaturalLogHasTheSameBitsOnEveryPlatform(t *testing.T) {
	pins := []struct {
		x    float64
		want uint64
	}{
		{1e300, 0x4085963447f87fb5},
		{1 + 1e-12, 0x3d7197fffffff654},
	}
	for _, p := range pins {
		if bits := math.Float64bits(naturalLog(p.x)); bits != p.want {
			t.Errorf("naturalLog(%v) bits = %#016x, want %#016x", p.x, bits, p.want)
		}
	}
}

// The score pin comes from this implementation too.
func TestCountTermScoreHasTheSameBitsOnEveryPlatform(t *testing.T) {
	const want uint64 = 0x402901f7d42d323e
	got := termScore(Term{CountScale: 700}, 107.53604047016051)
	if bits := math.Float64bits(got); bits != want {
		t.Errorf("score bits = %#016x (%v), want %#016x", bits, got, want)
	}
}
