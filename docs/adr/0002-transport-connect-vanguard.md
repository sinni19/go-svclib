# 0002. Transport: ConnectRPC and Vanguard

- Status: Accepted
- Date: 2026-11-01

## Context

Services expose one API to gRPC clients, browsers, and REST/JSON clients. All calls must pass the same security, validation, and telemetry, for unary and streaming RPCs.

## Decision

Use **ConnectRPC** (`connectrpc.com/connect`) for handlers and **Vanguard** (`connectrpc.com/vanguard`) for REST/JSON from `google.api.http` annotations. Both run on `net/http` in one process, behind one interceptor chain.

## Alternatives

- **grpc-go + grpc-gateway v2.** Widely used. REST passes through a second, loopback gRPC hop, or bypasses gRPC interceptors in in-process mode. Two interceptor stacks.
- **grpc-go + hand-written REST.** Two handler sets for each API.

## Consequences

- One handler serves gRPC, gRPC-Web, Connect, and REST. No loopback hop.
- Connect interceptors cover unary and streaming with one API.
- Risk: Vanguard is pre-1.0. Mitigation: pin the version, keep it behind the `transport` package, run REST contract tests. Fallback: grpc-gateway v2 with a loopback connection.
