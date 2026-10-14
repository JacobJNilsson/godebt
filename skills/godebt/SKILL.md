---
name: godebt
description: Measures structural debt in a Go module with the godebt CLI and checks that a change does not raise it. Use before you start a change in a Go module, and when asked about structural debt, complexity, or duplication.
---

# godebt

`godebt` gives a Go module an index from 0 to 100. Lower is better. It lists two kinds of findings, each with a file and lines: functions with too many branches, and blocks of repeated code. Run `godebt audit --help` for the terms in the output.

To check a change, save a report before you start and compare the finished code with it. The report lives outside the repository for the length of the task.

If `godebt` is not installed, run `go install github.com/JacobJNilsson/godebt/cmd/godebt@latest`. If the user only asks about debt, run `godebt audit .` and answer the question.

## Check a change

1. Before you change code, save a report outside the repository:

   ```sh
   godebt audit . --quiet --out "${TMPDIR:-/tmp}/godebt-$(basename "$PWD")-before.json"
   ```

   If you changed code before you saved the report, audit the commit you started from. Run `git worktree add --detach <dir> <commit>`, run the same `godebt audit` command in `<dir>` with the same `--out` path, then run `git worktree remove <dir>`.

2. When the change is done, compare the code with the saved report:

   ```sh
   godebt audit . --baseline "${TMPDIR:-/tmp}/godebt-$(basename "$PWD")-before.json" --quiet --out "${TMPDIR:-/tmp}/godebt-$(basename "$PWD")-after.json"
   godebt compare "${TMPDIR:-/tmp}/godebt-$(basename "$PWD")-before.json" "${TMPDIR:-/tmp}/godebt-$(basename "$PWD")-after.json"
   ```

   `godebt compare` prints the index before and after, and each new or resolved finding with its file and lines, for example `production  complexity.hotspot  plan.go:4-34  .:Plan`. Exit `2` from `godebt audit` means a policy in `godebt.yaml` failed or the comparison was refused. Stderr names the reason, for example `policy: maxIndex: index 33 is above 30`.

3. If the index rose, a new finding appeared, or a policy failed, fix it. Report a finding that comes from a problem outside your task to the user. If you cannot fix the rest inside the task, stop. Tell the user the index before and after and the findings that grew.

4. Tell the user the index before and after the change.

If a command refuses or exits `1`, stop and tell the user. `godebt` refuses to compare two reports from different `godebt` versions or from different configurations. A reinstall of `godebt` or an edit to `godebt.yaml` during the task causes that refusal.

## What not to do

- Do not add trivial code to lower the share of debt, because padding lowers the index without paying the debt.
- Do not move logic into test files, because test code never enters the index.
- Do not add `exclude` entries to hide debt, because the debt stays and only the measurement hides it.
- Do not raise `policy.maxIndex` or loosen any other policy check to pass. The policy is the limit the user chose. Only the user changes `godebt.yaml`.
