package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/formula"
)

func TestNewVersionsAndScore(t *testing.T) {
	r := newReport(t, "clones/nested")
	if r.SchemaVersion != SchemaVersion || SchemaVersion != "1.0.0" {
		t.Errorf("schema version = %q", r.SchemaVersion)
	}
	if r.AnalyzerVersion != testAnalyzer || r.ScoringVersion != formula.ScoringVersion {
		t.Errorf("versions = %q, %q", r.AnalyzerVersion, r.ScoringVersion)
	}
	if r.ConfigDigest != contract.Digest(contract.Config{}) {
		t.Errorf("config digest = %q", r.ConfigDigest)
	}
	if r.Completeness != Complete {
		t.Errorf("completeness = %q, want complete", r.Completeness)
	}
	if want := formula.Score(r.Metrics); r.Score.Index != want.Index || r.Score.Index == 0 {
		t.Errorf("score index = %d, want %d and above 0", r.Score.Index, want.Index)
	}
}

func TestCompletenessFollowsProductionOnly(t *testing.T) {
	incomplete := contract.Metric{State: contract.Incomplete, Unit: "count"}
	if got := completeness(map[string]contract.Metric{"erosion.mass.test": incomplete}); got != Complete {
		t.Errorf("incomplete test metric: completeness %q, want complete", got)
	}
	if got := completeness(map[string]contract.Metric{"erosion.mass.production": incomplete}); got != Incomplete {
		t.Errorf("incomplete production metric: completeness %q, want incomplete", got)
	}
	if r := newReport(t, "broken"); r.Completeness != Incomplete || !r.Score.Partial {
		t.Errorf("broken fixture: completeness %q, partial %v", r.Completeness, r.Score.Partial)
	}
}

// TestLoadRoundTrip marshals each fixture report, loads it, and marshals it again.
func TestLoadRoundTrip(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			r := newReport(t, name)
			r.Meta.DurationMs = 812
			first, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(bytes.NewReader(first))
			if err != nil {
				t.Fatal(err)
			}
			second, err := json.Marshal(loaded)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, second) {
				t.Errorf("round trip differs:\n%s\n%s", first, second)
			}
		})
	}
}

// TestLoadRejects edits one field of a valid report and asserts that the error names it.
func TestLoadRejects(t *testing.T) {
	valid, err := json.Marshal(newReport(t, "clones/nested"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, old, replacement, field string
	}{
		{"unknown field", `"repo":`, `"extra":1,"repo":`, "extra"},
		{"schema version", `"schemaVersion":"1.0.0"`, `"schemaVersion":"2.0.0"`, "schemaVersion"},
		{"metric state", `"state":"complete"`, `"state":"done"`, "state"},
		{"metric type", `"state":"complete"`, `"state":7`, "state"},
		{"metric value", `"state":"not-applicable","unit":"ratio"`, `"state":"not-applicable","value":1,"unit":"ratio"`, "value"},
		{"completeness", `"completeness":"complete"`, `"completeness":"most"`, "completeness"},
		{"direction", `"direction":"lower-is-better"`, `"direction":"up"`, "score.direction"},
		{"trailing object", `"meta":{"durationMs":0}}`, `"meta":{"durationMs":0}}{}`, "(end)"},
		{"trailing brace", `"meta":{"durationMs":0}}`, `"meta":{"durationMs":0}}}`, "(end)"},
		{"trailing bracket", `"meta":{"durationMs":0}}`, `"meta":{"durationMs":0}}]`, "(end)"},
		{"term value", `"metricId":"erosion.excess-share.production","state":"complete"`, `"metricId":"erosion.excess-share.production","state":"incomplete"`, "value"},
		{"metric unit", `"unit":"count",`, ``, "unit"},
		{"null locations", `"locations":[]`, `"locations":null`, "locations"},
		{"missing notes", `,"notes":"No agent hook is declared."`, ``, "notes"},
		{"finding kind", `"kind":"complexity.hotspot"`, `"kind":"complexity.spot"`, "findings[0].kind"},
		{"finding set", `"sourceSet":"production"`, `"sourceSet":"generated"`, "findings[0].sourceSet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(string(valid), tc.old) {
				t.Fatalf("report lacks %s", tc.old)
			}
			edited := strings.Replace(string(valid), tc.old, tc.replacement, 1)
			_, err := Load(strings.NewReader(edited))
			assertField(t, err, tc.field)
		})
	}
}

// TestLoadValidates builds reports that decode cleanly and break one rule each.
func TestLoadValidates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edit  func(r *Report)
		field string
	}{
		{"index above 100", func(r *Report) { r.Score.Index = 101 }, "score.index"},
		{"index below 0", func(r *Report) { r.Score.Index = -1 }, "score.index"},
		{"contributions", func(r *Report) { r.Score.Contributions[0].Points++ }, "score.contributions"},
		{"findings order", func(r *Report) { r.Findings[0], r.Findings[1] = r.Findings[1], r.Findings[0] }, "findings[1]"},
		{"clone facts on a hotspot", func(r *Report) { r.Findings[0].Facts = r.Findings[2].Facts }, "findings[0].facts"},
		{"hotspot facts on a clone", func(r *Report) { r.Findings[2].Facts = r.Findings[0].Facts }, "findings[2].facts"},
		{"safeguard evidence", func(r *Report) { r.Safeguards[0].Evidence = "wired" }, "safeguards[0].evidence"},
		{"safeguard order", func(r *Report) { r.Safeguards[0], r.Safeguards[1] = r.Safeguards[1], r.Safeguards[0] }, "safeguards[0].id"},
		{"safeguard repeat", func(r *Report) { r.Safeguards[1].ID = r.Safeguards[0].ID }, "safeguards[1].id"},
		{"unknown safeguard", func(r *Report) { r.Safeguards[7].ID = "zz-check" }, "safeguards[7].id"},
		{"missing safeguard", func(r *Report) { r.Safeguards = r.Safeguards[1:] }, "safeguards"},
		{"negative location line", func(r *Report) { r.Safeguards[0].Locations = []contract.Location{{Path: "a", Line: -1}} }, "safeguards[0].locations[0].line"},
		{"finding without identity", func(r *Report) { r.Findings[0].Identity = "" }, "findings[0].identity"},
		{"extra safeguard", func(r *Report) { r.Safeguards = append(r.Safeguards, r.Safeguards[7]) }, "safeguards"},
		{"broken reference with a set", func(r *Report) { r.Findings = []contract.Finding{brokenReference(contract.Production)} }, "findings[0].sourceSet"},
		{"broken reference with a bogus set", func(r *Report) { r.Findings = []contract.Finding{brokenReference("bogus")} }, "findings[0].sourceSet"},
		{"hotspot facts on a broken reference", func(r *Report) {
			f := brokenReference("")
			f.Facts = r.Findings[0].Facts
			r.Findings = []contract.Finding{f}
		}, "findings[0].facts"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newReport(t, "clones/nested")
			tc.edit(r)
			data, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Load(bytes.NewReader(data))
			assertField(t, err, tc.field)
		})
	}
}

// brokenReference builds a finding of a hook line that names a missing make target.
func brokenReference(set contract.SourceSet) contract.Finding {
	return contract.Finding{
		Kind: contract.KindBrokenReference, Path: ".githooks/pre-commit", StartLine: 3, EndLine: 3,
		Identity: ".githooks/pre-commit:verify", SourceSet: set,
		Facts: contract.Facts{BrokenReference: &contract.BrokenReferenceFacts{Command: "make verify", Ref: "verify"}},
	}
}

func TestLoadAcceptsBrokenReference(t *testing.T) {
	r := newReport(t, "clones/nested")
	r.Findings = append(r.Findings, brokenReference(""))
	contract.SortFindings(r.Findings)
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"sourceSet":""`) {
		t.Errorf("a broken reference writes an empty source set")
	}
	if _, err := Load(bytes.NewReader(data)); err != nil {
		t.Errorf("Load: %v", err)
	}
}

func assertField(t *testing.T, err error, field string) {
	t.Helper()
	var fieldErr *contract.FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Field != field {
		t.Errorf("error %v, want a FieldError naming %q", err, field)
	}
}

// TestLoadRejectsHollowFacts replaces the facts of a hotspot and of a clone group with null and with an empty object, which reads as hotspot facts without cc.
func TestLoadRejectsHollowFacts(t *testing.T) {
	r := newReport(t, "clones/nested")
	valid, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, 2} {
		facts, err := json.Marshal(r.Findings[i].Facts)
		if err != nil {
			t.Fatal(err)
		}
		for replacement, field := range map[string]string{"null": "facts", "{}": "facts.cc"} {
			edited := strings.Replace(string(valid), `"facts":`+string(facts), `"facts":`+replacement, 1)
			_, err := Load(strings.NewReader(edited))
			assertField(t, err, field)
		}
	}
}
