package godebt

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/report"
)

// pairDirection is the index move that one paired refactor must produce.
type pairDirection string

const (
	lower  pairDirection = "lower"
	higher pairDirection = "higher"
	equal  pairDirection = "equal"
	near   pairDirection = "near"
)

// nearTolerance is the largest index move, in points, that a near pair allows.
const nearTolerance = 3

// calibrationPairs is the table of spec 004, one row per paired refactor.
var calibrationPairs = []struct {
	name string
	want pairDirection
}{
	{name: "extract-clone", want: lower},
	{name: "split-function", want: lower},
	{name: "add-clone", want: higher},
	{name: "add-branches", want: higher},
	{name: "reduce-branches", want: lower},
	{name: "trim-to-threshold", want: near},
	{name: "rename", want: equal},
	{name: "move", want: equal},
	{name: "tests-only", want: equal},
	{name: "comments", want: equal},
	{name: "pad", want: near},
}

// TestPairedRefactors asserts that every pair moves the index in its direction.
func TestPairedRefactors(t *testing.T) {
	for _, p := range calibrationPairs {
		before := pairReport(t, p.name+"/before")
		after := pairReport(t, p.name+"/after")
		t.Logf("%s: %d -> %d, want %s. Terms: %s -> %s", p.name, before.Score.Index, after.Score.Index, p.want, pairTerms(before), pairTerms(after))
		switch p.want {
		case lower:
			if after.Score.Index >= before.Score.Index {
				t.Errorf("%s: index %d -> %d, want a lower index", p.name, before.Score.Index, after.Score.Index)
			}
		case higher:
			if after.Score.Index <= before.Score.Index {
				t.Errorf("%s: index %d -> %d, want a higher index", p.name, before.Score.Index, after.Score.Index)
			}
		case equal:
			if after.Score.Index != before.Score.Index {
				t.Errorf("%s: index %d -> %d, want an equal index", p.name, before.Score.Index, after.Score.Index)
			}
		case near:
			if diff := after.Score.Index - before.Score.Index; diff < -nearTolerance || diff > nearTolerance {
				t.Errorf("%s: index %d -> %d, want a move of at most %d points", p.name, before.Score.Index, after.Score.Index, nearTolerance)
			}
		}
	}
}

// TestPairedRefactorsScoreAboveZero asserts that every before module scores above 0.
func TestPairedRefactorsScoreAboveZero(t *testing.T) {
	for _, p := range calibrationPairs {
		before := pairReport(t, p.name+"/before")
		if before.Score.Index <= 0 {
			t.Errorf("%s: before index %d, want a value above 0", p.name, before.Score.Index)
		}
	}
}

// TestPairedRefactorsCoversEveryFixture asserts that the table names every pair directory.
func TestPairedRefactorsCoversEveryFixture(t *testing.T) {
	entries, err := os.ReadDir(pairsDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := make([]string, 0, len(calibrationPairs))
	for _, p := range calibrationPairs {
		want = append(want, p.name)
	}
	sort.Strings(names)
	sort.Strings(want)
	if !reflect.DeepEqual(names, want) {
		t.Errorf("fixture directories = %v, want %v", names, want)
	}
}

// TestTestsOnlyPairAddsTestDebt asserts that the tests-only pair adds the test debt, which an equal index alone would not prove.
func TestTestsOnlyPairAddsTestDebt(t *testing.T) {
	before := pairReport(t, "tests-only/before")
	after := pairReport(t, "tests-only/after")
	for _, id := range []string{"duplication.groups.test", "erosion.eroded-count.test"} {
		if got := after.Metrics[id]; got.State != contract.Complete || got.Value < 1 {
			t.Errorf("after %s = %+v, want at least 1", id, got)
		}
		if got := before.Metrics[id]; got.State != contract.Complete || got.Value != 0 {
			t.Errorf("before %s = %+v, want 0", id, got)
		}
	}
}

// pairChange is the production metric that one pair must move, and the move that the fixtures name.
type pairChange struct {
	pair   string
	metric string
	// check returns an empty string when the before and after values show the change.
	check func(before, after float64) string
}

func someToNone(before, after float64) string {
	if before < 1 || after != 0 {
		return fmt.Sprintf("%v -> %v, want at least 1 -> 0", before, after)
	}
	return ""
}

func unchangedAboveZero(before, after float64) string {
	if before < 1 || before != after {
		return fmt.Sprintf("%v -> %v, want the same count of at least 1", before, after)
	}
	return ""
}

func rises(before, after float64) string {
	if after <= before {
		return fmt.Sprintf("%v -> %v, want a rise", before, after)
	}
	return ""
}

func falls(before, after float64) string {
	if after >= before {
		return fmt.Sprintf("%v -> %v, want a fall", before, after)
	}
	return ""
}

func fallsAboveZero(before, after float64) string {
	if after >= before || after <= 0 {
		return fmt.Sprintf("%v -> %v, want a fall that stays above 0", before, after)
	}
	return ""
}

// pairChanges pins the named change of each pair, so fixture drift cannot pass on the index direction alone.
var pairChanges = []pairChange{
	{pair: "add-branches", metric: "erosion.eroded-count.production", check: rises},
	{pair: "reduce-branches", metric: "erosion.excess-mass.production", check: falls},
	{pair: "reduce-branches", metric: "erosion.eroded-count.production", check: unchangedAboveZero},
	{pair: "trim-to-threshold", metric: "erosion.eroded-count.production", check: falls},
	{pair: "trim-to-threshold", metric: "erosion.excess-mass.production", check: fallsAboveZero},
	{pair: "split-function", metric: "erosion.eroded-count.production", check: someToNone},
	{pair: "add-clone", metric: "duplication.groups.production", check: rises},
	{pair: "extract-clone", metric: "duplication.groups.production", check: falls},
}

// equalPairs are the pairs whose production metrics must not move.
var equalPairs = []string{"rename", "move", "tests-only", "comments"}

// productionMetrics are the four metrics that feed the index.
var productionMetrics = []string{
	"erosion.excess-share.production", "erosion.excess-mass.production",
	"duplication.density.production", "duplication.groups.production",
}

func unchanged(before, after float64) string {
	if before != after {
		return fmt.Sprintf("%v -> %v, want no change", before, after)
	}
	return ""
}

func init() {
	for _, pair := range equalPairs {
		for _, m := range productionMetrics {
			pairChanges = append(pairChanges, pairChange{pair: pair, metric: m, check: unchanged})
		}
	}
}

// assertAboutDouble asserts that the after module holds about twice the production lines of the before module.
func assertAboutDouble(t *testing.T, before, after *Report) {
	t.Helper()
	if ratio := float64(after.Coverage.Production.SLOC) / float64(before.Coverage.Production.SLOC); ratio < 1.8 || ratio > 2.2 {
		t.Errorf("production lines %v -> %v, want about double", before.Coverage.Production.SLOC, after.Coverage.Production.SLOC)
	}
}

// assertMetricsMove asserts, for each id, that the value moves by at most limit.
func assertMetricsMove(t *testing.T, before, after *Report, limit float64, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if b, a := before.Metrics[id].Value, after.Metrics[id].Value; math.Abs(a-b) > limit {
			t.Errorf("%s %v -> %v, want a move of at most %v", id, b, a, limit)
		}
	}
}

// TestPadPairAddsOnlyCleanCode asserts that the pad pair doubles the production lines and keeps the eroded count and the clone groups.
func TestPadPairAddsOnlyCleanCode(t *testing.T) {
	before := pairReport(t, "pad/before")
	after := pairReport(t, "pad/after")
	assertAboutDouble(t, before, after)
	assertMetricsMove(t, before, after, 0, "erosion.excess-mass.production", "duplication.groups.production")
	assertMetricsMove(t, before, after, 0.05, "erosion.excess-share.production", "duplication.density.production")
}

// TestMovePairMovesTheFunction asserts that the move pair takes tangle.go out of the root and into a helper package.
func TestMovePairMovesTheFunction(t *testing.T) {
	root := filepath.Join(pairsDir, "move")
	if _, err := os.Stat(filepath.Join(root, "before", "tangle.go")); err != nil {
		t.Errorf("before holds no tangle.go: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "after", "tangle.go")); err == nil {
		t.Error("after still holds a top-level tangle.go")
	}
	if goSources(t, "move/after")["helper/helper.go"] == "" {
		t.Error("after holds no helper/helper.go")
	}
}

// TestPairsShowTheirNamedChange asserts that each pair changes the production metric that its name claims.
func TestPairsShowTheirNamedChange(t *testing.T) {
	for _, c := range pairChanges {
		before := pairReport(t, c.pair+"/before").Metrics[c.metric]
		after := pairReport(t, c.pair+"/after").Metrics[c.metric]
		if before.State != contract.Complete || after.State != contract.Complete {
			t.Errorf("%s: %s states %q -> %q, want complete", c.pair, c.metric, before.State, after.State)
			continue
		}
		if msg := c.check(before.Value, after.Value); msg != "" {
			t.Errorf("%s: %s %s", c.pair, c.metric, msg)
		}
	}
}

// pairTerms lists the measured terms of a report.
func pairTerms(r *Report) string {
	var out []string
	for _, c := range r.Score.Contributions {
		for _, term := range c.Terms {
			out = append(out, fmt.Sprintf("%s %s=%.4f", c.Dimension, term.MetricID, term.Value))
		}
	}
	return strings.Join(out, " ")
}

// pairsDir is the root of the paired fixtures of spec 004.
const pairsDir = "testdata/fixtures/pairs"

// pairReport audits one module of a pair and fails the test when the audit is incomplete.
func pairReport(t *testing.T, dir string) *Report {
	t.Helper()
	r := audit(t, filepath.Join(pairsDir, dir))
	if r.Completeness != report.Complete || len(r.Limits) != 0 {
		t.Fatalf("%s: completeness %q with %d limits, want a complete audit", dir, r.Completeness, len(r.Limits))
	}
	return r
}

// TestPairsDifferInContent asserts that the before and after trees of every pair differ in their Go files, so no after is a copy of its before.
func TestPairsDifferInContent(t *testing.T) {
	for _, p := range calibrationPairs {
		if reflect.DeepEqual(goSources(t, p.name+"/before"), goSources(t, p.name+"/after")) {
			t.Errorf("%s: before and after hold the same Go files", p.name)
		}
	}
}

// goSources maps the path of every .go file of one pair module to its content.
func goSources(t *testing.T, dir string) map[string]string {
	t.Helper()
	root := filepath.Join(pairsDir, dir)
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".go" {
			return err
		}
		data, readErr := os.ReadFile(path)
		rel, _ := filepath.Rel(root, path)
		out[rel] = string(data)
		return readErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
