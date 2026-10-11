# Distributed Wager Processing Wallet Service

[![Go Version](https://img.shields.io/badge/go-1.27.2-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-336791?logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Echo](https://img.shields.io/badge/Echo-v4-00BFFF?logo=go&logoColor=white)](https://echo.labstack.com/)
[![Uber Fx](https://img.shields.io/badge/Uber%20Fx-v1.24-black?logo=uber&logoColor=white)](https://github.com/uber-go/fx)
[![AWS SQS](https://img.shields.io/badge/AWS%20SQS-FIFO-FF9900?logo=amazonsqs&logoColor=white)](https://aws.amazon.com/sqs/)
[![Keycloak](https://img.shields.io/badge/Keycloak-26.1-blue?logo=keycloak&logoColor=white)](https://www.keycloak.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

A high-performance, fault-tolerant, and distributed wallet service built for gaming platforms and wagering transaction processing in Go. Designed to guarantee strict financial integrity, persistent idempotency, append-only ledger immutability, zero floating-point arithmetic, and row-level concurrency coordination without global locks.

> The original challenge specifications are documented in [`docs/CHALLENGE.md`](docs/CHALLENGE.md).  
> In-depth technical design, invariants, and trade-offs are documented in [`ARCHITECTURE.md`](ARCHITECTURE.md).

---

## Project Status

All 10 project phases have been implemented, tested under `-race`, and verified against real infrastructure containers.

| Phase | Scope | Status | Evidence |
| :---: | :--- | :---: | :--- |
| **1** | **Bootstrap**: Composition root with Uber Fx, configuration loader, structured JSON logging, Echo HTTP server, Docker Compose, and CI pipeline | Done | Pull Request #1 |
| **2** | **Pure Domain Model**: `Money` (fixed-point centavos), `Wallet` aggregate, `WagerTransaction` state machine, `WalletLedgerEntry`, typed domain events, and unit tests | Done | Pull Request #2 |
| **3** | **Persistence**: PostgreSQL migrations (Goose), repository implementations, database constraints, triggers blocking ledger mutation, and `TxManager` | Done | Pull Request #3 |
| **4** | **Use Cases & Wagering API**: `OpenWallet`, `ProcessWager`, pagination cursor for ledger entries, canonical payload hashing, and persistent idempotency | Done | Pull Request #4 |
| **5** | **Authentication & Authorization**: Keycloak OIDC integration, JWKS cache, JWT middleware, provider tenant isolation, and administrative route guards | Done | Pull Request #5 |
| **6** | **Background Workers**: Pending reference resolution worker, transactional outbox publisher with distributed lease locks, and backoff retry logic | Done | Pull Request #6 |
| **7** | **SQS Consumer & Messaging**: SQS FIFO long-polling consumer, atomic inbox deduplication, dead-letter queue (DLQ) redrive, and graceful termination | Done | Pull Request #7 |
| **8** | **Observability**: Prometheus metrics (`/metrics`), request correlation tracking (`X-Correlation-Id`), and PostgreSQL/SQS readiness probes | Done | Pull Request #8 |
| **9** | **Concurrency & E2E Resilience**: 50 parallel duplicate bets, balance contention, cross-wallet parallelism, out-of-order reversals, and cross-protocol deduplication | Done | Pull Request #9 |
| **10** | **Documentation**: Architecture specifications, complete operational guide, migration commands, and API reference | Done | Pull Request #10 |

---

## Architectural Highlights

- **Zero Floating-Point Arithmetic**: All monetary values are represented as minor units (`int64` centavos) using the `money.Money` value object. Floating-point types (`float32`, `float64`) are banned across all application layers.
- **Pure Go Domain Core**: The `internal/domain` package has zero third-party dependencies, guaranteeing isolation from transport drivers, ORMs, and cloud SDKs.
- **Row-Level Concurrency Control**: Wallet balance modifications serialize strictly via row-level locking (`SELECT ... FOR UPDATE` per wallet). No global locks are ever acquired, enabling distinct wallets to execute concurrently.
- **Persistent Idempotency & Conflict Detection**: Operations are indexed by `(provider_id, idempotency_key)` and `(provider_id, external_transaction_id)`. Business fields are hashed using deterministic canonical SHA-256 (omitting transport headers). Replays return the exact original balance and status, while duplicate keys with differing payloads trigger `409 Conflict`.
- **Append-Only Immutable Ledger**: Every balance transition atomically writes to `wallet_ledger_entries`. A database trigger and restricted database privileges strictly prohibit `UPDATE`, `DELETE`, and `TRUNCATE`.
- **Out-of-Order Reversals**: Reversals (`REFUND`, `ROLLBACK`) that arrive prior to their referenced transaction are accepted with `202 Accepted` (`PENDING_REFERENCE`) and settled automatically by a background worker once the referenced bet is ingested.
- **Transactional Outbox & Inbox**: Domain events are committed in the same SQL transaction as domain entities. The outbox publisher uses distributed lease locks (`FOR UPDATE SKIP LOCKED`) to dispatch events to SQS FIFO. The SQS consumer uses an atomic inbox table to ensure at-most-once processing.
- **Automated Continuous Reconciliation**: An atomic reconciliation query verifies that `storedBalance - (Σcredits - Σdebits) = 0.00` on demand.

---

## Project Structure

```
├── cmd/
│   ├── migrate/                     # Database migration CLI runner (Goose embedded)
│   └── wallet/                      # Application entrypoint (Fx composition root)
├── db/
│   └── migrations/                  # Versioned SQL migration files (.sql)
├── deploy/
│   ├── keycloak/                    # Keycloak realm definition and client export
│   └── sqs/                         # SQS FIFO queue initialization script
├── docs/
│   └── CHALLENGE.md                 # Original challenge specification and rubric
├── internal/
│   ├── adapters/
│   │   ├── httpapi/                 # Echo HTTP routes, JWT middleware, handlers
│   │   ├── postgres/                # PostgreSQL pgxpool repositories and TxManager
│   │   └── sqsconsumer/             # SQS FIFO consumer and atomic inbox handler
│   ├── app/
│   │   ├── usecase/                 # Application use cases (ProcessWager, OpenWallet, Reconcile)
│   │   └── worker/                  # Outbox publisher and Pending Reference background workers
│   ├── bootstrap/                   # Uber Fx lifecycle modules and wireup
│   ├── config/                      # Environment variable loader and validator
│   ├── domain/                      # Pure Go domain (zero framework imports)
│   │   ├── event/                   # Typed domain event envelopes
│   │   ├── ledger/                  # Wallet ledger entry entity and repository port
│   │   ├── money/                   # Money value object (int64 minor units)
│   │   ├── port/                    # Repository and transaction ports
│   │   ├── wager/                   # Wager transaction entity and state machine
│   │   └── wallet/                  # Wallet aggregate root
│   └── platform/
│       ├── aws/                     # AWS SDK v2 SQS client configuration
│       ├── correlation/             # Correlation ID propagation via context
│       ├── observability/           # Prometheus metrics and log/slog configuration
│       └── postgres/                # pgxpool connection management
├── test/
│   └── e2e/                         # End-to-end concurrency, resilience, and isolation tests
├── ARCHITECTURE.md                  # Detailed architectural design decisions and invariants
├── docker-compose.yml               # Production-like local stack (app, pg, keycloak, ministack)
├── docker-compose.multi.yml         # Multi-instance topology (app-2 on :8082, app-3 on :8083)
├── Dockerfile                       # Multi-stage optimized application container build
├── Makefile                         # Standard automation targets
└── README.md                        # Documentation and user guide
```

---

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (v24.0+) & Docker Compose (v2.20+)
- [Go](https://go.dev/dl/) 1.27+ (only required if running tests or building binaries locally outside containers)
- `make` (optional, for automation commands)

---

## Getting Started

### 1. Environment Configuration

Clone the repository and copy the environment configuration:

```bash
cp .env.example .env
```

Review [`.env.example`](.env.example) for available configuration options. The application validates all required environment variables on startup.

### 2. Start the Infrastructure Stack

Launch the complete stack (Application, PostgreSQL, Keycloak, and MiniStack):

```bash
docker compose up --build -d
```

### 3. Service Endpoints

| Service | Endpoint | Credentials / Details |
| :--- | :--- | :--- |
| **Wallet HTTP API** | `http://localhost:8080` | Core application REST endpoints |
| **Keycloak IdP** | `http://localhost:8081` | Admin console: `admin` / `admin` |
| **MiniStack (SQS)** | `http://localhost:4566` | SQS FIFO broker emulation |
| **PostgreSQL** | `localhost:5432` | DB: `wallet`, User: `wallet`, Pass: `wallet` |

Verify service readiness:

```bash
curl -i http://localhost:8080/health/ready
# HTTP/1.1 200 OK
# {"status":"ready","postgres":"up","sqs":"up"}
```

---

## Database Migrations

Database migrations are managed using Goose and embedded into the application binary.

Apply all migrations:
```bash
make migrate-up
# or: go run ./cmd/migrate up
```

Check migration status:
```bash
make migrate-status
# or: go run ./cmd/migrate status
```

Roll back the last migration:
```bash
make migrate-down
# or: go run ./cmd/migrate down
```

### Schema Architecture
- `wallets`: Stores aggregate balance (`balance_minor BIGINT`), version counter, and `(player_id, currency)` unique constraint.
- `wallet_ledger_entries`: Append-only immutable ledger recording sequential sequence number (`seq BIGSERIAL`), balance before, balance after, amount, and direction (`DEBIT`/`CREDIT`).
- `wager_transactions`: Stores external transactions, provider ID, state (`PENDING`, `PROCESSED`, `REJECTED`, `PENDING_REFERENCE`), canonical payload hash, and single-reversal unique index.
- `inbox_messages`: Deduplication table for SQS message consumption.
- `outbox_events`: Transactional outbox table with distributed lease lock fields (`locked_by`, `locked_until`, `attempts`).

---

## Authentication & Authorization

The service enforces OAuth2 / OIDC Client Credentials authentication via Keycloak.

### Test Client Credentials

The Keycloak realm (`wallet-realm`) is automatically provisioned with the following clients:

| Client ID | Client Secret | Roles | Allowed Operations |
| :--- | :--- | :--- | :--- |
| `internal-client` | `internal-secret` | `wallet-admin` | Wallet creation, balance queries, ledger extraction, reconciliation |
| `provider-client` | `provider-secret` | `wagering` | Submitting and querying wagering transactions for `providerId = provider-client` |

### Obtaining an Access Token

```bash
# Internal Admin Token
INTERNAL_TOKEN=$(curl -s -X POST "http://localhost:8081/realms/wallet-realm/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials&client_id=internal-client&client_secret=internal-secret" | jq -r .access_token)

# Provider Client Token
PROVIDER_TOKEN=$(curl -s -X POST "http://localhost:8081/realms/wallet-realm/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials&client_id=provider-client&client_secret=provider-secret" | jq -r .access_token)
```

---

## API Reference

### 1. Open Wallet

Creates a new wallet for a player. Requires internal client authorization.

- **Endpoint**: `POST /wallets`
- **Headers**:
  - `Authorization: Bearer <INTERNAL_TOKEN>`
  - `Content-Type: application/json`

```bash
curl -i -X POST http://localhost:8080/wallets \
  -H "Authorization: Bearer $INTERNAL_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "playerId": "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
    "initialBalance": {
      "amount": "100.00",
      "currency": "BRL"
    }
  }'
```

**Response (`201 Created`)**:
```json
{
  "id": "0192f291-27dd-7d3f-8071-5f8685deef37",
  "walletId": "0192f291-27dd-7d3f-8071-5f8685deef37",
  "playerId": "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
  "balance": {
    "amount": "100.00",
    "currency": "BRL"
  },
  "version": 1
}
```

---

### 2. Get Wallet Balance

Retrieves current stored balance and version.

- **Endpoint**: `GET /wallets/:walletId`
- **Headers**: `Authorization: Bearer <INTERNAL_TOKEN>`

```bash
curl -i http://localhost:8080/wallets/0192f291-27dd-7d3f-8071-5f8685deef37 \
  -H "Authorization: Bearer $INTERNAL_TOKEN"
```

**Response (`200 OK`)**:
```json
{
  "id": "0192f291-27dd-7d3f-8071-5f8685deef37",
  "walletId": "0192f291-27dd-7d3f-8071-5f8685deef37",
  "playerId": "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
  "currency": "BRL",
  "balance": {
    "amount": "100.00",
    "currency": "BRL"
  },
  "version": 1
}
```

---

### 3. Process Wagering Transaction

Executes a wager operation (`BET`, `WIN`, `LOSS`, `REFUND`, `ROLLBACK`). Requires provider client authorization.

- **Endpoint**: `POST /wagering/transactions`
- **Headers**:
  - `Authorization: Bearer <PROVIDER_TOKEN>`
  - `Content-Type: application/json`
  - `Idempotency-Key: provider-client:bet-001`

#### Example: Placed Bet (Debit)
```bash
curl -i -X POST http://localhost:8080/wagering/transactions \
  -H "Authorization: Bearer $PROVIDER_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: provider-client:bet-001" \
  -d '{
    "providerId": "provider-client",
    "externalTransactionId": "bet-001",
    "playerId": "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
    "walletId": "0192f291-27dd-7d3f-8071-5f8685deef37",
    "kind": "BET",
    "roundId": "round-987",
    "gameId": "fortune-chimp",
    "money": {
      "amount": "25.00",
      "currency": "BRL"
    }
  }'
```

**Response (`200 OK`)**:
```json
{
  "transactionId": "0192f298-345e-7e38-af88-e43f851a819d",
  "status": "PROCESSED",
  "balance": {
    "amount": "75.00",
    "currency": "BRL"
  },
  "balanceAfter": "75.00",
  "idempotentReplay": false
}
```

*Note: Resending the exact same request will return `200 OK` with `"idempotentReplay": true` and unchanged balance.*

#### Example: Business Rejection (Insufficient Funds)
If the player attempts to bet more than their available balance:
```json
{
  "transactionId": "0192f299-1234-7e38-af88-e43f851a819e",
  "status": "REJECTED",
  "failureCode": "INSUFFICIENT_FUNDS",
  "idempotentReplay": false
}
```

#### Example: Out-of-Order Refund (`202 Accepted`)
If a `REFUND` arrives before the original referenced `BET`:
```json
{
  "transactionId": "0192f29a-5678-7e38-af88-e43f851a819f",
  "status": "PENDING_REFERENCE",
  "idempotentReplay": false
}
```

---

### 4. Paginated Ledger History

Extracts append-only ledger entries using drift-free cursor pagination.

- **Endpoint**: `GET /wallets/:walletId/ledger?limit=10&cursor=<CURSOR>`
- **Headers**: `Authorization: Bearer <INTERNAL_TOKEN>`

```bash
curl -i "http://localhost:8080/wallets/0192f291-27dd-7d3f-8071-5f8685deef37/ledger?limit=10" \
  -H "Authorization: Bearer $INTERNAL_TOKEN"
```

**Response (`200 OK`)**:
```json
{
  "items": [
    {
      "id": "0192f291-27e0-7d3f-8071-5f8685deef38",
      "walletId": "0192f291-27dd-7d3f-8071-5f8685deef37",
      "transactionId": "0192f291-27df-7d3f-8071-5f8685deef37",
      "direction": "CREDIT",
      "amount": "100.00",
      "currency": "BRL",
      "balanceBefore": "0.00",
      "balanceAfter": "100.00",
      "createdAt": "2026-10-11T12:00:00Z"
    },
    {
      "id": "0192f298-3460-7e38-af88-e43f851a819e",
      "walletId": "0192f291-27dd-7d3f-8071-5f8685deef37",
      "transactionId": "0192f298-345e-7e38-af88-e43f851a819d",
      "direction": "DEBIT",
      "amount": "25.00",
      "currency": "BRL",
      "balanceBefore": "100.00",
      "balanceAfter": "75.00",
      "createdAt": "2026-10-11T12:01:00Z"
    }
  ],
  "nextCursor": null
}
```

---

### 5. Wallet Reconciliation

Performs on-demand verification of the wallet against all ledger entries.

- **Endpoint**: `POST /wallets/:walletId/reconciliation`
- **Headers**: `Authorization: Bearer <INTERNAL_TOKEN>`

```bash
curl -i -X POST http://localhost:8080/wallets/0192f291-27dd-7d3f-8071-5f8685deef37/reconciliation \
  -H "Authorization: Bearer $INTERNAL_TOKEN"
```

**Response (`200 OK`)**:
```json
{
  "walletId": "0192f291-27dd-7d3f-8071-5f8685deef37",
  "storedBalance": {
    "amount": "75.00",
    "currency": "BRL"
  },
  "calculatedBalance": {
    "amount": "75.00",
    "currency": "BRL"
  },
  "difference": {
    "amount": "0.00",
    "currency": "BRL"
  },
  "consistent": true,
  "checkedEntries": 2
}
```

---

## SQS Messaging Integration

The service consumes wager transactions from an SQS FIFO queue and publishes domain events using the Transactional Outbox pattern.

### Ingestion Queue (`wager-transactions.fifo`)
Messages posted to `http://localhost:4566/000000000000/wager-transactions.fifo` are ingested by the long-polling consumer.

- `MessageGroupId`: `walletId` (guarantees strict ordering per wallet, enables parallel processing across distinct wallets).
- `MessageDeduplicationId`: `externalTransactionId`.

Example SQS Message Payload:
```json
{
  "providerId": "provider-client",
  "externalTransactionId": "sqs-bet-001",
  "idempotencyKey": "provider-client:sqs-bet-001",
  "playerId": "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
  "walletId": "0192f291-27dd-7d3f-8071-5f8685deef37",
  "currency": "BRL",
  "amount": "10.00",
  "kind": "BET",
  "roundId": "round-sqs-1",
  "gameId": "game-sqs"
}
```

### Outbox Queue (`outbox.fifo`)
Events published after committed transactions:
- `WalletBalanceChanged`
- `WagerTransactionProcessed`
- `WagerTransactionRejected`
- `WagerTransactionPendingReference`

---

## Testing & Verification

### Unit Tests
Runs all pure Go unit tests across money, wallet, wager, and ledger packages:
```bash
make test
```

### Race Detector Validation
Executes all unit and integration tests with Go's race detector:
```bash
make test-race
```

### Code Formatting & Linting
```bash
make fmt
make lint
```

### End-to-End & Concurrency Test Suite
Executes the comprehensive E2E suite against the running Docker stack:
```bash
make test-e2e
# or: go test -v -race -count=1 ./test/e2e/...
```

The E2E test suite validates:
1. `TestConcurrency_50DuplicateBets`: 50 concurrent duplicate requests produce exactly 1 debit and 50 idempotent replays with consistent ledger reconciliation.
2. `TestConcurrency_Two80BetsOn100Balance`: 2 concurrent 80.00 bets against 100.00 balance yield exactly 1 `PROCESSED` and 1 `REJECTED` (`INSUFFICIENT_FUNDS`), leaving an exact 20.00 balance.
3. `TestConcurrency_ParallelWallets`: 5 distinct wallets processed simultaneously without cross-wallet lock contention.
4. `TestConcurrency_ReversalBeforeReference`: `REFUND` arriving before `BET` accepted with `202 PENDING_REFERENCE` and automatically settled upon referenced bet ingestion.
5. `TestAuthorization_ProviderIsolation`: Enforces 403 Forbidden on foreign provider access and administrative routes.
6. `TestConcurrency_CrossHttpAndSQS`: Enforces cross-protocol deduplication between HTTP and SQS.

---

## Multi-Instance Topology

To simulate and test high-availability deployments with multiple independent application instances competing for work:

```bash
docker compose -f docker-compose.yml -f docker-compose.multi.yml up -d --build
```

This starts:
- `app` (Instance 1) on port `:8080`
- `app-2` (Instance 2) on port `:8082`
- `app-3` (Instance 3) on port `:8083`

All 3 instances share the same PostgreSQL database, SQS FIFO queues, and Keycloak realm, demonstrating safe distributed locking, outbox lease recovery, and pending reference settlement across nodes.

To stop the multi-instance stack:
```bash
docker compose -f docker-compose.yml -f docker-compose.multi.yml down
```

---

## Observability

- **Metrics**: Exposed at `http://localhost:8080/metrics` in Prometheus format.
- **Liveness Probe**: `GET /health/live` returns `200 OK` when the process is alive.
- **Readiness Probe**: `GET /health/ready` returns `200 OK` when PostgreSQL and SQS are connected and healthy.
- **Structured Logging**: Emits newline-delimited JSON with `time`, `level`, `msg`, `service`, `instanceId`, and `correlationId`.

---

## License

This project is licensed under the [MIT License](LICENSE).
