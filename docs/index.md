---
layout: home

hero:
  text: A statechart engine for Go that only <em>computes</em>.
  tagline: Hierarchical states, parallel regions, and history, with no dependencies beyond the standard library.
  install: go get github.com/arisros/fate
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: Concepts
      link: /concepts
    - theme: alt
      text: GitHub
      link: https://github.com/arisros/fate

highlights:
  - title: Statecharts, not flat FSMs
    details: Compound and parallel states, deep and shallow history, guards, actions, and final states.
    link: /concepts
    glyph: nested
  - title: No dependencies
    details: The engine imports only the standard library. The Temporal integration is a separate module.
    link: /guide/getting-started
    glyph: none
  - title: Deterministic and persistable
    details: An Actor serialises to JSON and restores byte for byte, which makes replay exact.
    link: /guide/persistence-and-determinism
    glyph: final
  - title: Effects are data
    details: Delays and invocations surface as pending effects. An adapter performs them.
    link: /guide/effects-and-adapters
    glyph: effect
  - title: Runs inside Temporal
    details: No side effects in the engine, so a machine survives workflow replay unchanged.
    link: /guide/temporal
    glyph: clock
  - title: Render and diff
    details: Render any machine to ASCII, Mermaid, or graph JSON, and diff two snapshots.
    link: /cli
    glyph: diff
---
