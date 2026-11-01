# 0004. Security: mTLS and JWT, fail-closed authorization

- Status: Accepted
- Date: 2026-11-01

## Context

Services receive calls from other services and from end users. Trusting identity headers set by a gateway is safe only with strict network isolation, and it fails open when that isolation breaks.

## Decision

- Verify identity in the service.
  - **mTLS** gives the service identity (SPIFFE ID or certificate CN).
  - **JWT** with JWKS gives the user identity (`iss`, `aud`, `exp`, `nbf`, `kid`).
- Both produce one `auth.Principal` in the request context.
- Authorization is **fail-closed**: the `authz` interceptor calls `Service.Authorize`, and the default denies.
- Public procedures are marked by a proto option, not by path.

## Alternatives

- **Trust gateway headers.** Simple, but spoofable without strict network controls.
- **JWT only** or **mTLS only.** Each covers only one caller type.

## Consequences

- Services work safely without a trusted gateway.
- Each service must implement `Authorize`; the RBAC helper reduces the work.
- JWKS fetch and certificate reload are background tasks owned by the lifecycle.
