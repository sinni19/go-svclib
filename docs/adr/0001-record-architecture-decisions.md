# 0001. Record architecture decisions

- Status: Accepted
- Date: 2026-11-01

## Context

go-svclib is a public library with a long support period. Users and new contributors need the reasons for design choices, not only the result.

## Decision

Record each architecture decision as an ADR in `docs/adr/`, numbered in sequence, with the format in `template.md`. A change to an accepted decision is a new ADR that supersedes the old one.

## Consequences

- Decisions are reviewable in pull requests.
- The plan (`docs/plan.md`) links to ADRs; the ADR is the source of truth for each decision.
