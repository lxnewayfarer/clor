# clor — Improvement Plan

## Analysis Summary

After reviewing the full codebase (Go backend + embedded frontend), I identified functional gaps, UX issues, and visual improvements across both layers.

---

## Shared Context & Constraints

- **Single-binary architecture**: everything lives in Go files + one embedded `web/index.html`. No build step, no npm.
- **No new dependencies**: stdlib-only Go, vanilla JS + Drawflow CDN.
- **Backward-compatible**: existing pipeline JSON format must keep working (additive changes only).
- **All CSS/JS is inline** in `web/index.html` — changes must stay within that file or Go source files.

---

## Functional Improvements

### F1. Run History & Replay
Currently runs are fire-and-forget — once the page reloads, there's no way to see past runs. The `~/.config/clor/runs/` directory stores status.json and logs, but the UI never lists them.

### F2. Pipeline Duplicate
No way to clone an existing pipeline. Users must export → rename → import.

### F3. Node Copy/Paste Between Pipelines
Ctrl+D duplicates within one canvas, but there's no cross-pipeline clipboard.

### F4. Confirm Before Destructive Actions
Deleting a pipeline or node has no confirmation — single misclick destroys work.

### F5. Search/Filter in Sidebar
When many pipelines or projects exist, there's no way to filter the sidebar lists.

### F6. Connection Validation
Drawflow allows connecting any output to any input. There's no guard against task→task or creating cycles in the editor (only caught at run time).

### F7. Node Elapsed Time Display Improvement
Elapsed time shows raw seconds (`running 127s`). Should show `2m 7s` for readability.

### F8. Log Viewer: Auto-scroll Toggle
Log viewer auto-scrolls to bottom on every chunk. When user scrolls up to read earlier output, it snaps back. Need a toggle or smart auto-scroll (only if already at bottom).

---

## Visual / UX Improvements

### V1. Toast Notifications Instead of Footer Status
Currently all feedback (saved, loaded, errors) goes to a tiny footer `<span>`. Replace with ephemeral toast notifications that stack and auto-dismiss.

### V2. Connection Lines Styling
Connections are plain gray lines. Add color coding: green for task→agent, purple for agent→reviewer, amber for review loop return path. Also increase stroke-width on hover for better visibility.

### V3. Empty State for Canvas
When no nodes are on canvas, show a centered helper: "Drag agents from the sidebar or double-click a preset to begin."

### V4. Node Status Animated Icons
Replace text-only status with animated micro-icons: spinning gear for running, checkmark for done, X for error, hourglass for queued.

### V5. Dark/Light Theme Toggle
The app is dark-only. Add a theme toggle in the header that switches CSS variables for a light theme.

### V6. Config Panel Sections Collapsible
The config panel is a long scroll. Group fields into collapsible sections (General, Prompt, Tools, Artifacts, Advanced) with disclosure triangles.

### V7. Minimap / Zoom Controls
Large pipelines are hard to navigate. Add zoom in/out/reset buttons in the canvas corner and optionally a small minimap overlay.

### V8. Keyboard Shortcut Help Overlay
No discoverability for shortcuts (Ctrl+S, Ctrl+D, Ctrl+Z, Ctrl+E, Ctrl+Enter, Delete, Escape). Add a `?` button or Ctrl+/ overlay showing all shortcuts.

---

## Subtasks

1. **Toast notification system** — Replace footer `#run-status` text updates with a stacking toast system: add `.toast-container` fixed bottom-right, `showToast(message, level)` JS function (`info`/`success`/`error`/`warning`), auto-dismiss after 4s with CSS transition. Replace all `textContent = '...'` assignments to `#run-status` with `showToast()` calls. Keep footer bar but show last status there too.

2. **Empty canvas state** — Add a centered overlay `#empty-state` div inside `#canvas` with instructional text and icon. Show it when there are zero Drawflow nodes; hide it on first `nodeCreated` event. Style it with muted text, large icon, and subtle border.

3. **Run history sidebar section** — Add `GET /api/runs` endpoint in `server.go` that lists `~/.config/clor/runs/` directories sorted by modification time (newest first, limit 20). Return `{runs: [{id, created_at, status_summary}]}`. In the sidebar add a "History" section. Each item shows run ID (truncated), timestamp, and status (computed from status.json). Clicking loads the status onto the current canvas nodes. Add a delete button per run.

4. **Pipeline duplicate button** — In the sidebar pipeline list, add a duplicate icon next to each pipeline name. `duplicatePipeline(name)` fetches the pipeline JSON, appends `-copy` to the name, POSTs it as a new pipeline, and refreshes the list.

5. **Confirmation dialogs for destructive actions** — Add a reusable `confirmDialog(message)` that returns a Promise<boolean> using a modal overlay. Gate `deletePipeline()`, `deleteNodeBtn()`, and `editor.clear()` (on load/import that replaces existing canvas) behind this confirm. Style the modal with red accent for destructive actions.

6. **Connection line color coding** — Override Drawflow SVG path styles via `connectionCreated` event: inspect source/target node types and apply CSS classes. Task→Agent = green stroke, Agent→Reviewer = purple stroke, any→Committer = coral stroke, default = current gray. Add corresponding CSS rules with `:hover` thickening.

7. **Formatted elapsed time** — In `applyStatuses()` JS, replace raw `s.elapsed + 's'` with a `fmtElapsed(seconds)` helper that returns `Xs` for <60, `Xm Ys` for <3600, `Xh Ym` otherwise. Apply the same formatting in headless Go output (already done in `formatDuration` but not used for node status display).

8. **Log viewer smart auto-scroll** — Track scroll position in the log viewer: only auto-scroll if user is within 50px of the bottom. Add a small "⬇ Follow" button that appears when not following, clicking it scrolls to bottom and re-enables follow mode.

9. **Dark/Light theme toggle** — Define `:root[data-theme="light"]` CSS variables (white bg, dark text, adjusted accents). Add a toggle button in `<header>` that flips `document.documentElement.dataset.theme` between `dark` and `light`. Persist choice in `localStorage`. Default to dark.

10. **Config panel collapsible sections** — Wrap config panel groups in `<details><summary>Section Name</summary>...</details>` elements. Sections: General (label, description, project), Prompt, Tools, Artifacts, Advanced (temp files, interactive, decompose, review). Style `<summary>` with disclosure triangle and section header styling. All sections open by default.

11. **Zoom controls overlay** — Add a fixed-position control group in the bottom-right of `#canvas`: zoom in (+), zoom out (-), reset (fit), and percentage display. Wire to `editor.zoom_in()`, `editor.zoom_out()`, `editor.zoom_reset()`. Style as a small floating toolbar matching the dark theme.

12. **Keyboard shortcut help overlay** — Add a `?` button in the header (or Ctrl+/ handler) that opens a modal listing all keyboard shortcuts in a two-column grid: shortcut key → description. Include: Ctrl+S (Save), Ctrl+D (Duplicate), Ctrl+Z/Shift+Z (Undo/Redo), Ctrl+E (Export), Ctrl+Enter (Run/Stop), Delete (Remove node), Escape (Deselect/Close).

13. **Sidebar search/filter** — Add a search input at the top of `#sidebar`. On input, filter the pipeline list and project list to show only items matching the query (case-insensitive substring match). Clear button to reset. Compact styling that doesn't take much space.

14. **Cycle detection on connection** — Hook into Drawflow's `connectionCreated` event. After each new connection, run a quick cycle check on the current graph. If a cycle is detected, immediately remove the connection with `editor.removeSingleConnection()` and show a toast warning "Connection would create a cycle". Also prevent task→task connections (no meaningful data flow).

15. **Node status animated icons** — Replace plain text status prefixes with inline SVG/CSS icons: spinning gear (⚙ with CSS `@keyframes spin`) for running, animated dots for queued, green checkmark for done, red X for error, purple pulse for waiting_for_input. Keep text after the icon for details.
