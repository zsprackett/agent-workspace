# Agent Workspace

A terminal-based session manager for AI coding tools. Manages multiple Claude, OpenCode, Gemini, Codex, or custom command sessions in tmux with Git worktree integration and a TUI built on [tview](https://github.com/rivo/tview).

## Features

- **Session management** - Create, start, stop, restart, and delete tool sessions
- **Group organization** - Organize sessions into named groups
- **Git worktree integration** - Automatically creates isolated Git worktrees from a GitHub URL set on a group
- **Live status monitoring** - Detects running, waiting, idle, error, and stopped states by parsing tmux output
- **Dirty worktree indicator** - `*` prefix on session rows when the worktree has uncommitted changes
- **Notifications** - macOS system alert (and optional webhook) when a session transitions to waiting for input
- **Session notes** - Per-session freeform notes, editable from the dashboard or from within a session
- **Persistent state** - Stores session metadata in SQLite at `~/.agent-workspace/state.db`
- **In-session shortcuts** - Keyboard bindings and mouse scrolling available while attached to a session

## Requirements

- Go 1.21+
- tmux

## Installation

```bash
make install
```

This builds and installs the `agent-workspace` binary to `~/.local/bin`.

## Usage

```bash
agent-workspace
```

The TUI opens with a dual-column layout: session list on the left, session preview on the right.

### Dashboard shortcuts

| Key | Action |
|-----|--------|
| `n` | New session (on group) / Edit notes (on session) |
| `d` | Delete session or group |
| `s` | Stop session |
| `x` | Restart session |
| `e` | Edit session or group |
| `g` | New group |
| `m` | Move session to group |
| `1`-`9` | Jump to group |
| `?` | Help |
| `q` | Quit |
| `Enter` / `a` | Attach to session |
| `←` / `→` | Collapse / expand group |

### In-session shortcuts (while attached)

Press `Ctrl+\` to open the command menu. A floating legend appears -- press the highlighted key to run the action or `Esc` to cancel.

| Key | Action |
|-----|--------|
| `s` | Git status |
| `d` | Git diff |
| `p` | Open GitHub PR in browser |
| `n` | View / edit session notes |
| `t` | Open terminal split |
| `x` | Detach to dashboard |

Mouse mode is enabled while attached: scroll wheel navigates history, click-drag selects text. Hold `Option`/`Alt` to bypass tmux mouse handling for terminal-level copy.

## Configuration

Config is loaded from `~/.agent-workspace/config.json`. All fields are optional.

```json
{
  "defaultTool": "claude",
  "defaultGroup": "my-sessions",
  "worktree": {
    "defaultBaseBranch": "main"
  },
  "notifications": {
    "enabled": true,
    "webhook": "https://hooks.slack.com/..."
  }
}
```

### Notifications

When `notifications.enabled` is `true`, a macOS system notification fires whenever a session transitions to the waiting state. Set `notifications.webhook` to receive a JSON POST as well:

```json
{
  "session": "swift-fox",
  "tool": "claude",
  "group": "my-sessions",
  "status": "waiting",
  "timestamp": "2026-02-18T12:00:00Z"
}
```

Data directory: `~/.agent-workspace/`

## Git Worktree Integration

Set a GitHub URL on a group and new sessions in that group will automatically:

1. Clone the repo as a bare clone under `~/.agent-workspace/repos/`
2. Create an isolated Git worktree under `~/.agent-workspace/worktrees/`
3. Launch the tool session in that worktree directory

The `*` indicator appears on a session row when the worktree has uncommitted changes. It is updated after each background `git fetch` and whenever you detach from a session.

Worktrees are removed when the session is deleted.

## Session Notes

Press `n` on any session row to open an editable notes modal. Notes persist in SQLite across restarts.

While attached to a session, press `Ctrl+\` to open the command menu, then `n` to open the same notes editor as a floating popup without detaching.

## Supported Tools

- `claude` - Claude Code CLI
- `opencode` - OpenCode
- `gemini` - Gemini CLI
- `codex` - OpenAI Codex CLI
- `custom` - Any command you specify (e.g. `/bin/bash`, `my-tool --flag`)

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
