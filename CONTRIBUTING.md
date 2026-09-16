# Contributing to fate

Thanks for your interest in fate. This document covers how to build, test, and
propose changes.

## Ground rules

1. **The root module stays dependency-free.** `github.com/arisros/fate` must
   import only the Go standard library. Any change that adds a non-stdlib
   `require` to the root `go.mod` (outside the `// test-only` set) will be
   rejected. Integrations that need third-party dependencies go in their own
   module (see `temporal/`).
2. **Determinism is a contract, not a nicety.** The engine must produce
   byte-identical persisted snapshots for the same machine and event sequence.
   Never iterate a map in a way that affects observable output without sorting
   keys first. Never call `time.Now`, `rand`, or do I/O inside the engine.
3. **Every exported symbol has a godoc comment.** This is enforced by the
   linter (`revive` / `golangci-lint`). No exceptions for exported types,
   functions, methods, constants, or struct fields that form the public API.
4. **Tests accompany every change.** Behavioural changes need behavioural
   tests; new public API needs a testable `Example`.

## Building and testing

```sh
# Engine (zero-dependency module)
go build ./...
go test -race ./...
go vet ./...
golangci-lint run

# Temporal integration (separate module)
cd temporal && go test -race ./...
```

Coverage gate: the root module must stay at or above **85%** line coverage.

```sh
go test -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1
```

## Determinism tests

The property-based suite (`*_property_test.go`, using `pgregory.net/rapid`)
asserts same-input determinism, persist/restore round-trips, and stable
iteration order. Run it with:

```sh
go test -run Property ./...
```

A property-test failure blocks merge.

## Commit / PR conventions

- Keep PRs focused; one logical change per PR.
- Reference the relevant ADR when changing a contract-level decision; add a new
  ADR under `docs/adr/` when introducing one.
- The CI matrix (multiple Go versions × root + temporal modules) must be green.
- Commit subjects and PR titles follow [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat:`, `fix:`, `feat!:` or a `BREAKING CHANGE:` footer). The release notes
  and version bump are generated from them, so write the subject for a reader of
  the changelog, and put migration notes for a breaking change in the footer.

## Releasing

Releases are automated with [release-please](https://github.com/googleapis/release-please).
Every push to `main` updates an open release PR that bumps `Version` in
`doc.go` and adds the new section to [CHANGELOG.md](./CHANGELOG.md); do not
edit either by hand. Merging that PR tags `vX.Y.Z`, creates the GitHub release,
and runs GoReleaser to attach the binaries. While in `v0.x`, a breaking change
bumps the minor version.

The release PR is opened by `GITHUB_TOKEN`, which does not start CI. Close and
reopen it to run the required checks before merging. The `temporal/` module is
excluded and still tagged by hand as `temporal/vX.Y.Z`.
