package report

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/complexity"
	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/duplication"
)

func render(t *testing.T, r *Report) string {
	t.Helper()
	var b bytes.Buffer
	if err := Render(&b, r); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func lines(s string) []string { return strings.Split(s, "\n") }

// lineWith returns the first line that starts with the prefix, and it fails the test when there is none.
func lineWith(t *testing.T, out, prefix string) string {
	t.Helper()
	for _, l := range lines(out) {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	t.Fatalf("output lacks a line that starts with %q:\n%s", prefix, out)
	return ""
}

func TestTerminalGolden(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			compareGolden(t, "terminal-"+strings.ReplaceAll(name, "/", "-")+".txt", []byte(render(t, newReport(t, name))))
		})
	}
}

func TestTerminalHeaderAndIndex(t *testing.T) {
	r := newReport(t, "functions")
	got := lines(render(t, r))
	if got[0] != "godebt "+testAnalyzer+"  example.com/functions" || got[1] != "" {
		t.Errorf("header = %q, %q", got[0], got[1])
	}
	want := fmt.Sprintf("index  %d/100  lower is better  scoring %s", r.Score.Index, r.ScoringVersion)
	if got[2] != want {
		t.Errorf("index line = %q, want %q", got[2], want)
	}
	contributions := fmt.Sprintf("       complexity-erosion %d  duplication %d", r.Score.Contributions[0].Points, r.Score.Contributions[1].Points)
	if got[3] != contributions {
		t.Errorf("contributions line = %q, want %q", got[3], contributions)
	}
	if out := render(t, &Report{}); !strings.HasPrefix(out, "godebt   (no go.mod)\n\nindex  0/100  lower is better  scoring \n\n") {
		t.Errorf("empty report:\n%s", out)
	}
}

func TestTerminalPartial(t *testing.T) {
	got := lines(render(t, newReport(t, "broken")))
	if !strings.HasSuffix(got[2], "scoring 0.2.0 (partial)") {
		t.Errorf("index line = %q, want the partial mark", got[2])
	}
	if got[3] != "incomplete: bad.go, bad_test.go" {
		t.Errorf("incomplete line = %q", got[3])
	}
}

func TestTerminalIncompleteReasons(t *testing.T) {
	stopped := contract.Metric{
		State: contract.Incomplete, Unit: "count",
		Detail: contract.Detail{Errors: []string{"b.go", "a.go"}, Limit: &contract.LimitDetail{Cap: "tokens", Observed: 2417311}},
	}
	r := &Report{
		Metrics: map[string]contract.Metric{"duplication.groups.production": stopped, "duplication.density.production": stopped},
		Limits: []contract.Limit{
			{MetricID: "duplication.density.production", Reason: "tokens cap 2000000 exceeded"},
			{MetricID: "duplication.groups.production", Reason: "tokens cap 2000000 exceeded"},
		},
	}
	want := "incomplete: a.go, b.go, tokens cap 2000000 exceeded (production)"
	if got := lineWith(t, render(t, r), "incomplete:"); got != want {
		t.Errorf("incomplete line = %q, want %q", got, want)
	}
}

func TestTerminalTable(t *testing.T) {
	for _, tc := range []struct {
		fixture          string
		production, test []string
	}{
		{
			"functions",
			[]string{"production", "9", "146", "23", "1/11/15", "3", "(52%)", "0", "0"},
			[]string{"test", "1", "13", "3", "2/12/12", "1", "(77%)", "0", "0"},
		},
		{
			"clones/nested",
			[]string{"production", "3", "149", "3", "17/17/17", "2", "(88%)", "2", "141", "(95%)"},
			[]string{"test", "0", "0", "0", "-", "-", "0", "0"},
		},
		{
			"broken",
			[]string{"production", "2", "2", "-", "-", "-", "-", "-"},
			[]string{"test", "2", "5", "-", "-", "-", "-", "-"},
		},
	} {
		out := render(t, newReport(t, tc.fixture))
		for _, want := range [][]string{tc.production, tc.test} {
			if got := strings.Fields(lineWith(t, out, want[0]+" ")); !reflect.DeepEqual(got, want) {
				t.Errorf("%s: %s row = %q, want %q", tc.fixture, want[0], got, want)
			}
		}
	}
}

// TestTerminalTableWidths asserts that the header and both rows have one width, so every column lines up.
func TestTerminalTableWidths(t *testing.T) {
	for _, fixture := range []string{"functions", "broken", "empty", "clones/nested"} {
		out := render(t, newReport(t, fixture))
		header := lineWith(t, out, strings.Repeat(" ", 19)+"files")
		for _, prefix := range []string{"production ", "test "} {
			if row := lineWith(t, out, prefix); len(row) != len(header) {
				t.Errorf("%s: %q is %d wide, header %d", fixture, row, len(row), len(header))
			}
		}
		if !strings.HasSuffix(header, "dup lines") {
			t.Errorf("%s: header = %q", fixture, header)
		}
	}
}

func TestTerminalOtherSets(t *testing.T) {
	if got := lineWith(t, render(t, newReport(t, "sets")), "other:"); got != "other: generated 1  vendored 1  testdata 1  excluded 6  unsupported 2" {
		t.Errorf("other line = %q", got)
	}
	r := &Report{Coverage: Coverage{Testdata: Counted{Files: 45}}}
	if got := lineWith(t, render(t, r), "other:"); got != "other: testdata 45  unsupported 0" {
		t.Errorf("other line = %q", got)
	}
}

func TestTerminalLists(t *testing.T) {
	out := render(t, newReport(t, "functions"))
	want := strings.Join([]string{
		"hotspots (production, 3)",
		"  66.0  cc 11  cc.go:35-70  Long",
		"  30.0  cc 15  cc.go:73-76  Short",
		"  19.1  cc 11  cc.go:79-81  wrap#1",
		"",
		"clones (production, 0)",
		"",
		"hotspots (test, 1)",
		"  24.0  cc 12  functions_test.go:14-17  allSet",
		"",
		"clones (test, 0)",
		"",
	}, "\n")
	if !strings.Contains(out, want+"\nsafeguards\n") {
		t.Errorf("lists:\n%s\nwant, before the safeguards:\n%s", out, want)
	}
	clones := render(t, newReport(t, "clones/nested"))
	want = strings.Join([]string{
		"clones (production, 2)",
		"  321 tokens  2 members  4801221e89fa382b",
		"    a.go:4-62",
		"    b.go:4-62",
		"  122 tokens  3 members  a9131f502c4085e8",
		"    a.go:6-28",
		"    b.go:6-28",
		"    light.go:7-29",
		"",
	}, "\n")
	if !strings.Contains(clones, want) {
		t.Errorf("clones:\n%s\nwant:\n%s", clones, want)
	}
}

func TestTerminalSafeguards(t *testing.T) {
	r := &Report{Safeguards: []contract.Safeguard{
		{ID: "agent-hooks", Evidence: contract.EvidenceAbsent, Locations: []contract.Location{}},
		{ID: "pre-commit-hook", Evidence: contract.EvidenceStructurallyWired, Locations: []contract.Location{{Path: "Makefile", Line: 12}, {Path: ".githooks/pre-commit"}}, Notes: "Not shown."},
		{ID: "test-check", Evidence: contract.EvidenceUnknown, Locations: []contract.Location{{Path: "a", Line: 1}, {Path: "b"}, {Path: "c", Line: 3}, {Path: "d"}, {Path: "e"}}},
	}}
	want := strings.Join([]string{
		"",
		"safeguards",
		"  agent-hooks        absent",
		"  pre-commit-hook    structurally-wired  Makefile:12 .githooks/pre-commit",
		"  test-check         unknown             a:1 b c:3 +2",
		"",
	}, "\n")
	if out := render(t, r); !strings.HasSuffix(out, want) {
		t.Errorf("safeguards:\n%s\nwant suffix:\n%s", out, want)
	}
	if out := render(t, &Report{}); strings.Contains(out, "safeguards") {
		t.Errorf("a report without safeguards prints the block:\n%s", out)
	}
}

func TestTerminalBrokenReferences(t *testing.T) {
	var findings []contract.Finding
	for i := 1; i <= 12; i++ {
		findings = append(findings, contract.Finding{
			Kind: contract.KindBrokenReference, Path: "Makefile", StartLine: i, EndLine: i,
			Facts: contract.Facts{BrokenReference: &contract.BrokenReferenceFacts{Command: "make gone", Ref: "gone"}},
		})
	}
	out := render(t, &Report{Findings: findings})
	if !strings.Contains(out, "\nbroken references (12, showing 10)\n  Makefile:1  make gone\n") || strings.Contains(out, "Makefile:11") {
		t.Errorf("broken references:\n%s", out)
	}
	if strings.Contains(markdown(t, &Report{Findings: findings}), "Makefile:11") {
		t.Error("the Markdown list is not bounded")
	}
	if out := render(t, &Report{}); strings.Contains(out, "broken references") {
		t.Errorf("a report without broken references prints the block:\n%s", out)
	}
}

// TestTerminalPathColumn pads the locations of one list to one width, so the identities line up.
func TestTerminalPathColumn(t *testing.T) {
	r := &Report{Findings: []contract.Finding{
		withCC(hotspot("a.go", 1, ".:A", 20), 9), hotspot("internal/long/path.go", 100, "internal/long:B", 10),
	}}
	out := render(t, r)
	for _, want := range []string{
		"  20.0  cc  9  a.go:1-2                     A\n",
		"  10.0  cc 11  internal/long/path.go:100-2  B\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q:\n%s", want, out)
		}
	}
}

func hotspot(path string, line int, identity string, mass float64) contract.Finding {
	return contract.Finding{
		Kind: complexity.KindHotspot, Path: path, StartLine: line, EndLine: 2, Identity: identity,
		SourceSet: contract.Production, Facts: contract.Facts{Hotspot: &contract.HotspotFacts{CC: 11, SLOC: 1, Mass: mass}},
	}
}

func withCC(f contract.Finding, cc int) contract.Finding {
	f.Facts.Hotspot.CC = cc
	return f
}

func TestTerminalHotspotBound(t *testing.T) {
	r := &Report{}
	for i := range 12 {
		r.Findings = append(r.Findings, hotspot(fmt.Sprintf("f%02d.go", i), 1, fmt.Sprintf(".:F%02d", i), float64(i)))
	}
	out := render(t, r)
	if !strings.Contains(out, "hotspots (production, 12, showing 10)\n  11.0  cc 11  f11.go:1-2  F11\n  10.0  cc 11  f10.go:1-2  F10\n") {
		t.Errorf("render lacks the bounded list with the largest mass first:\n%s", out)
	}
	if strings.Contains(out, "F01") || !strings.Contains(out, "F02") {
		t.Errorf("render shows the wrong ten hotspots:\n%s", out)
	}
}

// TestTerminalHotspotTies shuffles hotspots of equal mass into a report and asserts that the list keeps the report order.
func TestTerminalHotspotTies(t *testing.T) {
	// Thirty entries of two masses, because sort.Slice keeps a short or already sorted list stable by chance.
	var findings []contract.Finding
	var want []string
	for i := range 30 {
		mass := float64(11 - 6*(i%2))
		findings = append(findings, hotspot(fmt.Sprintf("f%02d.go", i/2), 1+i%2, fmt.Sprintf(".:F%02d", i), mass))
		if mass == 11 && len(want) < 10 {
			want = append(want, fmt.Sprintf("F%02d", i))
		}
	}
	rng := rand.New(rand.NewSource(1))
	for range 20 {
		shuffled := append([]contract.Finding(nil), findings...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		// A report holds its findings sorted by path, line, and identity, which here is the index order.
		sort.Slice(shuffled, func(i, j int) bool { return shuffled[i].Identity < shuffled[j].Identity })
		all := lines(render(t, &Report{Findings: shuffled}))
		start := slices.Index(all, "hotspots (production, 30, showing 10)") + 1
		if start == 0 {
			t.Fatalf("render lacks the hotspot header:\n%s", strings.Join(all, "\n"))
		}
		var got []string
		for _, l := range all[start : start+10] {
			fields := strings.Fields(l)
			got = append(got, fields[len(fields)-1])
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("hotspots = %v, want %v", got, want)
		}
	}
}

func TestTerminalCloneBound(t *testing.T) {
	r := &Report{}
	for i := range 14 {
		r.Findings = append(r.Findings, contract.Finding{
			Kind: duplication.KindCloneGroup, Path: fmt.Sprintf("f%02d.go", i), StartLine: 1, EndLine: 5, SourceSet: contract.Test,
			Facts: contract.Facts{Clone: &contract.CloneFacts{GroupID: fmt.Sprintf("%016x", i), Tokens: 100 + 10*(i/2), Members: []contract.CloneMember{
				{Path: fmt.Sprintf("f%02d.go", i), StartLine: 1, EndLine: 5}, {Path: "z.go", StartLine: i + 1, EndLine: i + 5},
			}}},
		})
	}
	out := render(t, r)
	want := "clones (test, 14, showing 10)\n" +
		"  160 tokens  2 members  000000000000000c\n    f12.go:1-5\n    z.go:13-17\n" +
		"  160 tokens  2 members  000000000000000d\n"
	if !strings.Contains(out, want) {
		t.Errorf("render lacks the bounded list:\n%s\nwant:\n%s", out, want)
	}
	if strings.Contains(out, "0000000000000003") || !strings.Contains(out, "0000000000000004") {
		t.Errorf("render shows the wrong ten groups:\n%s", out)
	}
}

func TestTerminalUnmeasuredList(t *testing.T) {
	out := render(t, newReport(t, "broken"))
	for _, header := range []string{"hotspots (production, -)", "clones (production, -)", "hotspots (test, -)", "clones (test, -)"} {
		if !slices.Contains(lines(out), header) {
			t.Errorf("render lacks %q:\n%s", header, out)
		}
	}
}

var errWrite = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWrite }

func TestRenderWriteFailure(t *testing.T) {
	if err := Render(failingWriter{}, &Report{}); !errors.Is(err, errWrite) {
		t.Errorf("error = %v, want %v", err, errWrite)
	}
}

// TestTerminalBoundEdge lists ten hotspots without the suffix and eleven with it.
func TestTerminalBoundEdge(t *testing.T) {
	for _, tc := range []struct {
		n      int
		header string
	}{{10, "hotspots (production, 10)"}, {11, "hotspots (production, 11, showing 10)"}} {
		r := &Report{}
		for i := range tc.n {
			r.Findings = append(r.Findings, hotspot(fmt.Sprintf("f%02d.go", i), 1, fmt.Sprintf(".:F%02d", i), float64(i)))
		}
		out := lines(render(t, r))
		start := slices.Index(out, tc.header)
		if start < 0 {
			t.Fatalf("%d hotspots: render lacks %q:\n%s", tc.n, tc.header, strings.Join(out, "\n"))
		}
		if shown := slices.Index(out[start:], "") - 1; shown != 10 {
			t.Errorf("%d hotspots: %d lines listed, want 10", tc.n, shown)
		}
	}
}

// TestTerminalDashWithFindings marks the total of an incomplete set with a dash even when some findings exist.
func TestTerminalDashWithFindings(t *testing.T) {
	r := &Report{Metrics: map[string]contract.Metric{"complexity.functions.production": {State: contract.Incomplete}}}
	for i := range 3 {
		r.Findings = append(r.Findings, hotspot(fmt.Sprintf("f%d.go", i), 1, fmt.Sprintf(".:F%d", i), 1))
	}
	out := lines(render(t, r))
	start := slices.Index(out, "hotspots (production, -)")
	if start < 0 || !strings.HasSuffix(out[start+1], "F0") {
		t.Errorf("render lacks the dash header over the listed hotspots:\n%s", strings.Join(out, "\n"))
	}
}
