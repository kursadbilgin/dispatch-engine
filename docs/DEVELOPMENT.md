# Development Guide

This document covers local setup, testing, and day-to-day development workflow for Dispatch Engine.

## Prerequisites

- Go 1.25.6+
- Docker + Docker Compose
- Make

## Quick Start

```bash
# 1) Prepare environment variables
cp .env.example .env

# 2) Start infra + application stack
make docker-up

# 3) Health checks
curl http://localhost:8080/livez
curl http://localhost:8080/readyz
```

## Project Structure

| Directory | Purpose |
|---|---|
| `cmd/api/` | Entry point, dependency wiring, process lifecycle |
| `internal/config/` | Environment-based configuration |
| `internal/domain/` | Domain models and rules |
| `internal/handler/` | HTTP handlers and error mapping |
| `internal/infra/postgresql/` | PostgreSQL connection and migrations |
| `internal/infra/redis/` | Redis client and rate limiter implementation |
| `internal/repository/` | GORM repository implementations |
| `internal/queue/` | RabbitMQ publisher/consumer/topology |
| `internal/provider/` | External webhook provider client |
| `internal/service/` | Notification, worker, retry workflows |
| `internal/observability/` | Logging and metrics utilities |
| `docs/` | Technical documentation |
| `postman/` | Postman collection/environment |

## Local Run Options

### Option 1: Full Docker Stack

```bash
make docker-up
```

### Option 2: Local App + Infra Containers

```bash
# Infra services
make docker-up

# Run app locally (optional alternative)
go run ./cmd/api
```

## Testing Strategy

| Type | Command | Notes |
|---|---|---|
| Targeted tests | `go test ./internal/domain ./internal/service ./internal/handler` | Fast feedback |
| Full project | `go test ./...` | Broader coverage |
| Race + coverage | `go test -race -coverprofile=coverage.out ./...` | Race detection and coverage |

## Common Commands

| Command | Description |
|---|---|
| `make build` | Build binary to `bin/api` |
| `make run` | Run application locally |
| `make test` | Run tests with race + coverage |
| `make lint` | Run `golangci-lint` |
| `make docker-up` | Start Docker Compose stack |
| `make docker-down` | Stop stack |
| `make clean` | Remove build/test artifacts |

## Recommended Development Flow

1. Pull latest branch changes.
2. Verify `.env` values.
3. Run targeted tests for impacted packages.
4. Run lint.
5. Run `go test ./...` before PR.
6. Update docs when behavior/contracts change.

## Coding Conventions

### Style
- Keep `gofmt` output clean.
- Keep lint checks green.
- Use explicit names for service/repository methods.

### Error Handling
- Wrap errors with context where appropriate.
- Map domain/service errors to HTTP status in handler layer.
- Avoid swallowing errors.

### Context Usage
- Accept `context.Context` at service/repository/external boundaries.
- Propagate cancellation to DB/Redis/HTTP calls.

### Testing
- Prefer table-driven tests for parse/validation logic.
- Test critical worker branches (success, transient retry, permanent failure).
- Validate handler behavior with `app.Test()`.
