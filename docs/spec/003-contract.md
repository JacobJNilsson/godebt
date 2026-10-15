# 003: Contract

The report, the configuration file, the command line, and the public
package. Everything here is versioned and tested with golden files.

## Versions

Three versions travel with every report:

- `schemaVersion`: the shape of this document. Starts at `1.0.0`.
- `analyzerVersion`: the `godebt` release that produced the measurements.
  Equals the module version.
- `scoringVersion`: the formula constants. It is `0.2.0`, the version the
  calibration in [004-implementation.md](004-implementation.md) checked. A
  change to any constant raises it.

Two reports are comparable when all three versions match and their
configuration digests match. An incomparable pair is refused with the
reason. It is never compared silently. A report with an unknown schema
version fails to load, so a schema refusal can arise only from reports
built in memory.

## Determinism

The measurement payload excludes timestamps, durations, absolute paths
outside `repo.root`, and machine identifiers. Same files, same
configuration, same versions means a byte-equal payload. Timing goes under
`meta` and never enters comparison.

A baseline is the reproducible form of a report. It sets `repo.root` to `.`
and `meta.durationMs` to 0. The same files, configuration, and versions then
give a byte-equal file on any machine, in any directory.

Every list in the report has a defined sort order. Map iteration order never
reaches the output.

## Report

```jsonc
{
  "schemaVersion": "1.0.0",
  "analyzerVersion": "0.1.0",
  "scoringVersion": "0.2.0",
  "repo": { "root": "/abs/path", "module": "github.com/x/y" },   // module "" when no go.mod
  "configDigest": "sha256:…",                                      // of the normalized effective config
  "coverage": {
    "production": { "files": 210, "sloc": 13280 },
    "test":       { "files": 96,  "sloc": 5100 },
    "generated":  { "files": 4 },
    "vendored":   { "files": 0 },
    "testdata":   { "files": 12 },
    "excluded":   { "files": 3 },
    "unsupported":{ "files": 30 },
    "nestedModules": ["tools/gen"]
  },
  "completeness": "complete",            // complete | incomplete, rolled up over production metrics
  "metrics": {
    "erosion.excess-share.production": {
      "state": "complete",               // complete | incomplete | not-applicable
      "value": 0.05, "unit": "ratio",
      "numerator": 114.6, "denominator": 2291.7,
      "detail": {}
    }
  },
  "score": {
    "index": 7,
    "direction": "lower-is-better",
    "partial": false,
    "contributions": [
      { "dimension": "complexity-erosion", "points": 7, "weight": 0.6,
        "terms": [
          { "metricId": "erosion.excess-share.production", "state": "complete", "value": 0.05, "saturatesAt": 0.55, "score": 9.09 },
          { "metricId": "erosion.excess-mass.production", "state": "complete", "value": 114.6, "countScale": 700, "score": 13.17 }
        ] },
      { "dimension": "duplication", "points": 0, "weight": 0.4,
        "terms": [
          { "metricId": "duplication.density.production", "state": "complete", "value": 0, "saturatesAt": 0.3, "score": 0 },
          { "metricId": "duplication.groups.production", "state": "complete", "value": 0, "countScale": 15, "score": 0 }
        ] }
    ]
  },
  "findings": [
    { "kind": "complexity.hotspot", "path": "internal/audit/run.go", "startLine": 41, "endLine": 128,
      "identity": "internal/audit:Runner.Run", "ambiguous": false, "sourceSet": "production",
      "facts": { "cc": 23, "nesting": 4, "sloc": 71, "mass": 193.8 } }
  ],
  "safeguards": [
    { "id": "pre-commit-hook", "evidence": "structurally-wired",
      "locations": [ { "path": "Makefile", "line": 12 } ],
      "notes": "core.hooksPath set by target setup; hook runs make check-scoped" }
  ],
  "limits": [ { "metricId": "duplication.groups.test", "reason": "tokens cap 2000000 exceeded" } ],
  "meta": { "durationMs": 812 }
}
```

A `safeguard.broken-reference` finding has no `sourceSet`, because it sits
in a configuration file and not in measured code. Every other finding kind
carries one.

Metric ids follow `<dimension>.<name>.<set>`. Every metric listed in
[002-metrics.md](002-metrics.md) appears for both `production` and `test`,
even when `not-applicable`. Unknown fields are rejected on load. `detail`
and `facts` are typed objects with a fixed shape per metric or finding kind,
never open maps, and a load rejects facts whose shape does not match the
kind. `value`, `numerator`, and `denominator` appear only when the state is
`complete`, and the pair appears only on ratios. A score term follows the
same rule: `value` appears only when the term's state is `complete`.

## Configuration

`godebt.yaml` at the audited root, or the file named by `--config`. Every key
is optional. Unknown keys are an error. Scoring keys do not exist here: the
policy gates a run and never changes how the index is computed.

```yaml
exclude:                      # source set excluded, glob per 002
  - "internal/gen/**"
classify:
  test:                       # extra files that count as test code
    - "internal/testutil/**"
policy:
  maxIndex: 40                # fail when index > 40
  regression:                 # only evaluated with a baseline
    maxIncrease: 2            # absolute index points
    maxIncreasePercent: 10    # relative to the baseline index
  budgets:
    duplication.density.production: { max: 0.05 }
  failOnNew:                  # finding kinds that fail the run when a new one appears
    - complexity.hotspot
```

## Comparison

`Compare(before, after)` takes two loaded reports and returns:

- the index delta and each metric's delta
- new, resolved, and persistent findings

Finding matching is conservative. A named hotspot matches on identity;
line numbers do not matter. An ambiguous identity matches in two cases:
when it is the sole finding of its kind on its path in both reports, or
when both reports hold a finding with the same identity string on the same
path and the same facts. Otherwise it is reported as one resolved and one
new finding, so an unchanged tree compared with itself always reports every
finding as persistent. A clone group matches on its group id.

## Policy

`Evaluate(policy, report, baseline)` returns a list of structured reasons.
Each check runs independently; a better index cannot suppress a `failOnNew`
failure.

- `maxIndex`: fails when `report.score.index > maxIndex`, and always when
  the score is `partial`. Missing analysis never passes a gate.
- `budgets`: fails when a named metric's value exceeds `max`. A metric that
  is `incomplete` fails the budget; missing analysis never passes.
- `regression`: requires a baseline. Fails when the index rose by more than
  `maxIncrease` points or more than `maxIncreasePercent` of the baseline.
- `failOnNew`: requires a baseline. Fails when the comparison lists a new
  finding of a named kind.

Without a baseline, the baseline-dependent checks are skipped and the
command names them on stderr. The report itself carries no policy state.
A budget on a metric id that no report can hold is a configuration error,
not a policy failure. Integer knobs reject fractions.

## Command line

```
godebt audit <path> [--config <file>] [--baseline <report.json>] [--json|--md] [--out <file>] [--quiet]
godebt compare <before.json> <after.json> [--json|--md]
godebt baseline [path] [--config <file>] [--out <file>] [--check] [--quiet]
godebt version
godebt --version
```

- `audit` measures, scores, evaluates the policy, and prints the report. The
  default renderer is terminal text. `--out` writes the report to a file and
  prints a one-line confirmation to stderr; `--quiet` suppresses the
  confirmation. Policy reasons go to stderr. With `--baseline`, the JSON on
  stdout carries a `comparison` block and is not a valid baseline. A file
  written by `--out` never carries it, so a saved report is always a valid
  baseline for a later run. Save reports with `--out`, not by redirecting
  stdout.
- `version` and `--version` print `godebt <version>` to stdout and exit
  `0`.
- `compare` reads two saved JSON reports and prints the comparison. It runs
  no audit.
- `baseline` audits like `audit` and writes the reproducible form of the
  report as JSON. The path defaults to `.`. `--out` defaults to
  `godebt-baseline.json` at the audited root, and `--quiet` suppresses the
  confirmation on stderr. The written file loads with `LoadReport`, so it is
  a valid `--baseline` for `audit` and an argument for `compare`. A
  repository commits the file on `main`, and its CI runs `baseline --check`.
- `baseline --check` writes nothing. It audits, builds the same bytes, and
  compares them with the file at `--out`. When the bytes are equal, the
  command exits `0` and prints one line to stderr unless `--quiet`. When
  they differ, the command exits `2`. It prints that the baseline has not
  been updated and the command that regenerates it. For comparable reports
  it prints the index before and after and the counts of new and resolved
  findings, otherwise `refused:` and the reason. It then prints one
  `changed:` line per differing leaf of the report, without `meta`, as a
  dotted path with the old and new value. An array prints only its path.
  It adds the item counts when they differ. The lines for `score` come
  first, then `findings`, then every other path in alphabetical order. The
  output stops at 10 lines and ends with `+N more`. A missing or unreadable
  file exits `1` and names the path. A file that fails to load, for example
  after a schema version change, also exits `1`, with the error and the
  regenerate command. Any change to a
  measurement makes the file stale, including a new non-Go file, because
  coverage counts it.
- The baseline run leaves its output file out of the audit. The walk skips
  the file at the `--out` path, so it appears in no source set and no
  coverage count. Writing the file for the first time therefore does not
  change the measurement, and `--check` finds it current. A different file
  at another path counts as usual. A plain `audit` counts the committed
  baseline like any other file, so its coverage differs from the baseline's
  by that file. `Options.Omit` carries the path in the
  public package.
- Exit codes: `0` when the run passed, `2` when the policy failed or a
  comparison was refused, `1` on an operational error (a path that does not
  exist, an invalid configuration, an unreadable report). A policy failure
  still emits the full report so CI keeps the evidence.

The default run is stateless. Without `--out` it writes nothing.

## Public package

```go
package godebt // import "github.com/JacobJNilsson/godebt"

func Audit(ctx context.Context, root string, opts Options) (*Report, error)
func LoadReport(path string) (*Report, error)
func Baseline(ctx context.Context, root string, opts Options) ([]byte, error)
func Compare(before, after *Report) *Comparison
func Evaluate(policy Policy, report *Report, baseline *Report) PolicyResult
func LoadConfig(path string) (*Config, error)
```

A refused comparison is a `Comparison` with `Comparable` false and a
`Refusal`, not an error. `PolicyResult` carries `Failed`, `Reasons`,
`Skipped`, and `Refusal`. A refused baseline sets `Refusal` and `Failed`
whatever the policy declares, so the SDK and the command line agree on the
same inputs.

`Options` holds the config (loaded or in memory). `Audit` returns an
operational error only for conditions that stop the audit. Incomplete
analysis is not an error; it is a state inside the report.

`cmd/godebt` calls these functions and does nothing else. A deep-equal test
proves that the CLI's JSON output is the marshalled `Report` from `Audit`.
