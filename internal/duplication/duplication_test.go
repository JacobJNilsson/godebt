package duplication

import (
	"context"
	"encoding/json"
	"fmt"
	"go/token"
	"hash/fnv"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/JacobJNilsson/scree/internal/contract"
	"github.com/JacobJNilsson/scree/internal/discover"
	"github.com/JacobJNilsson/scree/internal/inventory"
)

const fixtures = "../../testdata/fixtures"

func build(t *testing.T, dir string) *inventory.Inventory {
	t.Helper()
	tree, err := discover.Walk(context.Background(), dir, discover.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return inventory.Build(tree)
}

// buildSource writes files given as slash paths and contents into a temporary module and builds its inventory.
func buildSource(t *testing.T, files map[string]string) *inventory.Inventory {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return build(t, dir)
}

// group is a clone-group finding in a form that a test can write by hand.
type group struct {
	tokens  int
	members []string
}

func groups(findings []contract.Finding, set contract.SourceSet) []group {
	var out []group
	for _, f := range findings {
		if f.SourceSet != set {
			continue
		}
		g := group{tokens: f.Facts.Clone.Tokens}
		for _, m := range f.Facts.Clone.Members {
			g.members = append(g.members, fmt.Sprintf("%s:%d-%d", m.Path, m.StartLine, m.EndLine))
		}
		out = append(out, g)
	}
	return out
}

func count(value float64) contract.Metric {
	return contract.Metric{State: contract.Complete, Value: value, Unit: "count"}
}

func ratio(num, den float64) contract.Metric {
	return contract.Metric{State: contract.Complete, Value: num / den, Unit: "ratio", Numerator: num, Denominator: den}
}

// setMetrics returns the three duplication metrics of a set.
func setMetrics(set string, groups, lines, sloc float64) map[string]contract.Metric {
	return map[string]contract.Metric{
		"duplication.groups." + set:           count(groups),
		"duplication.duplicated-lines." + set: count(lines),
		"duplication.density." + set:          ratio(lines, sloc),
	}
}

func checkMetrics(t *testing.T, got, want map[string]contract.Metric) {
	t.Helper()
	for id, w := range want {
		if g, ok := got[id]; !ok || !reflect.DeepEqual(g, w) {
			t.Errorf("%s:\n got %+v\nwant %+v", id, g, w)
		}
	}
}

func checkGroups(t *testing.T, findings []contract.Finding, set contract.SourceSet, want []group) {
	t.Helper()
	if got := groups(findings, set); !reflect.DeepEqual(got, want) {
		t.Errorf("%s groups:\n got %+v\nwant %+v", set, got, want)
	}
}

// The expected numbers below come from the fixture sources.
// Checksum spans 22 code lines and 120 symbols, from "func" to the semicolon after its closing brace.
// In the exact fixture it also holds a blank line, a line comment, and a block comment of two lines.

func TestExact(t *testing.T) {
	metrics, findings, limits := Measure(build(t, filepath.Join(fixtures, "clones", "exact")))
	checkGroups(t, findings, contract.Production, []group{{120, []string{"a.go:6-31", "b.go:4-29"}}})
	// The test copies group with each other and never with production.
	checkGroups(t, findings, contract.Test, []group{{120, []string{"a_test.go:6-31", "b_test.go:4-29"}}})
	// Each member spans 26 lines, of which 22 are code lines. Production has 25 + 23 code lines, and test has 29 + 23.
	want := setMetrics("production", 1, 44, 48)
	for id, m := range setMetrics("test", 1, 44, 52) {
		want[id] = m
	}
	checkMetrics(t, metrics, want)
	if len(metrics) != 6 {
		t.Errorf("%d metrics, want 6: %v", len(metrics), metrics)
	}
	if len(limits) != 0 {
		t.Errorf("limits = %v, want none", limits)
	}
	f := findings[0]
	if f.Kind != KindCloneGroup || f.Path != "a.go" || f.StartLine != 6 || f.EndLine != 31 || f.Ambiguous ||
		f.Identity != f.Facts.Clone.GroupID || len(f.Identity) != 16 {
		t.Errorf("finding = %+v", f)
	}
}

// TestRenamedMatchesExact shows that renamed identifiers and changed literals keep the group and its id.
func TestRenamedMatchesExact(t *testing.T) {
	_, exact, _ := Measure(build(t, filepath.Join(fixtures, "clones", "exact")))
	metrics, renamed, _ := Measure(build(t, filepath.Join(fixtures, "clones", "renamed")))
	checkGroups(t, renamed, contract.Production, []group{{120, []string{"a.go:6-27", "b.go:4-25"}}})
	// b.go holds the package clause, 22 lines of Fold, 4 lines of the type declaration, and 2 helpers.
	checkMetrics(t, metrics, setMetrics("production", 1, 44, 25+29))
	if renamed[0].Identity != exact[0].Identity {
		t.Errorf("renamed id %s differs from exact id %s", renamed[0].Identity, exact[0].Identity)
	}
}

func TestFourway(t *testing.T) {
	metrics, findings, _ := Measure(build(t, filepath.Join(fixtures, "clones", "fourway")))
	checkGroups(t, findings, contract.Production, []group{{120, []string{"a.go:4-25", "b.go:4-25", "c.go:4-25", "d.go:4-25"}}})
	checkMetrics(t, metrics, setMetrics("production", 1, 4*22, 4*23))
}

func TestIdiom(t *testing.T) {
	metrics, findings, _ := Measure(build(t, filepath.Join(fixtures, "clones", "idiom")))
	if len(findings) != 0 {
		t.Errorf("findings = %+v, want none", groups(findings, contract.Production))
	}
	checkMetrics(t, metrics, map[string]contract.Metric{
		"duplication.groups.production":           count(0),
		"duplication.duplicated-lines.production": count(0),
		"duplication.density.production":          {State: contract.Complete, Unit: "ratio", Denominator: 45 + 37},
		"duplication.groups.test":                 count(0),
		"duplication.density.test":                {State: contract.NotApplicable, Unit: "ratio"},
	})
}

// TestNear keeps the 125-symbol run before the edit and drops the 73-symbol run after it.
// The run before the edit ends with the identifier of the changed line 25.
func TestNear(t *testing.T) {
	metrics, findings, _ := Measure(build(t, filepath.Join(fixtures, "clones", "near")))
	checkGroups(t, findings, contract.Production, []group{{125, []string{"a.go:4-25", "b.go:4-25"}}})
	checkMetrics(t, metrics, setMetrics("production", 1, 44, 2*40))
}

func TestWithin(t *testing.T) {
	metrics, findings, _ := Measure(build(t, filepath.Join(fixtures, "clones", "within")))
	// The run starts after the function name, so each copy has 118 symbols.
	checkGroups(t, findings, contract.Production, []group{{118, []string{"within.go:4-25", "within.go:27-48", "within.go:50-71"}}})
	// The file has the package clause and three functions of 22 lines.
	checkMetrics(t, metrics, setMetrics("production", 1, 66, 67))
}

// TestCrossPackageCopy keeps a function copied into another package as a group, and checks that no member of any group overlaps another.
func TestCrossPackageCopy(t *testing.T) {
	_, findings, _ := Measure(build(t, filepath.Join(fixtures, "clones", "selfmatch")))
	for _, f := range findings {
		members := f.Facts.Clone.Members
		for i, a := range members {
			for _, b := range members[i+1:] {
				if a.Path == b.Path && a.StartLine <= b.EndLine && b.StartLine <= a.EndLine {
					t.Errorf("group %s: %s:%d-%d overlaps %s:%d-%d", f.Identity, a.Path, a.StartLine, a.EndLine, b.Path, b.StartLine, b.EndLine)
				}
			}
		}
	}
	checkGroups(t, findings, contract.Production, []group{{214, []string{"render/render.go:4-31", "summary/summary.go:4-31"}}})
}

func TestNested(t *testing.T) {
	metrics, findings, _ := Measure(build(t, filepath.Join(fixtures, "clones", "nested")))
	// The inner run starts at the semicolon that ends the line before the loop, and the loop holds 121 symbols.
	// The line range skips that semicolon, and the third member in light.go keeps the inner group.
	checkGroups(t, findings, contract.Production, []group{
		{321, []string{"a.go:4-62", "b.go:4-62"}},
		{122, []string{"a.go:6-28", "b.go:6-28", "light.go:7-29"}},
	})
	// Only light.go adds lines to the outer members.
	checkMetrics(t, metrics, setMetrics("production", 2, 59+59+23, 60+60+29))
}

// TestLadder reports no group for a switch table, because the longest run of rows overlaps itself and every shorter run lies inside it.
func TestLadder(t *testing.T) {
	metrics, findings, _ := Measure(build(t, filepath.Join(fixtures, "clones", "ladder")))
	// Twelve rows of 15 symbols repeat with a shift of one row, so the longest run holds eleven rows and overlaps itself.
	checkGroups(t, findings, contract.Production, nil)
	checkMetrics(t, metrics, setMetrics("production", 0, 0, 32))
}

// TestSubsumedGroup drops a block that repeats only inside two copies of a larger block.
func TestSubsumedGroup(t *testing.T) {
	// The inner function occurs twice in each outer block with other symbols around each copy, so it forms a maximal repeat of four members.
	inner := strings.Replace(checksum(t), "func ", "func Inner", 1)
	outer := "package p\n\n" + inner + "\nconst c = 1\n\n" + strings.Replace(inner, "Inner", "Again", 1) + "\nvar v = 2\n"
	metrics, findings, _ := Measure(buildSource(t, map[string]string{
		"go.mod": "module example.com/m\n",
		"a.go":   outer,
		"b.go":   outer,
	}))
	// The outer block is the whole file, two functions of 120 symbols and two declarations of 5 symbols.
	checkGroups(t, findings, contract.Production, []group{{250, []string{"a.go:3-51", "b.go:3-51"}}})
	if m := metrics["duplication.groups.production"]; m.Value != 1 {
		t.Errorf("groups metric = %+v, want 1", m)
	}
}

func TestIncompleteSet(t *testing.T) {
	metrics, _, limits := Measure(build(t, filepath.Join(fixtures, "broken")))
	wantErrors := map[string][]string{"production": {"bad.go"}, "test": {"bad_test.go"}}
	for id, m := range metrics {
		set := id[strings.LastIndex(id, ".")+1:]
		if m.State != contract.Incomplete || m.Detail.Limit != nil || !reflect.DeepEqual(m.Detail.Errors, wantErrors[set]) {
			t.Errorf("%s = %+v, want incomplete with errors %v", id, m, wantErrors[set])
		}
		checkNoValue(t, id, m)
	}
	if len(metrics) != 6 || len(limits) != 0 {
		t.Errorf("metrics %v, limits %v", metrics, limits)
	}
}

func checkNoValue(t *testing.T, id string, m contract.Metric) {
	t.Helper()
	if m.Value != 0 || m.Numerator != 0 || m.Denominator != 0 {
		t.Errorf("%s keeps numbers in memory: %+v", id, m)
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

func TestEmptySet(t *testing.T) {
	metrics, findings, _ := Measure(build(t, filepath.Join(fixtures, "empty")))
	checkMetrics(t, metrics, map[string]contract.Metric{
		"duplication.groups.production":           count(0),
		"duplication.duplicated-lines.production": count(0),
		"duplication.density.production":          {State: contract.NotApplicable, Unit: "ratio"},
	})
	if len(findings) != 0 {
		t.Errorf("findings = %v", findings)
	}
}

// TestCaps lowers each cap below the exact fixture and asserts incomplete metrics with the cap named and no value.
func TestCaps(t *testing.T) {
	inv := build(t, filepath.Join(fixtures, "clones", "exact"))
	prodWork := indexWork(inv, contract.Production)
	// Production has 120 + 7 + 120 symbols without the package clauses and imports, and test has 120 + 35 + 120.
	for _, tc := range []struct {
		name               string
		maxTokens, maxWork int
		want               map[contract.SourceSet]*contract.LimitDetail
		wantReason         map[contract.SourceSet]string
	}{
		{
			name: "tokens", maxTokens: 250, maxWork: 1 << 40,
			want:       map[contract.SourceSet]*contract.LimitDetail{contract.Test: {Cap: "tokens", Observed: 275}},
			wantReason: map[contract.SourceSet]string{contract.Test: "tokens cap 250 exceeded"},
		},
		{
			name: "work", maxTokens: 1 << 30, maxWork: 10,
			want: map[contract.SourceSet]*contract.LimitDetail{
				contract.Production: {Cap: "work", Observed: indexWork(inv, contract.Production)},
				contract.Test:       {Cap: "work", Observed: indexWork(inv, contract.Test)},
			},
			wantReason: map[contract.SourceSet]string{contract.Production: "work cap 10 exceeded", contract.Test: "work cap 10 exceeded"},
		},
		{
			// Production passes the cap at the second interval of its clone run, and the larger test set passes it while indexing.
			name: "work during extraction", maxTokens: 1 << 30, maxWork: prodWork + 1,
			want: map[contract.SourceSet]*contract.LimitDetail{
				contract.Production: {Cap: "work", Observed: prodWork + 2},
				contract.Test:       {Cap: "work", Observed: indexWork(inv, contract.Test)},
			},
			wantReason: map[contract.SourceSet]string{
				contract.Production: fmt.Sprintf("work cap %d exceeded", prodWork+1),
				contract.Test:       fmt.Sprintf("work cap %d exceeded", prodWork+1),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metrics, findings, limits := measureWithCaps(inv, tc.maxTokens, tc.maxWork)
			var wantLimits []contract.Limit
			for _, set := range []contract.SourceSet{contract.Production, contract.Test} {
				for _, name := range []string{"density", "duplicated-lines", "groups"} {
					id := "duplication." + name + "." + string(set)
					m := metrics[id]
					if tc.want[set] == nil {
						if m.State != contract.Complete {
							t.Errorf("%s = %+v, want complete", id, m)
						}
						continue
					}
					if m.State != contract.Incomplete || !reflect.DeepEqual(m.Detail.Limit, tc.want[set]) || m.Detail.Errors != nil {
						t.Errorf("%s = %+v with limit %+v, want incomplete with limit %+v", id, m, m.Detail.Limit, tc.want[set])
					}
					checkNoValue(t, id, m)
					wantLimits = append(wantLimits, contract.Limit{MetricID: id, Reason: tc.wantReason[set]})
					if len(groups(findings, set)) != 0 {
						t.Errorf("%s set reports groups past the cap", set)
					}
				}
			}
			sort.Slice(wantLimits, func(i, j int) bool { return wantLimits[i].MetricID < wantLimits[j].MetricID })
			if !reflect.DeepEqual(limits, wantLimits) {
				t.Errorf("limits:\n got %+v\nwant %+v", limits, wantLimits)
			}
		})
	}
}

// indexWork is the work of the suffix array and the LCP array of a set, where the work cap first applies.
func indexWork(inv *inventory.Inventory, set contract.SourceSet) int {
	s := newStream(inv, set, 1<<30)
	work := 0
	suffixArray(s.symbols, s.alphabet(), &work)
	return work + len(s.symbols)
}

// TestCapWithErrors names both the errors and the cap in the detail of a broken set.
func TestCapWithErrors(t *testing.T) {
	metrics, _, limits := measureWithCaps(build(t, filepath.Join(fixtures, "broken")), 1, 1<<40)
	m := metrics["duplication.groups.production"]
	if m.State != contract.Incomplete || !reflect.DeepEqual(m.Detail.Errors, []string{"bad.go"}) || m.Detail.Limit == nil {
		t.Errorf("groups = %+v, want errors and a limit", m)
	}
	if len(limits) != 6 {
		t.Errorf("limits = %v, want 6", limits)
	}
}

// TestDropsImportsAndPackage shows that files that share only a package clause and a long import block do not match.
func TestDropsImportsAndPackage(t *testing.T) {
	var imports strings.Builder
	imports.WriteString("package p\n\nimport (\n")
	for i := range 40 {
		fmt.Fprintf(&imports, "\tx%d \"example.com/m/p%d\"\n", i, i)
	}
	imports.WriteString(")\n\nimport \"fmt\"\n")
	src := imports.String()
	inv := buildSource(t, map[string]string{
		"go.mod": "module example.com/m\n",
		"a.go":   src + "\nvar a = fmt.Sprint(1)\n",
		"b.go":   src + "\nconst b = 2\n",
	})
	metrics, findings, _ := Measure(inv)
	if len(findings) != 0 {
		t.Errorf("findings = %+v, want none", groups(findings, contract.Production))
	}
	// Each file has 45 code lines, of which only the last holds symbols of the stream.
	checkMetrics(t, metrics, map[string]contract.Metric{"duplication.density.production": {State: contract.Complete, Unit: "ratio", Denominator: 90}})
	s := newStream(inv, contract.Production, 1<<30)
	var kinds []string
	for _, sym := range s.symbols {
		if int(sym) < sentinelBase {
			kinds = append(kinds, token.Token(sym).String())
		}
	}
	want := "var IDENT = IDENT . IDENT ( INT ) ; const IDENT = INT ;"
	if got := strings.Join(kinds, " "); got != want {
		t.Errorf("stream = %s, want %s", got, want)
	}
}

// checksum returns the Checksum body as Sum0 of the fourway fixture, from its func keyword to its last newline, with no blank or comment lines.
func checksum(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(fixtures, "clones", "fourway", "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	return string(src[strings.Index(string(src), "func "):])
}

// TestMultiLineString ends two members at a raw string and counts every line that the string spans.
func TestMultiLineString(t *testing.T) {
	// The copies differ right after the string, so both members end at it.
	fn := strings.Replace(checksum(t), "\treturn total\n}", "\t_ = `a\nb\nc`SUFFIX\n\treturn total\n}", 1)
	inv := buildSource(t, map[string]string{
		"go.mod": "module example.com/m\n",
		"a.go":   "package p\n\n" + strings.Replace(fn, "SUFFIX", "", 1),
		"b.go":   "package p\n\n// G is a copy.\n" + strings.Replace(fn, "SUFFIX", ` + "x"`, 1),
	})
	metrics, findings, _ := Measure(inv)
	// The run drops "return total }" with its two semicolons and adds "_ = STRING".
	checkGroups(t, findings, contract.Production, []group{{120 - 5 + 3, []string{"a.go:3-25", "b.go:4-26"}}})
	// Each file has the package clause and 25 lines of the function.
	checkMetrics(t, metrics, setMetrics("production", 1, 2*23, 2*26))
}

// TestShortMember drops a member of two lines, and a group that keeps one member.
func TestShortMember(t *testing.T) {
	// compact holds the symbols of Checksum on two lines, with explicit semicolons where the scanner inserts them.
	compact := "func H(words []string, seed int) int {\n" +
		"total := seed; for i, w := range words { if len(w) > 3 { total += i * len(w); } else { total -= i + 7; }; " +
		"switch { case total > 500: total /= 2; case total < 0: total = -total; }; total = total%1000 + 1; }; " +
		"for len(words) > 0 && total > seed { words = words[1:]; total -= len(words); }; return total; }\n"
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []group
	}{
		{
			name:  "two members left",
			files: map[string]string{"a.go": compact, "b.go": checksum(t), "c.go": checksum(t)},
			want:  []group{{120, []string{"b.go:3-24", "c.go:3-24"}}},
		},
		{
			name:  "one member left",
			files: map[string]string{"a.go": compact, "b.go": checksum(t)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{"go.mod": "module example.com/m\n"}
			for name, src := range tc.files {
				files[name] = "package p\n\n" + src
			}
			_, findings, _ := Measure(buildSource(t, files))
			checkGroups(t, findings, contract.Production, tc.want)
		})
	}
}

// TestFindingsIgnoreFileOrder shuffles the files of a tree and asserts the same metrics and findings.
func TestFindingsIgnoreFileOrder(t *testing.T) {
	for _, name := range []string{"exact", "nested", "within", "fourway", "selfmatch", "ladder"} {
		inv := build(t, filepath.Join(fixtures, "clones", name))
		wantMetrics, wantFindings, _ := Measure(inv)
		rng := rand.New(rand.NewSource(1))
		for range 10 {
			files := inv.Tree.Files
			rng.Shuffle(len(files), func(i, j int) { files[i], files[j] = files[j], files[i] })
			metrics, findings, _ := Measure(inv)
			if !reflect.DeepEqual(metrics, wantMetrics) || !reflect.DeepEqual(findings, wantFindings) {
				t.Fatalf("%s: output changes with the file order:\n got %+v\nwant %+v", name, findings, wantFindings)
			}
		}
	}
}

func TestSortMembers(t *testing.T) {
	want := []member{
		{path: "a.go", startLine: 9, endLine: 20, start: 50},
		{path: "b.go", startLine: 1, endLine: 20, start: 90},
		{path: "b.go", startLine: 3, endLine: 5, start: 10},
		{path: "b.go", startLine: 3, endLine: 6, start: 5},
		{path: "b.go", startLine: 3, endLine: 6, start: 7},
	}
	rng := rand.New(rand.NewSource(1))
	for range 20 {
		got := append([]member(nil), want...)
		rng.Shuffle(len(got), func(i, j int) { got[i], got[j] = got[j], got[i] })
		sortMembers(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("order:\n got %+v\nwant %+v", got, want)
		}
	}
}

func TestSortLimits(t *testing.T) {
	want := []contract.Limit{{MetricID: "duplication.density.test"}, {MetricID: "duplication.groups.production"}, {MetricID: "duplication.groups.test"}}
	rng := rand.New(rand.NewSource(1))
	for range 20 {
		got := append([]contract.Limit(nil), want...)
		rng.Shuffle(len(got), func(i, j int) { got[i], got[j] = got[j], got[i] })
		sortLimits(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("order:\n got %+v\nwant %+v", got, want)
		}
	}
}

// TestGroupID compares the id of a short symbol sequence with the FNV-1a hash of the standard library.
func TestGroupID(t *testing.T) {
	h := fnv.New64a()
	h.Write([]byte("IDENT\x00;\x00"))
	want := fmt.Sprintf("%016x", h.Sum64())
	if got := groupID([]int32{int32(token.IDENT), int32(token.SEMICOLON)}); got != want {
		t.Errorf("groupID = %s, want %s", got, want)
	}
}

func TestMetricIDsSorted(t *testing.T) {
	metrics, _, _ := Measure(build(t, filepath.Join(fixtures, "clones", "exact")))
	ids := make([]string, 0, len(metrics))
	for id := range metrics {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	want := []string{
		"duplication.density.production", "duplication.density.test",
		"duplication.duplicated-lines.production", "duplication.duplicated-lines.test",
		"duplication.groups.production", "duplication.groups.test",
	}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v", ids)
	}
}

// TestTokenCapBoundary measures a set of exactly maxTokens symbols and stops a set of one more.
func TestTokenCapBoundary(t *testing.T) {
	inv := build(t, filepath.Join(fixtures, "clones", "exact"))
	// Production holds 247 symbols without its package clauses and imports.
	for _, tc := range []struct {
		maxTokens int
		want      contract.MetricState
	}{{247, contract.Complete}, {246, contract.Incomplete}} {
		metrics, _, _ := measureWithCaps(inv, tc.maxTokens, 1<<40)
		m := metrics["duplication.groups.production"]
		if m.State != tc.want {
			t.Errorf("cap %d: groups = %+v, want %s", tc.maxTokens, m, tc.want)
		}
		if tc.want == contract.Incomplete && !reflect.DeepEqual(m.Detail.Limit, &contract.LimitDetail{Cap: "tokens", Observed: 247}) {
			t.Errorf("cap %d: limit = %+v, want 247 observed", tc.maxTokens, m.Detail.Limit)
		}
	}
}

// TestLineDirective reports physical lines, because a //line comment must not move a member.
func TestLineDirective(t *testing.T) {
	fn := checksum(t)
	inv := buildSource(t, map[string]string{
		"go.mod": "module example.com/m\n",
		"a.go":   "package p\n\n" + fn,
		"b.go":   "package p\n\n//line other.go:900\n" + strings.Replace(fn, "func ", "func B", 1),
	})
	metrics, findings, _ := Measure(inv)
	checkGroups(t, findings, contract.Production, []group{{120, []string{"a.go:3-24", "b.go:4-25"}}})
	checkMetrics(t, metrics, setMetrics("production", 1, 44, 46))
}

// TestFindingsSorted returns the findings in the order of spec 002, although the walk finds the longer group in c.go first.
func TestFindingsSorted(t *testing.T) {
	heavy, err := os.ReadFile(filepath.Join(fixtures, "clones", "nested", "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	inPackage := strings.Replace(string(heavy), "package nested", "package p", 1)
	inv := buildSource(t, map[string]string{
		"go.mod": "module example.com/m\n",
		"a.go":   "package p\n\n" + checksum(t),
		"b.go":   "package p\n\n" + strings.Replace(checksum(t), "func ", "func B", 1),
		"c.go":   inPackage,
		"d.go":   strings.ReplaceAll(inPackage, "Heavy", "D"),
	})
	_, findings, _ := Measure(inv)
	var got []string
	for _, f := range findings {
		got = append(got, fmt.Sprintf("%s:%d %d", f.Path, f.StartLine, f.Facts.Clone.Tokens))
	}
	if want := []string{"a.go:3 120", "c.go:4 321"}; !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
}

// TestSkipsBrokenFile keeps a file with a parse error out of the stream, although it holds a third copy of a clone.
func TestSkipsBrokenFile(t *testing.T) {
	fn := checksum(t)
	inv := buildSource(t, map[string]string{
		"go.mod": "module example.com/m\n",
		"a.go":   "package p\n\n" + fn + "\n" + strings.Replace(fn, "func ", "func Again", 1),
		// The copy parses up to the last line, where a missing brace breaks the file.
		"bad.go": "package p\n\n" + strings.Replace(fn, "func ", "func Bad", 1) + "\nfunc broken( {\n",
	})
	metrics, findings, _ := Measure(inv)
	checkGroups(t, findings, contract.Production, []group{{120, []string{"a.go:3-24", "a.go:26-47"}}})
	if m := metrics["duplication.groups.production"]; m.State != contract.Incomplete || !reflect.DeepEqual(m.Detail.Errors, []string{"bad.go"}) {
		t.Errorf("groups = %+v, want incomplete with bad.go", m)
	}
}
