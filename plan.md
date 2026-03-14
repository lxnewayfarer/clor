# Plan: Clean up temp files before Committer agent runs

## Problem

Currently `cleanupTempFiles()` is called at line 105 of `orchestrator.go` — **after all waves complete**. This means temporary files (e.g. `plan.md` from an Architect node) still exist on disk when the Committer agent runs `git add -A`, causing them to be staged and committed.

The Committer is identified by its label (preset: `"Committer"`), similar to how reviewers are detected via `isReviewerNode()` which checks `strings.Contains(label, "review")`.

## Approach

Move temp file cleanup to happen **before** any Committer node executes, inside `executeNode()`. This follows the same pattern as `isReviewerNode()` — add an `isCommitterNode()` helper and call `cleanupTempFiles()` early in `executeNode()` when the node is a committer.

Keep the existing end-of-run `cleanupTempFiles()` call as a safety net (idempotent since `os.Remove` on a missing file is a no-op).

### Key files
- `orchestrator.go` — add `isCommitterNode()`, call cleanup before committer execution

## Subtasks

1. Add `isCommitterNode(n NodeConfig) bool` helper in `orchestrator.go` (next to `isReviewerNode`) that returns `true` when `strings.ToLower(n.Label)` contains `"commit"`
2. In `executeNode()`, after the task-node early return (line ~114) and before prompt expansion, add a check: if `isCommitterNode(node)` then call `o.cleanupTempFiles()` — this ensures temp files are removed before the committer agent sees the working directory
