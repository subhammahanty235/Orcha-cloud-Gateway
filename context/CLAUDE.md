# Orcha Cloud --- Claude Code Project Context

> This file is the working context for Claude Code. Read it before
> making changes. The project is being built incrementally as a
> learning-oriented backend engineering project that may later become a
> usable developer platform.

## 1. Project overview

**Project name:** Orcha Cloud

**Goal:** Build a self-hosted LLM inference platform, starting with one
small model on an Oracle Cloud Always Free ARM VM and evolving into a
developer-facing API platform.

The user writes and tests code on their **local development machine**,
commits and pushes it to Git, then pulls/deploys it on the Oracle VM. Do
not assume code is authored directly on the VM.

The project has two long-term goals:

1.  Learn and demonstrate strong backend engineering: networking,
    concurrency, API design, distributed systems, reliability,
    observability, security, and system design.
2.  Eventually provide a usable developer platform where applications
    can call hosted models through API keys and an OpenAI-compatible
    API.

Build engineering depth first, then expand product capabilities.

## 2. Current infrastructure and verified state

### Oracle Cloud VM

-   Cloud: Oracle Cloud Infrastructure (OCI)
-   Home region: India West (Mumbai)
-   Instance shape: `VM.Standard.A1.Flex`
-   Allocated compute: 2 OCPU, 12 GB RAM
-   OS: Ubuntu 24.04 Minimal
-   Architecture: ARM64 / aarch64
-   User: `ubuntu`
-   Project root on VM: `/home/ubuntu/Orcha`
-   Git-based deployment workflow: local development → Git push → VM Git
    pull/deploy
-   A public IP exists, but do not hardcode it in source files or
    documentation.

The VM is resource-constrained. Avoid heavyweight components and
unnecessary services. Design for future scale, but implement only what
the current workload needs.

### Security and networking

-   UFW is enabled.
-   Default incoming traffic is denied; outgoing traffic is allowed.
-   SSH TCP port 22 is allowed.
-   Port 22 is open to the internet and gets constant brute-force
    attempts (~3,500 failed logins in 24h, checked 2026-11-06).
    Password and keyboard-interactive auth are off, root login is
    key-only, and fail2ban is not active.
-   Do not expose inference or gateway ports directly to the public
    internet.
-   Do not casually modify firewall rules or SSH configuration in a way
    that could lock the user out.
-   Docker is installed, but be aware Docker-published ports can bypass
    ordinary UFW expectations.
-   Keep secrets out of Git. Use environment variables or local
    untracked environment files.
-   Do not commit private SSH keys, OCI credentials, API keys, or model
    credentials.

### Inference engine

-   Engine: `llama.cpp`
-   Repository on VM: `/home/ubuntu/minillm/llama.cpp`
-   Build output:
    `/home/ubuntu/minillm/llama.cpp/build/bin/llama-server`
-   Model: Qwen 2.5 1.5B Instruct, Q4_K_M GGUF
-   Model path on VM, as reported by llama-server's `/v1/models` on
    2026-11-06:
    `/home/ubuntu/minillm/models/Qwen2.5-1.5B-Instruct-Q4_K_M.gguf`.
    Earlier notes said `/home/ubuntu/Orcha/`; paths and the service name
    were checked on the VM on 2026-11-06.
-   Model was downloaded from the `bartowski/Qwen2.5-1.5B-Instruct-GGUF`
    Hugging Face repository.
-   The VM also has an abliterated (reduced-refusal) variant of the same
    1.5B model, converted to `qwen-abliterated-Q4_K_M.gguf` and
    `qwen-abliterated-f16.gguf` in the same models directory, with the
    source in `qwen-abliterated-hf/`. As of 2026-11-06 it is not loaded
    by any service; `minillm.service` runs the original Instruct model.
-   CPU inference was tested successfully on the VM.
-   Measured through the gateway on 2026-11-06 (baselines, not
    guarantees):
    -   Generation: ~8.9 tokens/sec.
    -   Prompt processing: ~12 tokens/sec, so an uncached 500-token
        prompt takes ~40s. Prompt caching (`cache_n` in responses)
        avoids reprocessing a shared prefix.
    -   llama-server build `b12033`, 4 slots, `n_ctx` 2048. One request
        can use the full 2048 tokens; larger requests get a 400
        `exceed_context_size_error`.
    -   The 2048-token KV cache is shared by all 4 slots. When two
        conversations together exceed it, they evict each other's
        cached prompt (`failed to restore kv cache` in the service
        log), and every turn reprocesses the full prompt: ~1400
        tokens took ~115s. Observed on 2026-11-06 with two ~1400-token
        chats alternating.

### Inference server service

The inference server is managed by systemd as `minillm.service`
(`/etc/systemd/system/minillm.service`). Earlier notes called it
`Orcha.service`, which does not exist on the VM.

It listens only on:

-   Host: `127.0.0.1`
-   Port: `8080`

It was verified healthy using:

``` bash
curl http://127.0.0.1:8080/health
```

Expected response:

``` json
{"status":"ok"}
```

The service runs this command (verified with `ps` on 2026-11-06):

``` bash
/home/ubuntu/minillm/llama.cpp/build/bin/llama-server \
  -m /home/ubuntu/minillm/models/Qwen2.5-1.5B-Instruct-Q4_K_M.gguf \
  --host 127.0.0.1 \
  --port 8080 \
  -c 2048 \
  -t 2
```

Do not change the inference bind address to `0.0.0.0` unless there is a
deliberate, reviewed network/security design change.

### Remote development access

An SSH tunnel from the user's local machine to the VM has been tested
successfully. The inference API is reachable from the local machine
through the tunnel. Do not ask the user to expose port 8080 publicly.

## 3. Intended architecture

Start with a **modular monolith** for the Go gateway. Do not begin with
microservices.

Initial request flow:

``` text
Local / external client
        |
        v
Go API Gateway (initially 127.0.0.1:8081 on VM)
        |
        v
Inference client
        |
        v
llama-server (127.0.0.1:8080)
        |
        v
Qwen 2.5 1.5B GGUF
```

Longer-term conceptual architecture:

``` text
Client applications
        |
        v
API Gateway
  |       |        |
 Auth  Rate limit  Usage metering
        |
        v
    Model Router
        |
   +----+-----+
   |    |     |
 Worker 1   Worker 2 ... (future inference nodes)
        |
        v
 Metrics / logs / traces
```

Keep the gateway independent of the inference backend. The client-facing
API should remain stable if `llama.cpp` is later replaced by another
inference engine.

## 4. Technology choices

Use these choices unless there is a clear technical reason to revisit
them:

  -----------------------------------------------------------------------
  Concern                 Initial choice          Later possibility
  ----------------------- ----------------------- -----------------------
  Gateway                 Go standard library     Router/framework only
                          `net/http`              if justified

  Inference               llama.cpp               Other inference
                                                  backends

  Persistence             None initially          PostgreSQL

  Rate limiting           In-memory prototype     Redis for distributed
                          initially               limits

  Metrics                 Structured logs and     Prometheus + Grafana
                          basic metrics           

  Frontend                Not in initial          React + TypeScript
                          milestone               

  Deployment              Git-based deploy;       Docker Compose /
                          Docker available        stronger deployment
                                                  automation

  TLS / reverse proxy     Not configured yet      Caddy or Nginx
  -----------------------------------------------------------------------

Do not add Redis, PostgreSQL, Kafka, Kubernetes, or other infrastructure
just because they may be useful later. Introduce a dependency only when
a concrete requirement needs it.

## 5. Planned milestones

Implement incrementally and keep each milestone small, testable, and
documented.

### M1 --- Core API gateway

-   Go HTTP server
-   `GET /health`
-   `GET /v1/models`
-   `POST /v1/chat/completions`
-   Proxy requests to llama-server
-   Preserve OpenAI-compatible request/response semantics where
    practical
-   Sensible timeouts and error handling
-   Gateway listens on `127.0.0.1:8081` initially

### M2 --- Authentication

-   API key creation and revocation
-   Store only a secure hash of API keys, never plaintext keys
-   Bearer-token authentication middleware
-   Constant-time hash/token comparison where applicable
-   Clear unauthorized responses

### M3 --- Traffic management

-   Per-key request rate limits
-   Quotas
-   Concurrent request limits
-   Request body size limits
-   Backpressure and overload responses

### M4 --- Streaming

-   Server-Sent Events (SSE) streaming proxy
-   Correct flushing and cancellation behavior
-   Client disconnect propagation
-   Tests for partial streams and upstream failures

### M5 --- Usage accounting

-   Request-level usage records
-   Input/output token usage when reliable data is available
-   Latency and status tracking
-   Daily/monthly usage summaries
-   Be explicit when token counts are estimates rather than exact
    measurements

### M6 --- Persistence

-   PostgreSQL for users, API keys, model configuration, and usage
    records
-   Schema migrations
-   Repository/data-access boundaries
-   Avoid premature abstraction

### M7 --- Caching

-   Redis only when justified
-   Rate-limit counters and short-lived metadata first
-   Consider response/semantic caching only with clear correctness rules

### M8 --- Model routing

-   Multiple model configurations
-   Model aliases
-   Routing policies
-   Health-aware worker selection
-   Timeouts, retries, circuit breakers, and fallback behavior
-   Avoid retrying non-idempotent or partially streamed requests blindly

### M9 --- Observability

-   Structured logs
-   Request IDs
-   Metrics for request count, errors, latency, throughput, and
    saturation
-   Prometheus + Grafana when useful
-   Never log API keys, authorization headers, or sensitive prompts by
    default

### M10 --- Developer dashboard

-   React + TypeScript
-   User and API-key management
-   Usage and analytics
-   Model catalogue and configuration
-   Documentation and SDKs later

## 6. Initial API contract

The first gateway should expose:

### `GET /health`

Reports gateway health. Decide whether this is a liveness check only or
also checks upstream readiness; document the distinction.

### `GET /v1/models`

Returns the models currently exposed by the gateway in an
OpenAI-compatible shape where practical. Initially this can report the
configured Qwen model.

### `POST /v1/chat/completions`

Accepts an OpenAI-style chat completion request and forwards it to
llama-server.

Initial non-streaming support is acceptable. Streaming will be added as
a separate milestone without breaking the existing request contract.

Do not claim support for OpenAI API fields or endpoints that are not
actually implemented.

## 7. Code quality and collaboration instructions

Claude Code should act as a senior engineering pair, not as an
autonomous code generator.

-   First inspect the existing repository and its conventions before
    editing.
-   Explain the design choice and trade-offs briefly before significant
    changes.
-   Prefer small, reviewable changes over generating the entire platform
    at once.
-   Do not silently add dependencies.
-   Use idiomatic, readable Go.
-   Keep packages cohesive; avoid overengineering and speculative
    interfaces.
-   Handle errors explicitly.
-   Use `context.Context` for request-scoped cancellation and deadlines.
-   Set appropriate HTTP server and upstream client timeouts.
-   Avoid unbounded goroutines, unbounded request bodies, and unbounded
    buffering.
-   Make concurrency behavior explicit.
-   Add unit tests for handlers, middleware, and inference-client
    behavior.
-   Use `httptest` and fake upstream servers for tests; tests should not
    require the real model or Oracle VM.
-   Add integration tests where they provide real value.
-   Run `gofmt`, `go test ./...`, and relevant static checks after
    changes.
-   Update README and configuration examples when behavior changes.
-   Keep local development reproducible.
-   Do not assume local machine paths, SSH key paths, VM public IPs, or
    shell environment values.
-   Do not make infrastructure changes on the VM unless explicitly
    requested.
-   Do not commit generated binaries, model weights, secrets, or local
    environment files.
-   Ask before destructive actions, schema resets, or changes that could
    affect remote availability.

When a task is ambiguous, ask one focused clarification instead of
inventing requirements.

## 8. Local development and Git deployment workflow

The user writes code locally, then syncs it to the VM through Git.

Therefore:

-   All application source code must be developed and tested locally
    first.
-   Keep VM-specific absolute paths and deployment configuration outside
    portable application logic.
-   Provide a `.env.example` with safe placeholder values if environment
    variables are introduced.
-   Add `.env`, secrets, local binaries, and model files to `.gitignore`
    as appropriate.
-   Document the deployment steps separately from local development.
-   Do not assume the VM automatically receives local file changes.
-   A deployment should normally be: local tests → commit → push → VM
    pull → restart/reload service → smoke test.
-   Do not create or edit the systemd gateway service until the gateway
    is ready for deployment and the user approves the deployment
    approach.

## 9. Resource constraints and performance

The current VM has 2 OCPU and 12 GB RAM on ARM64.

-   Keep the gateway lightweight.
-   Remember inference is CPU-bound and competes for the same compute
    resources.
-   Avoid excessive goroutines or large in-memory queues.
-   Bound concurrency and request sizes.
-   Do not assume multiple simultaneous generation requests will have
    good latency.
-   Measure before optimizing.
-   Record baseline latency, throughput, memory, and CPU usage when
    adding significant features.
-   Keep inference worker scaling as a future architecture goal, not a
    current capability.

## 10. Security requirements

-   Never expose llama-server directly to the public internet.
-   Keep the inference service bound to `127.0.0.1`.
-   Keep the gateway loopback-bound initially; design public exposure
    deliberately later.
-   API keys must be high entropy and shown to the user only at creation
    time.
-   Store API key hashes, not raw keys.
-   Never log authorization headers or secrets.
-   Validate and bound request bodies.
-   Apply timeouts to upstream inference calls.
-   Protect against uncontrolled concurrency and resource exhaustion.
-   Use TLS when the gateway is eventually exposed outside the VM.
-   Treat prompt content and model output as untrusted data.
-   Do not execute model output as shell commands or code.

## 11. Current status and immediate next action

Verified complete:

-   OCI Ubuntu 24.04 ARM64 VM provisioned.
-   UFW enabled with inbound deny-by-default and SSH allowed.
-   Docker installed and tested.
-   llama.cpp compiled successfully on ARM64.
-   Qwen 2.5 1.5B Q4_K_M model downloaded.
-   Terminal inference tested successfully.
-   llama-server running as `minillm.service`.
-   llama-server health endpoint returns `{"status":"ok"}`.
-   Inference server is bound to `127.0.0.1:8080`.
-   SSH tunnel from local machine to inference server works.

-   Gateway repository initialized and pushed to
    `github.com/subhammahanty235/Orcha-cloud-Gateway` (branch `main`).
-   Gateway server with `GET /health` (liveness only) and non-streaming
    `POST /v1/chat/completions` proxy, tested locally against fake
    upstreams.
-   Gateway smoke-tested against the real llama-server through an SSH
    tunnel (2026-11-06).

Not yet confirmed complete:

-   `GET /v1/models`.
-   Gateway deployment to the VM and gateway systemd service.
-   Public reverse proxy / TLS.
-   Database, Redis, monitoring, or frontend.

### Current project state

-   **Active work:** M1, in progress. See `docs/roadmap.md` for status,
    known gaps, and the session handoff.
-   **Blockers:** none.
-   **Next task:** add `GET /v1/models`.
-   **Checks:** use `golangci-lint run ./...` for static checks; the
    locally installed `staticcheck` crashes on Go 1.24 export data.

## 12. Product naming and scope

Use **Orcha Cloud** as the project name for now.

Do not overpromise that the current single-VM setup is production-grade
or highly scalable. The project is a learning-oriented prototype that
should be engineered with sound foundations and evolved through measured
milestones.

The long-term target is a lightweight self-hosted LLM API platform with:

-   OpenAI-compatible APIs
-   API key management
-   Rate limits and quotas
-   Usage analytics
-   Multiple model routing
-   Streaming
-   Reliability and observability
-   A developer dashboard


## 13. Mandatory context and documentation maintenance protocol

Claude Code must maintain the project's documentation and working context continuously. Do not wait for the user to explicitly ask for documentation updates.

### Source-of-truth files

Maintain these files as the project grows:

- `CLAUDE.md`: durable project context and instructions for future Claude Code sessions. Keep it concise enough to be useful, but complete enough that a fresh session can continue without reconstructing decisions from chat history.
- `README.md`: what the project does, architecture overview, prerequisites, local setup, configuration, running, testing, and deployment instructions.
- `docs/architecture.md`: architecture diagrams, component responsibilities, request/data flows, important design decisions, and trade-offs.
- `docs/decisions/`: Architecture Decision Records (ADRs) for significant decisions. Use numbered files such as `0001-modular-monolith.md`.
- `docs/operations.md`: VM deployment, service management, logs, health checks, rollback, and operational troubleshooting.
- `docs/roadmap.md`: milestone status, completed work, current focus, known gaps, and next steps.
- `.env.example`: names and safe example values for required configuration; never include real secrets.

Create these files gradually when they become useful. Do not generate empty documentation files just to satisfy the list.

### When documentation must be updated

At the end of every meaningful implementation task or milestone, review the relevant documentation and update it in the same change whenever applicable.

Always update documentation when any of the following changes:

1. Architecture, package boundaries, component responsibilities, or request flow.
2. API routes, request/response contracts, error semantics, or compatibility claims.
3. Configuration variables, defaults, ports, timeouts, or runtime behavior.
4. Dependencies, database schema, migrations, or infrastructure components.
5. Security controls, authentication, authorization, secrets handling, or network exposure.
6. Local development, test, build, Git deployment, or VM operations procedures.
7. Roadmap status, milestone completion, known limitations, or next steps.
8. A technical decision is made that a future session would otherwise have to rediscover.

Do not update docs for trivial formatting-only changes unless they alter documented behavior.

### End-of-task context update

Before considering a non-trivial task complete:

1. Run formatting, tests, and relevant checks.
2. Summarize what changed and why.
3. Record important decisions and trade-offs.
4. Record what is implemented versus what is only planned.
5. Update `CLAUDE.md` if durable project context, current state, conventions, constraints, or next steps changed.
6. Update `README.md` and the relevant files under `docs/` for user-facing or operational changes.
7. Update `docs/roadmap.md` when milestone status or the immediate next task changes.
8. List remaining issues, limitations, and follow-up work explicitly.

Do not claim a task or feature is complete if it is only scaffolded, partially implemented, untested, or merely proposed.

### Keeping CLAUDE.md useful

- Keep verified facts separate from plans, assumptions, and open questions.
- Remove or revise stale instructions when decisions change.
- Record exact commands and configuration only when verified; label examples and placeholders clearly.
- Never put secrets, private keys, access tokens, or sensitive personal information in documentation.
- Avoid copying the entire README or every implementation detail into `CLAUDE.md`.
- Keep a short **Current project state** section with completed milestones, active work, known blockers, and the next concrete task.
- When a new Claude Code session starts, inspect the repository and docs, compare them with the actual code, and resolve obvious documentation drift before making assumptions.
- If the code and docs disagree, treat the code and tests as evidence of current behavior, investigate the mismatch, and update the docs. Do not silently assume either is correct.
- Do not rewrite or reorganize all documentation unnecessarily; make focused updates.

### Session handoff

When the user asks to stop, pause, or hand off work—or when a substantial task is complete—leave a concise handoff in `CLAUDE.md` or `docs/roadmap.md` containing:

- Current milestone and status.
- What was changed in this session.
- Important files and entry points.
- Commands used to run and test the project.
- Decisions made and their rationale.
- Known bugs, risks, and unfinished work.
- The single most useful next action.

The next session should be able to continue from the handoff without relying on this conversation's chat history.

### Git discipline for documentation

- Commit documentation changes alongside the implementation they describe.
- Keep `.env.example` in Git, but keep `.env` and all real credentials out of Git.
- Before finishing, inspect `git status` and ensure no model weights, binaries, logs, secrets, or local-only files were staged.
