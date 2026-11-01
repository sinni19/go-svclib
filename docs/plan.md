# go-svclib: Design and Build Plan

`go-svclib` is a Go library for production microservices. It uses the svclib design style (one service extension interface, functional options, proto-first contracts, a lifecycle state machine, secure defaults) with a modern technology stack. It selects **one technology for each job**.

Background: a study of existing Go microservice libraries, including GoFr (`gofr.dev`).

**Status of each decision:**

- **Confirmed**: the owner selected it.
- **Proposed**: a recommendation that needs the owner's confirmation before the related phase starts. Section 10 lists all of them.

> **Clean-room rule (Confirmed: public GitHub, Apache 2.0).** go-svclib takes design ideas only. Write all code from scratch. Do not copy code, names, or internal details from other libraries.

---

## 1. Goals and non-goals

### Goals

1. A service author writes only proto contracts, SQL, and business logic.
2. One contract serves gRPC, gRPC-Web, Connect, and REST/JSON through one interceptor chain.
3. Secure by default: mTLS and JWT verified in the service, authorization **fail-closed**, error text sanitized.
4. Full observability from the first request: logs, metrics, traces, health, profiles, dashboards.
5. Postgres, Redis, and Kafka with pooling, health, telemetry, and bounded startup.
6. Lifecycle-owned background work: Kafka consumers and scheduled jobs.
7. Resilient outbound calls.
8. A template and CLI that create a new service ready for CI and Kubernetes.
9. Stable public API (v1, semver) with a written compatibility promise.

### Non-goals for GA

- Other datasources or brokers (MySQL, Mongo, NATS, ...).
- A service mesh, an API gateway, or a deployment platform.
- Generation of business logic.

---

## 2. Design principles

| # | Principle |
|---|---|
| P1 | Proto is the contract. Handlers, REST mapping, validation rules, and auth rules come from it. |
| P2 | A service is an **extension** with a lifecycle. The host owns transport and plumbing. |
| P3 | Config by **functional options**; env variables fill defaults only. `New` validates everything. |
| P4 | An explicit **state machine** controls the lifecycle. |
| P5 | **No globals, no `init()`.** Inject all dependencies. |
| P6 | **One interceptor chain** for unary and streaming calls, and for all protocols. |
| P7 | **Fail closed.** If a service does not override `Authorize`, the call is denied. |
| P8 | **Every wait has a deadline.** Every goroutine has an owner and a stop path. |
| P9 | **One technology per job.** No alternative backends in core. |
| P10 | **One Go module.** Go links only imported packages, so binaries stay small. |

---

## 3. Technology selection

| Job | Selection | Status | Reason |
|---|---|---|---|
| Language | Go 1.25 minimum (`go.mod`); support N and N-1 | **Confirmed** | Current stable |
| Contracts | Protobuf + **Buf CLI** (`generate`, `lint`, `breaking`) | Proposed | Lint and breaking-change gate in CI |
| RPC transport | **ConnectRPC** (`connectrpc.com/connect`) | **Confirmed** | gRPC, gRPC-Web, Connect on `net/http`; one interceptor API for unary and stream |
| REST/JSON | **Vanguard** (`connectrpc.com/vanguard`) + `google.api.http` annotations | **Confirmed** | REST on the same handlers and interceptors, no loopback hop. Risk: pre-1.0 (see §9) |
| HTTP server | `net/http`, HTTP/2 with TLS, h2c without TLS | Proposed | Standard library |
| Input validation | **protovalidate** | Proposed | Rules in proto; same for all clients |
| Logging | **`log/slog`** JSON + trace correlation + `slog.LevelVar` | **Confirmed** | Standard library, runtime level |
| Tracing | **OpenTelemetry** SDK, OTLP exporter | **Confirmed** (with metrics) | Vendor neutral |
| Metrics | **OpenTelemetry** SDK; **Prometheus pull** on admin port + optional **OTLP push** | **Confirmed** | One API, both export modes |
| Health | gRPC Health v1 (`connectrpc.com/grpchealth`) + `/livez`, `/readyz` | Proposed | Kubernetes gRPC and HTTP probes |
| Profiling | `net/http/pprof` on admin port | Proposed | Standard library |
| Postgres driver | **pgx v5** + `pgxpool` | **Confirmed** | Native types, context, speed |
| SQL queries | **sqlc** | **Confirmed** | Type-safe code from SQL, no ORM |
| Migrations | **goose v3** (embedded SQL, advisory lock) | **Confirmed** | Library API, embed, lock |
| Redis | **go-redis v9** | Proposed | Official client; single, cluster, sentinel |
| Messaging | **Kafka** with **franz-go** (`kgo`) | **Confirmed** | Modern pure-Go client; consumer groups, idempotent producer |
| Kafka telemetry | franz-go `plugin/kotel` | Proposed | Spans and metrics, trace headers |
| Message encoding | Protobuf (no schema registry at GA) | Proposed | Same contracts as RPC |
| Scheduled jobs | Cron parser **robfig/cron/v3** (parser only) + host scheduler | Proposed | Standard cron syntax; host owns goroutines |
| Job lock | **Redis lock with TTL** (token + renewal + safe release) | **Confirmed** | One run per schedule across replicas |
| Authentication | **mTLS** (service identity) **and JWT/JWKS** (users) | **Confirmed** | Verified in the service |
| JWT libraries | `golang-jwt/jwt/v5` + `MicahParks/keyfunc/v3` | Proposed | `kid` lookup, key rotation |
| Authorization | `Authorize` method on the service, base denies; RBAC helper from proto options | **Confirmed** fail-closed; RBAC helper Proposed | Policy next to contract |
| TLS | `crypto/tls`, cert hot reload | Proposed | Rotation without restart |
| Outbound resilience | **failsafe-go** (retry + backoff + jitter, breaker, timeout, bulkhead) | Proposed | One library for all policies |
| Inbound rate limit | `golang.org/x/time/rate` (local); `go-redis/redis_rate` (distributed) | Proposed | Simple, known |
| IDs | UUIDv7 (`google/uuid`) | Proposed | Time-ordered standard |
| Env defaults | `caarlos0/env/v11` | Proposed | Typed struct |
| CLI | `cmd/svclib` with stdlib `flag` + embedded template | Proposed | No CLI framework dependency |
| Testing | `testing`, **testcontainers-go** (Postgres, Redis, Kafka/Redpanda), `goleak`, fuzzing | Proposed | Real dependencies in tests |
| Lint and security | golangci-lint v2, govulncheck, gosec, CodeQL, `buf lint` | Proposed | — |
| API compatibility | `gorelease` (apidiff) | Proposed | Blocks breaking changes after v1 |
| Release | GoReleaser, Syft SBOM, Cosign | Proposed | Supply-chain evidence |
| Dependency updates | Renovate | Proposed | — |

---

## 4. Architecture

### 4.1 Process view

```
                       ┌──────────────────────────── go-svclib process ───────────────────────────┐
 clients ──TLS/mTLS──► │ App listener :8443                                                       │
 (gRPC, gRPC-Web,      │  http.Server (h2) ─► Vanguard (REST→RPC) ─► Connect handlers              │
  Connect, REST)       │                      one interceptor chain (unary + stream)               │
                       │                                   │                                       │
 Kafka ◄──────────────►│ Consumers (franz-go) ─► consumer chain ─┐                                 │
                       │ Scheduler (cron) ─ Redis lock ─────────►├─► Services ─► Deps              │
                       │                                         │     (pgxpool, redis, kafka      │
                       │                                         │      producer, clients)         │
 Prometheus, kubelet ─►│ Admin listener :9090 (internal only)                                     │
                       │  /metrics /livez /readyz /debug/pprof /admin/loglevel (admin principal)  │
                       │ Telemetry: slog → stdout · OTel traces/metrics → OTLP / Prometheus        │
                       └──────────────────────────────────────────────────────────────────────────┘
```

Two listeners. Operator endpoints are never on the app port.

### 4.2 RPC interceptor chain (fixed order; unary and streaming)

| Order | Interceptor | Action | Failure code |
|---|---|---|---|
| 1 | `recover` | Catches panics, logs the stack | `Internal` |
| 2 | `otel` | Span + RPC metrics | — |
| 3 | `requestlog` | One access log line: trace ID, principal, procedure, code, latency | — |
| 4 | `sanitize` | Logs the raw error; a non-`connect.Error` becomes `Internal "internal error"` | — |
| 5 | `deadline` | Default and maximum deadline | `DeadlineExceeded` |
| 6 | `ratelimit` | Per procedure and per principal | `ResourceExhausted` |
| 7 | `readiness` | Rejects while not ready or draining | `Unavailable` |
| 8 | `authn` | mTLS peer identity and/or JWT → `Principal` in context | `Unauthenticated` |
| 9 | `validate` | protovalidate on each request message | `InvalidArgument` |
| 10 | `authz` | Calls `Service.Authorize`; base implementation denies | `PermissionDenied` |
| 11 | service interceptors | Appended by the service | — |
| 12 | handler | Business logic | — |

- Services can append interceptors, but cannot reorder or remove steps 1–10.
- `sanitize` wraps steps 5–12, so every error from them is sanitized.
- Public procedures (health, reflection) are marked by the proto option `(svclib.v1.auth).public = true`, not by a path prefix.

### 4.3 Lifecycle state machine

```
 New() ── validate options, build objects (no I/O) ──────────────────────► Created
 Run(ctx)
   Created ─► Initializing  connect Postgres, Redis, Kafka (bounded retry);
                            migrations (opt-in); Service.OnInit in order
           ─► Starting      bind listeners (fail fast); Service.OnStart in order;
                            start consumers and scheduler
           ─► Running       readiness = true
   signal / ctx done / fatal error
           ─► Draining      readiness = false; wait DrainDelay (default 5s)
           ─► Stopping      stop scheduler (wait running jobs); stop consumers (commit, leave group);
                            http.Server.Shutdown; Service.OnStop in reverse order;
                            flush producer; close pools; flush telemetry   (all under ShutdownTimeout)
           ─► Stopped       Run returns
```

| Rule | Value |
|---|---|
| Startup deadline | `StartupTimeout`, default 30s |
| Shutdown deadline | `ShutdownTimeout`, default 25s (less than `terminationGracePeriodSeconds`) |
| Exit | `Run` returns an error; `svc.Main` maps it to exit code 1. No `os.Exit` in library code. |
| Re-run | `ErrAlreadyRun` |

### 4.4 Service extension API (Confirmed: one interface + base struct)

```go
// Service is implemented by every service that the host runs.
// Embed svc.BaseService and override only what you need.
type Service interface {
	Name() string

	OnInit(ctx context.Context, d *Deps) error
	OnStart(ctx context.Context) error
	OnStop(ctx context.Context) error

	Ready(ctx context.Context) error
	Live(ctx context.Context) error

	Handlers(opts ...connect.HandlerOption) []Handler
	Authorize(ctx context.Context, p auth.Principal, procedure string, req any) error

	Consumers() []kafka.Consumer
	Jobs() []schedule.Job
}

// BaseService gives safe defaults. Authorize denies every call.
type BaseService struct{}
```

| Method | `BaseService` default |
|---|---|
| `OnInit`, `OnStart`, `OnStop` | no-op |
| `Ready`, `Live` | `nil` (healthy) |
| `Handlers`, `Consumers`, `Jobs` | none |
| `Authorize` | **`PermissionDenied`** (fail-closed) |

**Risk of the base pattern:** an override with a wrong signature is ignored without an error. Mitigations:

1. The host logs, for each service at startup, which methods it overrides.
2. `svctest.AssertOverrides(t, svc, "Authorize", ...)` checks this in tests.
3. A denied call is visible at once, because the default is to deny.

`Deps` is a plain struct: logger, meter, tracer, `*pgxpool.Pool`, `redis.UniversalClient`, Kafka producer, outbound client factory. Fields are nil only when the app did not enable that datasource.

### 4.5 Usage

```go
func main() {
	app, err := svc.New(
		svc.Name("orders"),
		svc.Version(version, commit),
		svc.TLS(tlsx.FromFiles(cert, key, ca), tlsx.RequireClientCert()),
		svc.JWT(auth.JWKS("https://idp.example.com/.well-known/jwks.json"), auth.Audience("orders")),
		svc.Postgres(postgres.FromEnv(), postgres.Migrations(migrationsFS)),
		svc.Redis(redis.FromEnv()),
		svc.Kafka(kafka.FromEnv()),
		svc.Services(orders.New()),
	)
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	svc.Main(app)
}
```

### 4.6 Repository layout (Confirmed: one module)

```
go-svclib/
  go.mod                    github.com/sinni19/go-svclib
  svc/                      App, options, lifecycle, Service, BaseService, Deps, Main
  config/                   env defaults, validation
  log/                      slog setup, trace handler, LevelVar
  telemetry/                OTel trace and metric providers, exporters
  transport/                Connect + Vanguard, http.Server, TLS/h2c
  interceptor/              recover, otel, requestlog, sanitize, deadline, ratelimit,
                            readiness, authn, validate, authz
  health/                   checks, gRPC health, /livez /readyz
  admin/                    admin server: metrics, health, pprof, log level
  auth/                     Principal, mTLS identity, JWT/JWKS, RBAC helper
  tlsx/                     cert loading, hot reload, secure defaults
  svcerr/                   public vs internal errors, google.rpc details
  postgres/                 pgxpool, InTx, goose, health, telemetry
  redis/                    client, health, telemetry, lock, distributed rate limit
  kafka/                    producer, consumer group runner, retry + DLQ, telemetry
  schedule/                 cron parsing, scheduler, Redis lock integration
  client/                   outbound Connect/HTTP clients, failsafe policies
  svctest/                  testcontainers helpers, in-process server, fake principal
  proto/svclib/v1/          options.proto (auth, rbac, ratelimit, idempotency)
  cmd/svclib/               CLI: new, migrate, version
  template/                 embedded service template
  examples/orders/          reference service (RPC + Postgres + Redis + Kafka + job)
  docs/                     guides, config reference, ADRs, runbooks
```

---

## 5. Component design

### 5.1 Errors (`svcerr`)

- Handlers return `connect.Error` built with helpers: `svcerr.NotFound("order", id)`, `svcerr.Invalid(field, reason)`.
- `svcerr.Wrap(code, publicMsg, cause)` keeps the cause for logs and spans only.
- Details use `google.rpc` types (`BadRequest`, `ErrorInfo`, `RetryInfo`).

### 5.2 Configuration (Confirmed: options in code + env defaults)

- Precedence: option in code > env variable > default.
- One env prefix (`SVC_`), typed struct, generated reference table in docs.
- `New` returns one joined error with every invalid field.
- Secrets use a redacting type (`slog.LogValuer`) and are never logged.

### 5.3 Observability

| Signal | Contract |
|---|---|
| Logs | JSON: `time`, `level`, `msg`, `service`, `version`, `trace_id`, `span_id`, `procedure`, `principal`; level change through the admin API |
| Traces | Spans for RPC, SQL, Redis, Kafka produce/consume, jobs, outbound calls; W3C propagation (also in Kafka headers); parent-based ratio sampler |
| Metrics | OTel semantic conventions: `rpc.server.duration`, `db.client.*`, pool gauges, Kafka lag and handler duration, job runs and failures, `process.*`, `go.*`, build info |
| Health | `/livez` (no dependency checks), `/readyz` (dependencies + `Service.Ready` + not draining), gRPC Health v1 |
| Shipped assets | Grafana dashboards, Prometheus alert rules, runbooks in `template/observability/` |

Metric labels use the procedure, topic, or job name, never raw paths or IDs.

### 5.4 Postgres

- `pgxpool` with explicit limits; connect in **Initializing** with bounded retry; startup fails if unreachable.
- `postgres.InTx(ctx, pool, fn)`: commit; rollback on error and panic; retry on serialization failure.
- goose with embedded `fs.FS` and advisory lock; run at startup (opt-in) or by `svclib migrate`.
- sqlc config in the template; generated code lives in the service.

### 5.5 Redis

- One options struct for single, cluster, sentinel; TLS.
- `PING` readiness check; telemetry hooks.
- `redis.Lock`: `SET NX PX` with a random token, renewal while held, release by compare-and-delete script.
- Distributed rate limit for the `ratelimit` interceptor.

### 5.6 Kafka (Confirmed: franz-go)

| Area | Design (Proposed details) |
|---|---|
| Producer | Idempotent, `acks=all`, protobuf value, trace headers; `Produce` waits for ack with ctx deadline |
| Consumer | Consumer group per `kafka.Consumer{Topic, Group, Handler}`; handler gets ctx with span and decoded message |
| Commit | After handler success only (at-least-once); handlers must be idempotent |
| Errors | Retry with backoff N times, then send to `<topic>.dlq` with error headers; then commit |
| Concurrency | One goroutine per partition; ordering kept per partition |
| Consumer chain | recover → otel → log → sanitize-for-log → handler (same rules as RPC) |
| Rebalance | Commit on revoke; stop fetching in **Draining** |
| Health | Ready when the client can reach brokers; lag as a metric, not a readiness signal |

### 5.7 Scheduled jobs (Confirmed: Redis lock with TTL)

- `schedule.Job{Name, Spec, Timeout, Run func(ctx) error}`; `Spec` is a 5- or 6-field cron expression. A parse error fails `New`.
- At each tick, a replica tries `redis.Lock("job:<name>:<tick>")` with TTL = `Timeout`. Only the lock holder runs the job.
- The lock is renewed while the job runs. If renewal fails, the job context is canceled.
- **Contract:** jobs must be idempotent. A lease can expire on a network split.
- Each run has a span, a log line, and duration and failure metrics. Panics are recovered.
- In **Stopping**, no new runs start; running jobs get the remaining shutdown time.

### 5.8 Security (Confirmed: mTLS + JWT, fail-closed)

| Control | Design |
|---|---|
| Transport | TLS 1.2 minimum (1.3 preferred); mTLS; certs reloaded on file change |
| Service identity | mTLS peer → `Principal{Kind: Service, ID: SPIFFE ID or CN}` |
| User identity | JWT: JWKS refresh with backoff, `kid` lookup, `iss`/`aud`/`exp`/`nbf`, clock skew limit → `Principal{Kind: User, ...}` |
| Authorization | `Service.Authorize`; base denies; RBAC helper reads roles from proto method options |
| Input | protovalidate; message size limits; read, header, and idle timeouts |
| Output | `sanitize`; no stack traces, SQL, or broker errors in responses |
| Admin | Separate port; log-level change needs an admin principal |
| Supply chain | govulncheck, SBOM, signed releases, pinned tools |

### 5.9 Outbound clients

- `client.Connect[T](name, url, opts...)` and `client.HTTP(name, url, opts...)`.
- Always: OTel, deadline propagation, mTLS, `Principal` forwarding (optional).
- Policies: retry only for procedures with `idempotency_level = NO_SIDE_EFFECTS` or `IDEMPOTENT`, with backoff and jitter; circuit breaker; per-call timeout; bulkhead.
- Breaker state is a metric. Background goroutines stop in **Stopping**.

### 5.10 Template and CLI

- `svclib new <name> [--postgres] [--redis] [--kafka]` writes a service from the embedded template.
- The template contains: `buf.yaml` and `buf.gen.yaml`, proto with REST annotations, `sqlc.yaml`, migrations, `main.go`, Makefile, Dockerfile (distroless, non-root), Kubernetes manifests (probes, resources, PDB), CI workflow, dashboards and alerts.
- `svclib migrate up|down|status` runs goose with the service's embedded migrations.

---

## 6. Build plan (Confirmed: 2–3 engineers, about 7 months)

| Phase | Weeks | Scope | Exit criteria |
|---|---|---|---|
| **0. Foundations** | 1 | Repo, license, CI (lint, race, govulncheck, buf), ADRs for confirmed decisions | CI green; ADRs merged |
| **1. Core host** | 2 | `svc` options, validation, state machine, `Service` + `BaseService`, `Main`, `log`, `config` | Lifecycle tests: order, deadlines, re-run, goleak clean |
| **2. Transport** | 3 | Connect + Vanguard, TLS/h2c, timeouts, interceptors 1–5, 9, `svcerr`, `examples/orders` skeleton | Same RPC over gRPC, Connect, REST; unary and stream through all interceptors |
| **3. Observability** | 2 | `telemetry`, admin server, health, gRPC health, pprof, log-level API | Traces in local collector; metrics scraped; probes pass in kind |
| **4. Postgres + Redis** | 3 | `postgres` (pool, InTx, goose, health, otel), `redis` (client, health, otel, lock), `svctest` | Integration tests with containers; fail-fast startup |
| **5. Security** | 2 | mTLS + reload, JWT/JWKS, `Principal`, authn/authz interceptors, RBAC helper, rate limit, admin authz | Security suite: no token, expired, wrong audience, bad cert, wrong role |
| **6. Kafka** | 3 | Producer, consumer runner, retry + DLQ, rebalance, consumer chain, telemetry | Integration tests: at-least-once, DLQ, rebalance, drain without loss |
| **7. Scheduled jobs** | 2 | Cron parsing, scheduler, Redis lock with renewal, telemetry | Multi-replica test: one run per tick; lease-loss cancels the job |
| **8. Outbound clients** | 2 | `client` with failsafe policies, OTel, deadline propagation | Fault-injection: retry only idempotent; breaker opens and recovers |
| **9. Template + CLI** | 2 | `cmd/svclib`, template, dashboards, alerts, k8s manifests | New service passes CI and deploys to kind in < 15 min |
| **10. Hardening** | 3 | Fuzzing, load (ghz, k6), 24h soak, chaos (kill DB, Redis, broker), docs site | Performance budget met; no leaks; docs complete |
| **11. Beta → GA** | 4 | Two pilot services in staging, then production; API freeze; external security review | GA checklist (§7) complete |

**Total: about 29 weeks (about 7 months).** Phases 6–8 can run in parallel with a third engineer after phase 5, which saves about 3 weeks.

### Testing strategy

| Level | Tooling | Gate |
|---|---|
| Unit | `go test -race`, table tests | ≥ 85% coverage on core packages |
| Leak | `goleak.VerifyTestMain` in each package | Must pass |
| Integration | testcontainers-go: Postgres, Redis, Kafka (Redpanda), OTel collector | Must pass in CI |
| Contract | `buf breaking` | Must pass |
| Fuzz | Decoders, error mapping, RBAC matcher, cron parser | Nightly |
| Load | ghz, k6 | Framework overhead budget, set in phase 2 |
| API compatibility | gorelease | No breaking change after v1.0.0 |

---

## 7. GA checklist

- [ ] Public API frozen; compatibility promise published.
- [ ] All interceptors run for unary and streaming, for every protocol (test matrix).
- [ ] `BaseService.Authorize` denies; admin endpoints need an admin principal on the admin port.
- [ ] No raw error text reaches clients (tests + fuzzing).
- [ ] Kafka: at-least-once verified; DLQ works; drain loses no committed work.
- [ ] Jobs: one run per tick across replicas; lease loss cancels the run.
- [ ] Every goroutine has an owner; goleak clean; 24h soak stable.
- [ ] Startup and shutdown deadlines enforced; drain verified under load in Kubernetes.
- [ ] Outage behavior of Postgres, Redis, Kafka documented and tested.
- [ ] Dashboards, alerts, runbooks shipped.
- [ ] govulncheck clean; SBOM and signatures; external security review closed.
- [ ] Docs: quick start, concepts, packages, config reference, ADRs.
- [ ] Template service deploys in < 15 minutes.
- [ ] Two pilots in production for 4 weeks without a framework-caused incident.

---

## 8. Common failure modes, and the go-svclib fix

| Failure mode | Fix |
|---|---|
| Streams skip interceptors | Connect interceptors wrap unary and stream (§4.2) |
| Raw errors from early interceptors | `sanitize` wraps steps 5–12 |
| Config endpoint without authz | Admin port + admin principal |
| Fail-open authz default | `BaseService.Authorize` denies |
| Shutdown without deadline | `ShutdownTimeout` + drain |
| Stats race, latency gauges | OTel histograms |
| Two hops for REST | Vanguard in process |
| Interceptors frozen after first register | Chain built once in `Starting`; no registration after `New` |
| Silent nil datasource | Fail fast in `Initializing` |
| Error text sent to client | `sanitize` |
| Uncommitted message skipped on error (Kafka) | Retry → DLQ → commit |
| Cron `*/0` hang | Parser rejects; fuzzed |
| Goroutines never stop | Lifecycle-owned + goleak |
| Globals | None (P5) |

---

## 9. Risks

| Risk | Impact | Mitigation |
|---|---|
| Vanguard is pre-1.0 | API change or bug in REST path | Pin version; wrap behind `transport`; contract tests for REST; fallback plan: grpc-gateway v2 with loopback |
| Base-struct pattern hides wrong overrides | Methods silently not called | Startup override log; `svctest.AssertOverrides`; fail-closed authz |
| One module pulls all dependencies | Larger `go.sum`, more CVE alerts for unused code | govulncheck reports only reachable code; accepted (Confirmed) |
| Redis lock lease expiry | Job runs twice | Idempotency contract; renewal; fencing token passed to job |
| Kafka at-least-once | Duplicate processing | Idempotency contract; message ID in headers; dedupe helper (post-GA) |
| Scope for 7 months | Schedule slip | Phase gates; parallel phases 6–8; cut list: dedupe helper, OTLP push, CLI `migrate` |

---

## 10. Open decisions (need owner confirmation)

| ID | Decision | Proposal | Needed before |
|---|---|---|---|
| D6 | Minimum Kubernetes version (Go 1.25 is Confirmed) | Kubernetes 1.30+ | Phase 9 |
| D7 | Redis client | go-redis v9 | Phase 4 |
| D8 | JWT libraries | golang-jwt v5 + keyfunc v3 | Phase 5 |
| D9 | Inbound rate limiting in GA | Yes: local + Redis | Phase 5 |
| D10 | Kafka message encoding and schema registry | Protobuf, no registry at GA | Phase 6 |
| D11 | Kafka delivery and error policy | At-least-once, retry N, DLQ topic | Phase 6 |
| D12 | Cron parser | robfig/cron/v3 parser only | Phase 7 |
| D13 | Outbound resilience library | failsafe-go | Phase 8 |
| D14 | CLI framework | stdlib `flag` | Phase 9 |
| D15 | Ports and probe paths | App 8443, admin 9090, `/livez`, `/readyz` | Phase 2 |
| D16 | Test broker image | Redpanda (Kafka API compatible, fast start) | Phase 6 |
