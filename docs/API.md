# API Reference

Dispatch Engine exposes a REST API for notification lifecycle management, health checks, and observability.

## Base URL

Default local URL: `http://localhost:8080`

## Endpoint Summary

### Health and Observability

| Endpoint | Method | Purpose |
|---|---|---|
| `/livez` | GET | Liveness probe |
| `/readyz` | GET | Readiness probe (PostgreSQL + Redis checks) |
| `/metrics` | GET | Prometheus metrics |

### Notification API

| Endpoint | Method | Purpose |
|---|---|---|
| `/v1/notifications` | POST | Create a single notification |
| `/v1/notifications` | GET | List notifications with filters/pagination |
| `/v1/notifications/:id` | GET | Get a single notification |
| `/v1/notifications/:id/cancel` | POST | Cancel a notification when allowed |
| `/v1/notifications/batch` | POST | Create notifications in batch |
| `/v1/batches/:batchId` | GET | Get batch summary |

## 1. Create Single Notification

**Endpoint**: `POST /v1/notifications`

Request fields:

| Field | Type | Required | Constraints |
|---|---|---|---|
| `channel` | string | Yes | `sms` \| `email` \| `push` |
| `priority` | string | Yes | `high` \| `normal` \| `low` |
| `recipient` | string | Yes | non-empty |
| `content` | string | Yes | `sms<=160`, `push<=240`, `email<=10000` chars |
| `idempotencyKey` | string | No | dedup key for repeated create requests |
| `correlationId` | string | No | request tracing correlation |
| `maxRetries` | int | No | `>0` |
| `scheduledAt` | string (RFC3339) | No | If in the future, record stays `ACCEPTED` until scheduler enqueue |

Example:

```bash
curl -X POST http://localhost:8080/v1/notifications \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: req-123" \
  -d '{
    "idempotencyKey": "msg-001",
    "channel": "sms",
    "priority": "normal",
    "recipient": "+905551112233",
    "content": "Your OTP is 123456"
  }'
```

Success response: `202 Accepted`.

Scheduling behavior:
- `scheduledAt` omitted or `<= now`: notification is enqueued immediately (`QUEUED`).
- `scheduledAt > now`: notification is persisted as `ACCEPTED`; scheduler enqueues it when due.

## 2. Create Notification Batch

**Endpoint**: `POST /v1/notifications/batch`

Rules:
- At least 1 notification
- Maximum 1000 notifications per request

Success response: `202 Accepted`.

## 3. Get Single Notification

**Endpoint**: `GET /v1/notifications/:id`

Status codes:
- `200` found
- `404` not found

## 4. Cancel Notification

**Endpoint**: `POST /v1/notifications/:id/cancel`

Behavior:
- `200`: cancelable states (`ACCEPTED`, `QUEUED`)
- `409`: non-cancelable states

## 5. List Notifications

**Endpoint**: `GET /v1/notifications`

Query parameters:

| Parameter | Default | Description |
|---|---|---|
| `page` | `1` | Page number |
| `pageSize` | `50` | Page size (`1..100`) |
| `status` | - | Status filter |
| `channel` | - | Channel filter |
| `from` | - | RFC3339 lower bound |
| `to` | - | RFC3339 upper bound |

Note:
- When `from` and `to` are both provided, `from <= to` must hold; otherwise the API returns `400`.

## 6. Get Batch Summary

**Endpoint**: `GET /v1/batches/:batchId`

Response includes:
- `batchId`
- `totalCount`
- `status`
- per-status counters

## Error Response Shape

```json
{
  "error": "human readable message"
}
```

Common HTTP status codes:
- `400`: validation error
- `404`: not found
- `409`: conflict
- `500`: internal error
- `503`: readiness failure

## OpenAPI

Machine-readable API contract: [`../openapi.yaml`](openapi.yaml)
