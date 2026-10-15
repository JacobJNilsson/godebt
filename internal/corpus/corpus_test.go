package main

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt"
	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/report"
)

// stubAudit returns a report with the metric values that a row needs.
func stubAudit(index int) *godebt.Report {
	return &godebt.Report{
		Completeness: report.Complete,
		Coverage:     report.Coverage{Production: report.Measured{SLOC: 900}},
		Metrics: map[string]contract.Metric{
			"complexity.functions.production": {State: contract.Complete, Value: 40},
			"erosion.eroded-count.production": {State: contract.Complete, Value: 3},
			"erosion.excess-mass.production":  {State: contract.Complete, Value: 114.64},
			"duplication.groups.production":   {State: contract.Complete, Value: 2},
		},
		Score: contract.Score{Index: index, Contributions: []contract.Contribution{
			{Dimension: "complexity-erosion", Points: 12},
			{Dimension: "duplication", Points: 5},
		}},
	}
}

func TestParseList(t *testing.T) {
	in := "# a comment\n\ngithub.com/google/uuid@v1.6.0\n  gopkg.in/yaml.v3@v3.0.1  \n"
	entries, err := parseList(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []entry{{path: "github.com/google/uuid", version: "v1.6.0"}, {path: "gopkg.in/yaml.v3", version: "v3.0.1"}}
	if len(entries) != 2 || entries[0] != want[0] || entries[1] != want[1] {
		t.Errorf("entries = %+v, want %+v", entries, want)
	}
	if _, err := parseList(strings.NewReader("github.com/google/uuid\n")); err == nil {
		t.Error("a line without a version gave no error")
	}
	if _, err := parseList(strings.NewReader("@v1.0.0\n")); err == nil {
		t.Error("a line without a path gave no error")
	}
}

func TestMeasureRows(t *testing.T) {
	entries := []entry{{path: "example.com/a", version: "v1.0.0"}}
	dirs := map[string]string{"example.com/a@v1.0.0": "/cache/a"}
	rows, err := measure(context.Background(), entries,
		func(_ context.Context, e entry) (string, error) { return dirs[e.path+"@"+e.version], nil },
		func(_ context.Context, _ string) (*godebt.Report, error) { return stubAudit(17), nil })
	if err != nil {
		t.Fatal(err)
	}
	want := tableRow{entry: entries[0], index: 17, complexityPoints: 12, duplicationPoints: 5, productionLines: 900, functions: 40, eroded: 3, excessMass: 114.64, cloneGroups: 2, completeness: string(report.Complete)}
	if len(rows) != 1 || rows[0] != want {
		t.Errorf("rows = %+v, want %+v", rows, want)
	}
}

func TestMeasureReportsAFailedModule(t *testing.T) {
	entries := []entry{{path: "example.com/a", version: "v1.0.0"}}
	failing := func(context.Context, entry) (string, error) { return "", context.DeadlineExceeded }
	if _, err := measure(context.Background(), entries, failing, nil); err == nil {
		t.Error("a failed download gave no error")
	}
	measuring := func(context.Context, string) (*godebt.Report, error) { return nil, context.Canceled }
	if _, err := measure(context.Background(), entries, func(context.Context, entry) (string, error) { return "/cache/a", nil }, measuring); err == nil {
		t.Error("a failed audit gave no error")
	}
}

func TestWriteTable(t *testing.T) {
	var out strings.Builder
	if err := writeTable(&out, []tableRow{{entry: entry{path: "example.com/a", version: "v1.0.0"}, index: 17, complexityPoints: 12, duplicationPoints: 5, productionLines: 900, functions: 40, eroded: 3, excessMass: 114.64, cloneGroups: 2, completeness: string(report.Complete)}}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if !strings.HasPrefix(lines[0], "# Corpus results") {
		t.Errorf("table starts with %q, want a heading", lines[0])
	}
	rows := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "| example.com/a") {
			rows++
			for _, cell := range []string{"v1.0.0", "17", "12", "5", "900", "40", "3", "114.6", "2", "complete"} {
				if !strings.Contains(line, cell) {
					t.Errorf("row %q lacks the cell %q", line, cell)
				}
			}
		}
	}
	if rows != 1 {
		t.Errorf("table holds %d rows for one module, want 1", rows)
	}
}

func TestRunWritesTheTable(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "modules.txt")
	if err := os.WriteFile(listPath, []byte("example.com/a@v1.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "results.md")
	fetch := func(context.Context, entry) (string, error) { return filepath.Join(dir, "cache"), nil }
	measuring := func(context.Context, string) (*godebt.Report, error) { return stubAudit(17), nil }
	if err := run(context.Background(), listPath, outPath, fetch, measuring); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "| example.com/a | v1.0.0 | 17 |") {
		t.Errorf("table = %q, want a row for the one module of the list", data)
	}
	if err := run(context.Background(), filepath.Join(dir, "absent.txt"), outPath, fetch, measuring); err == nil {
		t.Error("a missing list file gave no error")
	}
}

// writeModule writes a Go module of one production function at the root of dir.
func writeModule(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/a\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := "package a\n\n// F holds one branch.\nfunc F(n int) int {\n\tif n > 1 {\n\t\treturn n * 2\n\t}\n\treturn n\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAuditReadsNoConfiguration(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "godebt.yaml"), []byte("exclude: [\"**\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := audit(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Coverage.Production.Files != 1 {
		t.Errorf("production files = %d, want 1, so the godebt.yaml of the module was ignored", r.Coverage.Production.Files)
	}
}

// failingIO fails every read and every write, so an error path runs offline.
type failingIO struct{}

func (failingIO) Read([]byte) (int, error)  { return 0, errors.New("no room") }
func (failingIO) Write([]byte) (int, error) { return 0, errors.New("no room") }

func TestWriteTableReportsAFailedWrite(t *testing.T) {
	if err := writeTable(failingIO{}, []tableRow{{entry: entry{path: "example.com/a", version: "v1.0.0"}}}); err == nil {
		t.Error("a failed write gave no error")
	}
}

func TestRunRejectsUnreadablePaths(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "modules.txt")
	if err := os.WriteFile(listPath, []byte("example.com/a@v1.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fetch := func(context.Context, entry) (string, error) { return dir, nil }
	measuring := func(context.Context, string) (*godebt.Report, error) { return stubAudit(17), nil }
	if err := run(context.Background(), dir, filepath.Join(dir, "results.md"), fetch, measuring); err == nil {
		t.Error("a directory as the list gave no error")
	}
	if err := run(context.Background(), listPath, dir, fetch, measuring); err == nil {
		t.Error("a directory as the output gave no error")
	}
}

func TestRowCountsAnUnmeasuredMetricAsZero(t *testing.T) {
	r := stubAudit(0)
	r.Metrics["duplication.groups.production"] = contract.Metric{State: contract.Incomplete}
	got := row(entry{path: "example.com/a", version: "v1.0.0"}, r)
	if got.cloneGroups != 0 || got.functions != 40 {
		t.Errorf("row = %+v, want the incomplete group count as 0 and the measured function count", got)
	}
}

func TestRunReportsAFailedFetch(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "modules.txt")
	if err := os.WriteFile(listPath, []byte("example.com/a@v1.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fetch := func(context.Context, entry) (string, error) { return "", errors.New("no proxy") }
	measuring := func(context.Context, string) (*godebt.Report, error) { return stubAudit(17), nil }
	if err := run(context.Background(), listPath, filepath.Join(dir, "results.md"), fetch, measuring); err == nil {
		t.Error("a failed download gave no error")
	}
}

func TestParseListReportsAFailedRead(t *testing.T) {
	if _, err := parseList(failingIO{}); err == nil {
		t.Error("a failed read gave no error")
	}
}

func TestModuleDir(t *testing.T) {
	dir, err := moduleDir([]byte(`{"Dir":"/cache/a","Error":""}`))
	if err != nil || dir != "/cache/a" {
		t.Errorf("moduleDir = %q, %v, want the directory of the module", dir, err)
	}
	for _, out := range []string{`{"Error":"unknown revision"}`, `{}`, `not json`} {
		if _, err := moduleDir([]byte(out)); err == nil {
			t.Errorf("moduleDir(%s) gave no error", out)
		}
	}
}

func TestWriteResultsReplacesTheTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.md")
	if err := os.WriteFile(path, []byte("old table\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rows := []tableRow{{entry: entry{path: "example.com/a", version: "v1.0.0"}, index: 17, completeness: string(report.Complete)}}
	if err := writeResults(path, rows); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "| example.com/a | v1.0.0 | 17 |") {
		t.Errorf("table = %q, want the new table", data)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d files, want the table and no temporary file", len(entries))
	}
	if err := writeResults(dir, rows); err == nil {
		t.Error("a directory as the output gave no error")
	}
}

func TestParseListRejectsAnEmptyList(t *testing.T) {
	if _, err := parseList(strings.NewReader("# only a comment\n\n")); err == nil {
		t.Error("a list without a module gave no error")
	}
}

func TestWriteAndCloseReportsAFailedTable(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "table")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writeAndClose(f, []tableRow{{entry: entry{path: "example.com/a", version: "v1.0.0"}}}); err == nil {
		t.Error("a closed file gave no error")
	}
}

func TestRefusedPrefersTheReportedReason(t *testing.T) {
	exit := errors.New("exit status 1")
	if got := refused([]byte(`{"Error":"module lookup disabled"}`), exit); !errors.Is(got, errRefused) {
		t.Errorf("refused = %v, want errRefused", got)
	}
	if got := refused([]byte(""), exit); !errors.Is(got, exit) {
		t.Errorf("refused = %v, want the exit status when the JSON names no reason", got)
	}
}

func TestWrapAddsStderrOnlyWhenThereIsSome(t *testing.T) {
	e := entry{path: "example.com/a", version: "v1.0.0"}
	if got := wrap(e, errors.New("boom"), "  \n"); got.Error() != "example.com/a@v1.0.0: boom" {
		t.Errorf("wrap = %q, want no trailing separator", got)
	}
	if got := wrap(e, errors.New("boom"), "proxy said no"); got.Error() != "example.com/a@v1.0.0: boom: proxy said no" {
		t.Errorf("wrap = %q, want the stderr in the message", got)
	}
}

func TestModuleDirOf(t *testing.T) {
	e := entry{path: "example.com/a", version: "v1.0.0"}
	dir, err := moduleDirOf(e, []byte(`{"Dir":"/cache/a"}`), "", nil)
	if err != nil || dir != "/cache/a" {
		t.Errorf("moduleDirOf = %q, %v, want the directory of the module", dir, err)
	}
	refusedErr := errors.New("exit status 1")
	dir, err = moduleDirOf(e, []byte(`{"Error":"unknown revision"}`), "", refusedErr)
	if !errors.Is(err, errRefused) || dir != "" {
		t.Errorf("moduleDirOf = %q, %v, want the refusal the proxy reported", dir, err)
	}
	if _, err = moduleDirOf(e, nil, "proxy said no", refusedErr); !errors.Is(err, refusedErr) {
		t.Errorf("moduleDirOf = %v, want the exit status of the command", err)
	}
	if _, err = moduleDirOf(e, []byte("{}"), "", nil); !errors.Is(err, errNoDirectory) {
		t.Errorf("moduleDirOf = %v, want errNoDirectory", err)
	}
}

func TestParseListRejectsAMalformedEntry(t *testing.T) {
	cases := []struct {
		name, in string
		line     int
	}{
		{"whitespace inside", "# c\nexample.com/a @v1.0.0\n", 2},
		{"two at signs", "\nexample.com/a@v1.0.0@v2.0.0\n", 2},
		{"no leading v", "example.com/a@1.0.0\n", 1},
		{"not a version", "example.com/a@vfoo\n", 1},
		{"two parts", "example.com/a@v1.2\n", 1},
	}
	for _, c := range cases {
		_, err := parseList(strings.NewReader(c.in))
		var at *lineError
		if !errors.Is(err, errBadEntry) || !errors.As(err, &at) || at.line != c.line {
			t.Errorf("%s: error %v, want errBadEntry at line %d", c.name, err, c.line)
		}
	}
}

func TestParseListRejectsADuplicatePath(t *testing.T) {
	_, err := parseList(strings.NewReader("example.com/a@v1.0.0\nexample.com/b@v1.0.0\nexample.com/a@v2.0.0\n"))
	var at *lineError
	if !errors.Is(err, errDuplicate) || !errors.As(err, &at) || at.line != 3 {
		t.Errorf("error %v, want errDuplicate at line 3", err)
	}
}

func TestResultsHoldOneRowPerListEntry(t *testing.T) {
	entries, err := parseListFile(filepath.Join("..", "..", "corpus", "modules.txt"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "corpus", "results.md"))
	if err != nil {
		t.Fatal(err)
	}
	var got []entry
	for _, line := range strings.Split(string(data), "\n") {
		cells := strings.Split(line, "|")
		if strings.HasPrefix(line, "| ") && len(cells) > 3 && strings.HasPrefix(strings.TrimSpace(cells[2]), "v") && strings.TrimSpace(cells[1]) != "module" {
			got = append(got, entry{path: strings.TrimSpace(cells[1]), version: strings.TrimSpace(cells[2])})
		}
	}
	if !reflect.DeepEqual(got, entries) {
		t.Errorf("results rows = %v, want the list %v", got, entries)
	}
}

func TestRanksAveragesTies(t *testing.T) {
	got := ranks([]float64{10, 20, 20, 30})
	if want := []float64{1, 2.5, 2.5, 4}; !reflect.DeepEqual(got, want) {
		t.Errorf("ranks = %v, want %v", got, want)
	}
}

// The tied case is worked by hand. The ranks of xs are 1, 2.5, 2.5, 4 and the ranks of ys are 1, 2, 3, 4.
// Both means are 2.5, the cross sum is 4.5, the sums of squares are 4.5 and 5, and 4.5 / sqrt(22.5) is 0.94868.
func TestSpearmanWithTies(t *testing.T) {
	rho, ok := spearman([]float64{10, 20, 20, 30}, []float64{1, 2, 3, 4})
	if !ok || math.Abs(rho-0.9486833) > 1e-6 {
		t.Errorf("spearman = %v, %v, want 0.9486833", rho, ok)
	}
	if rho, ok := spearman([]float64{1, 2, 3}, []float64{9, 5, 1}); !ok || math.Abs(rho+1) > 1e-9 {
		t.Errorf("reversed order gave %v, %v, want -1", rho, ok)
	}
}

func TestSpearmanIsUndefinedWithoutSpread(t *testing.T) {
	for name, tc := range map[string]struct{ xs, ys []float64 }{
		"one value":    {[]float64{1}, []float64{1}},
		"flat xs":      {[]float64{2, 2, 2}, []float64{1, 2, 3}},
		"flat ys":      {[]float64{1, 2, 3}, []float64{4, 4, 4}},
		"unequal size": {[]float64{1, 2}, []float64{1}},
	} {
		if rho, ok := spearman(tc.xs, tc.ys); ok {
			t.Errorf("%s: spearman = %v, want undefined", name, rho)
		}
	}
}

// Three of the four modules lie above 5,000 lines. Their indexes 70, 80, 90 rise with size and fall with debt per thousand lines.
func TestWriteTableAddsCorrelations(t *testing.T) {
	rows := []tableRow{
		{entry: entry{path: "a", version: "v1"}, index: 10, productionLines: 100, eroded: 9},
		{entry: entry{path: "b", version: "v1"}, index: 70, productionLines: 6000, eroded: 60},
		{entry: entry{path: "c", version: "v1"}, index: 80, productionLines: 7000, eroded: 35},
		{entry: entry{path: "d", version: "v1"}, index: 90, productionLines: 8000, eroded: 16},
	}
	var out strings.Builder
	if err := writeTable(&out, rows); err != nil {
		t.Fatal(err)
	}
	want := "Over the 3 modules above 5000 production lines, the Spearman rank correlation of the index is +1.00 with production code lines and -1.00 with debt per thousand production lines."
	if !strings.Contains(out.String(), want) {
		t.Errorf("table lacks %q:\n%s", want, out.String())
	}
	var few strings.Builder
	if err := writeCorrelations(&few, rows[:1]); err != nil || !strings.Contains(few.String(), "is undefined with production code lines and undefined with") {
		t.Errorf("one module gave %q, %v, want undefined correlations", few.String(), err)
	}
}

// correlationFailWriter fails on the first write that holds the correlation text.
type correlationFailWriter struct{}

func (correlationFailWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "Spearman") {
		return 0, errors.New("no room")
	}
	return len(p), nil
}

func TestWriteTableReportsAFailedCorrelationWrite(t *testing.T) {
	if err := writeTable(correlationFailWriter{}, []tableRow{{entry: entry{path: "a", version: "v1"}}}); err == nil {
		t.Error("a failed write of the correlation line gave no error")
	}
}
