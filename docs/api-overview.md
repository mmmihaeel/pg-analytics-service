# API Overview

Base URL: `/api/v1`

All responses follow a common envelope:

```json
{
  "data": {},
  "meta": {},
  "error": {
    "code": "invalid_request",
    "message": "..."
  }
}
```

Only one of `data` or `error` is returned.

## Public Endpoints

## GET `/health`

Returns service and dependency health.

## GET `/reports`

Lists report definitions.

Query params:

- `search` (optional)
- `limit` (default `20`, max `100`)
- `offset` (default `0`)
- `sort` (`slug|name|updated_at|cache_ttl_seconds`)
- `order` (`asc|desc`)

## GET `/reports/{slug}`

Returns report metadata and supported filters.

## GET `/reports/{slug}/run`

Runs a report query over precomputed snapshots.

Common query params:

- `window` (`day|week`, default `day`)
- `date_from` (`YYYY-MM-DD`)
- `date_to` (`YYYY-MM-DD`)
- `limit` (default `50`, max `100`)
- `offset` (default `0`)

Optional query params:

- `breakdown`
  - `volume-by-period`: `source` or omitted
  - `status-counts`: `period` or omitted
- `source` (for `volume-by-period` with source filtering)
- `status` (for `status-counts`)

## Management Endpoints

Management endpoints require either:

- `X-Management-Key: <key>`
- `Authorization: Bearer <key>`

## POST `/recomputations`

Triggers asynchronous recomputation.

Request body:

```json
{
  "report_slug": "status-counts",
  "window": "day",
  "date_from": "2026-01-01",
  "date_to": "2026-02-01",
  "requested_by": "ops-user"
}
```

Returns `202 Accepted` with run payload.

## GET `/recomputations/{id}`

Returns recompute run status and summary.

## GET `/audit-entries`

Lists audit events.

Query params:

- `action` (optional)
- `actor` (optional)
- `limit` (default `20`, max `100`)
- `offset` (default `0`)

## Error Codes

- `invalid_request` -> 400
- `unauthorized` -> 401
- `not_found` -> 404
- `conflict` -> 409
- `service_unavailable` -> 503
- `internal_error` -> 500
