# 0002: Forward chat requests as raw bytes

**Status:** Accepted

## Context

`POST /v1/chat/completions` has a large and evolving request schema. The gateway
needs only a few fields from it today.

## Decision

Parse only the fields the gateway acts on (`messages`, `stream`), then forward the
original bytes rather than re-encoding a typed struct. Return backend JSON responses
unchanged, and replace non-JSON responses with a `502`.

Reject `stream: true` until streaming exists (M4), rather than buffering a stream or
silently returning a non-streaming response.

## Consequences

- Fields are not dropped or altered, and the gateway does not need updating for each
  new one.
- The body is parsed once for validation but not re-encoded, so forwarding adds no
  further serialization cost.
- The gateway does not validate the full schema or the `model` value, so most invalid
  fields are handled by the backend, or ignored by it.
- The `error` shape of a backend error is whatever the backend sends. This keeps
  useful messages such as "Loading model", but it ties clients to `llama-server`'s
  format for those errors. Normalize them if the backend is replaced or starts
  exposing internal detail.
- A future backend that is not OpenAI-compatible would need translation in
  `internal/inference`, which would have to change this decision.
