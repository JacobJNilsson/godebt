package complexity

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/discover"
	"github.com/JacobJNilsson/godebt/internal/inventory"
)

const fixtures = "../../testdata/fixtures"

func build(t *testing.T, fixture string) *inventory.Inventory {
	t.Helper()
	tree, err := discover.Walk(context.Background(), filepath.Join(fixtures, fixture), discover.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return inventory.Build(tree)
}

func complete(value float64) contract.Metric {
	return contract.Metric{State: contract.Complete, Value: value, Unit: "count"}
}

// checkMetrics compares the metrics whose ids start with prefix.
func checkMetrics(t *testing.T, got map[string]contract.Metric, prefix string, want map[string]contract.Metric) {
	t.Helper()
	for id, m := range got {
		if strings.HasPrefix(id, prefix) {
			if _, ok := want[id]; !ok {
				t.Errorf("unexpected metric %s = %+v", id, m)
			}
		}
	}
	for id, w := range want {
		if g, ok := got[id]; !ok || !reflect.DeepEqual(g, w) {
			t.Errorf("%s:\n got %+v\nwant %+v", id, g, w)
		}
	}
}

// TestDistributionsFunctionsFixture checks numbers that a person derived by hand from the fixture source.
func TestDistributionsFunctionsFixture(t *testing.T) {
	got, _ := Measure(build(t, "functions"))
	// Production CC ascending: seventeen 1s, then 2, 5, 10, 11, 11, 15. Rank 12 is 1, rank 21 is 11.
	// Test CC ascending: 1, 2, 12. Rank 2 is 2, rank 3 is 12.
	checkMetrics(t, got, "complexity.", map[string]contract.Metric{
		"complexity.functions.production": complete(23),
		"complexity.cc.p50.production":    complete(1),
		"complexity.cc.p90.production":    complete(11),
		"complexity.cc.max.production":    complete(15),
		"complexity.functions.test":       complete(3),
		"complexity.cc.p50.test":          complete(2),
		"complexity.cc.p90.test":          complete(12),
		"complexity.cc.max.test":          complete(12),
	})
}

func TestDistributionsEmptyFixture(t *testing.T) {
	got, _ := Measure(build(t, "empty"))
	want := map[string]contract.Metric{}
	for _, set := range []string{"production", "test"} {
		want["complexity.functions."+set] = complete(0)
		for _, name := range []string{"p50", "p90", "max"} {
			want["complexity.cc."+name+"."+set] = contract.Metric{State: contract.NotApplicable, Unit: "count"}
		}
	}
	checkMetrics(t, got, "complexity.", want)
}

// TestIncomplete asserts that every metric of a set with an error is incomplete and that its JSON has no number.
func TestIncomplete(t *testing.T) {
	got, _ := Measure(build(t, "broken"))
	wantErrors := map[string][]string{"production": {"bad.go"}, "test": {"bad_test.go"}}
	// Nine metrics for each of the two sets.
	if len(got) != 18 {
		t.Fatalf("%d metrics, want 18: %v", len(got), got)
	}
	for id, m := range got {
		set := id[strings.LastIndex(id, ".")+1:]
		if m.State != contract.Incomplete {
			t.Errorf("%s: state %s, want incomplete", id, m.State)
		}
		// The set still holds a parsed function, so a stale count or CC would show here.
		if m.Value != 0 || m.Numerator != 0 || m.Denominator != 0 {
			t.Errorf("%s keeps numbers in memory: %+v", id, m)
		}
		if errs := m.Detail.Errors; !reflect.DeepEqual(errs, wantErrors[set]) {
			t.Errorf("%s: detail.errors = %v, want %v", id, errs, wantErrors[set])
		}
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{`"value"`, `"numerator"`, `"denominator"`} {
			if strings.Contains(string(data), key) {
				t.Errorf("%s: JSON %s reports %s", id, data, key)
			}
		}
	}
}

// TestIncompleteRootReadError puts a read error without a set into the details of both sets.
func TestIncompleteRootReadError(t *testing.T) {
	inv := &inventory.Inventory{Errors: []inventory.Error{
		{Kind: inventory.KindParse, Path: "a.go", Set: discover.Production},
		{Kind: inventory.KindRead, Path: "locked"},
	}}
	got, _ := Measure(inv)
	for id, want := range map[string][]string{
		"complexity.functions.production": {"a.go", "locked"},
		"complexity.functions.test":       {"locked"},
	} {
		if m := got[id]; m.State != contract.Incomplete || !reflect.DeepEqual(m.Detail.Errors, want) {
			t.Errorf("%s = %+v, want incomplete with errors %v", id, m, want)
		}
	}
}

func TestPercentile(t *testing.T) {
	sorted := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, tc := range []struct{ p, want int }{{50, 5}, {90, 9}, {100, 10}, {1, 1}} {
		if got := percentile(sorted, tc.p); got != tc.want {
			t.Errorf("p%d = %d, want %d", tc.p, got, tc.want)
		}
	}
	if got := percentile([]int{7}, 50); got != 7 {
		t.Errorf("p50 of one value = %d, want 7", got)
	}
}
