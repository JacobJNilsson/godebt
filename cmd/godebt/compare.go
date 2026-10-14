package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/JacobJNilsson/godebt"
	"github.com/JacobJNilsson/godebt/internal/compare"
)

func compareCommand(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("godebt compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprintln(stderr, usage) }
	wantHelp := helpFlags(fs)
	asJSON := fs.Bool("json", false, "print the comparison as JSON")
	asMarkdown := fs.Bool("md", false, "print the comparison as Markdown")
	paths, ok := parseInterleaved(fs, args)
	switch {
	case !ok:
		return 1
	case *wantHelp:
		return writeText(stdout, compareHelp)
	case len(paths) != 2:
		fs.Usage()
		return 1
	}
	cmp, err := compareFiles(paths[0], paths[1], *asJSON, *asMarkdown, stdout)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "godebt: %v\n", err)
		return 1
	}
	if !cmp.Comparable {
		return exitPolicy
	}
	return 0
}

// compareFiles loads both reports and prints their comparison.
func compareFiles(beforePath, afterPath string, asJSON, asMarkdown bool, stdout io.Writer) (*godebt.Comparison, error) {
	if asJSON && asMarkdown {
		return nil, errExclusive
	}
	render := compare.Render
	switch {
	case asJSON:
		render = writeComparisonJSON
	case asMarkdown:
		render = compare.RenderMarkdown
	}
	before, err := godebt.LoadReport(beforePath)
	if err != nil {
		return nil, err
	}
	after, err := godebt.LoadReport(afterPath)
	if err != nil {
		return nil, err
	}
	cmp := godebt.Compare(before, after)
	return cmp, render(stdout, cmp)
}

func writeComparisonJSON(w io.Writer, cmp *godebt.Comparison) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(cmp)
}

// parseInterleaved parses flags before and after the positional arguments, because the flag package stops at the first argument.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, bool) {
	var positional []string
	for err := fs.Parse(args); ; err = fs.Parse(fs.Args()[1:]) {
		if err != nil {
			return nil, false
		}
		if fs.NArg() == 0 {
			return positional, true
		}
		positional = append(positional, fs.Arg(0))
	}
}
