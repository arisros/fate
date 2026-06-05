<!-- Thanks for contributing to fate! -->

## What & why

<!-- What does this change do, and why? Link any issue. -->

## Checklist

- [ ] Tests added/updated (behavioural change → behavioural test; new public API → an `Example`).
- [ ] `go test -race ./...` passes (root module **and** `temporal/` if touched).
- [ ] `go vet ./...` and `golangci-lint run` are clean (every exported symbol documented).
- [ ] Root module is still standard-library only (no new `require` in the root `go.mod`).
- [ ] Coverage stays at or above the 85% gate.
- [ ] A contract-level change is recorded in an ADR under `docs/adr/`.
- [ ] `CHANGELOG.md` updated under **Unreleased**.
