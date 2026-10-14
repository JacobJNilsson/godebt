// The corpus command audits a list of public modules, so the scoring constants are read against real code.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/JacobJNilsson/godebt"
	"github.com/JacobJNilsson/godebt/internal/contract"
)

// The reasons one download can fail, so a caller and a test name one without reading a message.
var (
	errRefused     = errors.New("the module proxy refused the module")
	errNoDirectory = errors.New("the download names no directory")
	errBadEntry    = errors.New("the list holds a malformed entry")
	errDuplicate   = errors.New("the list names a module twice")
)

// entry is one line of the corpus list.
type entry struct {
	path    string
	version string
}

// tableRow is one measured module of the corpus table.
type tableRow struct {
	entry
	index             int
	complexityPoints  int
	duplicationPoints int
	productionLines   int
	functions         int
	eroded            int
	cloneGroups       int
	completeness      string
}

// The dimension ids of the score, as spec 002 names them.
const (
	dimensionComplexity  = "complexity-erosion"
	dimensionDuplication = "duplication"
)

// run measures every module of the list and writes the table.
func run(ctx context.Context, listPath, outPath string, fetch downloadFunc, audit auditFunc) error {
	entries, err := parseListFile(listPath)
	if err != nil {
		return err
	}
	rows, err := measure(ctx, entries, fetch, audit)
	if err != nil {
		return err
	}
	return writeResults(outPath, rows)
}

// moduleDirOf reads the directory out of one run of the proxy command.
func moduleDirOf(e entry, data []byte, stderr string, runErr error) (string, error) {
	if runErr != nil {
		return "", wrap(e, refused(data, runErr), stderr)
	}
	dir, err := moduleDir(data)
	if err != nil {
		return "", wrap(e, err, stderr)
	}
	return dir, nil
}

// refused returns the reason that go mod download wrote as JSON, or else the exit error.
func refused(data []byte, err error) error {
	var out downloadJSON
	if json.Unmarshal(data, &out) == nil && out.Error != "" {
		return fmt.Errorf("%w: %s", errRefused, out.Error)
	}
	return err
}

// wrap names the module and adds what the command wrote to stderr.
func wrap(e entry, err error, stderr string) error {
	if text := strings.TrimSpace(stderr); text != "" {
		return fmt.Errorf("%s@%s: %w: %s", e.path, e.version, err, text)
	}
	return fmt.Errorf("%s@%s: %w", e.path, e.version, err)
}

// moduleDir reads the directory out of the output of go mod download.
func moduleDir(data []byte) (string, error) {
	var out downloadJSON
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", errors.New(out.Error)
	}
	if out.Dir == "" {
		return "", errNoDirectory
	}
	return out.Dir, nil
}

// downloadJSON is the part of the output of go mod download that the corpus needs.
type downloadJSON struct {
	Dir   string
	Error string
}

// audit measures one module directory with no configuration, so a godebt.yaml in the module has no effect.
func audit(ctx context.Context, dir string) (*godebt.Report, error) {
	return godebt.Audit(ctx, dir, godebt.Options{Config: &godebt.Config{}})
}

// parseListFile reads the corpus list from the file at path.
func parseListFile(path string) ([]entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	entries, err := parseList(f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return entries, nil
}

// semver matches v, MAJOR.MINOR.PATCH, and an optional prerelease and build part.
var semver = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// lineError ties an error of the corpus list to the line that caused it.
type lineError struct {
	line int
	err  error
}

func (e *lineError) Error() string { return fmt.Sprintf("line %d: %v", e.line, e.err) }

func (e *lineError) Unwrap() error { return e.err }

// parseEntry reads one path@version line of the corpus list.
func parseEntry(text string) (entry, error) {
	if strings.ContainsAny(text, " \t") {
		return entry{}, fmt.Errorf("%w: %q holds whitespace", errBadEntry, text)
	}
	path, version, found := strings.Cut(text, "@")
	if !found || path == "" || version == "" || strings.Contains(version, "@") {
		return entry{}, fmt.Errorf("%w: %q is not a path@version", errBadEntry, text)
	}
	if !semver.MatchString(version) {
		return entry{}, fmt.Errorf("%w: the version of %q is not a semantic version with a leading v", errBadEntry, text)
	}
	return entry{path: path, version: version}, nil
}

// parseList reads the corpus list and skips blank lines and comments.
func parseList(r io.Reader) ([]entry, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var entries []entry
	seen := map[string]int{}
	for i, line := range strings.Split(string(data), "\n") {
		text := strings.TrimSpace(line)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		e, err := parseEntry(text)
		if err != nil {
			return nil, &lineError{line: i + 1, err: err}
		}
		if first, dup := seen[e.path]; dup {
			return nil, &lineError{line: i + 1, err: fmt.Errorf("%w: %s first appears on line %d", errDuplicate, e.path, first)}
		}
		seen[e.path] = i + 1
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return nil, errors.New("the list names no module")
	}
	return entries, nil
}

// measure downloads and audits every entry, in the order of the list.
func measure(ctx context.Context, entries []entry, fetch downloadFunc, audit auditFunc) ([]tableRow, error) {
	rows := make([]tableRow, 0, len(entries))
	for _, e := range entries {
		fmt.Fprintf(os.Stderr, "corpus: auditing %s@%s\n", e.path, e.version)
		dir, err := fetch(ctx, e)
		if err != nil {
			return nil, err
		}
		r, err := audit(ctx, dir)
		if err != nil {
			return nil, fmt.Errorf("%s@%s: %w", e.path, e.version, err)
		}
		rows = append(rows, row(e, r))
	}
	return rows, nil
}

// downloadFunc returns the directory of one module.
type downloadFunc func(ctx context.Context, e entry) (string, error)

// auditFunc measures one module directory.
type auditFunc func(ctx context.Context, dir string) (*godebt.Report, error)

// row reads one report into a table row.
func row(e entry, r *godebt.Report) tableRow {
	out := tableRow{entry: e, index: r.Score.Index, completeness: string(r.Completeness)}
	for _, c := range r.Score.Contributions {
		switch c.Dimension {
		case dimensionComplexity:
			out.complexityPoints = c.Points
		case dimensionDuplication:
			out.duplicationPoints = c.Points
		}
	}
	out.productionLines = r.Coverage.Production.SLOC
	out.functions = metric(r, "complexity.functions.production")
	out.eroded = metric(r, "erosion.eroded-count.production")
	out.cloneGroups = metric(r, "duplication.groups.production")
	return out
}

// metric returns the value of one counted metric, or 0 when the metric is not measured.
func metric(r *godebt.Report, id string) int {
	m, ok := r.Metrics[id]
	if !ok || m.State != contract.Complete {
		return 0
	}
	return int(m.Value)
}

// writeResults replaces the file through a temporary file, so a failed write leaves the old file in place.
func writeResults(path string, rows []tableRow) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := writeAndClose(f, rows); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	fmt.Printf("corpus: wrote %s with %d modules\n", path, len(rows))
	return nil
}

// writeAndClose sets mode 0644 because os.CreateTemp creates the file owner-only.
func writeAndClose(f *os.File, rows []tableRow) error {
	if err := f.Chmod(0o644); err != nil {
		_ = f.Close()
		return err
	}
	if err := writeTable(f, rows); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// writeTable writes the results table.
func writeTable(w io.Writer, rows []tableRow) error {
	_, err := fmt.Fprint(w, header)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(w, "| %s | %s | %d | %d | %d | %d | %d | %d | %d | %s |\n",
			r.path, r.version, r.index, r.complexityPoints, r.duplicationPoints,
			r.productionLines, r.functions, r.eroded, r.cloneGroups, r.completeness); err != nil {
			return err
		}
	}
	return writeCorrelations(w, rows)
}

// correlationMinLines is the production size above which a module enters the correlations.
const correlationMinLines = 5000

// writeCorrelations covers only modules above correlationMinLines, because spec 004 sets that cutoff.
func writeCorrelations(w io.Writer, rows []tableRow) error {
	var index, lines, debt []float64
	for _, r := range rows {
		if r.productionLines <= correlationMinLines {
			continue
		}
		index = append(index, float64(r.index))
		lines = append(lines, float64(r.productionLines))
		debt = append(debt, float64(r.eroded+r.cloneGroups)*1000/float64(r.productionLines))
	}
	_, err := fmt.Fprintf(w, "\nOver the %d modules above %d production lines, the Spearman rank correlation of the index is %s with production code lines and %s with debt per thousand production lines. Debt is eroded functions plus clone groups.\n",
		len(index), correlationMinLines, formatRank(spearman(index, lines)), formatRank(spearman(index, debt)))
	return err
}

// formatRank formats one correlation, and names an undefined one.
func formatRank(rho float64, ok bool) string {
	if !ok {
		return "undefined"
	}
	return fmt.Sprintf("%+.2f", rho)
}

// spearman reports false when the correlation is undefined, which happens with fewer than two values or no spread on one side.
func spearman(xs, ys []float64) (float64, bool) {
	if len(xs) < 2 || len(xs) != len(ys) {
		return 0, false
	}
	rx, ry := ranks(xs), ranks(ys)
	mx, my := mean(rx), mean(ry)
	var sxy, sxx, syy float64
	for i := range rx {
		sxy += (rx[i] - mx) * (ry[i] - my)
		sxx += (rx[i] - mx) * (rx[i] - mx)
		syy += (ry[i] - my) * (ry[i] - my)
	}
	if sxx == 0 || syy == 0 {
		return 0, false
	}
	return sxy / math.Sqrt(sxx*syy), true
}

// ranks returns the 1-based rank of each value, with tied values sharing the average of their positions.
func ranks(values []float64) []float64 {
	order := make([]int, len(values))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return values[order[a]] < values[order[b]] })
	out := make([]float64, len(values))
	for start := 0; start < len(order); {
		end := start
		for end+1 < len(order) && values[order[end+1]] == values[order[start]] {
			end++
		}
		for k := start; k <= end; k++ {
			out[order[k]] = float64(start+end)/2 + 1
		}
		start = end + 1
	}
	return out
}

func mean(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// header holds the heading and the column names of the table.
const header = `# Corpus results

Every row is one public module at one pinned version, audited with no
configuration. The index is 0 to 100 and lower is better. The two points columns
hold the points that each dimension contributed to the index. Production counts
cover the measured production set only. A run of make corpus rewrites this file.

| module | version | index | complexity-erosion points | duplication points | production code lines | production functions | eroded functions | clone groups | completeness |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
`
