package report

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

// RenderMarkdown prints the report as a bounded Markdown summary.
func RenderMarkdown(w io.Writer, r *Report) error {
	return RenderMarkdownCompared(w, r, nil)
}

// RenderMarkdownCompared prints the Markdown summary with a sentence on the baseline after the index, and a nil baseline prints none.
func RenderMarkdownCompared(w io.Writer, r *Report, base *Baseline) error {
	var b strings.Builder
	module := r.Repo.Module
	if module == "" {
		module = "(no go.mod)"
	}
	fmt.Fprintf(&b, "# godebt report\n\n%s, godebt %s.\n\n", module, r.AnalyzerVersion)
	fmt.Fprintf(&b, "The index is %d/100, lower is better, scoring %s", r.Score.Index, r.ScoringVersion)
	if r.Score.Partial {
		b.WriteString(", partial")
	}
	b.WriteString(".\n")
	if base != nil {
		fmt.Fprintf(&b, "Against the baseline index %d the delta is %s, with %d new and %d resolved findings.\n", base.Index, Signed(base.Delta), base.New, base.Resolved)
	}
	if len(r.Score.Contributions) > 0 {
		parts := make([]string, 0, len(r.Score.Contributions))
		for _, c := range r.Score.Contributions {
			parts = append(parts, fmt.Sprintf("%s %d", c.Dimension, c.Points))
		}
		fmt.Fprintf(&b, "Contributions: %s.\n", strings.Join(parts, ", "))
	}
	b.WriteString("\n")
	row(&b, "set", "files", "sloc", "funcs", "cc p50/p90/max", "eroded", "clones", "dup lines")
	row(&b, "---", "---:", "---:", "---:", "---:", "---:", "---:", "---:")
	row(&b, setCells(r, contract.Production, r.Coverage.Production)...)
	row(&b, setCells(r, contract.Test, r.Coverage.Test)...)
	fmt.Fprintf(&b, "\nOther files: %s.\n", strings.Join(otherSets(r.Coverage), ", "))
	markdownList(&b, r, hotspots, "## Hotspots", []string{"path", "lines", "identity", "cc", "mass"}, func(f contract.Finding) []string {
		h := f.Facts.Hotspot
		return []string{escape(f.Path), lineRange(f.StartLine, f.EndLine), escape(f.Identity), strconv.Itoa(h.CC), fmt.Sprintf("%.1f", h.Mass)}
	})
	markdownList(&b, r, clones, "## Clones", []string{"id", "tokens", "members"}, func(f contract.Finding) []string {
		c := f.Facts.Clone
		members := make([]string, 0, len(c.Members))
		for _, m := range c.Members {
			members = append(members, escape(m.Path)+":"+lineRange(m.StartLine, m.EndLine))
		}
		return []string{c.GroupID, strconv.Itoa(c.Tokens), strings.Join(members, "<br>")}
	})
	markdownSafeguards(&b, r.Safeguards)
	markdownBroken(&b, r.Findings)
	if reasons := incompleteReasons(r); len(reasons) > 0 {
		b.WriteString("\n## Incomplete\n\n")
		for _, reason := range reasons {
			fmt.Fprintf(&b, "- %s\n", reason)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// markdownList prints one section with a table per measured set.
func markdownList(b *strings.Builder, r *Report, l list, title string, header []string, cells func(contract.Finding) []string) {
	fmt.Fprintf(b, "\n%s\n", title)
	for _, set := range []contract.SourceSet{contract.Production, contract.Test} {
		shown, total := selection(l, r, set)
		fmt.Fprintf(b, "\n### %s (%s)\n", set, total)
		if len(shown) == 0 {
			continue
		}
		b.WriteString("\n")
		row(b, header...)
		align := make([]string, len(header))
		for i, name := range header {
			align[i] = "---"
			if name == "lines" || name == "cc" || name == "mass" || name == "tokens" {
				align[i] = "---:"
			}
		}
		row(b, align...)
		for _, f := range shown {
			row(b, cells(f)...)
		}
	}
}

func markdownSafeguards(b *strings.Builder, guards []contract.Safeguard) {
	if len(guards) == 0 {
		return
	}
	b.WriteString("\n## Safeguards\n\n")
	row(b, "id", "evidence", "locations", "notes")
	row(b, "---", "---", "---", "---")
	for _, g := range guards {
		row(b, g.ID, string(g.Evidence), escape(strings.Join(locationTexts(g.Locations, 0), "<br>")), escape(g.Notes))
	}
}

func markdownBroken(b *strings.Builder, findings []contract.Finding) {
	shown, total := brokenReferences(findings)
	if len(shown) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### Broken references (%s)\n\n", total)
	row(b, "location", "command")
	row(b, "---", "---")
	for _, f := range shown {
		row(b, escape(fmt.Sprintf("%s:%d", f.Path, f.StartLine)), escape(f.Facts.BrokenReference.Command))
	}
}

// row prints one table row, and the caller escapes every cell that holds a path or an identity.
func row(b *strings.Builder, cells ...string) {
	fmt.Fprintf(b, "| %s |\n", strings.Join(cells, " | "))
}

// escape keeps a pipe inside its table cell.
func escape(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

func lineRange(start, end int) string {
	return fmt.Sprintf("%d-%d", start, end)
}
