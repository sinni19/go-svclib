GOLANGCI_LINT_VERSION ?= v2.15.0
GOVULNCHECK_VERSION   ?= v1.9.0

.PHONY: check test lint vuln tidy

check: tidy lint test vuln

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

tidy:
	go mod tidy
	git diff --exit-code -- go.mod go.sum
