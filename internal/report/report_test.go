package report

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/complexity"
	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/discover"
	"github.com/JacobJNilsson/godebt/internal/duplication"
	"github.com/JacobJNilsson/godebt/internal/inventory"
	"github.com/JacobJNilsson/godebt/internal/safeguards"
)

func newReport(t *testing.T, fixture string) *Report {
	t.Helper()
	tree, err := discover.Walk(context.Background(), "../../testdata/fixtures/"+fixture, discover.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return measure(inventory.Build(tree))
}

// measure runs both measures over an inventory and merges their results the way godebt.Audit does.
func measure(inv *inventory.Inventory) *Report {
	metrics, findings := complexity.Measure(inv)
	dupMetrics, clones, limits := duplication.Measure(inv)
	for id, m := range dupMetrics {
		metrics[id] = m
	}
	findings = append(findings, clones...)
	guards, broken := safeguards.Inspect(inv.Tree)
	findings = append(findings, broken...)
	contract.SortFindings(findings)
	return New(inv, metrics, findings, limits, guards, Run{AnalyzerVersion: testAnalyzer})
}

var update = flag.Bool("update", false, "rewrite the golden files")

// TestDeterministicJSON shuffles the functions behind the findings and asserts byte-equal JSON.
func TestDeterministicJSON(t *testing.T) {
	tree, err := discover.Walk(context.Background(), "../../testdata/fixtures/functions", discover.Options{})
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.Build(tree)
	marshal := func() []byte {
		data, err := json.Marshal(measure(inv))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	want := marshal()
	rng := rand.New(rand.NewSource(1))
	for range 10 {
		rng.Shuffle(len(inv.Functions), func(i, j int) { inv.Functions[i], inv.Functions[j] = inv.Functions[j], inv.Functions[i] })
		if got := marshal(); !bytes.Equal(got, want) {
			t.Fatalf("JSON after a shuffle differs:\n%s\nwant:\n%s", got, want)
		}
	}
}

// testAnalyzer stands in for godebt.Version, which this package cannot import.
const testAnalyzer = "0.0.0-test"

// fixtures lists the fixture modules that have a golden report.
var fixtures = []string{
	"sets", "functions", "broken", "empty",
	"clones/exact", "clones/renamed", "clones/fourway", "clones/idiom", "clones/near", "clones/within", "clones/nested", "clones/ladder",
	"safeguards/odd",
}

func TestGolden(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			r := newReport(t, name)
			got, err := json.MarshalIndent(r, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			rootJSON, err := json.Marshal(r.Repo.Root)
			if err != nil {
				t.Fatal(err)
			}
			got = bytes.ReplaceAll(got, rootJSON, []byte(`"<root>"`))
			compareGolden(t, "report-"+strings.ReplaceAll(name, "/", "-")+".json", append(got, '\n'))
		})
	}
}

// compareGolden compares output with a golden file, and it rewrites the file first under -update.
func compareGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "golden", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden file, run make golden and review the diff:\n%s", path, got)
	}
}

func TestNewSetsFixture(t *testing.T) {
	r := newReport(t, "sets")
	want := Coverage{
		Production:    Measured{Files: 2, SLOC: 4},
		Test:          Measured{Files: 1, SLOC: 7},
		Generated:     Counted{Files: 1},
		Vendored:      Counted{Files: 1},
		Testdata:      Counted{Files: 1},
		Excluded:      Counted{Files: 6},
		Unsupported:   Counted{Files: 2},
		NestedModules: []string{"tools/gen"},
	}
	if !reflect.DeepEqual(r.Coverage, want) {
		t.Errorf("coverage:\n got %+v\nwant %+v", r.Coverage, want)
	}
	got := []float64{r.Metrics["complexity.functions.production"].Value, r.Metrics["complexity.functions.test"].Value}
	if !reflect.DeepEqual(got, []float64{2, 1}) {
		t.Errorf("functions = %v, want [2 1]", got)
	}
}

// TestNewSortsLimits shuffles the limits of a report and asserts that New sorts them by metric id.
func TestNewSortsLimits(t *testing.T) {
	inv := inventory.Build(&discover.Tree{})
	want := []contract.Limit{
		{MetricID: "duplication.density.production", Reason: "work cap 1 exceeded"},
		{MetricID: "duplication.groups.production", Reason: "work cap 1 exceeded"},
		{MetricID: "duplication.groups.test", Reason: "tokens cap 1 exceeded"},
	}
	rng := rand.New(rand.NewSource(1))
	for range 20 {
		shuffled := append([]contract.Limit(nil), want...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		if got := New(inv, nil, nil, shuffled, nil, Run{}).Limits; !reflect.DeepEqual(got, want) {
			t.Fatalf("limits:\n got %v\nwant %v", got, want)
		}
	}
	if got := New(inv, nil, nil, nil, nil, Run{}).Safeguards; got == nil || len(got) != 0 {
		t.Errorf("safeguards = %#v, want an empty list", got)
	}
}
