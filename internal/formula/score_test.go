package formula

import (
	"math"
	"reflect"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

const (
	erodedShare = "erosion.eroded-share.production"
	erodedCount = "erosion.eroded-count.production"
	density     = "duplication.density.production"
	groups      = "duplication.groups.production"
)

func complete(v float64) contract.Metric {
	return contract.Metric{State: contract.Complete, Value: v}
}

func clean() map[string]contract.Metric {
	return map[string]contract.Metric{
		erodedShare: complete(0), erodedCount: complete(0), density: complete(0), groups: complete(0),
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.005 }

func scoreOf(t *testing.T, s contract.Score, id string) float64 {
	t.Helper()
	for _, c := range s.Contributions {
		for _, term := range c.Terms {
			if term.MetricID == id {
				return term.Score
			}
		}
	}
	t.Fatalf("no term %s", id)
	return 0
}

// The worked example: share term 22.5, count term 100 × ln(1.15)/(1+ln(1.15)) = 12.26, dimension 17.38, weighted 10.43, index 10.
func TestWorkedExample(t *testing.T) {
	m := clean()
	m[erodedShare] = complete(0.18)
	m[erodedCount] = complete(3)
	s := Score(m)
	if got := scoreOf(t, s, erodedShare); !near(got, 22.5) {
		t.Errorf("share term = %v, want 22.5", got)
	}
	if got := scoreOf(t, s, erodedCount); !near(got, 12.26) {
		t.Errorf("count term = %v, want 12.26", got)
	}
	if s.Index != 10 || s.Partial || s.Direction != "lower-is-better" {
		t.Errorf("score = %+v, want index 10, not partial, lower-is-better", s)
	}
	if len(s.Contributions) != 2 {
		t.Fatalf("got %d contributions, want 2", len(s.Contributions))
	}
	erosion, dup := s.Contributions[0], s.Contributions[1]
	if erosion.Dimension != "complexity-erosion" || erosion.Points != 10 || erosion.Weight != 0.6 {
		t.Errorf("erosion contribution = %+v", erosion)
	}
	if dup.Dimension != "duplication" || dup.Points != 0 || dup.Weight != 0.4 {
		t.Errorf("duplication contribution = %+v", dup)
	}
	term := erosion.Terms[0]
	if term.MetricID != erodedShare || term.State != contract.Complete || term.Value != 0.18 || term.SaturatesAt != 0.8 {
		t.Errorf("share term = %+v", term)
	}
	if count := erosion.Terms[1]; count.CountScale != 20 || count.SaturatesAt != 0 {
		t.Errorf("count term = %+v", count)
	}
}

func TestSaturatingTerm(t *testing.T) {
	for _, tc := range []struct{ value, want float64 }{{0, 0}, {0.4, 50}, {0.8, 100}, {0.9, 100}} {
		m := clean()
		m[erodedShare] = complete(tc.value)
		if got := scoreOf(t, Score(m), erodedShare); !near(got, tc.want) {
			t.Errorf("value %v: term = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestCountTerm(t *testing.T) {
	for _, count := range []float64{0, 1, 15, 150, 1e6} {
		m := clean()
		m[groups] = complete(count)
		b := math.Log(1 + count/15)
		want := 100 * b / (1 + b)
		if got := scoreOf(t, Score(m), groups); math.Abs(got-want) > 1e-9 {
			t.Errorf("count %v: term = %v, want %v", count, got, want)
		}
	}
	m := clean()
	m[groups] = complete(1e12)
	if got := scoreOf(t, Score(m), groups); got >= 100 {
		t.Errorf("count term reached %v, want below 100", got)
	}
}

func TestDimensionIsShareWeightedSum(t *testing.T) {
	m := clean()
	m[density] = complete(0.3)
	m[groups] = complete(15)
	b := math.Ln2
	dim := 0.5*100 + 0.5*100*b/(1+b)
	want := int(math.Floor(0.4*dim + 0.5))
	if s := Score(m); s.Index != want {
		t.Errorf("index = %d, want %d", s.Index, want)
	}
}

func TestIndexRoundsHalfUp(t *testing.T) {
	// The share term scores 12.5, the dimension 6.25, and the weighted sum 3.75, which rounds to 4.
	m := clean()
	m[erodedShare] = complete(0.1)
	if s := Score(m); s.Index != 4 {
		t.Errorf("index = %d, want 4", s.Index)
	}
	// A density of 0.0225 gives a weighted sum of exactly 1.5 in IEEE-754 doubles, which rounds up to 2.
	m = clean()
	m[density] = complete(0.0225)
	if s := Score(m); s.Index != 2 {
		t.Errorf("index = %d, want 2 from an exact half", s.Index)
	}
	if s := Score(clean()); s.Index != 0 || s.Partial {
		t.Errorf("clean score = %+v, want 0 and not partial", s)
	}
}

func TestContributionsSumToIndex(t *testing.T) {
	values := []float64{0, 0.01, 0.07, 0.13, 0.2, 0.31}
	counts := []float64{0, 1, 2, 7, 40}
	for _, share := range values {
		for _, dens := range values {
			for _, count := range counts {
				s := Score(map[string]contract.Metric{
					erodedShare: complete(share), erodedCount: complete(count),
					density: complete(dens), groups: complete(count),
				})
				total := 0
				for _, c := range s.Contributions {
					total += c.Points
				}
				if total != s.Index {
					t.Fatalf("share %v density %v count %v: contributions sum to %d, index %d", share, dens, count, total, s.Index)
				}
			}
		}
	}
}

func TestApportionmentTiesByDimensionID(t *testing.T) {
	if got := apportion([]float64{0.5, 0.5}, 1); !reflect.DeepEqual(got, []int{1, 0}) {
		t.Errorf("apportion = %v, want [1 0]", got)
	}
	if got := apportion([]float64{2.2, 3.7}, 6); !reflect.DeepEqual(got, []int{2, 4}) {
		t.Errorf("apportion = %v, want [2 4]", got)
	}
}

func TestNotApplicableRatioBesideZeroCount(t *testing.T) {
	m := clean()
	m[erodedShare] = contract.Metric{State: contract.NotApplicable}
	m[density] = contract.Metric{State: contract.NotApplicable}
	s := Score(m)
	if s.Index != 0 || s.Partial {
		t.Errorf("score = %+v, want 0 and not partial", s)
	}
	if got := scoreOf(t, s, erodedShare); got != 0 {
		t.Errorf("not-applicable term = %v, want 0", got)
	}
}

func TestNotApplicableRatioBesideNonzeroCountIsPartial(t *testing.T) {
	m := clean()
	m[erodedShare] = contract.Metric{State: contract.NotApplicable}
	m[erodedCount] = complete(2)
	if s := Score(m); !s.Partial || s.Contributions[0].Points != 60 {
		t.Errorf("score = %+v, want partial with erosion at 60 points", s)
	}
}

func TestIncompleteOrAbsentMetricScoresFullWeight(t *testing.T) {
	incomplete := clean()
	incomplete[groups] = contract.Metric{State: contract.Incomplete, Value: 3}
	absent := clean()
	delete(absent, erodedCount)
	for name, tc := range map[string]struct {
		metrics map[string]contract.Metric
		index   int
		points  []int
	}{
		"incomplete": {incomplete, 40, []int{0, 40}},
		"absent":     {absent, 60, []int{60, 0}},
	} {
		s := Score(tc.metrics)
		if !s.Partial || s.Index != tc.index {
			t.Errorf("%s: score = %+v, want partial index %d", name, s, tc.index)
		}
		for i, c := range s.Contributions {
			if c.Points != tc.points[i] {
				t.Errorf("%s: %s points = %d, want %d", name, c.Dimension, c.Points, tc.points[i])
			}
		}
	}
	term := Score(incomplete).Contributions[1].Terms[1]
	if term.State != contract.Incomplete || term.Value != 0 {
		t.Errorf("incomplete term = %+v, want state incomplete and no value", term)
	}
}

// TestIndexStaysInRange drives every term to its largest value and asserts an index within 0..100.
func TestIndexStaysInRange(t *testing.T) {
	m := clean()
	for id, v := range map[string]float64{erodedShare: 1, erodedCount: 1e12, density: 1, groups: 1e12} {
		m[id] = complete(v)
	}
	if s := Score(m); s.Index > 100 || s.Index < 95 {
		t.Errorf("index = %d, want within 95..100", s.Index)
	}
}

func TestScoreIgnoresTestMetrics(t *testing.T) {
	m := clean()
	m["erosion.eroded-share.test"] = complete(1)
	m["duplication.groups.test"] = contract.Metric{State: contract.Incomplete}
	if s := Score(m); s.Index != 0 || s.Partial {
		t.Errorf("score = %+v, want 0 and not partial", s)
	}
}

// The build fails when Score gains a parameter, so the score cannot take configuration or options.
var _ func(map[string]contract.Metric) contract.Score = Score

func TestSumAddsInAscendingOrder(t *testing.T) {
	// Adding each 1 to 1e16 alone loses it, and adding the two ones first keeps them.
	if got := sum([]float64{1e16, 1, 1}); got != 1e16+2 {
		t.Errorf("sum = %v, want %v", got, 1e16+2)
	}
}
