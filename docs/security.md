# Security

The service is built for secure-by-default behavior within its current scope: public read-only reporting, protected management endpoints, bounded query execution, and auditable operational actions.

## Related Docs

- [README](../README.md)
- [API Overview](api-overview.md)
- [Deployment Notes](deployment-notes.md)

## Implemented Controls

| Control area | Current implementation |
| --- | --- |
| Management auth | Recomputation and audit endpoints require `X-Management-Key` or `Authorization: Bearer <key>`. |
| Secret comparison | Management key validation uses constant-time comparison. |
| Query bounds | Date ranges are limited by `MAX_REPORT_RANGE_DAYS`; `limit` is capped at `100`; `offset` must be non-negative. |
| Input validation | Windows, dates, JSON request bodies, and allowlisted sort fields are validated before application execution. |
| SQL safety | Repository queries are parameterized; dynamic sorting is restricted to an allowlist. |
| Duplicate-trigger protection | Redis locks prevent concurrent recompute requests for the same report, window, and date range. |
| Auditability | Management actions and worker outcomes are persisted in `audit_entries`. |
| Error shaping | API responses avoid stack traces and return stable error codes. |

## Access Model

The access boundary is intentionally simple:

- report catalog and report execution endpoints are public and read-only
- recomputation and audit visibility are management-only
- the current implementation uses a single shared management key rather than user, tenant, or role-specific credentials

That model is appropriate for the current repository scope, but it is not positioned as a production-grade multi-tenant authorization system.

## Validation and Query Safety

The main resource-protection measures in the current implementation are:

- bounded date windows through `MAX_REPORT_RANGE_DAYS`
- capped pagination for list and report queries
- allowlisted `sort` fields on report listing
- strict `day` and `week` window validation
- strict JSON decoding for recompute requests with unknown fields rejected

These checks are important because the service deliberately exposes analytical queries and recompute triggers over HTTP.

## Operational Considerations

Security posture depends on deployment choices as well as code behavior. For non-local deployments:

- rotate `MANAGEMENT_API_KEY` and never rely on the default fallback value
- terminate TLS at the edge or inside the platform network
- restrict management endpoints at the network perimeter
- use secret management rather than static env files where possible
- add rate limiting if the public read surface is internet-exposed

## Current Limitations

The current design is intentionally honest about what it does not yet provide:

- no tenant-aware or user-aware authorization model
- no request-rate limiting in the HTTP layer
- no dedicated secret rotation workflow
- no immutable audit export or tamper-evident log chain

These are reasonable next steps for a larger deployment, but they are outside the present repository scope.
