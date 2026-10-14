package compare

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/report"
)

// maxListed bounds the new and the resolved list of both renderers.
const maxListed = 10

// unmeasured is the cell text for a number that one report did not measure.
const unmeasured = "-"

// Render prints the comparison as a fixed-width terminal summary.
func Render(w io.Writer, c *Comparison) error {
	var b strings.Builder
	b.WriteString("godebt compare\n\n")
	fmt.Fprintf(&b, "before  %s\nafter   %s\n", summaryText(c.Before), summaryText(c.After))
	if !c.Comparable {
		fmt.Fprintf(&b, "\nrefused: %s\n", c.Refusal)
		return write(w, &b)
	}
	fmt.Fprintf(&b, "delta   %s\n\n", report.Signed(c.IndexDelta))
	width := len("metric")
	for _, m := range c.Metrics {
		width = max(width, len(m.ID))
	}
	for _, cells := range metricRows(c.Metrics) {
		fmt.Fprintf(&b, "%-*s  %10s%10s%10s\n", width, cells[0], cells[1], cells[2], cells[3])
	}
	terminalList(&b, "new", c.New)
	terminalList(&b, "resolved", c.Resolved)
	fmt.Fprintf(&b, "\npersistent %d\n", len(c.Persistent))
	return write(w, &b)
}

// RenderMarkdown prints the comparison as a bounded Markdown summary.
func RenderMarkdown(w io.Writer, c *Comparison) error {
	var b strings.Builder
	b.WriteString("# godebt comparison\n\n")
	fmt.Fprintf(&b, "Before: %s.\nAfter: %s.\n", summaryText(c.Before), summaryText(c.After))
	if !c.Comparable {
		fmt.Fprintf(&b, "\nRefused: %s.\n", c.Refusal)
		return write(w, &b)
	}
	fmt.Fprintf(&b, "The index delta is %s.\n\n## Metrics\n\n", report.Signed(c.IndexDelta))
	for i, cells := range metricRows(c.Metrics) {
		fmt.Fprintf(&b, "| %s |\n", strings.Join(cells[:], " | "))
		if i == 0 {
			b.WriteString("| --- | ---: | ---: | ---: |\n")
		}
	}
	markdownList(&b, "New findings", c.New)
	markdownList(&b, "Resolved findings", c.Resolved)
	fmt.Fprintf(&b, "\nPersistent findings: %d.\n", len(c.Persistent))
	return write(w, &b)
}

func write(w io.Writer, b *strings.Builder) error {
	_, err := io.WriteString(w, b.String())
	return err
}

func summaryText(s Summary) string {
	text := fmt.Sprintf("index %d/100", s.Index)
	if s.Partial {
		text += " (partial)"
	}
	return text + ", scoring " + s.ScoringVersion
}

// metricRows returns the header and one row per metric, each with the id, both values, and the delta.
func metricRows(metrics []MetricDelta) [][4]string {
	rows := [][4]string{{"metric", "before", "after", "delta"}}
	for _, m := range metrics {
		delta := unmeasured
		if m.Delta != nil {
			delta = signedValue(*m.Delta)
		}
		rows = append(rows, [4]string{m.ID, value(m.Before), value(m.After), delta})
	}
	return rows
}

// signedValue prints a metric delta with its sign, and no change as 0.
func signedValue(delta float64) string {
	if delta == 0 {
		return "0"
	}
	return fmt.Sprintf("%+.4g", delta)
}

func value(m *contract.Metric) string {
	if m == nil || m.State != contract.Complete {
		return unmeasured
	}
	return fmt.Sprintf("%.4g", m.Value)
}

func terminalList(b *strings.Builder, title string, findings []contract.Finding) {
	fmt.Fprintf(b, "\n%s %d\n", title, len(findings))
	for _, f := range findings[:min(len(findings), maxListed)] {
		fmt.Fprintf(b, "  %s  %s  %s:%s  %s\n", f.SourceSet, f.Kind, f.Path, lines(f), f.Identity)
	}
	if len(findings) > maxListed {
		fmt.Fprintf(b, "  and %d more\n", len(findings)-maxListed)
	}
}

func markdownList(b *strings.Builder, title string, findings []contract.Finding) {
	fmt.Fprintf(b, "\n## %s\n\n", title)
	if len(findings) == 0 {
		b.WriteString("None.\n")
		return
	}
	if len(findings) > maxListed {
		fmt.Fprintf(b, "The list shows %d of %d.\n\n", maxListed, len(findings))
	}
	b.WriteString("| set | kind | path | lines | identity |\n| --- | --- | --- | ---: | --- |\n")
	for _, f := range findings[:min(len(findings), maxListed)] {
		cells := []string{string(f.SourceSet), f.Kind, f.Path, lines(f), f.Identity}
		for i, cell := range cells {
			// A pipe inside a cell would end the cell.
			cells[i] = strings.ReplaceAll(cell, "|", `\|`)
		}
		fmt.Fprintf(b, "| %s |\n", strings.Join(cells, " | "))
	}
}

func lines(f contract.Finding) string {
	return strconv.Itoa(f.StartLine) + "-" + strconv.Itoa(f.EndLine)
}
