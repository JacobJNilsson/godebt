package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/JacobJNilsson/scree"
)

const usage = "usage: scree audit <path> [--config <file>] [--baseline <report.json>] [--json|--md] [--out <file>] [--quiet]\n       scree compare <before.json> <after.json> [--json|--md]\n       scree baseline [path] [--config <file>] [--out <file>] [--check] [--quiet]\n       scree version"

const help = `usage: scree <command>

commands:
  audit <path>                        measure the Go module at path and print the report
  compare <before.json> <after.json>  compare two saved JSON reports and print the comparison
  baseline [path]                     write the reproducible baseline report of the module at path
  version                             print the scree version

Run scree audit --help for the audit flags and terms.
Run scree baseline --help for the baseline flags.
`

const baselineHelp = `usage: scree baseline [path] [--config <file>] [--out <file>] [--check] [--quiet]

The command audits the Go module at path, which defaults to the current directory, and writes
the report as JSON with repo.root set to . and meta.durationMs set to 0. The same files,
configuration, and versions then give the same bytes on any machine. The file is a valid
--baseline for scree audit and an argument for scree compare.
The audit leaves the output file out, so the file does not count as an unsupported file.
With --check the command writes nothing and compares the bytes it would write with the file.
Exit code 0 means the file is current, 2 means it is stale, and 1 means the file is missing or unreadable.

flags:
  --config <file>  read the configuration from the file, not from scree.yaml at the path
  --out <file>     write the baseline to the file, default scree-baseline.json at the path
  --check          compare with the committed file and write nothing, so CI fails when a change did not update it
  --quiet          do not print the line that names the file or the current baseline
  --help, -h       print this help
`

const compareHelp = `usage: scree compare <before.json> <after.json> [--json|--md]

The command compares two reports that scree audit --out saved. It runs no audit.
Exit code 2 means the reports are not comparable.

flags:
  --json      print the comparison as JSON
  --md        print the comparison as Markdown
  --help, -h  print this help
`

const auditHelp = `usage: scree audit <path> [--config <file>] [--baseline <report.json>] [--json|--md] [--out <file>] [--quiet]

flags:
  --config <file>           read the configuration from the file, not from scree.yaml at the path
  --baseline <report.json>  compare with a report that scree audit --out saved, and run the policy checks that need a baseline
  --json                    print the report as JSON
  --md                      print the report as Markdown
  --out <file>              write the report to the file, in the format of --json, --md, the .json or .md extension, else text
  --quiet                   do not print the line that names the written file
  --help, -h                print this help

terms:
  index            0 to 100, the structural debt of the production code, lower is better
  production/test  the measured sets, test holds _test.go files and classify.test matches, production the other measured Go files
  cc               the cyclomatic complexity of one function
  p50/p90/max      the median, the 90th percentile, and the highest cc of a set
  eroded           the functions with cc above 10, with their share of the set mass
  mass             the cc of a function times the square root of its source lines
  count term       scores the number of eroded functions or clone groups, with no division by size
  saturating term  scores the eroded share or the duplication density, which divide debt by size
  clones           the groups of repeated code of at least 100 tokens and 3 lines
  dup lines        the source lines in clones, with their share of the set lines
  partial          a production input failed or hit a cap, so the index counts that part as full debt
  baseline         a saved report of an earlier audit, compared by index, metrics, and findings
  policy           the config file checks that fail the run with exit code 2, like a baseline scree cannot compare
  safeguards       process checks the repo declares, reported but never scored
`

// errExclusive reports two output formats in one run.
var errExclusive = errors.New("--json and --md are exclusive")

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("scree", flag.ContinueOnError)
	fs.SetOutput(stderr)
	// The program has no other place to report a failed write to stderr.
	fs.Usage = func() { _, _ = fmt.Fprintln(stderr, usage) }
	wantHelp := helpFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	switch {
	case *wantHelp:
		return writeText(stdout, help)
	case fs.NArg() == 1 && fs.Arg(0) == "version":
		return writeText(stdout, "scree "+scree.Version+"\n")
	case fs.NArg() > 0 && fs.Arg(0) == "audit":
		return audit(fs.Args()[1:], stdout, stderr)
	case fs.NArg() > 0 && fs.Arg(0) == "baseline":
		return baselineCommand(fs.Args()[1:], stdout, stderr)
	case fs.NArg() > 0 && fs.Arg(0) == "compare":
		return compareCommand(fs.Args()[1:], stdout, stderr)
	}
	fs.Usage()
	return 1
}

// helpFlags defines -h and --help, so that a help request prints to stdout and exits 0.
func helpFlags(fs *flag.FlagSet) *bool {
	wantHelp := fs.Bool("help", false, "print the help")
	fs.BoolVar(wantHelp, "h", false, "print the help")
	return wantHelp
}

func writeText(w io.Writer, text string) int {
	if _, err := io.WriteString(w, text); err != nil {
		return 1
	}
	return 0
}
