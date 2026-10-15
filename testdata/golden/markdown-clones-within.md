# godebt report

example.com/clones/within, godebt 0.0.0-test.

The index is 21/100, lower is better, scoring 0.3.0.
Contributions: complexity-erosion 0, duplication 21.
The test index is 0/100, not part of the index.

| set | files | sloc | funcs | cc p50/p90/max | eroded | clones | dup lines |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| production | 1 | 67 | 3 | 7/7/7 | 0 | 1 | 66 (99%) |
| test | 0 | 0 | 0 | - | - | 0 | 0 |

Other files: unsupported 1.

## Hotspots

### production (0)

### test (0)

## Clones

### production (1)

| id | tokens | members |
| --- | ---: | --- |
| 3aa4d00373694a3b | 118 | within.go:4-25<br>within.go:27-48<br>within.go:50-71 |

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
