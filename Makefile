# The anti-slop-go package patterns. The default covers the whole module.
ANTISLOP_PKGS ?= ./...

# The statement coverage bar in percent. The scripts read it from the environment.
COVERAGE_MIN ?= 90
export COVERAGE_MIN

.PHONY: build test gate-test lint lint-fast vet tidy tidy-check antislop check check-scoped setup clean golden selfcheck baseline corpus

build:
	go build ./...

test:
	go test -race -coverprofile=coverage.out ./...
	sh scripts/coverage-gate.sh coverage.out

# Only packages with golden files define the -update flag.
golden:
	go test ./internal/inventory ./internal/report ./internal/safeguards ./cmd/godebt -update

# The corpus needs the network, so no other target runs it.
corpus:
	go run ./internal/corpus -list corpus/modules.txt -out corpus/results.md

gate-test:
	sh scripts/coverage-gate-test.sh

lint:
	golangci-lint run ./...

lint-fast:
	golangci-lint run --fast-only ./...

vet:
	go vet ./...

tidy:
	go mod tidy

# The hooks run after staging, so this target reports a needed change and never rewrites go.mod.
tidy-check:
	go mod tidy -diff

# anti-slop-go v1.4.1 requires Go 1.26 or newer. GOTOOLCHAIN=auto downloads a newer toolchain.
# GOTOOLCHAIN=local on a host older than Go 1.26 fails.
antislop:
	go run github.com/JacobJNilsson/anti-slop-go/cmd/antislop@v1.4.1 $(ANTISLOP_PKGS)

# The self-audit fails the gate when godebt cannot audit its own repository or when the policy in godebt.yaml fails.
# The sed line prints the index, because the score block holds the only "index" key at four spaces of indent.
selfcheck:
	go run ./cmd/godebt audit . --out .godebt-self.json
	@sed -nE 's/^    "index": ([0-9]+),$$/self-audit index \1 (.godebt-self.json)/p' .godebt-self.json
	go run ./cmd/godebt baseline --check .

# Regenerate the committed baseline after a change that alters a measurement.
baseline:
	go run ./cmd/godebt baseline .

check: tidy-check vet lint gate-test test build antislop selfcheck

check-scoped:
	sh scripts/precommit-check.sh

setup:
	git config core.hooksPath .githooks

clean:
	rm -f coverage.out coverage.scoped.out .godebt-self.json
