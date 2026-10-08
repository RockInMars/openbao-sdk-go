SHELL := /bin/bash
export GOWORK := off
export GOFLAGS :=
.PHONY: test race vet build verify normal-test dependency-check dependency-export coverage-check contract-test integration-test consumer-test security-test tooling-test fuzz-test release-check

test:
	go test -count=1 ./...
race:
	go test -race -count=1 -coverprofile=coverage.out ./...
vet:
	go vet ./...
build:
	go build ./...
verify:
	go mod verify
dependency-check:
	python3 scripts/dependency-check.py --prepare
dependency-export:
	python3 scripts/dependency-bundle.py export --output .artifacts/openbao-public-dependencies.zip
normal-test:
	python3 scripts/normal-test.py
coverage-check:
	python3 scripts/coverage-check.py
# Supplemental only: excludes the real official-client factory; NOT a release gate.
contract-test:
	python3 scripts/contract-test.py
integration-test:
	python3 scripts/integration-test.py
consumer-test:
	python3 scripts/consumer-test.py
security-test:
	python3 scripts/security-test.py
tooling-test:
	python3 -m unittest discover -s scripts/tests -v
fuzz-test:
	python3 scripts/fuzz-test.py
release-check:
	python3 scripts/verify-release.py
