# Plan: UI Freezes & Missing Live Status During Pipeline Run

## Root Cause Analysis

### Bug 1 — UI "freezes" at start (architect занят, нет обновлений)

**Symptom:** UI freezes after clicking Run; no node updates visible until the architect finishes and produces Plan.md.

**Root cause:** The elapsed counter in `applyStatuses` is driven **only by SSE events**. The orchestrator calls `broadcast()` only when a status *changes* — i.e., when agent output arrives via `watchLogStream`. If the agent is silently running (no stdout for a while), no SSE events fire, and the frontend shows nothing new. The `elapsed` field is only recalculated server-side **on each broadcast**; if there is no broadcast, `elapsed` stays at `0s` in the last snapshot.

Additionally, for the "running 0s" issue: `s.elapsed` is 0 until the *first* `broadcast()` that happens after `StartedAt` is set. If there's a gap between `setStatus("running")` and the first log line, `elapsed` shows `0s`.

### Bug 2 — Coder shows "0s", unclear if running

**Symptom:** Coder node shows `running 0s` indefinitely with no progress visible.

**Root causes:**
1. Same as above — `elapsed` only updates when a new SSE event fires (i.e., when a log line arrives).
2. In `decompose` mode: subtasks run **sequentially** one by one with a single progress bar, but the node label still says `running 0s` if no output from the current subtask has arrived yet.
3. No client-side timer to increment `elapsed` independently of server events.

### Bug 3 — No indication of parallel execution

**Symptom:** When multiple coder nodes run in parallel within a wave, UI doesn't make it obvious.

**Root cause:** The current status display is purely text-based. Nodes in the same wave all show `running Xs`, but there's no visual grouping or "N agents running in parallel" indicator.

---

## Architecture of the Fix

### Fix A — Client-side elapsed ticker (no server changes needed)

The frontend should maintain a local `startedAt` map per node and use `setInterval` to increment the elapsed display every second, independent of SSE events. SSE updates sync the `startedAt` value; the ticker handles the visual increment.

### Fix B — Heartbeat broadcast in orchestrator

For nodes that produce infrequent output (architect thinking deeply), the orchestrator should send a periodic heartbeat `broadcast()` every ~2 seconds so the SSE stream stays alive and `elapsed` updates.

### Fix C — "Running agents" counter in header

Show a real-time counter like `2 agents running in parallel` in the run-status bar when multiple nodes are in `running` state simultaneously.

### Fix D — Better initial status transition

When a node transitions to `running`, immediately broadcast with `elapsed=0` so the frontend sees the state change without waiting for the first log line.

---

## Subtasks

1. **[frontend]** Add client-side elapsed ticker: maintain a `nodeStartTimes` map, update it from SSE/poll events, and run a `setInterval` every 1s to re-render elapsed for all `running` nodes without waiting for new SSE events.

2. **[backend]** Add heartbeat goroutine in `executeNode`: when a node is running, broadcast a status update every 2 seconds so SSE clients receive fresh `elapsed` values even if the agent produces no output.

3. **[frontend]** Add "N agents running" indicator: in `applyStatuses`, count nodes with `status === 'running'` and display `N agent(s) running` in the `#run-status` element alongside the run ID.

4. **[frontend]** Fix "running 0s" on first render: when `s.elapsed === 0` and `s.status === 'running'`, show `running ...` or `running <1s` instead of `running 0s` to avoid the confusing frozen zero.
