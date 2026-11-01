# 0005. Observability: slog and OpenTelemetry

- Status: Accepted
- Date: 2026-11-01

## Context

Every request, query, message, and job must produce correlated logs, traces, and metrics without code in the service.

## Decision

- **Logs:** `log/slog` with a JSON handler. A handler adds `trace_id` and `span_id`. `slog.LevelVar` allows level change at run time through the admin API.
- **Traces and metrics:** OpenTelemetry SDK. Traces export by OTLP. Metrics export by a Prometheus endpoint on the admin port, and optionally by OTLP push.
- Metric names follow OpenTelemetry semantic conventions; labels never contain raw paths or IDs.

## Alternatives

- zap or zerolog: fast, but external APIs; slog is standard.
- `prometheus/client_golang` directly: mature, but a second API next to OTel traces.
- OTLP push only: needs a collector in every environment.

## Consequences

- No logging dependency outside the standard library.
- One telemetry API for all instrumentation (`otelconnect`, `otelpgx`, `redisotel`, `kotel`).
- Telemetry providers are flushed in the `Stopping` state.
