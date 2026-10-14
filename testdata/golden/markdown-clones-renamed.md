# godebt report

example.com/clones/renamed, godebt 0.0.0-test.

The index is 21/100, lower is better, scoring 0.2.0.
Contributions: complexity-erosion 0, duplication 21.

| set | files | sloc | funcs | cc p50/p90/max | eroded | clones | dup lines |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| production | 2 | 54 | 4 | 1/7/7 | 0 | 1 | 44 (81%) |
| test | 0 | 0 | 0 | - | - | 0 | 0 |

Other files: unsupported 1.

## Hotspots

### production (0)

### test (0)

## Clones

### production (1)

| id | tokens | members |
| --- | ---: | --- |
| e9223f9221094ed5 | 120 | a.go:6-27<br>b.go:4-25 |

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
