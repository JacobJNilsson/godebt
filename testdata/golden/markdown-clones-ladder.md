# scree report

example.com/clones/ladder, scree 0.0.0-test.

The index is 31/100, lower is better, scoring 0.2.0.
Contributions: complexity-erosion 31, duplication 0.

| set | files | sloc | funcs | cc p50/p90/max | eroded | clones | dup lines |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| production | 1 | 32 | 2 | 1/13/13 | 1 (99%) | 0 | 0 |
| test | 0 | 0 | 0 | - | - | 0 | 0 |

Other files: unsupported 1.

## Hotspots

### production (1)

| path | lines | identity | cc | mass |
| --- | ---: | --- | ---: | ---: |
| ladder.go | 4-32 | .:Code | 13 | 70.0 |

### test (0)

## Clones

### production (0)

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
