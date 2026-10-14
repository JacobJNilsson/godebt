# godebt report

example.com/broken, godebt 0.0.0-test.

The index is 100/100, lower is better, scoring 0.2.0, partial.
Contributions: complexity-erosion 60, duplication 40.

| set | files | sloc | funcs | cc p50/p90/max | eroded | clones | dup lines |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| production | 2 | 2 | - | - | - | - | - |
| test | 2 | 5 | - | - | - | - | - |

Other files: unsupported 1.

## Hotspots

### production (-)

### test (-)

## Clones

### production (-)

### test (-)

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

## Incomplete

- bad.go
- bad_test.go
