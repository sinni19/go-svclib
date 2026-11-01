# 0007. Messaging: Kafka with franz-go

- Status: Accepted
- Date: 2026-11-01

## Context

Services publish and consume events. Consumers must follow the same lifecycle, telemetry, and error rules as RPC handlers.

## Decision

Use **Kafka** with the **franz-go** (`github.com/twmb/franz-go/pkg/kgo`) client. The library provides a producer and a consumer-group runner owned by the lifecycle.

The delivery policy, encoding, and DLQ details are open decisions D10 and D11 in `docs/plan.md`.

## Alternatives

- NATS JetStream: lighter to operate, smaller ecosystem.
- Redis Streams: no new technology, fewer guarantees.
- confluent-kafka-go (cgo) or sarama: cgo dependency or older API.

## Consequences

- Pure Go client, no cgo.
- Consumers stop fetching in `Draining` and commit on partition revoke.
- Tests need a Kafka-compatible broker container.
