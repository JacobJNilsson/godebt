package policy

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/compare"
	"github.com/JacobJNilsson/godebt/internal/config"
	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/report"
)

const density = "duplication.density.production"

func rep(index int) *report.Report {
	return &report.Report{
		Score: contract.Score{Index: index},
		Metrics: map[string]contract.Metric{
			density:                          {State: contract.Complete, Value: 0.1, Unit: "ratio"},
			"duplication.density.test":       {State: contract.NotApplicable, Unit: "ratio"},
			"complexity.hotspots.test":       {State: contract.Incomplete, Unit: "count"},
			"complexity.hotspots.production": {State: contract.Complete, Value: 4, Unit: "count"},
		},
	}
}

func cmp(before, after int, newFindings ...contract.Finding) *compare.Comparison {
	return &compare.Comparison{
		Comparable: true,
		Before:     compare.Summary{Index: before},
		After:      compare.Summary{Index: after},
		IndexDelta: after - before,
		New:        newFindings,
	}
}

func refused() *compare.Comparison {
	return &compare.Comparison{Refusal: `configDigest differs: before "sha256:aa", after "sha256:bb"`}
}

func hotspot(path string, line int) contract.Finding {
	return contract.Finding{Kind: "complexity.hotspot", Path: path, StartLine: line, EndLine: line + 9, Identity: "p:F", SourceSet: contract.Production}
}

func intp(v int) *int           { return &v }
func floatp(v float64) *float64 { return &v }

func checks(r Result) []string {
	out := []string{}
	for _, reason := range r.Reasons {
		out = append(out, reason.Check)
	}
	return out
}

func assertReasons(t *testing.T, r Result, failed bool, want ...string) {
	t.Helper()
	if r.Failed != failed {
		t.Errorf("failed = %v, want %v (%+v)", r.Failed, failed, r.Reasons)
	}
	if got := checks(r); !reflect.DeepEqual(got, append([]string{}, want...)) {
		t.Errorf("checks = %v, want %v (%+v)", got, want, r.Reasons)
	}
}

func TestEmptyPolicyPasses(t *testing.T) {
	r := Evaluate(config.Policy{}, rep(90), nil)
	assertReasons(t, r, false)
	if len(r.Skipped) != 0 {
		t.Errorf("skipped = %v", r.Skipped)
	}
}

func TestMaxIndex(t *testing.T) {
	p := config.Policy{MaxIndex: intp(40)}
	assertReasons(t, Evaluate(p, rep(40), nil), false)
	r := Evaluate(p, rep(41), nil)
	assertReasons(t, r, true, "maxIndex")
	if want := "index 41 is above 40"; r.Reasons[0].Message != want {
		t.Errorf("message = %q, want %q", r.Reasons[0].Message, want)
	}
}

func TestPolicyIgnoresTestIndex(t *testing.T) {
	r := rep(10)
	r.TestScore = contract.Score{Index: 90, Partial: true}
	p := config.Policy{MaxIndex: intp(40)}
	assertReasons(t, Evaluate(p, r, nil), false)
}

func TestPartialScoreFailsMaxIndex(t *testing.T) {
	partial := rep(10)
	partial.Score.Partial = true
	r := Evaluate(config.Policy{MaxIndex: intp(40)}, partial, nil)
	assertReasons(t, r, true, "maxIndex")
	if !strings.Contains(r.Reasons[0].Message, "partial") {
		t.Errorf("message = %q, want it to say the index is partial", r.Reasons[0].Message)
	}
}

func TestBudgets(t *testing.T) {
	cases := map[string]struct {
		id     string
		max    float64
		failed bool
		text   string
	}{
		"within":         {density, 0.1, false, ""},
		"above":          {density, 0.05, true, "duplication.density.production is 0.1, above 0.05"},
		"incomplete":     {"complexity.hotspots.test", 100, true, "incomplete"},
		"absent":         {"complexity.nothing.production", 100, true, "not in the report"},
		"not-applicable": {"duplication.density.test", 0, false, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := config.Policy{Budgets: map[string]config.Budget{c.id: {Max: &c.max}}}
			r := Evaluate(p, rep(0), nil)
			if !c.failed {
				assertReasons(t, r, false)
				return
			}
			assertReasons(t, r, true, "budget")
			if !strings.Contains(r.Reasons[0].Message, c.text) || !strings.Contains(r.Reasons[0].Message, c.id) {
				t.Errorf("message = %q, want %q and the metric id", r.Reasons[0].Message, c.text)
			}
		})
	}
}

func TestRegressionMaxIncrease(t *testing.T) {
	p := config.Policy{Regression: &config.Regression{MaxIncrease: intp(2)}}
	assertReasons(t, Evaluate(p, rep(22), cmp(20, 22)), false)
	r := Evaluate(p, rep(23), cmp(20, 23))
	assertReasons(t, r, true, "regression")
	if want := "index rose 3 points, above 2"; r.Reasons[0].Message != want {
		t.Errorf("message = %q, want %q", r.Reasons[0].Message, want)
	}
}

func TestRegressionMaxIncreasePercent(t *testing.T) {
	p := config.Policy{Regression: &config.Regression{MaxIncreasePercent: floatp(10)}}
	assertReasons(t, Evaluate(p, rep(22), cmp(20, 22)), false)
	r := Evaluate(p, rep(23), cmp(20, 23))
	assertReasons(t, r, true, "regression")
	if want := "index rose 3 points, above 10% of 20"; r.Reasons[0].Message != want {
		t.Errorf("message = %q, want %q", r.Reasons[0].Message, want)
	}
}

func TestRegressionBothKnobs(t *testing.T) {
	p := config.Policy{Regression: &config.Regression{MaxIncrease: intp(1), MaxIncreasePercent: floatp(5)}}
	assertReasons(t, Evaluate(p, rep(23), cmp(20, 23)), true, "regression", "regression")
}

func TestRegressionOnRefusedComparison(t *testing.T) {
	p := config.Policy{Regression: &config.Regression{MaxIncrease: intp(100)}}
	r := Evaluate(p, rep(0), refused())
	assertReasons(t, r, true, "regression")
	if want := "skipped, baseline not comparable"; r.Reasons[0].Message != want {
		t.Errorf("message = %q, want %q", r.Reasons[0].Message, want)
	}
}

func TestFailOnNew(t *testing.T) {
	clone := contract.Finding{Kind: "duplication.clone-group", Path: "c.go", StartLine: 1, EndLine: 5, Identity: "g1"}
	p := config.Policy{FailOnNew: []string{"complexity.hotspot"}}
	r := Evaluate(p, rep(0), cmp(0, 0, hotspot("a/a.go", 3), hotspot("b/b.go", 7), clone))
	assertReasons(t, r, true, "failOnNew", "failOnNew")
	want := Reason{Check: "failOnNew", Kind: "complexity.hotspot", Message: "new complexity.hotspot p:F at a/a.go:3-12"}
	if r.Reasons[0] != want {
		t.Errorf("reason = %+v, want %+v", r.Reasons[0], want)
	}
	assertReasons(t, Evaluate(p, rep(0), cmp(0, 0, clone)), false)
}

func TestFailOnNewOnRefusedComparison(t *testing.T) {
	r := Evaluate(config.Policy{FailOnNew: []string{"complexity.hotspot"}}, rep(0), refused())
	assertReasons(t, r, true, "failOnNew")
}

func TestAllFourFail(t *testing.T) {
	p := config.Policy{
		MaxIndex:   intp(10),
		Regression: &config.Regression{MaxIncrease: intp(0)},
		Budgets:    map[string]config.Budget{density: {Max: floatp(0)}},
		FailOnNew:  []string{"complexity.hotspot"},
	}
	// A lower index does not suppress the other failures.
	r := Evaluate(p, rep(15), cmp(14, 15, hotspot("a/a.go", 3)))
	assertReasons(t, r, true, "budget", "failOnNew", "maxIndex", "regression")
}

func TestReasonsSortedAfterShuffle(t *testing.T) {
	budgets := map[string]config.Budget{
		density: {Max: floatp(0)}, "complexity.hotspots.production": {Max: floatp(0)},
		"complexity.hotspots.test": {Max: floatp(0)}, "a.b.production": {Max: floatp(0)},
	}
	findings := []contract.Finding{hotspot("z.go", 1), hotspot("a.go", 1), hotspot("m.go", 1)}
	p := config.Policy{MaxIndex: intp(0), Budgets: budgets, FailOnNew: []string{"complexity.hotspot"}}
	want := Evaluate(p, rep(5), cmp(5, 5, findings...))
	for i := 1; i < len(want.Reasons); i++ {
		a, b := want.Reasons[i-1], want.Reasons[i]
		if a.Check > b.Check || (a.Check == b.Check && a.Message > b.Message) {
			t.Errorf("reasons unsorted at %d: %+v", i, want.Reasons)
		}
	}
	rng := rand.New(rand.NewSource(1))
	for range 10 {
		shuffled := append([]contract.Finding{}, findings...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		if got := Evaluate(p, rep(5), cmp(5, 5, shuffled...)); !reflect.DeepEqual(got, want) {
			t.Fatalf("shuffled input changed the result:\n%+v\n%+v", got, want)
		}
	}
}

func TestNoBaselineSkipsBaselineChecks(t *testing.T) {
	p := config.Policy{Regression: &config.Regression{MaxIncrease: intp(0)}, FailOnNew: []string{"complexity.hotspot"}}
	r := Evaluate(p, rep(99), nil)
	assertReasons(t, r, false)
	if want := []string{"regression", "failOnNew"}; !reflect.DeepEqual(r.Skipped, want) {
		t.Errorf("skipped = %v, want %v", r.Skipped, want)
	}
	if r = Evaluate(p, rep(99), cmp(99, 99)); len(r.Skipped) != 0 {
		t.Errorf("skipped with a baseline = %v", r.Skipped)
	}
}

func TestNoBaselineChecksNoBaseline(t *testing.T) {
	r := Evaluate(config.Policy{MaxIndex: intp(50)}, rep(10), nil)
	if len(r.Skipped) != 0 {
		t.Errorf("skipped = %v, want none", r.Skipped)
	}
}

func TestEmptyRegressionIsSkippedWithoutBaseline(t *testing.T) {
	p := config.Policy{Regression: &config.Regression{}}
	if r := Evaluate(p, rep(99), nil); r.Failed || !reflect.DeepEqual(r.Skipped, []string{"regression"}) {
		t.Errorf("no baseline = %+v, want a pass with regression skipped", r)
	}
	assertReasons(t, Evaluate(p, rep(99), cmp(0, 99)), false)
}

func TestRefusedBaselineFailsAnEmptyPolicy(t *testing.T) {
	r := Evaluate(config.Policy{}, rep(0), refused())
	if !r.Failed || r.Refusal != refused().Refusal || len(r.Reasons) != 0 {
		t.Errorf("result = %+v, want a failure that carries the refusal", r)
	}
	if ok := Evaluate(config.Policy{}, rep(0), cmp(0, 0)); ok.Failed || ok.Refusal != "" {
		t.Errorf("comparable baseline = %+v, want a pass", ok)
	}
}

func TestFailOnNewIgnoresChanged(t *testing.T) {
	grown := contract.Finding{Kind: "duplication.clone-group", Path: "c.go", StartLine: 1, EndLine: 9, Identity: "g2"}
	c := cmp(0, 0)
	c.Changed = []compare.Changed{{After: grown, BeforeGroupID: "g1", BeforeTokens: 40}}
	p := config.Policy{FailOnNew: []string{"duplication.clone-group"}}
	assertReasons(t, Evaluate(p, rep(0), c), false)
}
