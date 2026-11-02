# Orcha Cloud Gateway

The API gateway for Orcha Cloud, a learning-oriented, self-hosted LLM inference
platform. It sits in front of a `llama-server` process and exposes part of an
OpenAI-style API.

This is an early prototype running on a single VM. It is not production-grade:
there is no authentication or rate limiting yet, so keep it on loopback. See
[docs/roadmap.md](docs/roadmap.md) for status and known gaps, and
[docs/architecture.md](docs/architecture.md) for the design.

## API

| Route | Behavior |
| --- | --- |
| `GET /health` | Liveness only. Returns `{"status":"ok"}` without contacting the backend, so a `200` does not mean inference is available. |
| `POST /v1/chat/completions` | Non-streaming chat completion, forwarded to the backend. |

### `POST /v1/chat/completions`

The gateway checks that the body is valid JSON with a non-empty `messages` array,
then forwards the original bytes. It does not interpret any other field, so
whether a field such as `temperature` or `model` has an effect depends on the
backend. Client headers, including `Authorization`, are not forwarded.

JSON responses from the backend are returned with their original status and body,
including backend errors. Errors raised by the gateway look like this:

```json
{"error":{"message":"inference backend timed out","type":"upstream_error","code":"upstream_timeout"}}
```

| Status | `code` | Cause |
| --- | --- | --- |
| 400 | `invalid_body`, `invalid_json`, `missing_messages` | The request failed validation. |
| 400 | `stream_not_supported` | `stream: true` was sent. Streaming is planned for M4. |
| 413 | `request_too_large` | The body exceeds `ORCHA_MAX_REQUEST_BYTES`. |
| 502 | `upstream_unavailable` | The backend could not be reached or dropped the connection. |
| 502 | `bad_upstream_response` | The backend sent a non-JSON response, or one over 8 MiB. |
| 504 | `upstream_timeout` | The backend exceeded `ORCHA_UPSTREAM_TIMEOUT`. |

Unknown routes and wrong methods get Go's default plain-text `404` and `405`.

## Local development

Requires Go 1.24 or later. There are no third-party dependencies.

```bash
go run ./cmd/gateway
```

The gateway needs a backend for chat completions. To use the one on the VM, open
an SSH tunnel to it from another terminal:

```bash
ssh -N -L 8080:127.0.0.1:8080 <user>@<vm-address>
```

Then, in a third terminal:

```bash
curl http://127.0.0.1:8081/health

curl http://127.0.0.1:8081/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"messages":[{"role":"user","content":"Say hello"}],"max_tokens":32}'
```

## Configuration

Settings come from environment variables, and all are optional.

| Variable | Default | Purpose |
| --- | --- | --- |
| `ORCHA_LISTEN_ADDR` | `127.0.0.1:8081` | Address the gateway listens on. |
| `ORCHA_INFERENCE_URL` | `http://127.0.0.1:8080` | Base URL of the backend. |
| `ORCHA_UPSTREAM_TIMEOUT` | `300s` | Limit for one upstream call. |
| `ORCHA_MAX_REQUEST_BYTES` | `1048576` | Largest accepted request body. |

The gateway does not load `.env` files. To use one, copy `.env.example` to `.env`
and export it into your shell:

```bash
set -a; source .env; set +a
```

## Testing

```bash
gofmt -l .
go vet ./...
go test -race ./...
golangci-lint run ./...
```

Tests use fake upstream servers and need neither a model nor the VM.

## Deployment

Deployment to the VM is not set up yet. The planned flow is: local tests, commit,
push, pull on the VM, restart the service, smoke test.
