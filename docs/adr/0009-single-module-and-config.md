# 0009. One Go module; options in code with env defaults

- Status: Accepted
- Date: 2026-11-01

## Context

The library has many packages with heavy dependencies (pgx, go-redis, franz-go, OTel). Configuration must be explicit and testable, and also work in containers with env variables.

## Decision

- **One Go module**: `github.com/sinni19/go-svclib`, minimum `go 1.25`.
- **Configuration**: functional options in code. Env variables with the prefix `SVC_` fill defaults only. Precedence: option > env > default. `New` validates all options and returns one joined error.

## Alternatives

- Core module plus a module per datasource: smaller dependency graph for users, more release work.
- Env or file first (zero-config): less code, but config errors are found late.

## Consequences

- One version tag and one changelog.
- Every user downloads all dependencies. Go links only imported packages, so binary size is not affected. `govulncheck` reports only reachable code.
- All config errors are reported at startup, before any I/O.
