# Agent Workspace

A terminal-based session manager for AI coding tools. Manages multiple Claude, OpenCode, Gemini, Codex, or custom shell sessions in tmux with Git worktree integration and a TUI built on [tview](https://github.com/rivo/tview).

## Features

- **Session management** - Create, start, stop, restart, and delete tool sessions
- **Group organization** - Organize sessions into named groups
- **Git worktree integration** - Automatically creates isolated Git worktrees from a GitHub URL set on a group
- **Pre-launch commands** - Run a shell command before each new session; on failure shows output and asks whether to continue
- **Live status monitoring** - Detects running, waiting, idle, error, and stopped states by parsing tmux output
- **Persistent state** - Stores session metadata in SQLite at `~/.agent-workspace/state.db`
- **In-session shortcuts** - Keyboard bindings available while attached to a session

## Requirements

- Go 1.21+
- tmux

## Installation

```bash
make install
```

This builds and installs the `agent-workspace` binary to `$GOPATH/bin`.

## Usage

```bash
agent-workspace
```

The TUI opens with a dual-column layout: session list on the left, session details preview on the right.

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `n` | New session |
| `d` | Delete session |
| `s` | Stop session |
| `x` | Restart session |
| `e` | Edit session |
| `g` | New group |
| `m` | Move session to group |
| `?` | Help |
| `q` | Quit |
| `Enter` | Attach to session |

### In-session shortcuts (while attached)

| Key | Action |
|-----|--------|
| `Ctrl+G` | Git status |
| `Ctrl+F` | Git diff |
| `Ctrl+P` | GitHub PR view |
| `Ctrl+T` | Split pane |
| `Ctrl+D` | Detach |

## Configuration

Config is loaded from `~/.agent-workspace/config.json`. All fields are optional.

```json
{
  "default_tool": "claude",
  "default_group": "my-sessions",
  "worktree_base_branch": "main"
}
```

Data directory: `~/.agent-workspace/`

## Git Worktree Integration

Set a GitHub URL on a group and new sessions in that group will automatically:

1. Clone the repo as a bare clone under `~/.agent-workspace/repos/`
2. Create an isolated Git worktree under `~/.agent-workspace/worktrees/`
3. Launch the tool session in that worktree directory

Worktrees are removed when the session is deleted.

## Pre-launch Commands

Set a **Pre-launch command** on a group to run a shell command before each new session is created. The command receives positional arguments describing the session context:

- **No worktree:** `<cmd> <tool> <project-path>`
- **With worktree:** `<cmd> <tool> <bare-repo-path> <worktree-path>`

`<tool>` is the tool name (e.g. `claude`, `opencode`).

If the command exits non-zero, a dialog shows the error output and asks whether to continue or cancel session creation. Full output is always logged to `~/.agent-workspace/prelaunch.log`.

Example use cases: copy shared config files into the worktree, install dependencies, validate environment prerequisites.

## Supported Tools

- `claude` - Claude Code CLI
- `opencode` - OpenCode
- `gemini` - Gemini CLI
- `codex` - OpenAI Codex CLI
- `shell` - Custom shell command

## Status Icons

| Icon | Status |
|------|--------|
| `●` | Running |
| `◐` | Waiting for input |
| `○` | Idle |
| `◻` | Stopped |
| `✗` | Error |

## Development

```bash
make build    # Build binary
make test     # Run tests
make install  # Build and install
```
