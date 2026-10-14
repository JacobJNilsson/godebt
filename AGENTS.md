# Working in this repository

Guidance for any agent or contributor who changes code here. Read it before
the first commit.

## First thing, per clone

Run `make setup` once per clone or worktree. It points Git at the tracked
hooks in `.githooks/`:

- `pre-commit` runs the scoped gate (`make check-scoped`).
- `pre-push` runs the full `make check`.

`core.hooksPath` is local configuration and not version-controlled, so a
fresh clone has no hooks until you run `make setup`.

## The gate

`make check` is the one definition of green. Pre-push and CI run it
unchanged. It runs, in order: `go mod tidy -diff`, `go vet ./...`,
`golangci-lint run ./...`, the coverage gate self-test, `go test -race ./...`
with the coverage gate at `COVERAGE_MIN` (90), `go build ./...`, and
`make antislop`, which runs the anti-slop-go rules over the module.
anti-slop-go v1.4.1 requires Go 1.26 or newer. With the default
`GOTOOLCHAIN=auto`, the go command downloads a newer toolchain for
`make antislop`. With `GOTOOLCHAIN=local` on a host older than Go 1.26,
`make antislop` fails.
`make selfcheck` audits this repository with its own binary and is part of
`make check` from step 1 on. It fails when the policy in `godebt.yaml`
fails, for example when the index rises above `maxIndex`. It also fails when
`godebt-baseline.json` is stale, so a change that alters a measurement needs
`make baseline`, which runs `go run ./cmd/godebt baseline .`.

The pre-commit tier, `make check-scoped`, runs vet, lint, anti-slop-go, and
race tests on the packages a staged change touches, with the same coverage
bar over the scoped profile. Changes to `go.mod`, `go.sum`, `Makefile`,
`scripts/`, `.githooks/`, or `.golangci.yml` escalate to the full gate.

## Spec first

The spec in `docs/spec/` defines every metric, the report contract, and the
command line. Code that drifts from the spec changes the spec in the same
pull request, with the reason in the commit body. An implementation agent
does not edit the spec on its own; it reports the conflict with a suggested
change.

## Tests

Write the test from the spec before the code. Watch it fail. Then write the
code that makes it pass. A test written after the code pins what the code
does, not what the spec demands.

- Every sorted output has a shuffle test that asserts the same order.
- Every metric has a test that yields `incomplete` and asserts that no value
  is reported.
- Fixture modules under `testdata/fixtures/` are real Go source. Golden
  reports under `testdata/golden/` are regenerated with `make golden` and
  reviewed as diffs.

## Design restraint

Before building, ask whether the metric should exist at all, whether the
spec asks for less, and whether an existing tool already covers it. A metric
that moves on idiomatic Go with no structural change is a bug in the metric.
Flag speculative parts in the pull request instead of shipping them
silently.

## Merging

- Every change arrives through a pull request. Never push `main`.
- Rebase-merge. Story-sized Conventional Commits. A subject that needs "and"
  is two commits. A rename is its own commit.
- Never merge a pull request whose CI is not green.
- Never post comments, replies, or reviews on pull requests. Report findings
  in the run report instead.

## House style

- Prose follows Simplified Technical English: active voice, short
  sentences, one word for one meaning, no idioms, no em dashes.
- Code comments state a fact the code cannot show, in one sentence.
- No private repository names or identifiers in this repository. Public
  modules only in corpus and fixtures.
