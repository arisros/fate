# ADR-0001: Provenance, IP clearance, and license

- **Status:** Accepted
- **Date:** 2026-06-05
- **Deciders:** Aris Kurniawan

## Context

fate is a from-scratch, production-grade rebuild of a statechart engine. A
working proof-of-concept was developed inside an employer workspace as part of a
larger effort to migrate that platform's workflow state machines off an
XState-derived design onto a Go-native engine. That proof-of-concept proved the
model works and is the reference its author harvested topology, tests, and
diagrams from.

fate is a **personally-owned, open-source** library published at
`github.com/arisros/fate`, independent of any employer system and 100%
domain-agnostic.

## Decision

### IP clearance gate

Because the proof-of-concept originated in an employer workspace, no fate code
was published until permission to open-source it was obtained from the employer.

**Outcome:** open-sourcing was approved on 2026-06-05, **scoped to the generic
engine**: the library may contain no code, names, business logic, examples, or
documentation specific to the employer's domain. That scope governs what lives
in this repository.

To honour the scope:

- The engine is fully domain-agnostic; the Temporal integration is isolated in a
  separate, equally-generic module (see ADR-0002 / ADR-0005).
- All examples and tests use generic machines (counters, traffic lights, media
  players, pipelines) — never the employer's business state machines.
- No employer or platform names appear in the codebase or docs.

### License

fate is released under the **MIT License**. Rationale: shortest and most
permissive option for a solo-authored Go library, no patent-grant ceremony, and
no `NOTICE` file to maintain. The trade-off — MIT has no explicit patent grant,
unlike Apache-2.0 — is acceptable for a library of this nature.

### Clean-room separation as risk reduction

The engine module (`github.com/arisros/fate`) is dependency-free and contains
no employer-specific types, names, or business logic. Anything that touches the
employer's runtime (the Temporal determinism bridge) is a separate module and is
itself generic. This separation is both an API-quality decision (see ADR-0002)
and a provenance risk-reduction measure.

## Consequences

- Provenance and the scoped approval are on the record from commit one.
- The zero-dependency, domain-agnostic boundary is a hard constraint, not a
  preference — it is what keeps the library inside the approved scope.
- Any future contribution must stay generic; domain-specific glue lives in the
  consumer's own repository, never here.
