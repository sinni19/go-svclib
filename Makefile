GOLANGCI_LINT_VERSION ?= v2.15.0
GOVULNCHECK_VERSION   ?= v1.9.0
# buf v1.70+ needs Go 1.25.10 or later.
BUF_VERSION           ?= v1.69.0
BUF                   := go run github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)

.PHONY: check test lint vuln tidy proto proto-lint

check: tidy proto-lint lint test vuln

test:
	go test -race -count=1 ./...

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
