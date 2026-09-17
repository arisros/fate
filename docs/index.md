---
layout: home

hero:
  name: fate
  text: A statechart engine for Go
  tagline: Hierarchical states, parallel regions, and history — with no dependencies beyond the standard library.
  image:
    src: /logo.svg
    alt: fate
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

features:
  - title: Statecharts, not flat FSMs
    details: Compound and parallel states, deep and shallow history, guards, actions, and final states. The semantics follow SCXML and XState v5.
    link: /concepts
    linkText: Read the concepts
  - title: No dependencies
    details: The engine imports only the standard library. The Temporal integration is a separate module, so its SDK is never pulled in unless you ask for it.
    link: /guide/getting-started
    linkText: Install
  - title: Deterministic and persistable
    details: A Machine is immutable and shareable. An Actor serialises to JSON and restores byte for byte, which makes replay exact.
    link: /guide/persistence-and-determinism
    linkText: How persistence works
  - title: Effects are data
    details: Delayed transitions and invocations are surfaced as pending effects. An adapter performs them, so the engine itself never touches the clock or the network.
    link: /guide/effects-and-adapters
    linkText: The effect model
  - title: Runs inside Temporal
    details: Because the engine performs no side effects, a machine can be driven from a workflow and survive replay unchanged.
    link: /guide/temporal
    linkText: The Temporal guide
  - title: Render and diff
    details: Render any machine to ASCII, Mermaid, or graph JSON, and diff two snapshots from the command line.
    link: /cli
    linkText: The fate CLI
---
