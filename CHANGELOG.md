# Changelog

All notable changes to `ctxscope` are documented here.

## [Unreleased]

## [0.1.3] - 2026-08-12

### Added

- CI checks for Staticcheck, known Go vulnerabilities, GitHub Actions workflow
  syntax, and evaluation of every declared Nix development shell.

### Fixed

- Deadline polling now discards profile captures that began before the grace
  deadline and completed after it, preventing stale survivors and incorrect
  `task_descendant_survived` violations for tasks that stopped in time.
- Startup functions that also survive the shutdown grace period now report
  both `startup_timeout` and `shutdown_timeout` violations.
- The `x86_64-darwin` development shell now uses the final compatible nixpkgs
  release instead of failing evaluation against nixpkgs unstable.
- Generated `.direnv` cache files, including host-specific paths and Nix-store
  symlinks, are no longer tracked by Git.

## [0.1.2] - 2026-08-12

### Added

- A tested GitHub Actions report example with Markdown job summaries, JSON
  artifacts, stack-trace rendering, a copyable failure-enforcement workflow,
  and a manual repository showcase.

## [0.1.1] - 2026-08-12

### Added

- Reusable worker-pool examples covering clean cancellation, tasks that never
  start, tasks that remain running, and completed tasks with surviving
  descendants.
- Focused examples for named `Scope.Go` background tasks, `Stress` and
  `StressScoped`, and versioned JSON reports.

## [0.1.0] - 2026-08-12

Initial public release.

### Added

- Operation-scoped cancellation verification through `Inspect` and `Verify`.
- Goroutine ownership using inherited pprof labels instead of process-wide
  goroutine counts.
- Structured survivor reports containing labels and stack frames.
- Configurable operation names, startup deadlines, shutdown grace periods, and
  adaptive profile polling.
- `InspectScoped`, `VerifyScoped`, `Scope.Task`, and `Scope.Go` for tracking
  named work across worker pools and queues.
- Typed violations for startup timeouts, shutdown timeouts, tasks that never
  start, tasks still running, and completed tasks with surviving descendants.
- Task registration stacks, lifecycle timestamps, attributed survivor stacks,
  and versioned JSON report fields.
- `Stress` and `StressScoped` for repeated verification with failure counts and
  shutdown-latency percentiles.
- Executable happy-path, failure-path, stack-trace, and worker-pool examples.
- Nix development environment, multi-version CI, contributor documentation,
  comparison guidance, and release documentation.

### Fixed

- Scoped task labels are removed before a reusable worker accepts its next job.

[Unreleased]: https://github.com/lov3g00d/ctxscope/compare/v0.1.3...HEAD
[0.1.3]: https://github.com/lov3g00d/ctxscope/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/lov3g00d/ctxscope/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/lov3g00d/ctxscope/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/lov3g00d/ctxscope/releases/tag/v0.1.0
