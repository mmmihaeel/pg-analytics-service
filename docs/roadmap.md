# Roadmap

## Near Term

- Add OpenAPI schema generation and publish API contract.
- Add metrics endpoint and latency/error dashboards.
- Add request-level structured logs for report and recompute operations.

## Mid Term

- Move recompute queue to a durable external system.
- Implement scheduled recompute plans per report/window.
- Add incremental recompute strategy for high-volume date ranges.

## Future

- Multi-tenant support with scoped data access.
- Configurable report authorization policies.
- Query-level cost controls and adaptive caching policies.
