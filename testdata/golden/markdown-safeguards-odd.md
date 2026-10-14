# godebt report

example.com/odd, godebt 0.0.0-test.

The index is 0/100, lower is better, scoring 0.2.0.
Contributions: complexity-erosion 0, duplication 0.

| set | files | sloc | funcs | cc p50/p90/max | eroded | clones | dup lines |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| production | 0 | 0 | 0 | - | - | 0 | 0 |
| test | 0 | 0 | 0 | - | - | 0 | 0 |

Other files: excluded 3, unsupported 3.

## Hotspots

### production (0)

### test (0)

## Clones

### production (0)

### test (0)

## Safeguards

| id | evidence | locations | notes |
| --- | --- | --- | --- |
| agent-hooks | absent |  | No agent hook is declared. |
| ci-workflow | unknown | .github/workflows/test.yml | .github/workflows/test.yml has no on key. |
| coverage-budget | absent |  | No Makefile variable or recipe sets a coverage budget. |
| lint-config | absent |  | No .golangci file exists. |
| pre-commit-hook | unknown | .githooks/pre-commit:5 | Line 5 of .githooks/pre-commit is in no understood form. |
| pre-push-hook | configured | .husky/pre-push | .husky/pre-push exists, and no Makefile recipe sets core.hooksPath to .husky. |
| test-check | unknown | .github/workflows/test.yml:8 | The step at .github/workflows/test.yml:8 reaches go test, and the inspector cannot verify that step or a line on its path. |
| vet-check | unknown | .github/workflows/test.yml:9<br>.github/workflows/test.yml:11 | The step at .github/workflows/test.yml:9 reaches go vet, and the inspector cannot verify that step or a line on its path. |

### Broken references (2)

| location | command |
| --- | --- |
| .githooks/pre-commit:3 | make verify |
| .githooks/pre-commit:4 | sh scripts/missing.sh |
