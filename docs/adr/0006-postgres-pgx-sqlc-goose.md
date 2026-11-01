# 0006. Postgres: pgx, sqlc, goose

- Status: Accepted
- Date: 2026-11-01

## Context

Postgres is the primary datasource. Services need type-safe queries, connection pooling, transactions, migrations, health, and telemetry.

## Decision

- **pgx v5** with `pgxpool` as the driver and pool.
- **sqlc** generates type-safe Go code from SQL. The generated code lives in each service.
- **goose v3** runs migrations from an embedded `fs.FS`, with a Postgres advisory lock. Migrations run at startup (opt-in) or by `svclib migrate`.
- The pool connects in the `Initializing` state with a bounded retry. Startup fails if Postgres is not reachable.

## Alternatives

- GORM or ent: ORMs with reflection or large generated code and hidden queries.
- `database/sql` with lib/pq: slower, fewer native types.
- golang-migrate: weaker locking and dirty-state handling. Atlas: heavier, some paid features.

## Consequences

- Queries are plain SQL, reviewed as SQL, with compile-time types.
- The library provides `InTx` with rollback on error and panic, and retry on serialization failure.
- Services need the sqlc tool in their build (the template installs it).
