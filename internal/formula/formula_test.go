package formula

import (
	"math"
	"strings"
	"testing"
)

func TestTableWellFormed(t *testing.T) {
	const tolerance = 1e-12
	weights := 0.0
	for _, d := range Dimensions {
		weights += d.Weight
		shares := 0.0
		for _, term := range d.Terms {
			shares += term.Share
			if (term.SaturatesAt == 0) == (term.CountScale == 0) {
				t.Errorf("%s: set exactly one of SaturatesAt and CountScale", term.MetricID)
			}
			if !strings.HasSuffix(term.MetricID, ".production") {
				t.Errorf("%s: metric id lacks the .production suffix", term.MetricID)
			}
		}
		if math.Abs(shares-1) > tolerance {
			t.Errorf("%s: shares sum to %v, want 1", d.ID, shares)
		}
	}
	if math.Abs(weights-1) > tolerance {
		t.Errorf("weights sum to %v, want 1", weights)
	}
	for i := 1; i < len(Dimensions); i++ {
		if Dimensions[i-1].ID >= Dimensions[i].ID {
			t.Errorf("dimensions not sorted by id: %s before %s", Dimensions[i-1].ID, Dimensions[i].ID)
		}
	}
}

func TestTableMatchesSpec(t *testing.T) {
	want := map[string]float64{"complexity-erosion": 0.6, "duplication": 0.4}
	if len(Dimensions) != len(want) {
		t.Fatalf("got %d dimensions, want %d", len(Dimensions), len(want))
	}
	for _, d := range Dimensions {
		if d.Weight != want[d.ID] {
			t.Errorf("%s: weight %v, want %v", d.ID, d.Weight, want[d.ID])
		}
		if len(d.Terms) != 2 {
			t.Errorf("%s: %d terms, want 2", d.ID, len(d.Terms))
		}
	}
	if ScoringVersion != "0.3.0" {
		t.Errorf("ScoringVersion = %q", ScoringVersion)
	}
}
