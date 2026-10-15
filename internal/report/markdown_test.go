package report

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/duplication"
)

func markdown(t *testing.T, r *Report) string {
	t.Helper()
	var b bytes.Buffer
	if err := RenderMarkdown(&b, r); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestMarkdownGolden(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			compareGolden(t, "markdown-"+strings.ReplaceAll(name, "/", "-")+".md", []byte(markdown(t, newReport(t, name))))
		})
	}
}

func TestMarkdownSummary(t *testing.T) {
	out := markdown(t, newReport(t, "functions"))
	want := strings.Join([]string{
		"# godebt report",
		"",
		"example.com/functions, godebt " + testAnalyzer + ".",
		"",
		"The index is 5/100, lower is better, scoring 0.3.0.",
		"Contributions: complexity-erosion 5, duplication 0.",
		"The test index is 7/100, not part of the index.",
		"",
		"| set | files | sloc | funcs | cc p50/p90/max | eroded | clones | dup lines |",
		"| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |",
		"| production | 9 | 146 | 23 | 1/11/15 | 3 (52%) | 0 | 0 |",
		"| test | 1 | 13 | 3 | 2/12/12 | 1 (77%) | 0 | 0 |",
		"",
		"Other files: unsupported 1.",
		"",
		"## Hotspots",
		"",
		"### production (3)",
		"",
		"| path | lines | identity | cc | mass |",
		"| --- | ---: | --- | ---: | ---: |",
		"| cc.go | 35-70 | .:Long | 11 | 66.0 |",
		"| cc.go | 73-76 | .:Short | 15 | 30.0 |",
		"| cc.go | 79-81 | .:wrap#1 | 11 | 19.1 |",
		"",
		"### test (1)",
		"",
		"| path | lines | identity | cc | mass |",
		"| --- | ---: | --- | ---: | ---: |",
		"| functions_test.go | 14-17 | .:allSet | 12 | 24.0 |",
		"",
		"## Clones",
		"",
		"### production (0)",
		"",
		"### test (0)",
		"",
		"## Safeguards",
		"",
		"| id | evidence | locations | notes |",
		"| --- | --- | --- | --- |",
		"| agent-hooks | absent |  | No agent hook is declared. |",
		"| ci-workflow | absent |  | No workflow file has a step. |",
		"| coverage-budget | absent |  | No Makefile exists. |",
		"| lint-config | absent |  | No .golangci file exists. |",
		"| pre-commit-hook | absent |  | No hook file or hook tool declares pre-commit. |",
		"| pre-push-hook | absent |  | No hook file or hook tool declares pre-push. |",
		"| test-check | absent |  | No Makefile recipe runs go test. |",
		"| vet-check | absent |  | No Makefile recipe runs go vet. |",
		"",
	}, "\n")
	if out != want {
		t.Errorf("markdown:\n%s\nwant:\n%s", out, want)
	}
}

func TestMarkdownTestIndexLine(t *testing.T) {
	r := newReport(t, "functions")
	got := lines(markdown(t, r))
	want := fmt.Sprintf("The test index is %d/100, not part of the index.", r.TestScore.Index)
	if got[6] != want {
		t.Errorf("line after the contributions line = %q, want %q", got[6], want)
	}
}

func TestMarkdownPartialTestIndex(t *testing.T) {
	r := &Report{TestScore: contract.Score{Index: 9, Partial: true}}
	if want := "The test index is 9/100, not part of the index, partial.\n"; !strings.Contains(markdown(t, r), want) {
		t.Errorf("markdown lacks %q:\n%s", want, markdown(t, r))
	}
	r.TestScore.Partial = false
	if out := markdown(t, r); !strings.Contains(out, "not part of the index.\n") {
		t.Errorf("complete test index line has a partial mark:\n%s", out)
	}
}

func TestMarkdownIncomplete(t *testing.T) {
	out := markdown(t, newReport(t, "broken"))
	for _, want := range []string{
		"The index is 100/100, lower is better, scoring 0.3.0, partial.\n",
		"| production | 2 | 2 | - | - | - | - | - |\n",
		"### production (-)\n",
		"## Incomplete\n\n- bad.go\n- bad_test.go\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(markdown(t, newReport(t, "functions")), "## Incomplete") {
		t.Error("a complete report has an Incomplete section")
	}
}

func TestMarkdownClones(t *testing.T) {
	out := markdown(t, newReport(t, "clones/nested"))
	want := strings.Join([]string{
		"### production (2)",
		"",
		"| id | tokens | members |",
		"| --- | ---: | --- |",
		"| 4801221e89fa382b | 321 | a.go:4-62<br>b.go:4-62 |",
		"| a9131f502c4085e8 | 122 | a.go:6-28<br>b.go:6-28<br>light.go:7-29 |",
		"",
	}, "\n")
	if !strings.Contains(out, want) {
		t.Errorf("markdown lacks:\n%s\ngot:\n%s", want, out)
	}
}

func TestMarkdownBound(t *testing.T) {
	r := &Report{}
	for i := range 12 {
		r.Findings = append(r.Findings, hotspot(fmt.Sprintf("f%02d.go", i), 1, fmt.Sprintf(".:F%02d", i), float64(i)))
	}
	for i := range 14 {
		r.Findings = append(r.Findings, contract.Finding{
			Kind: duplication.KindCloneGroup, Path: fmt.Sprintf("g%02d.go", i), StartLine: 1, EndLine: 5, SourceSet: contract.Test,
			Facts: contract.Facts{Clone: &contract.CloneFacts{GroupID: fmt.Sprintf("%016x", i), Tokens: 100 + i, Members: []contract.CloneMember{
				{Path: fmt.Sprintf("g%02d.go", i), StartLine: 1, EndLine: 5},
			}}},
		})
	}
	out := markdown(t, r)
	for _, want := range []string{
		"### production (12, showing 10)\n\n| path | lines | identity | cc | mass |\n| --- | ---: | --- | ---: | ---: |\n| f11.go | 1-2 | .:F11 | 11 | 11.0 |\n",
		"### test (14, showing 10)\n\n| id | tokens | members |\n| --- | ---: | --- |\n| 000000000000000d | 113 | g13.go:1-5 |\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, ".:F01") || strings.Contains(out, "0000000000000003") {
		t.Errorf("markdown shows more than ten entries:\n%s", out)
	}
}

// TestMarkdownEscapesPipes keeps a path with a pipe inside one table cell.
func TestMarkdownEscapesPipes(t *testing.T) {
	r := &Report{Findings: []contract.Finding{hotspot("a|b.go", 1, ".:A", 1)}}
	if out := markdown(t, r); !strings.Contains(out, "| a\\|b.go | 1-2 |") {
		t.Errorf("markdown does not escape the pipe:\n%s", out)
	}
}

func TestMarkdownSafeguards(t *testing.T) {
	r := &Report{Safeguards: []contract.Safeguard{
		{ID: "agent-hooks", Evidence: contract.EvidenceAbsent, Locations: []contract.Location{}, Notes: "No agent hook is declared."},
		{ID: "pre-commit-hook", Evidence: contract.EvidenceStructurallyWired, Locations: []contract.Location{{Path: "Makefile", Line: 12}, {Path: "a|b"}}, Notes: "Runs a | b."},
	}}
	want := strings.Join([]string{
		"",
		"## Safeguards",
		"",
		"| id | evidence | locations | notes |",
		"| --- | --- | --- | --- |",
		"| agent-hooks | absent |  | No agent hook is declared. |",
		"| pre-commit-hook | structurally-wired | Makefile:12<br>a\\|b | Runs a \\| b. |",
		"",
	}, "\n")
	if out := markdown(t, r); !strings.Contains(out, want) {
		t.Errorf("markdown:\n%s\nwant:\n%s", out, want)
	}
	if out := markdown(t, &Report{}); strings.Contains(out, "## Safeguards") {
		t.Errorf("a report without safeguards prints the section:\n%s", out)
	}
}

func TestMarkdownWithoutModule(t *testing.T) {
	if out := markdown(t, &Report{}); !strings.HasPrefix(out, "# godebt report\n\n(no go.mod), godebt .\n") {
		t.Errorf("markdown:\n%s", out)
	}
}

func TestMarkdownWriteFailure(t *testing.T) {
	if err := RenderMarkdown(failingWriter{}, &Report{}); !errors.Is(err, errWrite) {
		t.Errorf("error = %v, want %v", err, errWrite)
	}
}
