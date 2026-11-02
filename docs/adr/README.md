# Architecture Decision Records

Each ADR records one decision: context, decision, and consequences. An ADR is not edited after it is accepted; a new ADR supersedes it.

| ADR | Title | Status |
|---|---|---|
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0002](0002-transport-connect-vanguard.md) | Transport: ConnectRPC and Vanguard | Accepted |
| [0003](0003-service-extension-interface.md) | Service extension: one interface and a base struct | Accepted |
| [0004](0004-security-mtls-jwt-fail-closed.md) | Security: mTLS and JWT, fail-closed authorization | Accepted |
| [0005](0005-observability-slog-otel.md) | Observability: slog and OpenTelemetry | Accepted |
| [0006](0006-postgres-pgx-sqlc-goose.md) | Postgres: pgx, sqlc, goose | Accepted |
| [0007](0007-messaging-kafka-franz-go.md) | Messaging: Kafka with franz-go | Accepted |
| [0008](0008-scheduled-jobs-redis-lock.md) | Scheduled jobs: Redis lock with TTL | Accepted |
| [0009](0009-single-module-and-config.md) | One Go module; options in code with env defaults | Accepted |
| [0010](0010-lifecycle-launch-shutdown.md) | Lifecycle: Launch, Shutdown, and rollback | Accepted |
| [0011](0011-transport-server-and-interceptors.md) | Transport: server, plaintext mode, and interceptor chain | Accepted |

Template: [template.md](template.md).
