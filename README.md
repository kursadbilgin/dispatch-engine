# Dispatch Engine

![Go Version](https://img.shields.io/badge/Go-1.25.6-00ADD8)
![Fiber](https://img.shields.io/badge/Fiber-v2-00ADD8)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-Database-336791)
![RabbitMQ](https://img.shields.io/badge/RabbitMQ-Queue-FF6600)
![Redis](https://img.shields.io/badge/Redis-RateLimit-DC382D)

A high-performance notification dispatch service built with Go.
It accepts requests over HTTP, persists state in PostgreSQL, enqueues work to RabbitMQ,
and processes messages asynchronously with worker goroutines.

## Quick Start

```bash
# Clone
git clone <your-repo-url>
cd dispatch-engine

# Configure environment
cp .env.example .env

# Start full stack (API, PostgreSQL, RabbitMQ, Redis, Prometheus)
make docker-up

# Verify
curl http://localhost:8080/livez
curl http://localhost:8080/readyz
```

For detailed setup and local workflow, see [docs/DEVELOPMENT.en.md](docs/DEVELOPMENT.md).

## Documentation

Comprehensive docs live under [`docs/`](docs/):

| Document | Description |
|---|---|
| [Architecture & Design](docs/ARCHITECTURE.md) | Queue topology, lifecycle, retry flow, system patterns |
| [API Reference](docs/API.md) | Endpoint contracts and examples |
| [Configuration](docs/CONFIGURATION.md) | Environment variables and runtime tuning |
| [Development](docs/DEVELOPMENT.md) | Local setup, testing strategy, workflow |
| [Deployment](docs/DEPLOYMENT.en.md) | Docker/Kubernetes deployment and operations |
| [Documentation Index](docs/README.en.md) | Documentation map |

## Architecture Overview

Stack: Go 1.25.6, Fiber v2, PostgreSQL, RabbitMQ, Redis, Prometheus, Zap.

Layered model:
- Transport: Fiber handlers and middleware
- Application: Notification service, worker service, retry scanner
- Domain: Notification/batch/attempt entities and rules
- Infrastructure: PostgreSQL repository, RabbitMQ adapter, Redis rate limiter, webhook provider

Key capabilities:
- Asynchronous channel-based processing (`sms`, `email`, `push`)
- Priority handling (`HIGH`, `NORMAL`, `LOW`)
- Idempotency with unique index on `idempotency_key`
- Retry with exponential backoff + jitter
- Distributed per-channel rate limiting via Redis (`INCR` + `EXPIRE`)
- Health probes (`/livez`, `/readyz`) and metrics (`/metrics`)

See [docs/ARCHITECTURE.en.md](docs/ARCHITECTURE.md) for details.

## Configuration

Core environment variables:

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_DSN` | Yes | - | PostgreSQL DSN |
| `RABBITMQ_URL` | Yes | - | RabbitMQ AMQP URL |
| `REDIS_URL` | Yes | - | Redis connection URL |
| `WEBHOOK_SITE_URL` | Yes | - | Provider endpoint |
| `RATE_LIMIT_PER_SEC` | No | `100` | Per-channel rate limit |
| `WORKER_CONCURRENCY` | No | `16` | Worker goroutine count |
| `RETRY_SCAN_INTERVAL` | No | `5s` | Retry scanner interval |
| `RETRY_SCAN_LIMIT` | No | `100` | Retry records per scan |
| `API_PORT` | No | `8080` | HTTP server port |
| `LOG_LEVEL` | No | `info` | Logger level |

See [docs/CONFIGURATION.en.md](docs/CONFIGURATION.md) for the full list.

## Development

```bash
# Run locally
go run ./cmd/api

# Build binary
make build

# Run tests
make test

# Run lint
make lint
```

See [docs/DEVELOPMENT.en.md](docs/DEVELOPMENT.md) for details.

## Project Structure

```text
dispatch-engine/
├── cmd/api/                 # Entry point
├── internal/
│   ├── config/              # Config loading
│   ├── domain/              # Domain models and rules
│   ├── handler/             # HTTP handlers
│   ├── infra/               # Postgres and Redis clients
│   ├── observability/       # Logging and metrics
│   ├── provider/            # Webhook provider client
│   ├── queue/               # RabbitMQ publisher/consumer
│   ├── ratelimit/           # Rate limiter abstractions
│   ├── repository/          # Persistence layer
│   └── service/             # Application services
├── docs/                    # Documentation
├── postman/                 # Postman collection + environment
├── openapi.yaml             # API contract
├── docker-compose.yml       # Local orchestration
└── Dockerfile               # Container image
```

## Key Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/livez` | GET | Liveness probe |
| `/readyz` | GET | Readiness probe (Postgres + Redis) |
| `/metrics` | GET | Prometheus metrics |
| `/v1/notifications` | POST | Create single notification |
| `/v1/notifications` | GET | Filtered/paginated list |
| `/v1/notifications/:id` | GET | Get single notification |
| `/v1/notifications/:id/cancel` | POST | Cancel notification |
| `/v1/notifications/batch` | POST | Create notification batch |
| `/v1/batches/:batchId` | GET | Get batch summary |

For full details, see [docs/API.en.md](docs/API.md) and [openapi.yaml](openapi.yaml).

## Distributed System Patterns

### Idempotency
- Duplicate create requests are handled safely via `idempotencyKey`.

### Retry with Backoff
- Transient provider failures trigger exponential retries with jitter.

### Rate Limiting
- Redis-based distributed per-channel limiting.

### Dead Letter Queues
- Per-channel DLQs (`dlq.sms`, `dlq.email`, `dlq.push`) isolate failed deliveries.

## Additional Resources

- Postman collection: [postman/dispatch-engine.postman_collection.json](postman/dispatch-engine.postman_collection.json)
- Postman environment: [postman/dispatch-engine.local.postman_environment.json](postman/dispatch-engine.local.postman_environment.json)
- OpenAPI spec: [openapi.yaml](openapi.yaml)

## License

Licensed under the [LICENSE](LICENSE) file.
