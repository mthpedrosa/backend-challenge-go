# Wallet Service — Distributed Wager Processing in Go

Wallet service for game providers, built with **Go 1.27**, **Uber Fx**, **Echo**, **PostgreSQL**, **SQS (MiniStack)** and **Keycloak**.

> The original challenge statement is in [`docs/CHALLENGE.md`](docs/CHALLENGE.md).
> Architecture decisions will be documented in `ARCHITECTURE.md`.

## Status

🚧 Work in progress — delivered phase by phase through pull requests.

| Phase | Scope | Status |
| --- | --- | --- |
| 1 | Bootstrap: Fx composition, config, JSON logging, Echo server, Docker Compose, CI | ✅ |
| 2 | Domain model (`Money`, `Wallet`, `WagerTransaction`, ledger, events) | ⏳ |
| 3 | Persistence (migrations, pgx repositories, DB constraints) | ⏳ |
| 4 | Use cases + HTTP API (idempotency, reconciliation) | ⏳ |
| 5 | Authentication / authorization (Keycloak OIDC) | ⏳ |
| 6 | Workers (pending references, transactional outbox) | ⏳ |
| 7 | SQS consumer (inbox, DLQ, graceful shutdown) | ⏳ |
| 8 | Observability (metrics, readiness) | ⏳ |
| 9 | Concurrency, multi-instance and failure tests | ⏳ |
| 10 | Documentation | ⏳ |

## Prerequisites

- Docker + Docker Compose v2
- Go 1.27+ (only to run tests/the binary outside Docker; with `GOTOOLCHAIN=auto` an older Go downloads 1.27 automatically)
- `make` (optional)

## Running

```sh
cp .env.example .env
docker compose up --build
```

| Service | URL |
| --- | --- |
| Wallet API | http://localhost:8080 |
| Keycloak | http://localhost:8081 (admin / admin) |
| MiniStack (SQS) | http://localhost:4566 |
| PostgreSQL | localhost:5432 (wallet / wallet) |

```sh
curl -i http://localhost:8080/health/live
# HTTP/1.1 200 OK
# X-Correlation-Id: ...
# {"status":"ok"}
```

## Configuration

All configuration comes from environment variables (see [`.env.example`](.env.example)). The service validates its configuration at startup and refuses to start if anything is invalid.

| Variable | Default | Description |
| --- | --- | --- |
| `APP_NAME` | `wallet-service` | Service name added to every log line |
| `APP_ENV` | `local` | Environment name |
| `APP_INSTANCE_ID` | hostname | Instance identity (logs, worker leases) |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `HTTP_ADDR` | `:8080` | Listen address |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | |
| `HTTP_READ_TIMEOUT` | `10s` | |
| `HTTP_WRITE_TIMEOUT` | `15s` | |
| `HTTP_IDLE_TIMEOUT` | `60s` | |
| `HTTP_SHUTDOWN_TIMEOUT` | `20s` | Max time to drain in-flight requests on `SIGTERM` |

## Tests

```sh
go test ./...
go test -race ./...
go vet ./...
```

Or with `make`: `make test`, `make test-race`, `make lint`. Run `make help` for all targets.

## Project layout

```
cmd/wallet/                 # entrypoint (fx.New(...).Run())
internal/
  bootstrap/                # Fx composition root
  config/                   # env loading + validation
  adapters/httpapi/         # Echo router, middlewares, handlers, server lifecycle
  platform/observability/   # slog JSON logger
  platform/correlation/     # correlation id propagation via context
docs/CHALLENGE.md           # original challenge statement
```

## Contributing workflow

- One short-lived branch per phase (`feat/bootstrap`, `feat/domain`, ...), merged into `main` through a PR.
- [Conventional Commits](https://www.conventionalcommits.org/) (`feat(scope): ...`, `test(scope): ...`).
- CI (GitHub Actions) runs `go mod tidy` check, `gofmt`, `go vet`, `go test -race` and a Docker build on every PR.
