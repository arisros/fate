# Security Policy

## Reporting a vulnerability

Please report security issues through GitHub's private vulnerability reporting:

**[Report a vulnerability](https://github.com/arisros/fate/security/advisories/new)**

(Security tab → Advisories → Report a vulnerability.) This keeps the report
private until a fix is released. Please do **not** open a public issue for a
security problem.

You should get an acknowledgement within a week. This is a single-maintainer
project, so please allow reasonable time for a fix before public disclosure.

## Supported versions

`fate` is pre-1.0. Only the latest released minor version receives security
fixes; there are no backports to earlier minors.

| Version | Supported |
| ------- | --------- |
| latest minor | yes |
| anything older | no |

## Scope

`fate` is a dependency-free statechart engine: it parses no untrusted input and
opens no network connections. The parts most likely to matter for security are
the ones that touch the outside world:

- **`httphandler`** — serves machines over HTTP. It is a development and
  simulation surface, not a hardened public endpoint. Do not expose it to
  untrusted traffic without your own authentication and authorization in front
  of it.
- **`Persist` / `Restore`** — treat a persisted snapshot as untrusted input if
  it can be supplied by a user. Restoring a snapshot reconstructs an actor's
  active configuration and re-arms its effects.

Reports about either of these are in scope, as is anything that lets a machine
definition or event cause the engine to behave outside its documented contract.
