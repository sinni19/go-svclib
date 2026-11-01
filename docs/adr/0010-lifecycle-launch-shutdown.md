# 0010. Lifecycle: Launch, Shutdown, and rollback

- Status: Accepted
- Date: 2026-11-01

## Context

The App runs several services and later owns listeners, pools, consumers, and jobs. The start and stop API must work from `main`, from tests, and under Kubernetes.

## Decision

- `Launch()` blocks until the App stops. It runs once; a second call returns `ErrAlreadyLaunched`.
- `Shutdown(ctx)` requests a stop and waits until `Launch` returns or `ctx` ends. The first call's `ctx` also limits the drain and `OnStop`.
- `Launch` stops on SIGINT and SIGTERM. `HandleSignals(false)` turns this off.
- `Status()` returns the current state: Created, Initializing, Starting, Running, Draining, Stopping, Stopped.
- Every service whose `OnInit` returned nil gets `OnStop` in reverse order, also when a later service fails. `OnStop` must work without `OnStart`.
- A stop request during startup is not an error. A hook error, panic, or timeout is.
- `New` requires `Name`, `Version` (semver), and `Commit`, and validates all options before any I/O.
- Defaults: startup 30s, drain 5s, shutdown 25s.

## Alternatives

- `Run(ctx) error` with stop by context cancel: more idiomatic Go, but no way to wait for the stop result from another goroutine.
- Re-launch from Stopped: needs every component to support restart.
- `OnStop` only after `OnStart`: leaks resources acquired in `OnInit`.

## Consequences

- `svc.Main(app)` is the only `os.Exit`.
- Hooks run synchronously. A hook that ignores its context delays `Launch` past the configured timeout; the timeout is still reported as an error when the hook returns.
