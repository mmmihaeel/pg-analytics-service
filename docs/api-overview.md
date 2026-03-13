# API Overview

The HTTP surface is intentionally small: public endpoints for report discovery and execution, plus management endpoints for recomputation and audit visibility.

## Related Docs

- [README](../README.md)
- [Architecture](architecture.md)
- [Domain Model](domain-model.md)
- [Security](security.md)

## API Conventions

| Concern | Behavior |
| --- | --- |
| Base path | All routes are served under `/api/v1`. |
| Response envelope | Responses follow `{ "data": ..., "meta": ..., "error": ... }`. Only `data` or `error` is populated for a given response. |
| Content type | JSON request and response bodies |
| Dates | Query and request-body dates use `YYYY-MM-DD`. |
| Default report window | If `date_from` and `date_to` are omitted for report execution, the handler defaults to the last 30 days ending today (UTC). |
| Pagination | `limit` defaults to `20` for listings and `50` for report execution; the maximum is `100`. |
| Management auth | Protected endpoints accept either `X-Management-Key: <key>` or `Authorization: Bearer <key>`. |

Example envelope:

```json
{
  "data": {},
  "meta": {
    "pagination": {
      "limit": 20,
      "offset": 0,
      "total": 3
    }
  },
  "error": null
}
```

## Endpoint Families

| Family | Routes | Notes |
| --- | --- | --- |
| Health | `GET /api/v1/health` | Returns service and dependency status. |
| Report catalog | `GET /api/v1/reports`, `GET /api/v1/reports/{slug}` | Public metadata for available report definitions. |
| Report execution | `GET /api/v1/reports/{slug}/run` | Public analytics queries over `metric_snapshots`. |
| Recomputation | `POST /api/v1/recomputations`, `GET /api/v1/recomputations/{id}` | Management-only trigger and status lookup for snapshot rebuilds. |
| Audit trail | `GET /api/v1/audit-entries` | Management-only access to persisted operational events. |

## Public Endpoints

### `GET /api/v1/health`

Returns:

- service name
- version
- timestamp
- dependency health for PostgreSQL and Redis

If a dependency is unavailable, the JSON payload reports a degraded state and the HTTP status becomes `503 Service Unavailable`.

### `GET /api/v1/reports`

Lists public report definitions from `report_definitions`.

| Query parameter | Meaning |
| --- | --- |
| `search` | Case-insensitive match against report slug or name |
| `limit` | Default `20`, maximum `100` |
| `offset` | Default `0` |
| `sort` | One of `slug`, `name`, `updated_at`, or `cache_ttl_seconds` |
| `order` | `asc` or `desc` |

### `GET /api/v1/reports/{slug}`

Returns one report definition, including:

- description
- cache TTL
- default window
- allowed windows
- supported filter metadata

### `GET /api/v1/reports/{slug}/run`

Runs a report query over `metric_snapshots`.

Common query parameters:

| Query parameter | Meaning |
| --- | --- |
| `window` | `day` or `week`; defaults to `day` |
| `date_from` | Inclusive lower bound in `YYYY-MM-DD` |
| `date_to` | Inclusive upper bound in `YYYY-MM-DD` |
| `limit` | Default `50`, maximum `100` |
| `offset` | Default `0` |
| `breakdown` | Report-dependent grouping mode |

Report-specific behavior:

| Report | Breakdown behavior | Optional filters |
| --- | --- | --- |
| `volume-by-period` | Omit `breakdown` or use `period` for period totals; use `source` for period plus source rows | `source` |
| `status-counts` | Omit `breakdown` or use `status` for totals by status; use `period` for period plus status rows | `status` |
| `top-entities` | `breakdown` is ignored by the current query shape | none |

Report execution responses include both result data and execution metadata:

- `generated_at`
- `row_count`
- `execution_ms`
- `source_system`
- `cache_hit`

The current implementation also mirrors `cache_hit` into the top-level `meta` object for convenience.

## Management Endpoints

### `POST /api/v1/recomputations`

Triggers an asynchronous snapshot rebuild for one report, one window, and one bounded date range.

Request body:

| Field | Required | Meaning |
| --- | --- | --- |
| `report_slug` | Yes | Target report definition |
| `window` | Yes | `day` or `week` |
| `date_from` | Yes | Inclusive lower bound |
| `date_to` | Yes | Inclusive upper bound |
| `requested_by` | No | Operator label; defaults to `management-api` unless `X-Actor` is provided |

Response behavior:

- `202 Accepted` on successful enqueue
- `409 Conflict` if a recompute for the same report, window, and date range is already in progress
- `503 Service Unavailable` if the in-process queue is full

### `GET /api/v1/recomputations/{id}`

Returns the stored recompute run, including:

- current status
- requested and actual timestamps
- summary counts on success
- `error_message` on failure

### `GET /api/v1/audit-entries`

Lists audit records created by management actions and recompute workers.

| Query parameter | Meaning |
| --- | --- |
| `action` | Optional exact-match filter |
| `actor` | Optional exact-match filter |
| `limit` | Default `20`, maximum `100` |
| `offset` | Default `0` |

Common audit actions in the current implementation:

- `recompute.triggered`
- `recompute.completed`
- `recompute.failed`

## Error Model

| Error code | HTTP status | Typical cause |
| --- | --- | --- |
| `invalid_request` | `400` | Invalid window, malformed dates, range too large, or unsupported parameters |
| `unauthorized` | `401` | Missing or invalid management key |
| `not_found` | `404` | Unknown route, report slug, or recompute run ID |
| `conflict` | `409` | Duplicate recompute trigger for the same scope |
| `service_unavailable` | `503` | Queue pressure or degraded dependency health |
| `internal_error` | `500` | Unexpected server-side failure |
