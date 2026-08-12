# Machine-readable report

`Inspect` and `InspectScoped` return JSON-compatible reports. This example
registers a task that never reaches a worker and writes the resulting report
to standard output.

Run it with:

```bash
go run ./examples/report
```

The output includes:

- `schema_version` for consumers that persist or parse reports;
- `parent_id` on nested tasks (`schema_version` 2 and newer);
- the operation name, scope ID, cancellation time, and durations;
- the pending task and its registration stack;
- the operation-level `shutdown_timeout` and task-specific
  `task_never_started` violations.

The command exits successfully after writing the report. A CI adapter can
decode it, archive it as an artifact, and decide whether `report.Passed()`
should fail a build.

Durations use nanoseconds in JSON. Timestamps use RFC 3339 formatting through
Go's standard `time.Time` JSON representation.
