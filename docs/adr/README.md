# Architecture Decision Records

ADRs capture the significant, hard-to-reverse decisions behind fate and the
reasoning at the time they were made.

| ADR | Title | Status |
|-----|-------|--------|
| [0001](./0001-provenance-and-license.md) | Provenance, IP clearance, and license | Accepted (pending IP confirmation) |
| [0002](./0002-public-api.md) | Public API design and package layout | Accepted |
| [0003](./0003-scheduler-and-timer-model.md) | Clock-agnostic core and the adapter timer model | Accepted |
| [0004](./0004-invoke-spawn-effects.md) | Invoke / spawn as effects-as-data | Accepted |
| [0005](./0005-temporal-integration-boundary.md) | Temporal integration boundary | Accepted |

Planned (to be written as the corresponding work lands):

- Snapshot persistence shape (full JSON schema + versioning)
- Determinism contract and testing strategy
