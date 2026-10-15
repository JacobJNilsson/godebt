package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/JacobJNilsson/godebt"
	"github.com/JacobJNilsson/godebt/internal/report"
)

// exitPolicy is the exit code of a run whose policy failed or whose baseline comparison was refused.
const exitPolicy = 2

// auditFlags holds the parsed command line of godebt audit.
type auditFlags struct {
	root, out, config, baseline, dir string
	dirSet                           bool
	asJSON, asMarkdown, quiet        bool
	help                             bool
}

// auditResult is what gate needs after the command writes the report.
type auditResult struct {
	cmp    *godebt.Comparison
	policy godebt.PolicyResult
}

// renderer writes a report in one format, with the comparison against a baseline when there is one.
type renderer func(io.Writer, *godebt.Report, *godebt.Comparison) error

func audit(args []string, stdout, stderr io.Writer) int {
	f, ok := parseAudit(args, stderr)
	if !ok {
		return 1
	}
	if f.help {
		return writeText(stdout, auditHelp)
	}
	result, err := auditTo(f, stdout)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "godebt: %v\n", err)
		return 1
	}
	if f.out != "" && !f.quiet {
		_, _ = fmt.Fprintf(stderr, "wrote %s\n", f.out)
	}
	return gate(result, stderr)
}

func parseAudit(args []string, stderr io.Writer) (auditFlags, bool) {
	var f auditFlags
	fs := flag.NewFlagSet("godebt audit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprintln(stderr, usage) }
	wantHelp := helpFlags(fs)
	fs.BoolVar(&f.asJSON, "json", false, "print the report as JSON")
	fs.BoolVar(&f.asMarkdown, "md", false, "print the report as Markdown")
	fs.StringVar(&f.out, "out", "", "write the report to the file")
	fs.BoolVar(&f.quiet, "quiet", false, "do not name the written file")
	fs.StringVar(&f.config, "config", "", "read the configuration from the file")
	fs.StringVar(&f.baseline, "baseline", "", "compare with a saved JSON report")
	fs.StringVar(&f.dir, "path", "", "print only the findings under the directory")
	paths, ok := parseInterleaved(fs, args)
	if !ok {
		return f, false
	}
	f.help = *wantHelp
	fs.Visit(func(fl *flag.Flag) { f.dirSet = f.dirSet || fl.Name == "path" })
	if len(paths) != 1 && !f.help {
		fs.Usage()
		return f, false
	}
	if len(paths) == 1 {
		f.root = paths[0]
	}
	return f, true
}

// auditTo audits, writes the report, and evaluates the policy, so the report exists before gate picks the exit code.
func auditTo(f auditFlags, stdout io.Writer) (auditResult, error) {
	render, err := format(f.asJSON, f.asMarkdown, f.out)
	if err != nil {
		return auditResult{}, err
	}
	dir, err := checkDir(f)
	if err != nil {
		return auditResult{}, err
	}
	cfg, err := loadConfig(f.config, f.root)
	if err != nil {
		return auditResult{}, err
	}
	var baseline *godebt.Report
	if f.baseline != "" {
		if baseline, err = godebt.LoadReport(f.baseline); err != nil {
			return auditResult{}, err
		}
	}
	r, err := godebt.Audit(context.Background(), f.root, godebt.Options{Config: cfg})
	if err != nil {
		return auditResult{}, err
	}
	var result auditResult
	if baseline != nil {
		result.cmp = godebt.Compare(baseline, r)
	}
	shown, shownCmp := printable(r, result.cmp, dir)
	if err := writeReport(f.out, stdout, render, shown, shownCmp); err != nil {
		return auditResult{}, err
	}
	var p godebt.Policy
	if cfg != nil {
		p = cfg.Policy
	}
	result.policy = godebt.EvaluateWithComparison(p, r, result.cmp)
	return result, nil
}

// loadConfig reads the named configuration, or godebt.yaml at root when the user names none, and nil means no configuration.
func loadConfig(named, root string) (*godebt.Config, error) {
	if named != "" {
		return godebt.LoadConfig(named)
	}
	return godebt.LoadDefaultConfig(root)
}

// writeReport writes the report to the file out, or to stdout when out is empty.
func writeReport(out string, stdout io.Writer, render renderer, r *godebt.Report, cmp *godebt.Comparison) error {
	if out == "" {
		return render(stdout, r, cmp)
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := render(f, r, cmp); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// gate prints the refusal and the policy reasons to stderr and returns the exit code.
func gate(result auditResult, stderr io.Writer) int {
	if result.policy.Refusal != "" {
		_, _ = fmt.Fprintf(stderr, "policy: baseline: %s\n", result.policy.Refusal)
	}
	for _, reason := range result.policy.Reasons {
		_, _ = fmt.Fprintf(stderr, "policy: %s: %s\n", reason.Check, reason.Message)
	}
	for _, check := range result.policy.Skipped {
		_, _ = fmt.Fprintf(stderr, "policy: %s skipped, no baseline\n", check)
	}
	if result.policy.Failed {
		return exitPolicy
	}
	return 0
}

// format picks the renderer from the flags, and from the extension of the output file when no flag names one.
func format(asJSON, asMarkdown bool, out string) (renderer, error) {
	switch {
	case asJSON && asMarkdown:
		return nil, errExclusive
	case asJSON:
		return jsonFor(out), nil
	case asMarkdown:
		return renderMarkdown, nil
	}
	switch filepath.Ext(out) {
	case ".json":
		return jsonFor(out), nil
	case ".md":
		return renderMarkdown, nil
	}
	return renderTerminal, nil
}

func renderTerminal(w io.Writer, r *godebt.Report, cmp *godebt.Comparison) error {
	return report.RenderCompared(w, r, baselineOf(cmp))
}

func renderMarkdown(w io.Writer, r *godebt.Report, cmp *godebt.Comparison) error {
	return report.RenderMarkdownCompared(w, r, baselineOf(cmp))
}

// baselineOf returns what the renderers print about a comparable baseline, and nil for no baseline or a refused one.
func baselineOf(cmp *godebt.Comparison) *report.Baseline {
	if cmp == nil || !cmp.Comparable {
		return nil
	}
	return &report.Baseline{Index: cmp.Before.Index, Delta: cmp.IndexDelta, New: len(cmp.New), Resolved: len(cmp.Resolved)}
}

// jsonFor leaves the comparison out of a report written to a file, so every saved report loads as a later baseline.
func jsonFor(out string) renderer {
	if out == "" {
		return writeJSON
	}
	return func(w io.Writer, r *godebt.Report, _ *godebt.Comparison) error {
		return writeJSON(w, r, nil)
	}
}

// writeJSON prints the same bytes as json.MarshalIndent with two spaces, plus a newline, and adds the comparison only with a baseline.
func writeJSON(w io.Writer, r *godebt.Report, cmp *godebt.Comparison) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if cmp == nil {
		return enc.Encode(r)
	}
	return enc.Encode(struct {
		*godebt.Report
		Comparison *godebt.Comparison `json:"comparison"`
	}{r, cmp})
}
