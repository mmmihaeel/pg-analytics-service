# Roadmap

The roadmap is intentionally grounded in the current codebase. It focuses on durability, operational clarity, and contract quality rather than adding infrastructure for its own sake.

## Related Docs

- [README](../README.md)
- [Architecture](architecture.md)
- [Deployment Notes](deployment-notes.md)

## Guiding Principles

- keep PostgreSQL at the center of the analytics design
- add operational depth only when it meaningfully improves durability, visibility, or safety
- preserve the readability of report and recompute logic as the system grows

## Near-Term

| Area | Why it matters |
| --- | --- |
| OpenAPI description | Makes the public and management surface easier to review, test, and consume. |
| Structured operational logging | Improves visibility into report execution and recompute outcomes. |
| Metrics and basic dashboards | Gives the service a clearer operational story around latency, failures, and dependency health. |

## Mid-Term

| Area | Why it matters |
| --- | --- |
| Durable recompute queue | Removes the biggest current operational limitation: in-process queue loss on restart. |
| Scheduled recompute plans | Moves recomputation beyond manual triggering while staying explicit about scope and cadence. |
| Incremental recompute strategies | Reduces rebuild cost for larger datasets and tighter refresh intervals. |

## Longer-Range

| Area | Why it matters |
| --- | --- |
| Tenant-aware authorization | Extends the repository from a single-scope management model to a safer shared environment model. |
| Report-level policy controls | Lets different report families expose different visibility and management rules. |
| Advanced cache policy | Opens the door for adaptive TTLs, warmup patterns, and more nuanced invalidation behavior. |

## Current Non-Goals

These are deliberately out of scope for now:

- adding external workflow systems before the recompute durability problem actually needs them
- expanding the report catalog with synthetic endpoints that do not deepen the architectural story
- presenting scheduling, multi-tenancy, or distributed workers as implemented behavior before they exist
