# Plan: Fix leftover files in project directories

## Problem

After a pipeline run, two files may remain in the user's project directory:

1. **`plan.md`** — created by the Architect agent (its prompt instructs it to write there). Nodes have a `TempFiles` field for cleanup, but the Architect preset doesn't populate it by default, so the file is never cleaned up.

2. **`.clor-last-output`** — written by `agent.go:122` unconditionally to `workdir+"/.clor-last-output"` after every successful agent run. This is a debug artifact with no cleanup logic at all.

## Root causes

### `.clor-last-output`
In `agent.go` line 122:
```go
os.WriteFile(workdir+"/.clor-last-output", stdout.Bytes(), 0644)
```
This writes the full agent stdout to the project directory on every run. It was likely added for debugging. The output is already captured in the proper log file under `~/.config/clor/runs/{runID}/logs/{nodeID}.log`. The `.clor-last-output` file is redundant and has no cleanup.

### `plan.md`
The Architect agent prompt says "Write the full plan to plan.md". The `TempFiles` mechanism exists in `orchestrator.go` (`cleanupTempFiles()`) but the Architect node preset in `web/index.html` doesn't set `temp_files: ["plan.md"]` by default, so the file is never removed.

## Solution

### Fix 1: Remove `.clor-last-output` entirely
The output is already stored in the proper run log (`logsDir/nodeID.log`). There is no need to also write it to the project workdir. Delete this line from `agent.go`.

### Fix 2: Add `plan.md` to Architect preset's TempFiles
In `web/index.html`, the Architect node preset should include `temp_files: ["plan.md"]` so the orchestrator's existing `cleanupTempFiles()` mechanism removes it after the run completes.

## Architectural constraints
- No new abstractions needed — the `TempFiles` cleanup mechanism already exists and works.
- The run logs (`~/.config/clor/runs/`) are already the canonical store for agent output. Removing `.clor-last-output` does not lose any information.
- `cleanupTempFiles()` is called at the end of every run in `orchestrator.go:105`, so Fix 2 requires only a data change in the preset definition.

## Subtasks
1. Remove the `.clor-last-output` write from `agent.go:122` (the `os.WriteFile` call and its surrounding `if logStream != nil` block, keeping the return statement)
2. Add `temp_files: ["plan.md"]` to the Architect node preset definition in `web/index.html`
