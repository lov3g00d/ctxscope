# Release process

`ctxscope` follows semantic versioning. Versions below `v1.0.0` may contain
breaking API changes.

1. Update `CHANGELOG.md` and move relevant entries out of `Unreleased`.
2. Run `go mod tidy` and confirm it produces no unexpected changes.
3. Run `go test ./...`, `go test -race ./...`, and `go vet ./...`.
4. Confirm CI passes on the release commit.
5. Create an annotated tag such as `v0.1.0`.
6. Push the commit and tag to GitHub.
7. Create GitHub release notes from the matching changelog section.
8. Verify the module is available from `proxy.golang.org` and `pkg.go.dev`.

Published tags are immutable. Fixes require a new version.
