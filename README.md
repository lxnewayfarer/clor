# clor

Visual pipeline editor for multi-agent [Claude Code](https://docs.anthropic.com/en/docs/claude-code) workflows. Build DAGs of Claude agents that work across multiple project directories — with review loops, file delivery, and parallel execution.

Single binary. No runtime dependencies.

## Install

```bash
# Build from source
go build -o clor -ldflags="-s -w" .
cp clor ~/.local/bin/   # or /usr/local/bin/
```

Requires: Go 1.22+, `claude` CLI in PATH.

## Usage

```bash
clor                    # start web UI on :998
clor -p 3000            # custom port
clor --no-browser       # don't auto-open browser
clor run pipeline.json  # headless execution
clor version
```

## How it works

1. **Register projects** — point clor at your project directories
2. **Build a pipeline** — drag agent nodes onto the canvas, connect them into a DAG
3. **Configure agents** — set prompts, allowed tools, target projects, output artifacts
4. **Run** — clor executes agents in topological waves, parallelizing independent nodes

### Node types

- **Task** — provides a description to downstream agents via `{task}`
- **Agent** — runs `claude -p` in a project directory with a prompt and tool set
- **Reviewer** — agent with review loop: parses output for PASS/FAIL, sends fixes back to coders
- **Report** — summary agent that runs after everything else

### Prompt variables

| Variable | Expands to |
|----------|-----------|
| `{task}` | Task node description |
| `{read:file.go}` | Contents of file from agent's project |
| `{read:alias:file.go}` | Contents of file from another project (by alias) |
| `{files:src/}` | List of files under path |
| `{review_issues}` | Extracted issues from review.md |

## Data

All data lives in `~/.config/clor/`:

```
~/.config/clor/
├── projects.json       # registered project directories
├── pipelines/          # saved pipeline configs (JSON)
└── runs/               # execution logs and status
```

## License

MIT
