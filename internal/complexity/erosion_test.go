package complexity

import (
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/discover"
	"github.com/JacobJNilsson/godebt/internal/inventory"
)

// checkNear compares a metric to a tolerance, because the expected sums below add their terms in another order.
func checkNear(t *testing.T, got map[string]contract.Metric, id string, want contract.Metric) {
	t.Helper()
	g := got[id]
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	if g.State != want.State || g.Unit != want.Unit || !near(g.Value, want.Value) ||
		!near(g.Numerator, want.Numerator) || !near(g.Denominator, want.Denominator) || !reflect.DeepEqual(g.Detail, want.Detail) {
		t.Errorf("%s:\n got %+v\nwant %+v", id, g, want)
	}
}

// TestErosionFunctionsFixture checks numbers that a person derived by hand from the fixture source.
func TestErosionFunctionsFixture(t *testing.T) {
	got, findings := Measure(build(t, "functions"))
	// Long has mass 11 × sqrt(36) = 66, Short 15 × sqrt(4) = 30, and the closure wrap#1 11 × sqrt(3) = 19.053.
	wrapMass := 11 * math.Sqrt(3)
	erodedMass := 66 + 30 + wrapMass
	// Production mass is 222.140 over 23 functions.
	prodMass := 10*math.Sqrt(29) + math.Sqrt(15) + math.Sqrt(12) + 3 + 2*math.Sqrt(6) + 4 + 2*math.Sqrt(3) +
		5*math.Sqrt(14) + 1 + math.Sqrt(6) + math.Sqrt(3) + 3 + math.Sqrt(7) + 1 + erodedMass
	// Test mass is sqrt(7) + 2 × sqrt(5) + 12 × sqrt(4), and allSet is eroded with mass 24.
	testMass := math.Sqrt(7) + 2*math.Sqrt(5) + 24
	for id, want := range map[string]contract.Metric{
		"erosion.mass.production":         {State: contract.Complete, Value: prodMass, Unit: "mass"},
		"erosion.eroded-count.production": {State: contract.Complete, Value: 3, Unit: "count"},
		"erosion.eroded-share.production": {State: contract.Complete, Value: erodedMass / prodMass, Unit: "ratio", Numerator: erodedMass, Denominator: prodMass},
		"erosion.mass.test":               {State: contract.Complete, Value: testMass, Unit: "mass"},
		"erosion.eroded-count.test":       {State: contract.Complete, Value: 1, Unit: "count"},
		"erosion.eroded-share.test":       {State: contract.Complete, Value: 24 / testMass, Unit: "ratio", Numerator: 24, Denominator: testMass},
	} {
		checkNear(t, got, id, want)
	}
	if math.Abs(prodMass-222.140) > 0.0005 || math.Abs(erodedMass-115.053) > 0.0005 || math.Abs(erodedMass/prodMass-0.518) > 0.0005 {
		t.Errorf("worked example: mass %.3f, eroded mass %.3f, share %.3f", prodMass, erodedMass, erodedMass/prodMass)
	}
	hotspot := func(identity, path string, set contract.SourceSet, start, end, cc, nesting, sloc int, mass float64) contract.Finding {
		return contract.Finding{
			Kind: KindHotspot, Path: path, StartLine: start, EndLine: end, Identity: identity, SourceSet: set,
			Facts: contract.Facts{Hotspot: &contract.HotspotFacts{CC: cc, Nesting: nesting, SLOC: sloc, Mass: mass}},
		}
	}
	closure := hotspot(".:wrap#1", "cc.go", contract.Production, 79, 81, 11, 0, 3, wrapMass)
	closure.Ambiguous = true
	want := []contract.Finding{
		hotspot(".:Long", "cc.go", contract.Production, 35, 70, 11, 1, 36, 66),
		hotspot(".:Short", "cc.go", contract.Production, 73, 76, 15, 0, 4, 30),
		closure,
		hotspot(".:allSet", "functions_test.go", contract.Test, 14, 17, 12, 0, 4, 24),
	}
	if !reflect.DeepEqual(findings, want) {
		t.Errorf("findings:\n got %+v\nwant %+v", findings, want)
	}
}

func TestErosionEmptyFixture(t *testing.T) {
	got, findings := Measure(build(t, "empty"))
	for _, set := range []string{"production", "test"} {
		checkNear(t, got, "erosion.mass."+set, contract.Metric{State: contract.Complete, Unit: "mass"})
		checkNear(t, got, "erosion.eroded-count."+set, contract.Metric{State: contract.Complete, Unit: "count"})
		checkNear(t, got, "erosion.eroded-share."+set, contract.Metric{State: contract.NotApplicable, Unit: "ratio"})
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
}

// TestErosionSumsMasses proves that the share is summed eroded mass over summed mass, not a mean of file shares.
func TestErosionSumsMasses(t *testing.T) {
	// One file holds a function with CC 20 and SLOC 100, so its mass is 200 and its own share is 1.
	inv := &inventory.Inventory{Functions: []inventory.Function{
		{Identity: "big:Tangle", Path: "big/big.go", Set: discover.Production, CC: 20, SLOC: 100},
	}}
	// Fifty files each hold a function with CC 1 and SLOC 4, so each has mass 2 and share 0.
	for i := range 50 {
		inv.Functions = append(inv.Functions, inventory.Function{
			Identity: fmt.Sprintf("small%d:Run", i), Path: fmt.Sprintf("small%d/small.go", i), Set: discover.Production, CC: 1, SLOC: 4,
		})
	}
	got, _ := Measure(inv)
	// Summed masses give 200 / 300. A mean of file shares would give 1 / 51.
	checkNear(t, got, "erosion.eroded-share.production", contract.Metric{
		State: contract.Complete, Value: 200.0 / 300.0, Unit: "ratio", Numerator: 200, Denominator: 300,
	})
}

// TestErosionIgnoresPaths renames every file and asserts that the mass and the share keep every bit.
func TestErosionIgnoresPaths(t *testing.T) {
	// named gives the functions paths in the given order, the way inventory.Build sorts them.
	named := func(prefixes []string) *inventory.Inventory {
		inv := &inventory.Inventory{}
		for i, sloc := range []int{3, 5, 6, 7, 10, 11, 13, 14} {
			path := fmt.Sprintf("%s%d.go", prefixes[i%2], i)
			inv.Functions = append(inv.Functions, inventory.Function{
				Identity: path, Path: path, Set: discover.Production, CC: 1 + 10*(i%2), SLOC: sloc,
			})
		}
		sort.Slice(inv.Functions, func(i, j int) bool { return inv.Functions[i].Path < inv.Functions[j].Path })
		return inv
	}
	before, after := named([]string{"a", "b"}), named([]string{"b", "a"})
	// A sum in path order gives different bits for the two inventories, so this test can fail.
	pathOrderSum := func(inv *inventory.Inventory) float64 {
		total := 0.0
		for _, f := range inv.Functions {
			// The explicit conversion stops the compiler from fusing the multiply and the add.
			total += float64(float64(f.CC) * math.Sqrt(float64(f.SLOC)))
		}
		return total
	}
	if pathOrderSum(before) == pathOrderSum(after) {
		t.Fatal("the renamed inventories give equal sums in path order, so the test proves nothing")
	}
	got, _ := Measure(before)
	renamed, _ := Measure(after)
	for _, id := range []string{"erosion.mass.production", "erosion.eroded-share.production"} {
		if !reflect.DeepEqual(got[id], renamed[id]) {
			t.Errorf("%s changes with a rename:\n got %+v\nwant %+v", id, renamed[id], got[id])
		}
	}
}

// TestFindingsOrder shuffles the inventory and asserts that the findings keep one order.
func TestFindingsOrder(t *testing.T) {
	eroded := func(identity, path string, set discover.SourceSet, start int) inventory.Function {
		return inventory.Function{Identity: identity, Path: path, Set: set, StartLine: start, EndLine: start, CC: 11, SLOC: 1}
	}
	fns := []inventory.Function{
		eroded("a:F", "a/a.go", discover.Production, 3),
		eroded("a:v#1", "a/a.go", discover.Production, 9),
		eroded("a:v#2", "a/a.go", discover.Production, 9),
		eroded("a:F", "a/a_test.go", discover.Test, 1),
		eroded("b:G", "b/b.go", discover.Production, 1),
	}
	var want []string
	for _, f := range fns {
		want = append(want, fmt.Sprintf("%s:%d %s", f.Path, f.StartLine, f.Identity))
	}
	rng := rand.New(rand.NewSource(1))
	for range 20 {
		shuffled := append([]inventory.Function(nil), fns...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		_, findings := Measure(&inventory.Inventory{Functions: shuffled})
		var got []string
		for _, f := range findings {
			got = append(got, fmt.Sprintf("%s:%d %s", f.Path, f.StartLine, f.Identity))
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("order:\n got %v\nwant %v", got, want)
		}
	}
}
