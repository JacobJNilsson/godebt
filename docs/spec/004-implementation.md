# 004: Implementation

## Layout

```
godebt/
├─ godebt.go               # public package: Audit, Compare, Evaluate, LoadReport, LoadConfig
├─ cmd/godebt/             # flag parsing and dispatch only
├─ internal/
│  ├─ discover/           # walk, git ls-files, .gitignore, source-set classification
│  ├─ inventory/          # one parse per file; functions with identity, CC, nesting, SLOC
│  ├─ complexity/         # distributions, erosion, hotspot findings
│  ├─ duplication/        # token stream, suffix array, LCP, groups, line accounting
│  ├─ contract/           # shared metric, finding, and report types; imports nothing internal
│  ├─ formula/            # every constant and the scoring function
│  ├─ report/             # report assembly, JSON round-trip, terminal and Markdown renderers
│  ├─ config/             # godebt.yaml load and strict validation
│  ├─ compare/            # report comparison
│  ├─ policy/             # policy evaluation over a report and a comparison
│  ├─ safeguards/         # configuration inspection
│  └─ corpus/             # corpus list, download, and results table for make corpus
├─ corpus/                # modules.txt and the recorded results.md
└─ docs/spec/
```

The pipeline runs one way: discover → inventory → measure → score → report.
No package imports a package to its right. `contract` and `formula` are
leaves: every stage may import them, and they import no other internal
package.

## Stack

- Go 1.23 as the floor in `go.mod`. `go mod tidy -diff` needs 1.23, and so
  does the `.gitignore` matcher chosen in step 1.
- Standard library for parsing (`go/parser`, `go/ast`, `go/scanner`,
  `go/token`) and for the CLI (`flag`).
- `gopkg.in/yaml.v3` with `KnownFields(true)` for configuration.
- The `git` command for the file list inside a Git work tree. The walk runs
  `git -C <root> ls-files -z --cached --others --exclude-standard` with
  `LC_ALL=C`. It runs `git -C <root> rev-parse --is-inside-work-tree
  --show-prefix` first and falls back to the `.gitignore` matcher when `git`
  is missing or the root is not inside a work tree. The prefix is the path
  of the root below the top level, which Git warnings start with. Both
  commands run without `GIT_DIR`,
  `GIT_WORK_TREE`, and `GIT_INDEX_FILE`, so a Git hook that runs the audit
  lists the files of the root, not of the repository that runs the hook.
- `github.com/boyter/gocodewalker/go-gitignore` for `.gitignore` matching
  outside a Git work tree. It matched `git check-ignore` on every case of
  a 26-file conformance fixture with nested ignore files, negation,
  anchors, `**`, directory-only
  patterns, and escaped trailing spaces, with two transitive modules. Call
  `repo.Absolute(path, isDir).Ignore()` on a repository opened with the
  file name `./.gitignore`. The promoted `repo.Ignore` and
  `repo.MatchIsDir` never match, and the exact name `.gitignore`
  makes the library read `.git/info/exclude` as well. The walk checks each
  `.gitignore` itself before the library opens it, because the library
  reports a failed open and a mid-read error only through its pattern
  error handler. Known gaps against Git: the library rejects `a/**b`, `***`,
  and a lone `!`, which Git applies. A symlinked `.gitignore` is a read
  error, as in Git. No other runtime dependency.
- `golangci-lint` v2 with the same linter set as `anti-slop-go`, plus the
  `anti-slop-go` standalone binary run by `make antislop` at a pinned
  version.

## Conventions

- Tests sit beside the unit as `<name>_test.go`. Golden files live under
  `testdata/golden/`. Fixture modules live under `testdata/fixtures/<name>/`
  and are real Go source that the fixture's own tests parse.
- Every sorted output has a test that shuffles the input and asserts the
  same order.
- Every metric has a test that produces `incomplete` and asserts that no
  value is reported.
- The coverage gate is the same two-tier `make check` and `make check-scoped`
  as `anti-slop-go`, with `COVERAGE_MIN` at 90.
- `make selfcheck` runs `godebt audit .` over this repository and is part of
  `make check` from step 1 on. Each step extends what the self-audit prints.
  From step 1 it must exit 0. From step 4 a rise in the repository's own
  index above the committed budget in `godebt.yaml` fails the gate.
  `make selfcheck` also runs `godebt baseline --check` against the committed
  `godebt-baseline.json`, so a change that alters a measurement must
  regenerate that file.
- No private repository names or identifiers in this repository. Corpus
  entries are public modules at pinned revisions.

## Steps

One pull request per step, each following the way-of-working: spec first,
tests from the spec, implementation, simplicity check, review loop, unslop
pass, draft PR.

0. Repository. `go.mod`, `Makefile`, tracked hooks, `.golangci.yml`, CI
   workflow, `AGENTS.md`, this spec, README with the trellis credit,
   `NOTICE`.
1. Discover and inventory. Walk, `.gitignore`, source sets, coverage counts,
   parse, functions with identity, CC, nesting, SLOC. Golden tests over
   fixture modules including build-tag variants and nested `go.mod`.
   `godebt.Audit` and `godebt audit <path> [--json]` exist from this step and
   print the provisional summary: repo, coverage, function counts, parse
   errors. `make selfcheck` runs it over this repository.
2. Complexity and erosion. Distributions, mass, eroded share and count,
   hotspot findings, `not-applicable` on empty sets.
3. Duplication. Token stream, SA-IS, Kasai, maximal repeats, groups,
   overlap-union lines, density, budgets that yield `incomplete`. Paired
   fixtures: exact copy, renamed copy, four-way copy, below-threshold idiom,
   near clone with two changed lines.
4. Contract, formula, renderers. `Report` type, strict JSON load, formula
   with apportionment, terminal and Markdown renderers, golden reports over
   five fixture repositories: clean, sloppy, incomplete, function-free,
   mixed-language.
5. Config, compare, policy, CLI. `godebt.yaml`, `Compare`, `Evaluate`,
   `cmd/godebt` with exit codes, CLI-to-package parity test.
6. Safeguards. Every surface in 002, evidence levels, broken references.
7. Corpus and calibration, as the next section states. Tag `v0.1.0`
   after this step.

## Calibration

Step 7 checks the scoring constants against paired refactors and records
the index over a corpus of public modules.

A paired refactor is two fixture modules, `before` and `after`, under
`testdata/fixtures/pairs/<name>/`. A test in `make check` audits both and
asserts the direction of the index.

| Pair | Change from `before` to `after` | Index |
|---|---|---|
| `extract-clone` | Two copies of a function become one shared function. | lower |
| `split-function` | A function with cc above 10 becomes functions with cc 10 or less. | lower |
| `add-clone` | A function gets a copy in another file. | higher |
| `add-branches` | A function with cc 10 or less gets branches to cc above 10. | higher |
| `rename` | Every identifier and literal gets a new name or value. | equal |
| `move` | A function moves to a file in another package. | equal |
| `tests-only` | A test file with a clone group and an eroded function appears. | equal |
| `comments` | Comments and blank lines appear. | equal |
| `pad` | The `after` module holds `before` plus a second package of clean code, about as many production lines as `before`, with no eroded function and no clone group. | near |

Lower and higher mean a move of at least one index point. Near means the
index moves by at most 3 points either way. In `pad`, the new code must not
clone the code of `before`. Duplication matches normalized tokens, so a
renamed copy counts as a clone. Each `before`
module scores above 0. An index that moves in the other direction, or
does not move, is a calibration defect. The fix changes a constant in
`internal/formula` and raises `ScoringVersion`. The fix never edits a
fixture to fit the constants.

`corpus/modules.txt` lists public Go modules, one `path@version` per line,
from under 2,000 production code lines to over 50,000. `make corpus`
downloads each module through the Go module proxy and audits the module
directory with no configuration. The audit sees the files of the module
zip. `make corpus` writes `corpus/results.md` with one row per module:
path, version, index, the two contributions, production code lines,
production functions, eroded functions, clone groups, and completeness.
The repository commits that file. `make corpus` needs the network, so
`make check` does not run it.

Under the table, `make corpus` also writes the Spearman rank correlation of
the index with production code lines, and of the index with debt per
thousand production lines. Both cover the modules above 5,000 production
lines. Debt is eroded functions plus clone groups.
A correlation that cannot be computed, because fewer than two modules qualify
or one side has no spread, prints as `undefined`.

One limit is known. A module padded with clean code lowers both share terms,
and no current term prevents that. The `pad` pair bounds the effect for a
small module. A large module still drops further.

The corpus shows how the index spreads over real code. It never changes a
constant by itself. When every pair passes, `ScoringVersion` drops the
`-provisional` suffix.

Deferred to a later version: SQLite history, fleet over `targets.yaml`, a
scored dimension over the package graph.
