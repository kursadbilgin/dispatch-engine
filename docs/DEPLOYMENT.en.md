# Deployment Guide

This document describes local and pipeline-based deployment flows for Dispatch Engine.

## 1. Supported Run Modes

- Full stack with Docker Compose (recommended for local)
- Local process run (`go run ./cmd/api`)
- GitHub Actions CI/CD (build, test, image publishing)

## 2. Prerequisites

- Docker + Docker Compose
- Go 1.25.6+
- (Optional) `golangci-lint`

## 3. Configuration and Secret Management

```bash
cp .env.example .env
```

Critical variables:
- `DATABASE_DSN`
- `RABBITMQ_URL`
- `REDIS_URL`
- `WEBHOOK_SITE_URL`

Recommendations:
- Use a secret manager in production instead of `.env`.
- Manage `WEBHOOK_SITE_URL` per environment.

## 4. Deploy with Docker Compose

### Start

```bash
make docker-up
```

Alternative:

```bash
docker-compose up -d
```

### Services

- API: `http://localhost:8080`
- RabbitMQ AMQP: `localhost:5672`
- RabbitMQ UI: `http://localhost:15672`
- PostgreSQL 18 (`postgres:18`): `localhost:5432`
- Redis: `localhost:6379`
- Prometheus: `http://localhost:9090`

### Stop

```bash
make docker-down
```

## 5. Run as Local Binary

If infra services are already running:

```bash
go build -o bin/api ./cmd/api
./bin/api
```

Notes:
- DB migrations run automatically on startup.
- Worker and retry scanner start in the same process.

## 6. Startup Sequence and Health Checks

Startup flow:
1. Load config
2. Initialize logger
3. Connect PostgreSQL and run migrations
4. Connect Redis
5. Connect RabbitMQ and declare topology
6. Start HTTP server
7. Start worker and retry scanner goroutines

Health endpoints:
- `GET /livez` -> `200`
- `GET /readyz` -> `200` when dependencies are healthy, otherwise `503`

Smoke test:

```bash
curl -i http://localhost:8080/livez
curl -i http://localhost:8080/readyz
curl -i http://localhost:8080/metrics
```

## 7. Monitoring

Prometheus config:
- `docker/prometheus.yml`

Recommended metrics:
- `dispatch_engine_http_requests_total`
- `dispatch_engine_http_request_duration_seconds`
- `dispatch_engine_notifications_sent_total`
- `dispatch_engine_notifications_failed_total`
- `dispatch_engine_worker_inflight`
- `dispatch_engine_retry_scheduled_total`

## 8. CI/CD Pipeline

Workflow file:
- `.github/workflows/ci.yml`

CI steps:
1. checkout
2. Go setup + dependency verification
3. gofmt check
4. golangci-lint
5. tests + race + coverage
6. build
7. govulncheck
8. coverage artifact upload

CD step:
- Runs only on push to `main` or `v*` tags
- GHCR login
- Docker image build and push (`ghcr.io/<owner>/<repo>`)

## 9. Production Readiness Notes

### Architecture
- Split API and worker into separate deployments.
- Scale consumers independently from API replicas.

### Security
- Restrict CORS policy.
- Add TLS termination.
- Store secrets in managed systems.
- Place RabbitMQ/Redis/PostgreSQL in private networks.

### Reliability
- Use platform-level liveness/readiness probes.
- Tune graceful shutdown timeout per environment.
- Add DLQ monitoring and alerting.

## 10. Troubleshooting

### `readyz` returns 503
- Check PostgreSQL/Redis are running.
- Verify `.env` connection strings.
- Inspect container states:

```bash
docker compose ps
docker compose logs app --tail=200
```

PostgreSQL 18 migration note:
- The compose file mounts PG data at `/var/lib/postgresql`.
- If you previously used a PG16 volume, start with a fresh volume or perform `pg_upgrade`.

### Queue is not being processed
- Check queue depth in RabbitMQ UI.
- Review worker logs.
- Verify provider endpoint availability.

### Retry backlog grows
- Revisit `RETRY_SCAN_INTERVAL` and `RETRY_SCAN_LIMIT` values.
- Verify `RATE_LIMIT_PER_SEC` is not too low.
