---
name: scree
description: Measures structural debt in a Go module with the scree CLI and checks that a change does not raise it. Use when a Go repository has `scree-baseline.json` or `scree.yaml`, or when asked about structural debt, complexity, or duplication.
---

# scree

`scree` gives a Go module an index from 0 to 100. Lower is better. The repository commits `scree-baseline.json` on `main`. CI runs `scree baseline --check .` and fails a pull request that changes a measurement without updating the file. The committed file is the report that you compare your change against. Run `scree audit --help` for the terms in the output.

If the repository has no baseline, run `scree audit .` and answer the question. Set up CI only if the user asks for it.

## Check a change

CI keeps `scree-baseline.json` current on `main`. `scree baseline --check .` only tells you whether that file is current. To learn whether your change made things worse, compare the code with the file.

1. After you change code, run `scree audit . --baseline scree-baseline.json --out "${TMPDIR:-/tmp}/scree-current.json"`. Exit `2` means a policy failed or the comparison was refused, and stderr names the reason, for example `policy: maxIndex: index 33 is above 30`. Run `scree compare scree-baseline.json "${TMPDIR:-/tmp}/scree-current.json"` to see each new finding with its file and lines, for example `production  complexity.hotspot  plan.go:4-34  .:Plan`.
2. If the index rose, a new finding appeared, or a policy failed, fix it. Report a finding that comes from a problem outside your task to the user. If you cannot fix the rest inside the task, stop. Tell the user the index before and after and the findings that grew.
3. If the comparison passes and shows nothing worse, run `scree baseline .` and commit `scree-baseline.json` with your change. Then `scree baseline --check .` passes, and that is what CI runs. A change can update the file without moving the index, for example a new non-Go file.
4. If a command refuses or exits `1`, stop and tell the user. The usual reason is a version refusal, which means your local `scree` differs from the version CI pins.

## Set up CI

1. Run `go install github.com/JacobJNilsson/scree/cmd/scree@v0.3.0`. The `v0.1.0` tag lacks `baseline`, so use `v0.2.0` or a later tag.
2. Run `scree baseline .` and commit `scree-baseline.json` on `main`.
3. Add a CI step that runs `scree baseline --check .`.

Pin the version, because a different `scree` version writes different bytes. `scree version` must match the version CI installs. Pin a tag, because every untagged commit prints the same string.

```yaml
      - name: Check the scree baseline
        run: |
          go install github.com/JacobJNilsson/scree/cmd/scree@v0.3.0
          scree baseline --check .
```

## What not to do

- Do not add trivial code to lower the share of debt, because padding lowers the index without paying the debt.
- Do not move logic into test files, because test code never enters the index.
- Do not add `exclude` entries to hide debt, because the debt stays and only the measurement hides it.
- Do not raise `policy.maxIndex` or loosen any other policy check to pass. The policy is the limit the user chose. Only the user changes `scree.yaml`.
- Do not run `scree baseline .` to silence a failed check before you read what changed, because that records a regression as the new normal.
