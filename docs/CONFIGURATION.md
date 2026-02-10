# Configuration Guide

Dispatch Engine configuration is loaded from environment variables into `internal/config.Config`.

## Resolution Order

1. Environment variable values
2. Defaults defined in struct tags

## Environment Variable Catalog

### Infrastructure Credentials (docker-compose)

| Variable | Default | Description |
|---|---|---|
| `POSTGRES_USER` | `dispatch` | PostgreSQL username |
| `POSTGRES_PASSWORD` | `dispatch` | PostgreSQL password |
| `POSTGRES_DB` | `dispatch_engine` | PostgreSQL database name |
| `RABBITMQ_DEFAULT_USER` | `dispatch` | RabbitMQ username |
| `RABBITMQ_DEFAULT_PASS` | `dispatch` | RabbitMQ password |

### Application Core

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_DSN` | Yes | - | PostgreSQL DSN |
| `RABBITMQ_URL` | Yes | - | RabbitMQ AMQP URL |
| `REDIS_URL` | Yes | - | Redis URL |
| `WEBHOOK_SITE_URL` | Yes | - | External provider endpoint |

### Runtime Tuning

| Variable | Required | Default | Description |
|---|---|---|---|
| `RATE_LIMIT_PER_SEC` | No | `100` | Per-channel allowed sends per second |
| `WORKER_CONCURRENCY` | No | `16` | Worker goroutine count |
| `RETRY_SCAN_INTERVAL` | No | `5s` | Retry scanner interval |
| `RETRY_SCAN_LIMIT` | No | `100` | Max retry records processed per scan |
| `API_PORT` | No | `8080` | HTTP server port |
| `LOG_LEVEL` | No | `info` | Log level |

### `.env.example` Note

| Variable | Default | Status |
|---|---|---|
| `MAX_RETRIES` | `5` | Currently not loaded into global config; request-level `maxRetries` is used |

## Example `.env`

```env
POSTGRES_USER=dispatch
POSTGRES_PASSWORD=dispatch
POSTGRES_DB=dispatch_engine
RABBITMQ_DEFAULT_USER=dispatch
RABBITMQ_DEFAULT_PASS=dispatch

DATABASE_DSN=host=localhost user=dispatch password=dispatch dbname=dispatch_engine port=5432 sslmode=disable
RABBITMQ_URL=amqp://dispatch:dispatch@localhost:5672/
REDIS_URL=redis://localhost:6379/0
WEBHOOK_SITE_URL=https://webhook.site/your-uuid-here

RATE_LIMIT_PER_SEC=100
WORKER_CONCURRENCY=16
RETRY_SCAN_INTERVAL=5s
RETRY_SCAN_LIMIT=100
API_PORT=8080
LOG_LEVEL=info
```

## Key Settings Notes

### `RATE_LIMIT_PER_SEC`
- Applied per channel (`sms`, `email`, `push`).
- Enforced with Redis `INCR + EXPIRE`.

### `WORKER_CONCURRENCY`
- Controls worker goroutines consuming from queues.
- Higher values increase throughput and backend/provider load.

### `RETRY_SCAN_INTERVAL` and `RETRY_SCAN_LIMIT`
- Directly affect retry latency and DB scanning pressure.

### `LOG_LEVEL`
Common supported values:
- `debug`
- `info`
- `warn`
- `error`
- `dpanic`
- `panic`
- `fatal`

## Validation Behavior

If any required variable is missing (`DATABASE_DSN`, `RABBITMQ_URL`, `REDIS_URL`, `WEBHOOK_SITE_URL`), startup fails fast.

## Security Recommendations

- Do not commit `.env` files.
- Use a secret manager in production.
- Rotate credentials regularly.
- Restrict Postgres/Redis/RabbitMQ to private networks.
