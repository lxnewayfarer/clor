# Canvas UX Improvement Plan

## Context & Goals

The current canvas has three pain points:
1. **Port rigidity** — inputs always on left, outputs always on right. When nodes are arranged vertically or in complex topologies, connection lines look messy.
2. **Layout alignment** — `autoLayout()` aligns nodes to the top of each wave-column. Nodes of different heights cause diagonal/uneven connection lines. Parallel connections are not considered.
3. **Subtask visibility** — the subtask progress bar (amber fill + `index/total` text) exists but is minimal; there's no clear per-subtask breakdown visible in the node body during a run.

---

## Architectural Decisions & Constraints

- **No new dependencies.** Drawflow is the only canvas library; all improvements must work within its model.
- **Drawflow port model.** Drawflow hard-codes input ports on the left and output ports on the right of a node. True floating/dynamic ports require either patching Drawflow's CSS/JS or replacing it. We will use **CSS transforms + absolute positioning tricks** to simulate centered ports rather than replacing Drawflow entirely.
- **Layout algorithm** lives entirely in `autoLayout()` in `web/index.html`. No Go-side changes needed for layout.
- **Subtask data** is already emitted by the backend in status events (`subtask_index`, `subtask_total`, `subtask_label`). The frontend just needs richer rendering.
- All changes are confined to `web/index.html`.

---

## Shared Technical Approach

### 1. Flexible Port Positioning (CSS-only)

Drawflow renders `.input` and `.output` dots as absolutely-positioned children of `.drawflow-node` using Drawflow's internal layout. To make them appear vertically centered on the node (regardless of height), we override:

```css
.drawflow .drawflow-node .input  { top: 50% !important; transform: translateY(-50%); }
.drawflow .drawflow-node .output { top: 50% !important; transform: translateY(-50%); }
```

This makes all connection lines land at the vertical midpoint of each node, eliminating the "crooked lines on different-height nodes" problem without touching Drawflow internals.

### 2. Improved Layout: Center-align nodes within each wave column

Replace top-alignment with vertical centering. The total column height = `n * NODE_HEIGHT + (n-1) * NODE_GAP`. Center that block around a shared `midY`.

Also account for parallel edges: when two nodes in the same wave both connect to the same target, offset them vertically so lines don't overlap.

### 3. Subtask Breakdown in Node Body

During a run, expand the subtask section inside the node to show a numbered list of completed/active/pending subtasks (up to a cap, e.g. 5 visible), similar to a mini checklist. Each subtask line gets a status icon (✓ done, ● active, ○ pending).

---

## Subtasks

1. Override Drawflow port CSS so input and output dots are always vertically centered on each node, fixing crooked connection lines when nodes have different heights.
2. Rewrite `autoLayout()` to center-align nodes vertically within each wave column (instead of top-aligning), using measured or estimated node heights, so connection lines stay horizontal between same-wave peers.
3. Improve subtask display inside the node body: render a scrollable mini-list of up to 5 subtask entries with done/active/pending icons, replacing the single-line text, so the user can see which subtasks have completed during a run.
