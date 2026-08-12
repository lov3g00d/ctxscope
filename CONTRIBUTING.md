# Contributing

Bug reports, focused feature proposals, documentation improvements, and code
contributions are welcome.

## Before coding

Search existing issues before opening a new one. For changes to the public API
or profile-matching behavior, open an issue first so the compatibility and
runtime implications can be discussed.

Do not disclose security vulnerabilities in public issues. Follow
`SECURITY.md` instead.

## Development environment

The minimum supported version is Go 1.24.

With Nix and direnv:

```bash
direnv allow
```

Or enter the shell directly:

```bash
nix develop
```

A local Go installation also works.

## Checks

Run these commands before submitting a pull request:

```bash
go fmt ./...
go mod tidy
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
```

Run the repository analysis checks with the current toolchain provided by
`nix develop` (the pinned analysis tools require Go 1.25 or newer):

```bash
go run honnef.co/go/tools/cmd/staticcheck@2026.1 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
nix flake check --all-systems --no-build
```

Keep tests deterministic. Goroutines created to simulate a leak must always be
released before the test exits.

## Code style

- Follow standard Go formatting and naming conventions.
- Document every exported identifier with a concise Go doc comment.
- Add inline comments only for non-obvious contracts, invariants, or cleanup.
- Keep the public API small and avoid exposing profile parser internals.
- Add tests for behavior changes, including cancellation and race-detector
  coverage where relevant.

## Pull requests

Keep pull requests focused. Describe the user-facing behavior, explain any API
or compatibility impact, and include the commands used to validate the change.

By participating, you agree to follow `CODE_OF_CONDUCT.md`.
