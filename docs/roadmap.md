# Roadmap

Full milestone definitions are in [context/CLAUDE.md](../context/CLAUDE.md).

| Milestone | Status |
| --- | --- |
| M1: Core API gateway | **In progress** |
| M2: Authentication | Not started |
| M3: Traffic management | Not started |
| M4: Streaming | Not started |
| M5: Usage accounting | Not started |
| M6: Persistence | Not started |
| M7: Caching | Not started |
| M8: Model routing | Not started |
| M9: Observability | Not started (basic request logs exist) |
| M10: Developer dashboard | Not started |

## M1 status

Done and covered by tests against fake upstreams:

- HTTP server with timeouts and graceful shutdown
- `GET /health` (liveness)
- Non-streaming `POST /v1/chat/completions` proxy, with error mapping and size limits
- Client disconnects cancel the upstream request

Smoke-tested against the real `llama-server` through an SSH tunnel (2026-11-06):
completions, a backend `400` for an oversized prompt passed through, and
`stream: true` rejected.

Remaining:

- `GET /v1/models`

## Known gaps

- No authentication, so the gateway must stay on loopback (M2).
- No limit on concurrent requests (M3). Inference on 2 OCPU will not handle many
  parallel generations well.
- `model` is not validated and is passed to the backend as sent.
- Unknown routes and wrong methods get Go's plain-text `404` and `405`, not the JSON
  error shape.
- It is unverified whether `llama-server` stops generating when the gateway cancels
  a request.
- Gateway overhead and resource use have not been measured. Backend baselines:
  ~8.9 tokens/sec generation and ~12 tokens/sec prompt processing, so long uncached
  prompts dominate latency.
- The gateway has not been deployed to the VM, and there is no systemd unit.

## Handoff

**Current focus:** M1. The most useful next action is to add `GET /v1/models`.

**Entry points:** `cmd/gateway/main.go`, `internal/gateway/chat.go`,
`internal/inference/client.go`.

**Run and test:** `go run ./cmd/gateway`, `go test -race ./...`,
`golangci-lint run ./...`.

**Tooling note:** the locally installed `staticcheck` (2023.1.7) crashes on Go 1.24
export data. `golangci-lint` works and is what has been used for static checks.

**Decisions:** see [decisions/](decisions/) and [architecture.md](architecture.md).
