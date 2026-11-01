# go-svclib

A Go library for production microservices. A service defines proto contracts and business logic; go-svclib supplies transport, security, observability, datasources, messaging, and lifecycle.

> **Status: pre-alpha.** No public API exists yet. See the [plan](docs/plan.md).

## Technology

| Job | Technology |
|---|---|
| RPC and REST | ConnectRPC + Vanguard (gRPC, gRPC-Web, Connect, REST/JSON from one proto) |
| Logs | `log/slog` |
| Traces and metrics | OpenTelemetry (OTLP; Prometheus endpoint) |
| Postgres | pgx v5, sqlc, goose |
| Redis | go-redis v9 (proposed) |
| Messaging | Kafka with franz-go |
| Security | mTLS and JWT, fail-closed authorization |

## Requirements

- Go 1.25 or later.
- golangci-lint v2 for `make lint`.

## Development

```sh
make test   # go test -race ./...
make lint   # golangci-lint run
make vuln   # govulncheck ./...
make check  # all of the above
```

## Documents

- [Plan](docs/plan.md): goals, architecture, technology, phases, GA checklist.
- [Architecture decision records](docs/adr/).
- [Contributing](CONTRIBUTING.md).

## License

[Apache License 2.0](LICENSE).
