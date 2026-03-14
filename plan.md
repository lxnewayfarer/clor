# clor — Improvement Plan

## Analysis Summary

After thorough review of the entire codebase (Go backend + frontend), I identified improvements across three categories: **functional bugs/robustness**, **UX/visual polish**, and **missing quality-of-life features**. The plan avoids over-engineering and focuses on high-impact changes that fit the project's stdlib-only, single-binary philosophy.

---

## Shared Context & Constraints

- **No new dependencies.** All changes use Go stdlib and vanilla JS.
- **Frontend is a single `web/index.html`** — all UI changes go there.
- **Backward compatibility** with existing saved pipelines must be preserved.
- **`activeRuns` map** has race conditions — any subtask touching it must use a mutex.
- **Node type detection** (reviewer, architect, coder) is label-based — keep this convention.
- **CSS variables** are defined in `:root` — reuse them for consistency.

---

## Subtasks

### Backend — Robustness & Concurrency Fixes

1. **Add mutex protection for `activeRuns` map in `server.go`.** Currently `activeRuns` is read/written from HTTP handlers and the orchestrator goroutine without synchronization. Add a `sync.RWMutex` and wrap all accesses (`handleStartRun`, `handleRunEvents`, `handleStopRun`, `handleRetryNode`, `handleSubmitAnswer`, and the goroutine `delete` in `handleStartRun`).

2. **Add graceful shutdown with `SIGINT`/`SIGTERM` handling in `main.go`.** Use `signal.NotifyContext` to create a cancellable context. On signal: cancel all active runs, wait up to 10s for in-flight agents to finish, then exit. This prevents orphaned `claude` subprocesses.

3. **Add pipeline execution duration tracking.** Record `started_at` and `finished_at` timestamps in a `run_meta.json` alongside `status.json`. Expose via `GET /api/run/{id}/status` response as top-level `started_at`, `finished_at`, `total_elapsed` fields. Show total elapsed in the frontend footer during/after runs.

4. **Fix `expandHome` to handle `~` without trailing slash.** Currently `expandHome("~")` returns `home + ""` which works, but `path[1:]` on `"~"` gives `""` — the `filepath.Join` still works but add an explicit check. Also, handle `~user` paths or document the limitation.

### Backend — New Features

5. **Add a `GET /api/run/{id}/summary` endpoint** that returns per-node final status, elapsed time, and first 200 chars of output. This enables a post-run summary view without loading full logs.

6. **Add pipeline-level timeout support.** Currently `timeout_seconds` in settings exists but is only passed to individual agent calls. Add a top-level `context.WithTimeout` in `Orchestrator.Run()` that cancels all nodes if the pipeline exceeds the configured timeout.

### Frontend — Visual Improvements

7. **Add connection path coloring based on execution status.** During a run, color edges: grey (idle), amber animated dash (running/data flowing), green (source done), red (source error). Use CSS classes on `.connection .main-path` and update them in `applyStatuses()` by mapping source node status to its outgoing connections.

8. **Add a toast notification system** replacing the footer status text for transient messages (save confirmations, errors, import results). Show a small toast in bottom-right that auto-dismisses after 3s. Keep the footer for persistent run status only.

9. **Add node execution order badges.** Show small wave number indicators (e.g., "W1", "W2") on each node during/after a run so the user can see the execution order at a glance. Compute waves client-side using the same topological sort logic already in `autoLayout()`.

10. **Improve the log viewer with ANSI color support and better formatting.** Parse basic ANSI escape codes (colors, bold) in log output and render as styled `<span>` elements. Add a "copy log" button and a "wrap/nowrap" toggle.

11. **Add an unsaved changes indicator.** Track whether the pipeline has been modified since last save. Show a dot/asterisk next to the pipeline name. Add a `beforeunload` handler to warn on tab close with unsaved changes. Use the existing undo stack length as a heuristic.

12. **Add node collapse/expand.** Allow nodes to be collapsed to just icon + label (hiding tools, project pills, descriptions) for cleaner canvas on large pipelines. Toggle via double-click on node header or a small chevron. Save collapse state in node data.

### Frontend — UX Improvements

13. **Add delete confirmation for pipelines.** Currently `deletePipeline()` fires immediately on click with no confirmation. Add a simple confirm dialog or a 3-second undo toast.

14. **Add project path validation in the project modal.** When user enters a path, make a lightweight API call (`GET /api/projects/{id}/validate` — new endpoint) to check if the directory exists and is accessible. Show green check or red X inline.

15. **Improve the config panel prompt editor.** Add a small toolbar above the prompt textarea with buttons to insert common variables (`{task}`, `{read:}`, `{subtask}`, `{review_issues}`, `{var:}`). Clicking inserts at cursor position. This helps users discover available variables.

16. **Add keyboard shortcut help overlay.** Show available shortcuts (Ctrl+S, Ctrl+D, Ctrl+Z, Ctrl+Enter, Delete, Escape, Ctrl+E) in a help panel toggled by `?` key. Small floating panel in bottom-left.

17. **Add node search/filter.** For pipelines with many nodes, add a search input in the sidebar that filters/highlights nodes by label or type. Typing dims non-matching nodes on canvas (reuse the project highlight mechanism).

18. **Add run history list in sidebar.** Show last 5 runs with timestamp and status (pass/fail) below the "Saved" section. Clicking a past run loads its status onto the current canvas nodes (read-only status overlay). Requires reading from `~/.config/clor/runs/` directory.
