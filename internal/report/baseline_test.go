package report

import (
	"bytes"
	"strings"
	"testing"
)

func TestTerminalBaselineLine(t *testing.T) {
	r := newReport(t, "functions")
	plain := render(t, r)
	cases := map[Baseline]string{
		{Index: 27, Delta: -2, Resolved: 2}:          "       baseline 27  delta -2  new 0  resolved 2  changed 0",
		{Index: 20, Delta: 3, New: 1}:                "       baseline 20  delta +3  new 1  resolved 0  changed 0",
		{Index: 20}:                                  "       baseline 20  delta 0  new 0  resolved 0  changed 0",
		{Index: 20, New: 1, Resolved: 1, Changed: 3}: "       baseline 20  delta 0  new 1  resolved 1  changed 3",
	}
	for b, want := range cases {
		var out bytes.Buffer
		if err := RenderCompared(&out, r, &b); err != nil {
			t.Fatal(err)
		}
		got := lines(out.String())
		if got[3] != want {
			t.Errorf("line under the index = %q, want %q", got[3], want)
		}
		if withoutLine := strings.Join(append(got[:3:3], got[4:]...), "\n"); withoutLine != plain {
			t.Errorf("the baseline changed more than one line:\n%s", out.String())
		}
	}
	var out bytes.Buffer
	if err := RenderCompared(&out, r, nil); err != nil || out.String() != plain {
		t.Errorf("a nil baseline changed the output: %v\n%s", err, out.String())
	}
}

func TestMarkdownBaselineSentence(t *testing.T) {
	r := newReport(t, "functions")
	plain := markdown(t, r)
	var out bytes.Buffer
	if err := RenderMarkdownCompared(&out, r, &Baseline{Index: 27, Delta: -2, Resolved: 2}); err != nil {
		t.Fatal(err)
	}
	sentence := "Against the baseline index 27 the delta is -2, with 0 new, 2 resolved, and 0 changed findings.\n"
	if !strings.Contains(out.String(), ".\n"+sentence) {
		t.Errorf("Markdown lacks %q:\n%s", sentence, out.String())
	}
	if strings.Replace(out.String(), sentence, "", 1) != plain {
		t.Errorf("the baseline changed more than one sentence:\n%s", out.String())
	}
}
