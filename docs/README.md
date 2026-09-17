# fate documentation

These pages are the source for the site at
**[fate.arisjirat.com](https://fate.arisjirat.com)**, and are readable as-is on
GitHub. For a first machine in a few lines, start with
[getting started](guide/getting-started.md).

## Guide

- [Getting started](guide/getting-started.md) — install, a first machine, and
  the shape of the API.
- [Defining machines](guide/defining-machines.md) — states, transitions, guards,
  actions, hierarchy, parallel regions, and history.
- [Effects and adapters](guide/effects-and-adapters.md) — delayed transitions
  and invocations as data, and how an adapter drives them.
- [Persistence and determinism](guide/persistence-and-determinism.md) — how an
  actor serialises to JSON, and the rules that keep replay exact.
- [Temporal](guide/temporal.md) — running a machine inside a Temporal workflow.

## Reference

- [Concepts](concepts.md) — what a statechart is, and the idea that shapes the
  whole library: the engine computes state, adapters perform effects.
- [The fate CLI](cli.md) — rendering, inspecting, and diffing machines from the
  command line.
- [Versioning and deprecation](versioning.md) — what a version number promises
  and how APIs are retired.
- [Architecture Decision Records](adr/) — the significant design choices, in the
  order they were made.
- [pkg.go.dev](https://pkg.go.dev/github.com/arisros/fate) — per-symbol API
  reference.

## The studio

The visual chart viewer and live simulator is a separate project,
[fate-studio](https://github.com/arisros/fate-studio), hosted at
[fate-studio.arisjirat.com](https://fate-studio.arisjirat.com). It is kept out of
this repository on purpose: the engine has no dependencies, and the studio needs
a web server.

## Building this site

The site is [VitePress](https://vitepress.dev). Everything it needs lives in
this directory.

```sh
make site-dev     # local dev server with hot reload
make site         # production build into docs/.vitepress/dist
make site-image   # container image, tagged with the released version
```

The version shown in the navigation and footer is read at build time from
`.release-please-manifest.json`, so it follows releases automatically.
[`Dockerfile`](Dockerfile) builds the site and [`server/`](server) serves it:
a standard-library-only static server that resolves clean URLs, redirects the
paths the previous site published, and answers `/healthz`.
