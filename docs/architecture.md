# Architecture

This describes what is implemented today. The long-term plan is in
[roadmap.md](roadmap.md).

## Request flow

```text
Client
  |
  v
Go gateway (127.0.0.1:8081)
  logging -> routing -> validation
  |
  v
inference.Client
  |
  v
llama-server (127.0.0.1:8080)
```

## Components

| Package | Responsibility |
| --- | --- |
| `cmd/gateway` | Process wiring, HTTP server timeouts, graceful shutdown. |
| `internal/config` | Loads and validates settings from the environment. |
| `internal/gateway` | Routes, request validation, error responses, request logging. |
| `internal/inference` | HTTP calls to the backend. Knows backend paths and owns upstream timeouts and response limits. |

`internal/gateway` depends on the concrete `inference.Client`. Nothing needs a
second implementation yet, and tests use fake HTTP upstreams, so there is no
interface. Add one when a second backend type exists.

## Chat completion flow

1. The body is read up to `ORCHA_MAX_REQUEST_BYTES`.
2. It is checked for valid JSON, a non-empty `messages` array, and no
   `stream: true`.
3. The original bytes are sent upstream, with no client headers.
4. The whole upstream response is buffered, up to 8 MiB.
5. A JSON response is returned with its original status. Anything else becomes
   a `502`.

See [ADR 0002](decisions/0002-raw-passthrough-chat-proxy.md) for why the bytes are
forwarded unchanged.

## Timeouts

| Timeout | Value | Purpose |
| --- | --- | --- |
| Read header | 5s | Limits slow clients sending headers. |
| Read (whole request) | 30s | Limits slow clients sending bodies. |
| Upstream call | `ORCHA_UPSTREAM_TIMEOUT` (300s) | Allows a full 2048-token context at the ~8.8 tokens/sec baseline, about 233s. |
| Write | read + upstream + 10s | Go starts this deadline once headers are read, so it has to cover the body read and the upstream wait. |
| Idle | 60s | Closes unused keep-alive connections. |
| Upstream dial | 5s | Fails fast when the backend is down. |
| Shutdown grace | 15s | In-flight requests still running after this are dropped. |

M4 streaming will need per-request write deadlines, because a fixed write timeout
would cut off long streams.

## Cancellation and concurrency

- Each request runs on the goroutine `net/http` gives it. The gateway starts no
  goroutines of its own per request.
- The request context is passed to the upstream call, so a client disconnect
  cancels it. This is logged with status `499`, which the client never receives.
  Tests confirm the upstream HTTP request is cancelled. Whether `llama-server`
  then stops generating has not been verified.
- **Concurrency is not limited yet.** Memory per request is bounded by the body
  and response limits, but the number of requests is not. M3 adds this.

## Health

`GET /health` is liveness only and never contacts the backend, so a slow or
restarting model does not make the gateway look dead to a supervisor. It therefore
says nothing about whether inference works. If a readiness signal becomes
necessary, it should be a separate endpoint.

## Logging

Logs are JSON via `log/slog`, on stdout. Request logs hold only the method, path,
status, and duration. Headers and bodies are never logged, because they can contain
credentials and prompts. Upstream failure details stay in the logs and are not
returned to clients, because they include internal addresses.

## Network exposure

Both defaults are loopback addresses. The upstream client ignores proxy environment
variables, so prompts cannot be rerouted away from the configured backend.
