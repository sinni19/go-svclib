# 0011. Transport: server, plaintext mode, and interceptor chain

- Status: Accepted
- Date: 2026-11-02

## Context

ADR 0002 selects Connect and Vanguard. This ADR records how the App serves them: ports, TLS, timeouts, the interceptor chain, and how services register handlers.

## Decision

- **Ports (D15).** The App port is 8443 (`SVC_ADDR`). The admin port 9090 with `/livez` and `/readyz` comes in phase 3.
- **TLS.** `New` requires `svc.TLS(cfg)` or `svc.Insecure()`. TLS serves HTTP/2 and HTTP/1.1 with TLS 1.2 or later. `Insecure` serves h2c and HTTP/1.1 for local development and tests.
- **Timeouts.** `ReadHeaderTimeout` 5s, `IdleTimeout` 120s, max message 4 MiB. Unary calls get a default deadline of 30s and a maximum of 60s. Streams have no server deadline. The server sets no `ReadTimeout` or `WriteTimeout`, because they end streams.
- **Interceptors.** The App installs a fixed chain on every handler: recover, requestlog, sanitize, deadline, validate (protovalidate). Interceptors 2 (otel), 6 to 8, 10, and 11 come in phases 3 and 5.
- **Handlers.** `Service.Handlers(opts...)` returns the service's handlers. `opts` holds the chain. Connect applies interceptors when a handler is built, so the App adds a probe interceptor and fails `Launch` when a service ignores `opts`.
- **Startup order.** The App collects handlers and binds the port after every `OnInit` and before any `OnStart`, so that a used port fails early. It accepts connections after every `OnStart`.
- **Shutdown.** The server stops first, inside `ShutdownTimeout`. When the timeout ends, the server closes the open connections, which cancels open streams.
- **Errors.** `svcerr` builds errors with a public message; the cause goes to logs only. The sanitize interceptor turns every other server error, and every error received from another server, into `Internal`.
- **Proto.** Buf CLI v1.69.0 (the last release that builds on Go 1.25). `buf lint`, `buf format`, a generated-code check, and `buf breaking` on PRs run in CI. Generated code is committed.

## Alternatives

- **h2c without opt-in.** Simpler, but a service with a missing TLS config serves plaintext in production.
- **Read and write timeouts on the server.** They protect against slow clients, but they end long streams. Deadlines per call do this job instead.
- **Interceptors through `http.Handler` middleware.** Middleware cannot see Connect error codes or stream messages.
- **Trust services to pass `opts`.** One missing argument removes recovery, sanitizing, and validation without any error. The probe makes this a startup failure.

## Consequences

- The newest Connect, Vanguard, and protovalidate need Go 1.26. go-svclib pins Connect v1.21.0, Vanguard v0.4.0, and protovalidate v1.3.0 until it requires Go 1.26.
- Streams have no server-side deadline. A client that does not set one keeps the stream open until shutdown.
- A service cannot remove or reorder the chain. Service interceptors (step 11) come with phase 5.
