# godebt report

example.com/clones/nested, godebt 0.0.0-test.

The index is 55/100, lower is better, scoring 0.2.0.
Contributions: complexity-erosion 33, duplication 22.

| set | files | sloc | funcs | cc p50/p90/max | eroded | clones | dup lines |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| production | 3 | 149 | 3 | 17/17/17 | 2 (88%) | 2 | 141 (95%) |
| test | 0 | 0 | 0 | - | - | 0 | 0 |

Other files: unsupported 1.

## Hotspots

### production (2)

| path | lines | identity | cc | mass |
| --- | ---: | --- | ---: | ---: |
| a.go | 4-62 | .:Heavy | 17 | 130.6 |
| b.go | 4-62 | .:HeavyCopy | 17 | 130.6 |

### test (0)

## Clones

### production (2)

| id | tokens | members |
| --- | ---: | --- |
| 4801221e89fa382b | 321 | a.go:4-62<br>b.go:4-62 |
| a9131f502c4085e8 | 122 | a.go:6-28<br>b.go:6-28<br>light.go:7-29 |

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
