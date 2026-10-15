package contract

import (
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
	"testing"
)

func TestMetricJSON(t *testing.T) {
	for _, tc := range []struct {
		name   string
		metric Metric
		want   string
	}{
		{
			name:   "count",
			metric: Metric{State: Complete, Value: 3, Unit: "count"},
			want:   `{"state":"complete","value":3,"unit":"count","detail":{}}`,
		},
		{
			name:   "ratio",
			metric: Metric{State: Complete, Value: 0.25, Unit: "ratio", Numerator: 1, Denominator: 4},
			want:   `{"state":"complete","value":0.25,"unit":"ratio","numerator":1,"denominator":4,"detail":{}}`,
		},
		{
			name:   "zero value",
			metric: Metric{State: Complete, Unit: "count"},
			want:   `{"state":"complete","value":0,"unit":"count","detail":{}}`,
		},
		{
			name:   "not applicable",
			metric: Metric{State: NotApplicable, Value: 1, Unit: "ratio", Numerator: 1, Denominator: 1},
			want:   `{"state":"not-applicable","unit":"ratio","detail":{}}`,
		},
		{
			name:   "incomplete",
			metric: Metric{State: Incomplete, Value: 1, Unit: "count", Detail: Detail{Errors: []string{"a.go"}}},
			want:   `{"state":"incomplete","unit":"count","detail":{"errors":["a.go"]}}`,
		},
		{
			name:   "limit",
			metric: Metric{State: Incomplete, Unit: "count", Detail: Detail{Limit: &LimitDetail{Cap: "tokens", Observed: 7}}},
			want:   `{"state":"incomplete","unit":"count","detail":{"limit":{"cap":"tokens","observed":7}}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.metric)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("JSON = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestScoreJSON(t *testing.T) {
	s := Score{
		Index: 25, Direction: "lower-is-better",
		Contributions: []Contribution{{
			Dimension: "complexity-erosion", Points: 25, Weight: 0.6,
			Terms: []Term{
				{MetricID: "erosion.excess-share.production", State: Complete, Value: 0.18, SaturatesAt: 0.25, Score: 72},
				{MetricID: "erosion.excess-mass.production", State: Complete, Value: 3, CountScale: 20, Score: 12.5},
			},
		}},
	}
	want := `{"index":25,"direction":"lower-is-better","partial":false,"contributions":[` +
		`{"dimension":"complexity-erosion","points":25,"weight":0.6,"terms":[` +
		`{"metricId":"erosion.excess-share.production","state":"complete","value":0.18,"saturatesAt":0.25,"score":72},` +
		`{"metricId":"erosion.excess-mass.production","state":"complete","value":3,"countScale":20,"score":12.5}]}]}`
	got, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("JSON = %s\nwant   %s", got, want)
	}
	unmeasured := Term{MetricID: "duplication.groups.production", State: Incomplete, Value: 4, CountScale: 15, Score: 100}
	got, err = json.Marshal(unmeasured)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"metricId":"duplication.groups.production","state":"incomplete","countScale":15,"score":100}`; string(got) != want {
		t.Errorf("incomplete term JSON = %s, want %s", got, want)
	}
}

func TestFactsJSON(t *testing.T) {
	hotspot := &HotspotFacts{CC: 11, Nesting: 2, SLOC: 9, Mass: 33}
	clone := &CloneFacts{GroupID: "00ff", Tokens: 120, Members: []CloneMember{{Path: "a.go", StartLine: 3, EndLine: 9}}}
	broken := &BrokenReferenceFacts{Command: "make verify", Ref: "verify"}
	for _, tc := range []struct {
		name  string
		facts Facts
		want  string
	}{
		{name: "hotspot", facts: Facts{Hotspot: hotspot}, want: `{"cc":11,"nesting":2,"sloc":9,"mass":33}`},
		{
			name:  "clone",
			facts: Facts{Clone: clone},
			want:  `{"groupId":"00ff","tokens":120,"members":[{"path":"a.go","startLine":3,"endLine":9}]}`,
		},
		{name: "broken reference", facts: Facts{BrokenReference: broken}, want: `{"command":"make verify","ref":"verify"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.facts)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("JSON = %s, want %s", got, tc.want)
			}
		})
	}
	for _, facts := range []Facts{{}, {Hotspot: hotspot, Clone: clone}, {Clone: clone, BrokenReference: broken}} {
		if _, err := json.Marshal(facts); !errors.Is(err, errFacts) {
			t.Errorf("Marshal(%+v) error = %v, want %v", facts, err, errFacts)
		}
	}
}

// TestSortFindings shuffles findings that differ in one key each and asserts one order.
func TestSortFindings(t *testing.T) {
	want := []Finding{
		{Kind: "a", Path: "z.go", StartLine: 9, Identity: "z"},
		{Kind: "b", Path: "a.go", StartLine: 9, Identity: "z"},
		{Kind: "b", Path: "b.go", StartLine: 1, Identity: "z"},
		{Kind: "b", Path: "b.go", StartLine: 2, Identity: "a"},
		{Kind: "b", Path: "b.go", StartLine: 2, Identity: "b"},
	}
	rng := rand.New(rand.NewSource(1))
	for range 20 {
		got := append([]Finding(nil), want...)
		rng.Shuffle(len(got), func(i, j int) { got[i], got[j] = got[j], got[i] })
		SortFindings(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("order:\n got %+v\nwant %+v", got, want)
		}
	}
}

func TestSafeguardJSON(t *testing.T) {
	s := Safeguard{ID: "lint-config", Evidence: EvidenceConfigured, Locations: []Location{{Path: ".golangci.yml", Line: 1}, {Path: "Makefile"}}, Notes: "A note."}
	got, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"lint-config","evidence":"configured","locations":[{"path":".golangci.yml","line":1},{"path":"Makefile"}],"notes":"A note."}`
	if string(got) != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}
