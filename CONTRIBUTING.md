# Contributing

## Rules

- Write all code from scratch. Do not copy code from other libraries unless the license allows it and the source is credited.
- No globals and no `init()`. Inject all dependencies.
- `context.Context` is the first parameter of any function that does I/O or waits.
- Every goroutine has an owner and a stop path. Tests use `goleak`.
- Every wait has a deadline.
- Exported identifiers have a doc comment that follows [Go doc comments](https://go.dev/doc/comment).
- New code has tests. Prefer table-driven tests.
- An architecture change needs an ADR in `docs/adr/`.

## Commits

```
Title in imperative mood, max 72 characters

Description: what changes and why.
```

## Pull requests

- Target branch: `main`.
- CI must pass: lint, race tests, govulncheck.
