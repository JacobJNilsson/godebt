package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt"
	"github.com/JacobJNilsson/godebt/internal/report"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const goldens = "../../testdata/golden"

func runArgs(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// saveBaseline audits the functions fixture into a JSON file, so a later audit of the same tree compares equal.
func saveBaseline(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "baseline.json")
	if code, _, stderr := runArgs(t, "audit", filepath.Join(fixtures, "functions"), "--json", "--quiet", "--out", path); code != 0 {
		t.Fatalf("baseline audit: exit %d, %s", code, stderr)
	}
	return path
}

func TestAuditPolicyPass(t *testing.T) {
	cfg := writeFile(t, "pass.yaml", "policy:\n  maxIndex: 100\n  failOnNew: [complexity.hotspot]\n")
	code, stdout, stderr := runArgs(t, "audit", filepath.Join(fixtures, "functions"), "--config", cfg, "--baseline", saveBaseline(t))
	if code != 0 || stderr != "" {
		t.Errorf("exit %d, stderr %q, want 0 and nothing", code, stderr)
	}
	if !strings.Contains(stdout, "\n       baseline 5  delta 0  new 0  resolved 0\n") {
		t.Errorf("stdout lacks the baseline line:\n%s", stdout)
	}
}

func TestAuditPolicyFailKeepsReport(t *testing.T) {
	cfg := writeFile(t, "fail.yaml", "policy:\n  maxIndex: 4\n  regression: {maxIncrease: 0}\n")
	code, stdout, stderr := runArgs(t, "audit", filepath.Join(fixtures, "functions"), "--json", "--config", cfg)
	if code != 2 {
		t.Errorf("exit code %d, want 2", code)
	}
	if _, err := report.Load(strings.NewReader(stdout)); err != nil {
		t.Errorf("stdout is not the full report: %v", err)
	}
	want := "policy: maxIndex: index 5 is above 4\npolicy: regression skipped, no baseline\n"
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func TestAuditWritesReportBeforeFailing(t *testing.T) {
	cfg := writeFile(t, "fail.yaml", "policy:\n  maxIndex: 0\n")
	out := filepath.Join(t.TempDir(), "r.md")
	code, _, stderr := runArgs(t, "audit", filepath.Join(fixtures, "functions"), "--config", cfg, "--out", out)
	if code != 2 || !strings.HasPrefix(stderr, "wrote "+out+"\npolicy: maxIndex: ") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
	if data, err := os.ReadFile(out); err != nil || !strings.HasPrefix(string(data), "# godebt report\n") {
		t.Errorf("report file: %v", err)
	}
}

func TestAuditRefusedBaseline(t *testing.T) {
	// The golden reports carry a test analyzer version, so a live audit never compares with them.
	code, stdout, stderr := runArgs(t, "audit", filepath.Join(fixtures, "functions"), "--json", "--baseline", filepath.Join(goldens, "report-functions.json"))
	if code != 2 {
		t.Errorf("exit code %d, want 2", code)
	}
	want := "policy: baseline: analyzerVersion differs: before \"0.0.0-test\", after \"" + godebt.Version + "\"\n"
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	var printed struct {
		Comparison *godebt.Comparison `json:"comparison"`
	}
	if err := json.Unmarshal([]byte(stdout), &printed); err != nil || printed.Comparison == nil || printed.Comparison.Comparable {
		t.Errorf("JSON comparison = %+v, %v, want a refused comparison", printed.Comparison, err)
	}
}

func TestAuditRefusedBaselineWithBaselineChecks(t *testing.T) {
	cfg := writeFile(t, "gate.yaml", "policy:\n  regression:\n  failOnNew: [complexity.hotspot]\n")
	baseline := filepath.Join(goldens, "report-functions.json")
	for _, format := range []string{"--md", "--quiet"} {
		code, stdout, stderr := runArgs(t, "audit", filepath.Join(fixtures, "functions"), format, "--config", cfg, "--baseline", baseline)
		want := "policy: baseline: analyzerVersion differs: before \"0.0.0-test\", after \"" + godebt.Version + "\"\n" +
			"policy: failOnNew: skipped, baseline not comparable\n" +
			"policy: regression: skipped, baseline not comparable\n"
		if code != 2 || stderr != want {
			t.Errorf("%s: exit %d, stderr %q, want 2 and %q", format, code, stderr, want)
		}
		// A refused baseline has no index to print a delta against.
		if strings.Contains(stdout, "baseline") || strings.Contains(stdout, "Against the baseline") {
			t.Errorf("%s: stdout has a baseline line:\n%s", format, stdout)
		}
	}
}

func TestAuditEmptyRegressionIsSkipped(t *testing.T) {
	for _, content := range []string{"policy:\n  regression:\n", "policy:\n  regression: {}\n"} {
		cfg := writeFile(t, "empty.yaml", content)
		code, _, stderr := runArgs(t, "audit", filepath.Join(fixtures, "functions"), "--json", "--config", cfg)
		if code != 0 || stderr != "policy: regression skipped, no baseline\n" {
			t.Errorf("%q: exit %d, stderr %q", content, code, stderr)
		}
	}
}

func TestAuditUnknownBudgetMetricIsConfigError(t *testing.T) {
	cfg := writeFile(t, "typo.yaml", "policy:\n  budgets:\n    foo.bar.production: {max: 1}\n")
	code, stdout, stderr := runArgs(t, "audit", filepath.Join(fixtures, "functions"), "--config", cfg)
	if code != 1 || stdout != "" || !strings.Contains(stderr, "policy.budgets.foo.bar.production: is not a metric id") {
		t.Errorf("exit %d, stdout %q, stderr %q, want 1 and a config error", code, stdout, stderr)
	}
}

func TestAuditOperationalErrors(t *testing.T) {
	root := filepath.Join(fixtures, "functions")
	bad := writeFile(t, "bad.yaml", "policy:\n  maxIndex: 200\n")
	missing := filepath.Join(t.TempDir(), "absent")
	cases := map[string][]string{
		"missing config":   {"audit", root, "--config", missing + ".yaml"},
		"invalid config":   {"audit", root, "--config", bad},
		"missing baseline": {"audit", root, "--baseline", missing + ".json"},
		"invalid baseline": {"audit", root, "--baseline", bad},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := runArgs(t, args...)
			if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "godebt: ") {
				t.Errorf("exit %d, stdout %q, stderr %q, want 1 and an error", code, stdout, stderr)
			}
		})
	}
}

func TestAuditJSONWithoutBaselineHasNoComparison(t *testing.T) {
	_, stdout, _ := runArgs(t, "audit", filepath.Join(fixtures, "functions"), "--json")
	if strings.Contains(stdout, `"comparison"`) {
		t.Error("JSON without a baseline has a comparison block")
	}
}

func TestAuditJSONWithBaseline(t *testing.T) {
	code, stdout, stderr := runArgs(t, "audit", filepath.Join(fixtures, "functions"), "--json", "--baseline", saveBaseline(t))
	if code != 0 {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	var printed struct {
		SchemaVersion string             `json:"schemaVersion"`
		Comparison    *godebt.Comparison `json:"comparison"`
	}
	if err := json.Unmarshal([]byte(stdout), &printed); err != nil {
		t.Fatal(err)
	}
	if printed.SchemaVersion == "" || printed.Comparison == nil || !printed.Comparison.Comparable || len(printed.Comparison.New) != 0 {
		t.Errorf("printed = %+v", printed)
	}
}

// TestBaselineGolden renders a comparison of two golden reports, which share their versions and digest.
func TestBaselineGolden(t *testing.T) {
	before, err := godebt.LoadReport(filepath.Join(goldens, "report-sets.json"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := godebt.LoadReport(filepath.Join(goldens, "report-functions.json"))
	if err != nil {
		t.Fatal(err)
	}
	cmp := godebt.Compare(before, after)
	for name, r := range map[string]renderer{"terminal-baseline.txt": renderTerminal, "markdown-baseline.md": renderMarkdown} {
		var out bytes.Buffer
		if err := r(&out, after, cmp); err != nil {
			t.Fatal(err)
		}
		compareGolden(t, name, out.Bytes())
	}
}

func TestSavedBaselineRunIsNextBaseline(t *testing.T) {
	root := filepath.Join(fixtures, "functions")
	saved := filepath.Join(t.TempDir(), "x.json")
	if code, _, stderr := runArgs(t, "audit", root, "--baseline", saveBaseline(t), "--json", "--quiet", "--out", saved); code != 0 {
		t.Fatalf("first run: exit %d, %s", code, stderr)
	}
	data, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"comparison"`) {
		t.Error("the saved report has a comparison block")
	}
	if code, _, stderr := runArgs(t, "audit", root, "--baseline", saved); code != 0 {
		t.Errorf("second run: exit %d, %s", code, stderr)
	}
}
