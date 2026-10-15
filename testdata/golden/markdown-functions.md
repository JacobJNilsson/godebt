# godebt report

example.com/functions, godebt 0.0.0-test.

The index is 5/100, lower is better, scoring 0.3.0.
Contributions: complexity-erosion 5, duplication 0.

| set | files | sloc | funcs | cc p50/p90/max | eroded | clones | dup lines |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| production | 9 | 146 | 23 | 1/11/15 | 3 (52%) | 0 | 0 |
| test | 1 | 13 | 3 | 2/12/12 | 1 (77%) | 0 | 0 |

Other files: unsupported 1.

## Hotspots

### production (3)

| path | lines | identity | cc | mass |
| --- | ---: | --- | ---: | ---: |
| cc.go | 35-70 | .:Long | 11 | 66.0 |
| cc.go | 73-76 | .:Short | 15 | 30.0 |
| cc.go | 79-81 | .:wrap#1 | 11 | 19.1 |

### test (1)

| path | lines | identity | cc | mass |
| --- | ---: | --- | ---: | ---: |
| functions_test.go | 14-17 | .:allSet | 12 | 24.0 |

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
