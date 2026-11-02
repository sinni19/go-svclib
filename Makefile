GOLANGCI_LINT_VERSION ?= v2.15.0
GOVULNCHECK_VERSION   ?= v1.9.0
# buf v1.70+ needs Go 1.25.10 or later.
BUF_VERSION           ?= v1.69.0
BUF                   := go run github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)

# Minimum statement coverage for each package and the total.
COVERAGE_MIN          ?= 85
COVERAGE_EXCLUDE      := /internal/gen/

.PHONY: check test cover lint vuln tidy proto proto-lint

check: tidy proto-lint lint cover vuln

test:
	go test -race -count=1 ./...

# cover runs the tests with coverage of every package by every test, then
# fails when a package or the total is below COVERAGE_MIN.
cover:
	go test -race -count=1 -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./...
	go run ./internal/tools/covercheck -min $(COVERAGE_MIN) -exclude $(COVERAGE_EXCLUDE) coverage.out

lint:
	golangci-lint run

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

tidy:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

# proto regenerates internal/gen from proto/.
proto:
	$(BUF) format -w
	$(BUF) generate

# proto-lint checks proto style and that the committed code is current.
proto-lint:
	$(BUF) lint
	$(BUF) format -d --exit-code
	$(BUF) generate
	git diff --exit-code -- internal/gen
