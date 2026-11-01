# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

go-svclib is a Go library (module `github.com/sinni19/go-svclib`, Go 1.25+) for production microservices. A service supplies proto contracts and business logic. The library supplies transport, security, observability, datasources, messaging, and lifecycle. Status: pre-alpha; the API can change.

The library is built phase by phase. `docs/plan.md` holds the architecture, technology choices, phases, GA checklist, and open decisions (section 10). `docs/adr/` holds the decisions. Read the plan section for a phase before you write code for it. Ask the owner about open decisions; do not assume.

## Commands

```sh
make test    # go test -race -count=1 ./...
make lint    # golangci-lint run (v2.15.0)
make vuln    # govulncheck
make tidy    # go mod tidy; fails if go.mod or go.sum change
make check   # tidy + lint + test + vuln

go test -race -count=1 -run 'TestName' ./svc   # one test
```

Run only targeted tests locally. CI (`.github/workflows/ci.yml`) runs the full suite on Go 1.25 and 1.26, plus `go vet`, lint, and govulncheck.

## Architecture

Packages that exist: `svc` (App, options, lifecycle, `Service`), `config` (env defaults), `log` (slog setup). The target layout is in `docs/plan.md` section 4.6.

- **App** (`svc/app.go`): `svc.New(opts...)` validates options and builds objects with no I/O. `Launch()` blocks and is one-shot (`ErrAlreadyLaunched`). `Shutdown(ctx)` requests a stop and waits. `svc.Main(app)` is the only `os.Exit` in the library.
- **Lifecycle**: Created → Initializing (`OnInit` in order) → Starting (`OnStart` in order) → Running → Draining (`DrainDelay`) → Stopping (`OnStop` in reverse order) → Stopped. Every service whose `OnInit` succeeded gets `OnStop`, also on startup failure. See ADR 0010.
- **Hook deadlines** (`App.call`): each hook runs in its own goroutine. At the phase deadline (`StartupTimeout` or `ShutdownTimeout`) the App abandons the hook and returns a timeout error. A stop hook that starts after the deadline gets a fresh `HookGracePeriod`. A hook that returns nil after its deadline still fails. A panic becomes an error; the stack goes to the log only.
- **Service extension** (`svc/service.go`): one `Service` interface plus `BaseService` with safe defaults. The interface grows by phase (`Handlers`/`Authorize` in phases 2 and 5, `Consumers` in phase 6, `Jobs` in phase 7). Add each new method to `BaseService` too, so that services that embed it do not break. `Authorize` must deny by default (fail-closed).
- **Configuration**: options in code take precedence over env defaults. `config.Env` reads env keys with the prefix `SVC_` (for example `SVC_SHUTDOWN_TIMEOUT`). A new setting needs an `Option` in `svc/options.go`, a field in `config.Env`, and a validation in `svc.validate`.
- **Planned transport**: ConnectRPC + Vanguard on one app listener, a separate admin listener for metrics, probes, and pprof, and a fixed interceptor chain that services can extend but not reorder (`docs/plan.md` sections 4.1–4.2).

## Rules (from CONTRIBUTING.md and .golangci.yml)

- No globals and no `init()` (linters `gochecknoglobals`, `gochecknoinits`). Inject all dependencies.
- `context.Context` is the first parameter of any function that does I/O or waits. Every wait has a deadline.
- Every goroutine has an owner and a stop path. Tests use `goleak`; a test that abandons a goroutine must release it.
- Wrap errors from other packages (`wrapcheck`). Use `errors.Is`/`As` (`errorlint`).
- Exported identifiers have a doc comment that follows https://go.dev/doc/comment.
- Prefer table-driven tests. New code has tests.
- An architecture change needs an ADR in `docs/adr/` (template: `docs/adr/template.md`). Update the plan when a decision changes.
- Write all code from scratch. Do not copy from gofr, svclib, or other libraries unless the license allows it and the source is credited.
- Commit message: `<type>(<scope>): <short summary>`, with an optional body and footer. Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`. Example: `feat(svc): add hook grace period`.
- Branch names start with `sinni19/`. PRs target `main`.
