# godebt

Measure structural debt in a Go module.

`godebt` reads a Go module from disk, parses every file once, and reports two
things: functions that have grown too many branches, and blocks of code that
repeat with renamed identifiers. It folds them into a 0 to 100 index. Lower is
better. Every point traces back to a raw measurement and a list of findings
with file and line.

## Use

```
go build ./cmd/godebt

godebt audit .                        # measure and print the report
godebt audit . --out report.json      # save a baseline
godebt audit . --baseline report.json # compare with the baseline
godebt compare before.json after.json # compare two saved reports
godebt baseline .                     # write godebt-baseline.json
godebt baseline --check .             # exit 2 when godebt-baseline.json is stale
```

Install the command with
`go install github.com/JacobJNilsson/godebt/cmd/godebt@latest`, or build the
command from a clone. Releases up to 0.3.0 use the name `scree` and the module
path `github.com/JacobJNilsson/scree`.

An audit prints the index, the two contributions, both source sets with their
distributions, the findings, and the safeguards the repository declares. The
flags `--json`, `--md`, `--out`, `--config`, `--baseline`, and `--quiet` shape
the output and the run. `godebt audit --help` names every flag and term.

Exit code `2` means the policy failed, a baseline could not be compared, or
`baseline --check` found a stale file. Exit code `1` means the command
could not run, and `0` means the run passed.

## Configuration

`godebt.yaml` at the audited root is optional. It sets the excluded paths, the
extra test patterns, and the policy that fails a run:

```yaml
exclude: [vendor/**]
classify:
  test: [fixtures/*.go]
policy:
  maxIndex: 40
  regression:
    maxIncrease: 2
  failOnNew: [complexity.hotspot]
```

Every check runs on its own. A better index never suppresses a `failOnNew`
failure. Without a baseline, the checks that need one are skipped and named.

## What it measures

Two dimensions weigh the index: complexity erosion at 0.6 and duplication at
0.4. A function is eroded when its cyclomatic complexity is above 10. A clone
group holds at least 100 tokens over at least 3 lines. The match runs over
normalized tokens. Renamed identifiers and changed literals still match.

Each dimension scores two terms with equal weight. A count term turns the
number of eroded functions or clone groups into a score that keeps rising with
the count. The count is not divided by repository size, so at the same debt
density a larger repository scores higher on it. A saturating term scores the
eroded share or the duplication density, which divide debt by the size of the
code. Compare a repository with its own baseline, not with a repository of
another size.

Test code is measured apart and never enters the index. Safeguards, such as a
tracked pre-commit hook, are reported with their evidence and never scored. A
default run writes nothing, and audits run offline.

## Calibration

Nine paired refactors under `testdata/fixtures/pairs/` decide whether the
constants hold. Each pair holds a `before` and an `after` module, and the only
difference between them is one stated refactor. A test asserts the direction of
the index: removing a clone or splitting a function with too many branches
lowers it, adding a clone or adding branches raises it, and renaming, moving a
function to another package, adding test code, or adding comments leaves it
alone. Doubling a module with clean code moves it by at most 3 points.

`corpus/modules.txt` holds 15 public modules at pinned versions, from 142
production lines to 75,973. Those line counts are the `coverage.production.sloc`
field of `godebt audit --json`. The production code lines column of the table
holds the same figure. `make corpus` audits each module and writes the
spread to `corpus/results.md`. That table never changes a constant by itself.

## Develop

`make check` is the one definition of green. It runs the tidy check, vet, lint,
the coverage gate, the tests, the build, and the anti-slop rules. It ends with
a self-audit that fails when this repository breaks its own policy in
`godebt.yaml`.
`make golden` regenerates the golden reports, and `make corpus` records the
corpus table.

The spec is in [docs/spec](docs/spec). Implementation follows the steps in
[004-implementation.md](docs/spec/004-implementation.md), and
[AGENTS.md](AGENTS.md) holds the working rules.

## Agent skill

The folder `skills/godebt` holds a skill for coding agents. It teaches the
baseline workflow. Install it by copying or linking the folder into the skills
folder that your agent reads.

## Influenced by

`godebt` is a Go re-imagining of [trellis](https://github.com/jayminwest/trellis)
by Jaymin West, a deterministic sloppiness audit for TypeScript. The metric
definitions, the traceable index, and the report contract come from that work.
So does the rule that test code and safeguards never offset production debt.
Code ported directly from trellis carries its MIT notice in [NOTICE](NOTICE).

## License

[MIT](LICENSE).
