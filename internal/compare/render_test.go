package compare

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

func renderBoth(t *testing.T, c *Comparison) (terminal, markdown string) {
	t.Helper()
	var a, b bytes.Buffer
	if err := Render(&a, c); err != nil {
		t.Fatal(err)
	}
	if err := RenderMarkdown(&b, c); err != nil {
		t.Fatal(err)
	}
	return a.String(), b.String()
}

func TestRenderBoundsFindingLists(t *testing.T) {
	var after []contract.Finding
	for i := range 12 {
		after = append(after, spot(fmt.Sprintf("p/f%02d.go", i), 1, fmt.Sprintf("p:F%02d", i), false))
	}
	terminal, markdown := renderBoth(t, Compare(rep(10), rep(30, after...)))
	if !strings.Contains(terminal, "\nnew 12\n") || !strings.Contains(terminal, "\n  and 2 more\n") {
		t.Errorf("terminal list is not bounded with a total:\n%s", terminal)
	}
	if strings.Contains(terminal, "p/f10.go") || !strings.Contains(terminal, "p/f09.go") {
		t.Errorf("terminal lists the wrong findings:\n%s", terminal)
	}
	if !strings.Contains(markdown, "## New findings\n\nThe list shows 10 of 12.\n") || strings.Contains(markdown, "p/f10.go") {
		t.Errorf("Markdown list is not bounded with a total:\n%s", markdown)
	}
}

func TestRenderRefusal(t *testing.T) {
	before, after := rep(20), rep(25)
	after.ScoringVersion = "0.2.0"
	terminal, markdown := renderBoth(t, Compare(before, after))
	refusal := `scoringVersion differs: before "0.1.0", after "0.2.0"`
	if !strings.Contains(terminal, "\nrefused: "+refusal+"\n") || strings.Contains(terminal, "metric") {
		t.Errorf("terminal refusal:\n%s", terminal)
	}
	if !strings.Contains(markdown, "\nRefused: "+refusal+".\n") || strings.Contains(markdown, "## Metrics") {
		t.Errorf("Markdown refusal:\n%s", markdown)
	}
}

func TestRenderUnmeasuredCells(t *testing.T) {
	after := rep(20)
	after.Metrics["duplication.density.production"] = contract.Metric{State: contract.Incomplete, Unit: "ratio"}
	terminal, markdown := renderBoth(t, Compare(rep(20), after))
	if !strings.Contains(terminal, "duplication.density.production        0.25         -         -\n") {
		t.Errorf("terminal row of an incomplete metric:\n%s", terminal)
	}
	if !strings.Contains(markdown, "| duplication.density.production | 0.25 | - | - |\n") {
		t.Errorf("Markdown row of an incomplete metric:\n%s", markdown)
	}
}

func TestRenderEmptyLists(t *testing.T) {
	terminal, markdown := renderBoth(t, Compare(rep(20), rep(20)))
	if !strings.Contains(terminal, "\nnew 0\n\nresolved 0\n\nchanged 0\n\npersistent 0\n") {
		t.Errorf("terminal counts:\n%s", terminal)
	}
	if !strings.Contains(markdown, "## New findings\n\nNone.\n") {
		t.Errorf("Markdown empty list:\n%s", markdown)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestRenderWriteFailure(t *testing.T) {
	c := Compare(rep(20), rep(20))
	if Render(failingWriter{}, c) == nil || RenderMarkdown(failingWriter{}, c) == nil {
		t.Error("a failed write returned no error")
	}
}

func TestRenderPartialAndPipes(t *testing.T) {
	before := rep(20)
	before.Score.Partial = true
	after := rep(40, spot("a/x|y.go", 3, "a:F", false))
	terminal, markdown := renderBoth(t, Compare(before, after))
	if !strings.Contains(terminal, "before  index 20/100 (partial), scoring") || !strings.Contains(terminal, "+20\n") {
		t.Errorf("terminal:\n%s", terminal)
	}
	if !strings.Contains(markdown, `| production | complexity.hotspot | a/x\|y.go | 3-13 | a:F |`) {
		t.Errorf("Markdown does not escape the pipe:\n%s", markdown)
	}
}

func changedComparison() *Comparison {
	before := rep(20, cloneGroup("old", 50, span{"a.go", 10, 20}, span{"b.go", 30, 40}))
	after := rep(20, cloneGroup("new", 70, span{"a.go", 10, 24}, span{"b.go", 30, 44}))
	return Compare(before, after)
}

func TestRenderChanged(t *testing.T) {
	terminal, markdown := renderBoth(t, changedComparison())
	if !strings.Contains(terminal, "\nchanged 1\n  production  duplication.clone-group  a.go:10-24  new  tokens 50 to 70\n") {
		t.Errorf("terminal changed list:\n%s", terminal)
	}
	if !strings.Contains(markdown, "## Changed findings\n\n| set | kind | path | lines | identity | tokens before | tokens after |\n") ||
		!strings.Contains(markdown, "| production | duplication.clone-group | a.go | 10-24 | new | 50 | 70 |\n") {
		t.Errorf("Markdown changed list:\n%s", markdown)
	}
}

func TestRenderChangedIsBounded(t *testing.T) {
	var before, after []contract.Finding
	for i := range 12 {
		p := fmt.Sprintf("p%02d", i)
		before = append(before, cloneGroup("o"+p, 5, span{p + "/x.go", 1, 9}, span{p + "/y.go", 1, 9}))
		after = append(after, cloneGroup("n"+p, 6, span{p + "/x.go", 1, 9}, span{p + "/y.go", 1, 9}))
	}
	terminal, markdown := renderBoth(t, Compare(rep(10, before...), rep(10, after...)))
	if !strings.Contains(terminal, "\nchanged 12\n") || !strings.Contains(terminal, "\n  and 2 more\n") || strings.Contains(terminal, "p10/x.go") {
		t.Errorf("terminal changed list is not bounded:\n%s", terminal)
	}
	if !strings.Contains(markdown, "## Changed findings\n\nThe list shows 10 of 12.\n") || strings.Contains(markdown, "p10/x.go") {
		t.Errorf("Markdown changed list is not bounded:\n%s", markdown)
	}
}
