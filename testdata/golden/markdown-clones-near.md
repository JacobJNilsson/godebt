# godebt report

example.com/clones/near, godebt 0.0.0-test.

The index is 54/100, lower is better, scoring 0.2.0.
Contributions: complexity-erosion 33, duplication 21.

| set | files | sloc | funcs | cc p50/p90/max | eroded | clones | dup lines |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| production | 2 | 80 | 2 | 11/11/11 | 2 (100%) | 1 | 44 (55%) |
| test | 0 | 0 | 0 | - | - | 0 | 0 |

Other files: unsupported 1.

## Hotspots

### production (2)

| path | lines | identity | cc | mass |
| --- | ---: | --- | ---: | ---: |
| a.go | 4-42 | .:Score | 11 | 68.7 |
| b.go | 4-42 | .:ScoreCopy | 11 | 68.7 |

### test (0)

## Clones

### production (1)

| id | tokens | members |
| --- | ---: | --- |
| 806501b520ea2592 | 125 | a.go:4-25<br>b.go:4-25 |

### test (0)

## Safeguards

| id | evidence | locations | notes |
| --- | --- | --- | --- |
| agent-hooks | absent |  | No agent hook is declared. |
| ci-workflow | absent |  | No workflow file has a step. |
| coverage-budget | absent |  | No Makefile exists. |
| lint-config | absent |  | No .golangci file exists. |
| pre-commit-hook | absent |  | No hook file or hook tool declares pre-commit. |
| pre-push-hook | absent |  | No hook file or hook tool declares pre-push. |
| test-check | absent |  | No Makefile recipe runs go test. |
| vet-check | absent |  | No Makefile recipe runs go vet. |
