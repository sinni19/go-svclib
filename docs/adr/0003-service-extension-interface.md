# 0003. Service extension: one interface and a base struct

- Status: Accepted
- Date: 2026-11-01

## Context

The host runs one or more services. Each service takes part in the lifecycle (init, start, stop), health, request handling, authorization, Kafka consumption, and scheduled jobs.

## Decision

Define one `svc.Service` interface with all extension methods. Provide `svc.BaseService` with safe defaults. A service embeds `BaseService` and overrides only what it needs. `BaseService.Authorize` denies every call.

## Alternatives

- **Small optional interfaces** detected by type assertion. Idiomatic, and a missing method is explicit. Rejected in favor of one discoverable contract.
- **Register on the App** (no service type). Flexible, but no single place that shows what a service provides.

## Consequences

- One contract, easy to discover in docs and IDEs.
- Risk: an override with a wrong signature is silently not called. Mitigations: the host logs the overridden methods at startup; `svctest.AssertOverrides` checks them in tests; authorization fails closed, so a missing `Authorize` is visible at once.
- Adding a method to `Service` after v1 is a breaking change for types that do not embed `BaseService`. The docs require embedding.
