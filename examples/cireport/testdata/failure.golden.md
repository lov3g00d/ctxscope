## ❌ ctxscope cancellation report

| Field | Value |
| --- | --- |
| Operation | cache shutdown |
| Scope | `scope-42` |
| Status | **Failed** |
| Grace period | `250ms` |
| Shutdown observation | `250ms` |
| Registered tasks | 1 |
| Surviving goroutines | 1 |

### Violations

| Kind | Task |
| --- | --- |
| `shutdown_timeout` | — |
| `task_descendant_survived` | refresh cache |

### Tasks

| Task | State | Attributed survivors |
| --- | --- | ---: |
| refresh cache | `completed` | 1 |

### Task registration stacks

<details>
<summary>refresh cache — <code>completed</code></summary>

```text
example.com/service.submitRefresh
  /workspace/cache_test.go:41
```
</details>

### Survivor stacks

<details>
<summary>Sample 1 — 1 goroutine(s)</summary>

```text
example.com/service.watchCache
  /workspace/cache.go:88
```
</details>
