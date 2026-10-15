package report

import (
	"fmt"
	"io"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/JacobJNilsson/godebt/internal/complexity"
	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/duplication"
)

// maxListed bounds each list of the terminal summary.
const maxListed = 10

// unmeasured stands in for a number that the audit did not measure.
const unmeasured = "-"

// tableRow lays out one row of the set table, with the label left-aligned and every other cell right-aligned.
const tableRow = "%-10s%14s%8s%8s%17s%11s%8s%13s\n"

// Render prints the report as a fixed-width terminal summary.
func Render(w io.Writer, r *Report) error {
	return RenderCompared(w, r, nil)
}

// RenderCompared prints the terminal summary with a line on the baseline under the index, and a nil baseline prints no line.
func RenderCompared(w io.Writer, r *Report, base *Baseline) error {
	var b strings.Builder
	module := r.Repo.Module
	if module == "" {
		module = "(no go.mod)"
	}
	fmt.Fprintf(&b, "godebt %s  %s\n\n", r.AnalyzerVersion, module)
	if r.Dir != "" {
		fmt.Fprintf(&b, "findings under %s\n\n", r.Dir)
	}
	renderIndex(&b, r, base)
	b.WriteString("\n")
	fmt.Fprintf(&b, tableRow, "", "files", "sloc", "funcs", "cc p50/p90/max", "eroded", "clones", "dup lines")
	renderSet(&b, r, contract.Production, r.Coverage.Production)
	renderSet(&b, r, contract.Test, r.Coverage.Test)
	renderOther(&b, r.Coverage)
	for _, set := range []contract.SourceSet{contract.Production, contract.Test} {
		for _, l := range []list{hotspots, clones} {
			b.WriteString("\n")
			renderList(&b, l, r, set)
		}
	}
	renderSafeguards(&b, r.Safeguards)
	renderBroken(&b, r.Findings)
	_, err := io.WriteString(w, b.String())
	return err
}

func renderIndex(b *strings.Builder, r *Report, base *Baseline) {
	fmt.Fprintf(b, "index  %d/100  lower is better  scoring %s", r.Score.Index, r.ScoringVersion)
	if r.Score.Partial {
		b.WriteString(" (partial)")
	}
	b.WriteString("\n")
	if base != nil {
		fmt.Fprintf(b, "       baseline %d  delta %s  new %d  resolved %d  changed %d\n", base.Index, Signed(base.Delta), base.New, base.Resolved, base.Changed)
	}
	if reasons := incompleteReasons(r); len(reasons) > 0 {
		fmt.Fprintf(b, "incomplete: %s\n", strings.Join(reasons, ", "))
	}
	if len(r.Score.Contributions) > 0 {
		parts := make([]string, 0, len(r.Score.Contributions))
		for _, c := range r.Score.Contributions {
			parts = append(parts, fmt.Sprintf("%s %d", c.Dimension, c.Points))
		}
		fmt.Fprintf(b, "       %s\n", strings.Join(parts, "  "))
	}
	fmt.Fprintf(b, "test index  %d/100  not part of the index", r.TestScore.Index)
	if r.TestScore.Partial {
		b.WriteString(" (partial)")
	}
	b.WriteString("\n")
}

// incompleteReasons lists every error path and every budget cap with its set, sorted and without repeats.
func incompleteReasons(r *Report) []string {
	var paths []string
	for _, m := range r.Metrics {
		paths = append(paths, m.Detail.Errors...)
	}
	slices.Sort(paths)
	var caps []string
	for _, l := range r.Limits {
		set := l.MetricID[strings.LastIndex(l.MetricID, ".")+1:]
		caps = append(caps, fmt.Sprintf("%s (%s)", l.Reason, set))
	}
	slices.Sort(caps)
	return append(slices.Compact(paths), slices.Compact(caps)...)
}

func renderSet(b *strings.Builder, r *Report, set contract.SourceSet, size Measured) {
	cells := setCells(r, set, size)
	fmt.Fprintf(b, tableRow, cells[0], cells[1], cells[2], cells[3], cells[4], cells[5], cells[6], cells[7])
}

// setCells returns the table cells of one set: name, files, sloc, funcs, cc, eroded, clones, and dup lines.
func setCells(r *Report, set contract.SourceSet, size Measured) []string {
	metric := func(name string) contract.Metric { return r.Metrics[name+"."+string(set)] }
	cc, eroded := unmeasured, unmeasured
	if metric("complexity.cc.max").State == contract.Complete {
		cc = fmt.Sprintf("%d/%d/%d", int(metric("complexity.cc.p50").Value), int(metric("complexity.cc.p90").Value), int(metric("complexity.cc.max").Value))
		eroded = withShare(metric("erosion.eroded-count"), metric("erosion.eroded-share"))
	}
	return []string{
		string(set), strconv.Itoa(size.Files), strconv.Itoa(size.SLOC), count(metric("complexity.functions")), cc, eroded,
		count(metric("duplication.groups")), withShare(metric("duplication.duplicated-lines"), metric("duplication.density")),
	}
}

// count prints a complete count, and the unmeasured mark otherwise.
func count(m contract.Metric) string {
	if m.State != contract.Complete {
		return unmeasured
	}
	return strconv.Itoa(int(m.Value))
}

// withShare prints a count and, when the count is above zero, its ratio as a whole percent.
func withShare(n, share contract.Metric) string {
	out := count(n)
	if n.State == contract.Complete && n.Value > 0 && share.State == contract.Complete {
		out += fmt.Sprintf(" (%d%%)", int(math.Round(share.Value*100)))
	}
	return out
}

func renderOther(b *strings.Builder, c Coverage) {
	fmt.Fprintf(b, "other: %s\n", strings.Join(otherSets(c), "  "))
}

// otherSets lists the file counts of the unmeasured sets, and it always lists the unsupported count.
func otherSets(c Coverage) []string {
	var parts []string
	for _, row := range []struct {
		set   contract.SourceSet
		files int
	}{
		{contract.Generated, c.Generated.Files}, {contract.Vendored, c.Vendored.Files},
		{contract.Testdata, c.Testdata.Files}, {contract.Excluded, c.Excluded.Files},
	} {
		if row.files > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", row.set, row.files))
		}
	}
	return append(parts, fmt.Sprintf("%s %d", contract.Unsupported, c.Unsupported.Files))
}

// list says how the terminal summary prints the findings of one kind.
type list struct {
	kind  string
	title string
	// metric is the id without the set suffix of the metric that says whether the list was measured.
	metric string
	// size orders the list, largest value first.
	size  func(f contract.Finding) float64
	write func(b *strings.Builder, fs []contract.Finding)
}

var hotspots = list{
	kind: complexity.KindHotspot, title: "hotspots", metric: "complexity.functions",
	size: func(f contract.Finding) float64 { return f.Facts.Hotspot.Mass },
	write: func(b *strings.Builder, fs []contract.Finding) {
		var mass, cc, where int
		for _, f := range fs {
			mass = max(mass, len(fmt.Sprintf("%.1f", f.Facts.Hotspot.Mass)))
			cc = max(cc, len(strconv.Itoa(f.Facts.Hotspot.CC)))
			where = max(where, len(location(f)))
		}
		for _, f := range fs {
			h := f.Facts.Hotspot
			name := f.Identity[strings.LastIndex(f.Identity, ":")+1:]
			fmt.Fprintf(b, "  %*.1f  cc %*d  %-*s  %s\n", mass, h.Mass, cc, h.CC, where, location(f), name)
		}
	},
}

var clones = list{
	kind: duplication.KindCloneGroup, title: "clones", metric: "duplication.groups",
	size: func(f contract.Finding) float64 { return float64(f.Facts.Clone.Tokens) },
	write: func(b *strings.Builder, fs []contract.Finding) {
		for _, f := range fs {
			c := f.Facts.Clone
			fmt.Fprintf(b, "  %d tokens  %d members  %s\n", c.Tokens, len(c.Members), c.GroupID)
			for _, m := range c.Members {
				fmt.Fprintf(b, "    %s:%d-%d\n", m.Path, m.StartLine, m.EndLine)
			}
		}
	},
}

func location(f contract.Finding) string {
	return fmt.Sprintf("%s:%d-%d", f.Path, f.StartLine, f.EndLine)
}

func renderList(b *strings.Builder, l list, r *Report, set contract.SourceSet) {
	shown, total := selection(l, r, set)
	fmt.Fprintf(b, "%s (%s, %s)\n", l.title, set, total)
	l.write(b, shown)
}

// selection returns at most maxListed findings of one kind and set, largest first, and the total, which is the mark for an unmeasured set.
func selection(l list, r *Report, set contract.SourceSet) ([]contract.Finding, string) {
	var matched []contract.Finding
	for _, f := range r.Findings {
		if f.Kind == l.kind && f.SourceSet == set {
			matched = append(matched, f)
		}
	}
	total := strconv.Itoa(len(matched))
	if r.Metrics[l.metric+"."+string(set)].State == contract.Incomplete {
		total = unmeasured
	}
	if len(matched) > maxListed {
		total += fmt.Sprintf(", showing %d", maxListed)
	}
	// A stable sort keeps the report order between equal sizes, so the list never depends on map or input order.
	sort.SliceStable(matched, func(i, j int) bool { return l.size(matched[i]) > l.size(matched[j]) })
	return matched[:min(len(matched), maxListed)], total
}

// brokenReferences returns at most maxListed broken-reference findings in report order, and a count that says when it cut the list.
func brokenReferences(findings []contract.Finding) ([]contract.Finding, string) {
	var matched []contract.Finding
	for _, f := range findings {
		if f.Kind == contract.KindBrokenReference {
			matched = append(matched, f)
		}
	}
	total := strconv.Itoa(len(matched))
	if len(matched) > maxListed {
		total += fmt.Sprintf(", showing %d", maxListed)
	}
	return matched[:min(len(matched), maxListed)], total
}

func renderBroken(b *strings.Builder, findings []contract.Finding) {
	shown, total := brokenReferences(findings)
	if len(shown) == 0 {
		return
	}
	fmt.Fprintf(b, "\nbroken references (%s)\n", total)
	for _, f := range shown {
		fmt.Fprintf(b, "  %s:%d  %s\n", f.Path, f.StartLine, f.Facts.BrokenReference.Command)
	}
}

// maxLocations bounds the locations that one safeguard row prints.
const maxLocations = 3

func renderSafeguards(b *strings.Builder, guards []contract.Safeguard) {
	if len(guards) == 0 {
		return
	}
	b.WriteString("\nsafeguards\n")
	for _, g := range guards {
		line := fmt.Sprintf("  %-18s %-19s %s", g.ID, g.Evidence, strings.Join(locationTexts(g.Locations, maxLocations), " "))
		fmt.Fprintln(b, strings.TrimRight(line, " "))
	}
}

// locationTexts prints path:line for each location, and at most limit of them followed by +N when limit is above zero.
func locationTexts(locations []contract.Location, limit int) []string {
	shown := locations
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
	}
	out := make([]string, 0, len(shown)+1)
	for _, l := range shown {
		text := l.Path
		if l.Line > 0 {
			text += ":" + strconv.Itoa(l.Line)
		}
		out = append(out, text)
	}
	if len(locations) > len(shown) {
		out = append(out, fmt.Sprintf("+%d", len(locations)-len(shown)))
	}
	return out
}
