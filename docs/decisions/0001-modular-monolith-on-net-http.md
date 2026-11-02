# 0001: Modular monolith on net/http

**Status:** Accepted

## Context

The gateway runs next to CPU-bound inference on a VM with 2 OCPU and 12 GB RAM.
Its first routes are few and simple, and part of the project's purpose is to learn
backend fundamentals.

## Decision

Build one Go binary with cohesive packages under `internal/`, using only the
standard library. Go 1.22 `net/http` routing covers method and path matching.

## Consequences

- Deployment is a single small binary, with no third-party code to track.
- Middleware is written by hand, which is manageable at this size.
- Revisit a router if route grouping or middleware composition becomes awkward.
- Package boundaries are enforced only by discipline, so splitting out a service
  later depends on keeping them clean.
