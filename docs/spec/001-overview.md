# 001: Overview

## Problem

A Go codebase gets harder to change long before it stops building. Functions
grow branches. Blocks get copied between packages with a rename. Nothing
fails, but every change costs more than the last one. Go has linters for
rule violations and `gocyclo` for a per-function number, but no tool that
measures how much structural debt a module carries, shows where it sits,
and tells you whether a change made it worse.

## Solution

`godebt` is one Go binary and one Go package. It reads a Go module from disk,
parses every file once, and measures two things:

- Complexity and erosion. Cyclomatic complexity per function, and how much of
  the module's function mass sits in functions above the complexity
  threshold.
- Duplication. Groups of code that repeat, with identifiers and literals
  renamed, above a minimum size.

It folds those into a 0–100 index where lower is better. Every point traces
to a raw metric, a threshold, and a list of findings with file and line. A
saved report is a baseline: a later audit compares against it and a
declared policy decides whether the run passes. A repository can also commit
a baseline on `main` with `godebt baseline`, and CI runs `godebt baseline
--check` to fail a change that did not update it.

Separately from the score, `godebt` reports which safeguards the repository
has configured: Git hooks, lint configuration, CI steps, coverage budgets.
These are evidence about the project's process. They never change the index.

The name says what the tool measures: structural debt in Go code. Releases
up to 0.3.0 used the name `scree`.

## Goals

- Deterministic. The same files and the same configuration produce a
  byte-equal report payload, on any machine.
- Offline. No network, no model, no credentials, no database. An audit reads
  files and does arithmetic.
- Zero footprint. A default audit writes nothing. It needs no `go.mod`, no
  Git, no built dependencies, and never runs the target's code, tests, or
  tools.
- Honest about gaps. A measurement that could not finish says so. Unsupported
  files count as coverage, never as clean code. A partial analysis never
  publishes a complete-looking score.
- One core. The CLI and the public package run the same code path.

## Non-goals

- Import cycle detection. The Go compiler rejects package cycles, so every
  module that builds has zero. Cycles within a package between files are
  normal Go.
- Rule-level linting. `anti-slop-go`, `staticcheck`, and `golangci-lint`
  cover that.
- Running the target's checks. `godebt` inspects configuration and never
  claims a check passes.
- History in a database and multi-repository fleet runs. Deferred to a later
  version. Baseline comparison over saved JSON covers the CI case.
- A web UI, hosted service, or automatic remediation.
- Any AI feature.

## Influenced by

`godebt` is a Go re-imagining of [trellis](https://github.com/jayminwest/trellis)
by Jaymin West, a deterministic sloppiness audit for TypeScript. The metric
definitions, the traceable index, the rule that test code and safeguards
never offset production debt, and the report contract come from that work.
Code ported directly from trellis carries its MIT notice in `NOTICE`.

## Related work

- [gocyclo](https://github.com/fzipp/gocyclo) and
  [gocognit](https://github.com/uudashr/gocognit) report per-function
  complexity. `godebt` uses the same cyclomatic definition and adds
  aggregation, erosion, and comparison.
- [dupl](https://github.com/mibk/dupl) finds Go clones over a syntax tree.
  `godebt` uses a normalized token stream so the same thresholds apply to
  any construct.
- [reporeport](https://github.com/JacobJNilsson/reporeport) shows many
  repositories in a dashboard. It can run `godebt` for its Go complexity
  numbers.
