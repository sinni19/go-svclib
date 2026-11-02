# Contributing

## Rules

- Write all code from scratch. Do not copy code from other libraries unless the license allows it and the source is credited.
- No globals and no `init()`. Inject all dependencies.
- `context.Context` is the first parameter of any function that does I/O or waits.
- Every goroutine has an owner and a stop path. Tests use `goleak`.
- Every wait has a deadline.
- Exported identifiers have a doc comment that follows [Go doc comments](https://go.dev/doc/comment).
- New code has tests. Prefer table-driven tests.
- Coverage is at least 85% for each package and for the total. `make cover` and CI check it. Generated code in `internal/gen` is excluded.
- An architecture change needs an ADR in `docs/adr/`.

## Commits

```
<type>(<scope>): <short summary>

<optional body: what changes and why>

<optional footer>
```

`<type>` is one of `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`. `<scope>` is the package or area, for example `svc` or `config`.

## Pull requests

- `.github/CODEOWNERS` lists the required reviewers.
- Branch names start with `sinni19/`, for example `sinni19/fix-foo`.
- Target branch: `main`.
- CI must pass: lint, race tests, coverage gate, proto checks, govulncheck.
