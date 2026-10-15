package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/JacobJNilsson/godebt"
	"github.com/JacobJNilsson/godebt/internal/report"
)

// baselineFile is the default output of godebt baseline, at the audited root.
const baselineFile = "godebt-baseline.json"

// baselineFlags holds the parsed command line of godebt baseline.
type baselineFlags struct {
	root, out, config string
	quiet, help       bool
	check             bool
}

func baselineCommand(args []string, stdout, stderr io.Writer) int {
	f, ok := parseBaseline(args, stderr)
	if !ok {
		return 1
	}
	if f.help {
		return writeText(stdout, baselineHelp)
	}
	if f.check {
		return checkBaseline(f, stderr)
	}
	if err := writeBaseline(f); err != nil {
		_, _ = fmt.Fprintf(stderr, "godebt: %v\n", err)
		return 1
	}
	if !f.quiet {
		_, _ = fmt.Fprintf(stderr, "wrote %s\n", f.out)
	}
	return 0
}

func parseBaseline(args []string, stderr io.Writer) (baselineFlags, bool) {
	f := baselineFlags{root: "."}
	fs := flag.NewFlagSet("godebt baseline", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprintln(stderr, usage) }
	wantHelp := helpFlags(fs)
	fs.StringVar(&f.out, "out", "", "write the baseline to the file")
	fs.BoolVar(&f.check, "check", false, "compare with the committed file and write nothing")
	fs.BoolVar(&f.quiet, "quiet", false, "do not name the written file")
	fs.StringVar(&f.config, "config", "", "read the configuration from the file")
	paths, ok := parseInterleaved(fs, args)
	if !ok {
		return f, false
	}
	if len(paths) > 1 {
		fs.Usage()
		return f, false
	}
	if len(paths) == 1 {
		f.root = paths[0]
	}
	if f.out == "" {
		f.out = filepath.Join(f.root, baselineFile)
	}
	f.help = *wantHelp
	return f, true
}

// buildBaseline audits with the output file left out of the walk, so the file never counts as an unsupported file of its own audit.
func buildBaseline(f baselineFlags) ([]byte, error) {
	cfg, err := loadConfig(f.config, f.root)
	if err != nil {
		return nil, err
	}
	return godebt.Baseline(context.Background(), f.root, godebt.Options{Config: cfg, Omit: f.out})
}

func writeBaseline(f baselineFlags) error {
	data, err := buildBaseline(f)
	if err != nil {
		return err
	}
	return os.WriteFile(f.out, data, 0o644)
}

func checkBaseline(f baselineFlags, stderr io.Writer) int {
	committed, err := os.ReadFile(f.out)
	if err == nil {
		if _, loadErr := report.Load(bytes.NewReader(committed)); loadErr != nil {
			_, _ = fmt.Fprintf(stderr, "godebt: %s: %v\nregenerate it with: %s\n", f.out, loadErr, regenerateCommand(f))
			return 1
		}
	}
	var fresh []byte
	if err == nil {
		fresh, err = buildBaseline(f)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "godebt: %v\n", err)
		return 1
	}
	if bytes.Equal(committed, fresh) {
		if !f.quiet {
			_, _ = fmt.Fprintf(stderr, "baseline is current: %s\n", f.out)
		}
		return 0
	}
	_, _ = fmt.Fprintf(stderr, "baseline has not been updated: %s\nregenerate it with: %s\n", f.out, regenerateCommand(f))
	explainStale(stderr, committed, fresh)
	return exitPolicy
}

func explainStale(stderr io.Writer, committed, fresh []byte) {
	before, errBefore := report.Load(bytes.NewReader(committed))
	after, errAfter := report.Load(bytes.NewReader(fresh))
	if errBefore == nil && errAfter == nil {
		cmp := godebt.Compare(before, after)
		if cmp.Comparable {
			_, _ = fmt.Fprintf(stderr, "index %d to %d  new %d  resolved %d  changed %d\n", cmp.Before.Index, cmp.After.Index, len(cmp.New), len(cmp.Resolved), len(cmp.Changed))
		} else {
			_, _ = fmt.Fprintf(stderr, "refused: %s\n", cmp.Refusal)
		}
	}
	for _, line := range changedLines(committed, fresh) {
		_, _ = fmt.Fprintln(stderr, line)
	}
}

func regenerateCommand(f baselineFlags) string {
	cmd := "godebt baseline " + shellQuote(f.root)
	if f.config != "" {
		cmd += " --config " + shellQuote(f.config)
	}
	if f.out != filepath.Join(f.root, baselineFile) {
		cmd += " --out " + shellQuote(f.out)
	}
	return cmd
}

var plainWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func shellQuote(s string) string {
	if plainWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
