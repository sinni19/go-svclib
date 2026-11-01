# 0008. Scheduled jobs: Redis lock with TTL

- Status: Accepted
- Date: 2026-11-01

## Context

Services run periodic jobs. With several replicas, each scheduled run must happen on one replica only.

## Decision

The host scheduler evaluates cron expressions on every replica. For each tick, a replica tries to take a Redis lock `job:<name>:<tick>`:

- `SET NX PX` with a random token, TTL = job timeout.
- The holder renews the lock while the job runs. A failed renewal cancels the job context.
- Release by a compare-and-delete script, so a replica never deletes another replica's lock.

## Alternatives

- Postgres advisory lock: strong consistency, but holds a connection for the job duration.
- No lock: every replica runs every job.

## Consequences

- Jobs need Redis.
- A lease can expire during a network split, so a job can run twice. **Jobs must be idempotent.** The lock token is passed to the job for fencing.
- Running jobs get the remaining shutdown time in `Stopping`; no new runs start.
