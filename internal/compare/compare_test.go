package compare

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/report"
)

const (
	hotspot = "complexity.hotspot"
	clone   = "duplication.clone-group"
)

func rep(index int, findings ...contract.Finding) *report.Report {
	contract.SortFindings(findings)
	return &report.Report{
		SchemaVersion:   "1.1.0",
		AnalyzerVersion: "0.1.0",
		ScoringVersion:  "0.1.0",
		ConfigDigest:    "sha256:aa",
		Metrics: map[string]contract.Metric{
			"duplication.density.production": {State: contract.Complete, Value: 0.25, Unit: "ratio"},
			"complexity.hotspots.production": {State: contract.Complete, Value: 3, Unit: "count"},
		},
		Score:    contract.Score{Index: index},
		Findings: findings,
	}
}

func spot(path string, line int, identity string, ambiguous bool) contract.Finding {
	return contract.Finding{
		Kind: hotspot, Path: path, StartLine: line, EndLine: line + 10, Identity: identity,
		Ambiguous: ambiguous, SourceSet: contract.Production,
		Facts: contract.Facts{Hotspot: &contract.HotspotFacts{CC: 12}},
	}
}

func group(path string, line int, id string) contract.Finding {
	return contract.Finding{
		Kind: clone, Path: path, StartLine: line, EndLine: line + 5, Identity: id,
		SourceSet: contract.Production, Facts: contract.Facts{Clone: &contract.CloneFacts{GroupID: id}},
	}
}

func identities(findings []contract.Finding) []string {
	out := []string{}
	for _, f := range findings {
		out = append(out, f.Identity)
	}
	return out
}

func assertLists(t *testing.T, c *Comparison, newIDs, resolved, persistent []string) {
	t.Helper()
	if !c.Comparable {
		t.Fatalf("refused: %s", c.Refusal)
	}
	if got := identities(c.New); !reflect.DeepEqual(got, newIDs) {
		t.Errorf("new = %v, want %v", got, newIDs)
	}
	if got := identities(c.Resolved); !reflect.DeepEqual(got, resolved) {
		t.Errorf("resolved = %v, want %v", got, resolved)
	}
	if got := identities(c.Persistent); !reflect.DeepEqual(got, persistent) {
		t.Errorf("persistent = %v, want %v", got, persistent)
	}
}

func TestIdenticalReports(t *testing.T) {
	findings := []contract.Finding{spot("a/a.go", 3, "a:F", false), group("a/b.go", 1, "g1")}
	c := Compare(rep(20, findings...), rep(20, findings...))
	assertLists(t, c, []string{}, []string{}, []string{"a:F", "g1"})
	if c.IndexDelta != 0 {
		t.Errorf("index delta = %d", c.IndexDelta)
	}
	for _, m := range c.Metrics {
		if m.Delta == nil || *m.Delta != 0 {
			t.Errorf("%s delta = %v, want 0", m.ID, m.Delta)
		}
	}
}

func TestRenamedFunction(t *testing.T) {
	c := Compare(rep(20, spot("a/a.go", 3, "a:F", false)), rep(20, spot("a/a.go", 3, "a:G", false)))
	assertLists(t, c, []string{"a:G"}, []string{"a:F"}, []string{})
}

func TestMovedFunctionPersists(t *testing.T) {
	c := Compare(rep(20, spot("a/a.go", 3, "a:F", false)), rep(22, spot("a/z.go", 90, "a:F", false)))
	assertLists(t, c, []string{}, []string{}, []string{"a:F"})
	if c.Persistent[0].Path != "a/z.go" || c.Persistent[0].StartLine != 90 {
		t.Errorf("persistent = %+v, want the finding of the after report", c.Persistent[0])
	}
	if c.IndexDelta != 2 {
		t.Errorf("index delta = %d, want 2", c.IndexDelta)
	}
}

func TestReportAgainstItselfIsAllPersistent(t *testing.T) {
	r := rep(20, spot("a/a.go", 3, "a:F#1", true), spot("a/a.go", 30, "a:F#2", true), spot("a/a.go", 60, "a:G", false))
	c := Compare(r, r)
	assertLists(t, c, []string{}, []string{}, []string{"a:F#1", "a:F#2", "a:G"})
}

// TestBrokenReferencesAgainstThemselves compares two equal broken lines of one hook, which differ only by line.
func TestBrokenReferencesAgainstThemselves(t *testing.T) {
	broken := func(line int, identity string) contract.Finding {
		return contract.Finding{
			Kind: contract.KindBrokenReference, Path: ".githooks/pre-commit", StartLine: line, EndLine: line, Identity: identity,
			Facts: contract.Facts{BrokenReference: &contract.BrokenReferenceFacts{Command: "make verify", Ref: "verify"}},
		}
	}
	r := rep(0, broken(2, ".githooks/pre-commit:2:verify"), broken(3, ".githooks/pre-commit:3:verify"))
	assertLists(t, Compare(r, r), []string{}, []string{}, []string{".githooks/pre-commit:2:verify", ".githooks/pre-commit:3:verify"})
}

func TestAmbiguousClosureWithChangedFacts(t *testing.T) {
	before := rep(20, spot("a/a.go", 3, "a:F#1", true), spot("a/a.go", 30, "a:F#2", true))
	changed := spot("a/a.go", 30, "a:F#2", true)
	changed.Facts = contract.Facts{Hotspot: &contract.HotspotFacts{CC: 13}}
	after := rep(20, spot("a/a.go", 5, "a:F#1", true), changed)
	c := Compare(before, after)
	assertLists(t, c, []string{"a:F#2"}, []string{"a:F#2"}, []string{"a:F#1"})
}

func TestAmbiguousIdentityOnAnotherPathNeverMatches(t *testing.T) {
	before := rep(20, spot("a/a.go", 3, "a:F#1", true), spot("a/a.go", 30, "a:F#2", true))
	after := rep(20, spot("a/b.go", 3, "a:F#1", true), spot("a/a.go", 30, "a:F#2", true))
	c := Compare(before, after)
	assertLists(t, c, []string{"a:F#1"}, []string{"a:F#1"}, []string{"a:F#2"})
}

func TestOneAmbiguousClosureOnAPath(t *testing.T) {
	before := rep(20, spot("a/a.go", 3, "a:F#1", true), spot("a/b.go", 3, "a:G", false))
	after := rep(20, spot("a/a.go", 8, "a:F#2", true), spot("a/b.go", 3, "a:G", false))
	c := Compare(before, after)
	assertLists(t, c, []string{}, []string{}, []string{"a:F#2", "a:G"})
}

func TestAmbiguousClosureBesideANamedHotspot(t *testing.T) {
	before := rep(20, spot("a/a.go", 3, "a:F#1", true), spot("a/a.go", 30, "a:G", false))
	after := rep(20, spot("a/a.go", 3, "a:F#2", true), spot("a/a.go", 30, "a:G", false))
	c := Compare(before, after)
	assertLists(t, c, []string{"a:F#2"}, []string{"a:F#1"}, []string{"a:G"})
}

func TestCloneGroupMatchesOnID(t *testing.T) {
	before := rep(20, group("a/a.go", 1, "g1"), group("a/b.go", 1, "g2"))
	after := rep(20, group("a/c.go", 40, "g1"), group("a/b.go", 1, "g3"))
	c := Compare(before, after)
	assertLists(t, c, []string{"g3"}, []string{"g2"}, []string{"g1"})
}

func TestSourceSetsNeverMatch(t *testing.T) {
	testSpot := spot("a/a_test.go", 3, "a:F", false)
	testSpot.SourceSet = contract.Test
	c := Compare(rep(20, spot("a/a.go", 3, "a:F", false)), rep(20, testSpot))
	assertLists(t, c, []string{"a:F"}, []string{"a:F"}, []string{})
	if c.New[0].SourceSet != contract.Test || c.Resolved[0].SourceSet != contract.Production {
		t.Errorf("new %v resolved %v", c.New[0].SourceSet, c.Resolved[0].SourceSet)
	}
}

func TestListsSortedAfterShuffle(t *testing.T) {
	before := []contract.Finding{
		spot("b/b.go", 3, "b:F", false), spot("a/a.go", 3, "a:F", false), group("c/c.go", 1, "g1"),
		spot("d/d.go", 1, "d:X", false), group("a/a.go", 9, "g2"),
	}
	after := []contract.Finding{
		spot("b/b.go", 5, "b:F", false), spot("a/a.go", 3, "a:G", false), group("c/c.go", 1, "g1"),
		spot("e/e.go", 1, "e:Y", false), group("a/a.go", 9, "g3"),
	}
	want := Compare(rep(20, before...), rep(20, after...))
	for _, list := range [][]contract.Finding{want.New, want.Resolved, want.Persistent} {
		if i := contract.UnsortedFinding(list); i != -1 {
			t.Errorf("list %v unsorted at %d", identities(list), i)
		}
	}
	rng := rand.New(rand.NewSource(1))
	for range 20 {
		b, a := rep(20, before...), rep(20, after...)
		rng.Shuffle(len(b.Findings), func(i, j int) { b.Findings[i], b.Findings[j] = b.Findings[j], b.Findings[i] })
		rng.Shuffle(len(a.Findings), func(i, j int) { a.Findings[i], a.Findings[j] = a.Findings[j], a.Findings[i] })
		if got := Compare(b, a); !reflect.DeepEqual(got, want) {
			t.Fatalf("shuffled input changed the comparison")
		}
	}
}

func TestRefusals(t *testing.T) {
	cases := map[string]func(r *report.Report){
		"schemaVersion":   func(r *report.Report) { r.SchemaVersion = "2.0.0" },
		"analyzerVersion": func(r *report.Report) { r.AnalyzerVersion = "0.2.0" },
		"scoringVersion":  func(r *report.Report) { r.ScoringVersion = "0.2.0" },
		"configDigest":    func(r *report.Report) { r.ConfigDigest = "sha256:bb" },
	}
	for field, change := range cases {
		t.Run(field, func(t *testing.T) {
			before, after := rep(20, spot("a/a.go", 3, "a:F", false)), rep(25, spot("a/a.go", 3, "a:F", false))
			change(after)
			c := Compare(before, after)
			if c.Comparable {
				t.Fatal("want a refusal")
			}
			if !strings.HasPrefix(c.Refusal, field+" ") {
				t.Errorf("refusal = %q, want it to name %s", c.Refusal, field)
			}
			if c.IndexDelta != 0 || c.Metrics != nil || c.New != nil || c.Resolved != nil || c.Persistent != nil {
				t.Errorf("refused comparison has deltas or findings: %+v", c)
			}
			if c.Before.Index != 20 || c.After.Index != 25 {
				t.Errorf("summaries = %+v, %+v", c.Before, c.After)
			}
		})
	}
}

func TestRefusalNamesFirstDifference(t *testing.T) {
	before, after := rep(20), rep(20)
	after.AnalyzerVersion, after.ConfigDigest = "0.2.0", "sha256:bb"
	c := Compare(before, after)
	want := `analyzerVersion differs: before "0.1.0", after "0.2.0"`
	if c.Refusal != want {
		t.Errorf("refusal = %q, want %q", c.Refusal, want)
	}
}

func TestIncompleteMetricHasNoDelta(t *testing.T) {
	before, after := rep(20), rep(20)
	after.Metrics["duplication.density.production"] = contract.Metric{State: contract.Incomplete, Unit: "ratio"}
	after.Metrics["duplication.groups.production"] = contract.Metric{State: contract.Complete, Value: 1, Unit: "count"}
	after.Metrics["complexity.hotspots.production"] = contract.Metric{State: contract.Complete, Value: 5, Unit: "count"}
	c := Compare(before, after)
	var ids []string
	for _, m := range c.Metrics {
		ids = append(ids, m.ID)
		if m.ID == "complexity.hotspots.production" && (m.Delta == nil || *m.Delta != 2) {
			t.Errorf("hotspots delta = %v, want 2", m.Delta)
		}
		if m.ID != "complexity.hotspots.production" && m.Delta != nil {
			t.Errorf("%s delta = %v, want nil", m.ID, *m.Delta)
		}
	}
	want := []string{"complexity.hotspots.production", "duplication.density.production", "duplication.groups.production"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("metric ids = %v, want %v", ids, want)
	}
}

func TestJSONNames(t *testing.T) {
	data, err := json.Marshal(Compare(rep(20), rep(21)))
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"comparable", "before", "after", "indexDelta", "metrics", "new", "resolved", "persistent"} {
		if _, ok := keys[key]; !ok {
			t.Errorf("JSON lacks %q: %s", key, data)
		}
	}
	if _, ok := keys["refusal"]; ok {
		t.Errorf("JSON has an empty refusal: %s", data)
	}
	if !strings.Contains(string(data), `"configDigest":"sha256:aa"`) {
		t.Errorf("summary JSON = %s", data)
	}
}

func TestOneSidedMetricOmitsTheAbsentSide(t *testing.T) {
	after := rep(20)
	after.Metrics["duplication.groups.production"] = contract.Metric{State: contract.Complete, Value: 1, Unit: "count"}
	c := Compare(rep(20), after)
	for _, m := range c.Metrics {
		if m.ID != "duplication.groups.production" {
			continue
		}
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		want := `{"id":"duplication.groups.production","after":{"state":"complete","value":1,"unit":"count","detail":{}},"delta":null}`
		if string(data) != want {
			t.Errorf("JSON = %s, want %s", data, want)
		}
		return
	}
	t.Fatal("the one-sided metric is missing")
}

func TestDuplicateKeyNeverMatches(t *testing.T) {
	before := rep(20, spot("a/a.go", 3, "a:F", false))
	after := rep(20, spot("a/a.go", 3, "a:F", false), spot("a/b.go", 3, "a:F", false))
	c := Compare(before, after)
	assertLists(t, c, []string{"a:F", "a:F"}, []string{"a:F"}, []string{})
}

// TestSoleRuleCountsOnlyItsKind asserts that a clone group on the same path does not stop a sole closure from matching.
func TestSoleRuleCountsOnlyItsKind(t *testing.T) {
	before := rep(20, spot("a/a.go", 3, "a:F#1", true), group("a/a.go", 40, "g1"))
	after := rep(20, spot("a/a.go", 8, "a:F#2", true), group("a/a.go", 40, "g1"))
	c := Compare(before, after)
	assertLists(t, c, []string{}, []string{}, []string{"a:F#2", "g1"})
}

func TestRefusedJSONOmitsIndexDelta(t *testing.T) {
	after := rep(25)
	after.ConfigDigest = "sha256:bb"
	data, err := json.Marshal(Compare(rep(20), after))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "indexDelta") {
		t.Errorf("refused JSON has an index delta: %s", data)
	}
	if data, _ = json.Marshal(Compare(rep(20), rep(20))); !strings.Contains(string(data), `"indexDelta":0`) {
		t.Errorf("comparable JSON lacks a zero index delta: %s", data)
	}
}
