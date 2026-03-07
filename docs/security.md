# Security

## Scope

This project targets secure-by-default behavior suitable for a public portfolio backend and local deployment.

## API Access Controls

- Public access is limited to read-only report and health endpoints.
- Management operations (`/recomputations`, `/audit-entries`) require a management API key.
- Key validation uses constant-time comparison.

## Input Validation and Query Bounds

- Date inputs must follow `YYYY-MM-DD`.
- Date range is bounded by `MAX_REPORT_RANGE_DAYS`.
- Pagination is constrained to a safe upper bound (`100`).
- Sort fields are allowlisted.
- Unsupported windows/breakdowns are rejected with `400`.

## Recompute Safety Controls

- Redis lock prevents duplicate recompute requests for identical scope.
- Expiring lock protects against abandoned lock state.
- Recompute activity is persisted in `recompute_runs` and `audit_entries`.

## Data and Error Handling

- SQL uses parameterized queries.
- Error payloads avoid stack traces and internal details.
- Sensitive management operations are auditable.

## Operational Hardening Recommendations

- Rotate `MANAGEMENT_API_KEY` regularly.
- Restrict management endpoint access at network perimeter.
- Add TLS termination for non-local deployments.
- Add request-rate limiting if internet-exposed.
- Add centralized secret management for production environments.
