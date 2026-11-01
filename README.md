# go-svclib

A Go library for production microservices. A service defines proto contracts and business logic; go-svclib supplies transport, security, observability, datasources, messaging, and lifecycle.

> **Status: pre-alpha.** The API changes without notice before v1. See the [plan](docs/plan.md).

## Example

```go
type hello struct{ svc.BaseService }

func (hello) Name() string { return "hello" }

func main() {
	app, err := svc.New(
		svc.Name("hello-app"),
		svc.Version("0.1.0"),
		svc.Commit("dev"),
		svc.Services(hello{}),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	svc.Main(app) // blocks until SIGINT or SIGTERM
}
```

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
