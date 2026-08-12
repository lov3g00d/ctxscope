# GitHub Actions report

This example turns a `ctxscope.Report` into two CI outputs:

- a GitHub-flavored Markdown job summary for people;
- the complete versioned JSON report for artifact storage and automation.

GitHub provides a per-step path in `GITHUB_STEP_SUMMARY`. Appending Markdown to
that file renders it on the workflow run page. The example keeps the renderer
outside the ctxscope library so applications can choose their own CI format.

## Test the renderer

The normal tests compare a deterministic report with
`testdata/failure.golden.md`, verify JSON round-tripping, and render a real
inspection report:

```bash
go test -count=1 -v ./examples/cireport
go test -race -count=1 ./examples/cireport
```

## Preview a real failure locally

The failure demo is behind the same opt-in build tag as the other live demos:

```bash
report_dir="$(mktemp -d)"
GITHUB_STEP_SUMMARY="$report_dir/summary.md" \
CTXSCOPE_REPORT_PATH="$report_dir/report.json" \
go test \
  -tags=ctxscope_demo_failure \
  -run '^TestFailureDemoRendersGitHubSummary$' \
  -count=1 \
  -v \
  ./examples/cireport

cat "$report_dir/summary.md"
cat "$report_dir/report.json"
```

The test is expected to exit with status `1`. It releases the deliberately
stuck goroutine before returning.

## Run it in GitHub Actions

The ctxscope repository includes a manual
[`ctxscope report showcase`](../../.github/workflows/ctxscope-showcase.yml)
workflow. It deliberately creates a cancellation violation and stays green
only when ctxscope detects that violation, making it safe to use as a public
demonstration. Its job summary renders the diagnostic and its artifact contains
the complete JSON report.

For production CI, use the failure-enforcing workflow below.

Copy `github-actions.yml` to `.github/workflows/ctxscope-report.yml`, commit it,
and start it with **Run workflow**. The workflow demonstrates an important
failure-reporting sequence:

1. The test writes Markdown directly to `GITHUB_STEP_SUMMARY` and returns a
   failure.
2. `continue-on-error` allows artifact upload to run.
3. `if: always()` uploads the JSON even though the contract check failed.
4. The final step restores the failed job result.

Do not replace that final enforcement step with the showcase's expected-failure
check in a real project. The showcase is green because its input is
intentionally broken; production CI should be red when a real cancellation
contract fails.

The example pins every third-party action to an exact commit. GitHub-hosted
runners support `actions/upload-artifact` v7; GitHub Enterprise Server users
must select the version supported by their server.

References:

- [GitHub job summaries](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-commands#adding-a-job-summary)
- [GitHub workflow artifacts](https://docs.github.com/en/actions/concepts/workflows-and-actions/workflow-artifacts)
