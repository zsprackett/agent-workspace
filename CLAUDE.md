# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build    # Build the agent-workspace binary
make test     # Run all tests
make install  # Build and install to ~/.local/bin
```

Run a single package's tests:
```bash
go test ./internal/tmux/...
go test ./internal/db/...
```

Run a single test by name:
```bash
go test ./internal/tmux/... -run TestParseToolStatus
```

## Architecture

`agent-workspace` is a TUI session manager for AI coding tools. It wraps tmux sessions in a tview UI, persists state to SQLite, and integrates with git worktrees.

### Data Flow

```
main.go → config + db + ui.NewApp
ui.App  → session.Manager (CRUD) + monitor.Monitor (status) + syncer.Syncer (git fetch)
```

### Package Roles

- **`internal/config/`** - Loads `~/.agent-workspace/config.json`; provides paths for DB, repos, and worktrees. Includes `NotificationsConfig` (enabled, webhook).
- **`internal/db/`** - All SQLite access. `types.go` defines `Session`, `Group`, `SessionStatus`, `Tool`. `db.go` has all queries. Single connection, WAL mode. Migrations use `ALTER TABLE ADD COLUMN` with duplicate-column error suppression for additive schema changes. `Session` carries `HasUncommitted bool` and `Notes string`. `Group` carries `PreLaunchCommand string`. `session_events` timestamps are stored as Unix milliseconds in `ts_ms INTEGER` (not the legacy `ts DATETIME` column).
- **`internal/session/`** - `Manager` handles create/stop/restart/delete/attach. Creating a session always creates a real tmux session. `GenerateTitle()` produces adjective-noun names used as both the session title and git branch name. On detach, updates `has_uncommitted` via `git.IsWorktreeDirty`. `prelaunch.go` provides `RunPreLaunchCommand(cmd, args...)` which runs a shell command with positional args appended (tool name, bare repo path, worktree path).
- **`internal/tmux/`** - Raw tmux command wrappers. `status.go` contains the status detection logic: `ParseToolStatus` uses regex on the last 30 lines of pane output; `IsPaneWaitingForInput` uses `ps wchan=ttyin` on the pane's foreground process.
- **`internal/monitor/`** - Background goroutine (500ms tick) that polls all tmux sessions, updates statuses in DB, and triggers a UI redraw via `QueueUpdateDraw`. Uses hysteresis: a status change is only committed after the new status is observed on two consecutive ticks, preventing spurious events during session startup. Tracks previous statuses to fire `notify.Notifier.Notify` on transitions to `waiting`.
- **`internal/notify/`** - `Notifier` sends macOS system notifications via `osascript` and optional JSON webhook POSTs when a session needs input.
- **`internal/syncer/`** - Background goroutine (2-minute tick) that runs `git fetch` on all bare repos associated with groups. After each fetch, updates `has_uncommitted` for sessions in that group.
- **`internal/git/`** - Bare clone and worktree operations. Bare repos live at `~/.agent-workspace/repos/<host>/<owner>/<repo>.git`; worktrees at `~/.agent-workspace/worktrees/<host>/<owner>/<repo>/<branch>`. `IsWorktreeDirty` runs `git status --porcelain`.
- **`internal/ui/`** - TUI built on `tview`. `app.go` is the controller that wires DB, managers, and dialogs. `home.go` is the main screen. `dialogs/` contains modal forms including `notes.go`. `notescmd/` is the standalone notes editor invoked as a subcommand inside tmux display-popup.

### Status Detection

Session status is resolved by combining two signals in `monitor.refresh()`:
1. `tmux.ParseToolStatus` - regex patterns against last 30 lines of `tmux capture-pane` output; Claude-specific patterns differ from generic tool patterns.
2. `tmux.IsPaneWaitingForInput` - checks `ps -t <tty> -o stat=,wchan=` for a foreground process blocked on `ttyin`.

Claude's idle state (waiting for next message at the prompt) maps to `StatusWaiting`, not `StatusIdle`.

### Tmux Session Names

Names follow the pattern `agws_<sanitized-title>-<hex-timestamp>` where the prefix `agws_` is the constant `tmux.SessionPrefix`.

### Pre-launch Commands

Groups support an optional `PreLaunchCommand` string. When a new session is created in a group that has one set, `session.RunPreLaunchCommand` is called before the tmux session is started. The command receives positional args:

- For worktree sessions: `<tool-cmd> <bare-repo-path> <worktree-path>`
- For non-worktree sessions: `<tool-cmd> <project-path>`

If the command exits non-zero, session creation is aborted and the error output is shown in a dialog.

### Restart Behavior

Pressing `x` (restart) on a session:
- If status is `stopped` or `error`: restarts immediately with no confirmation.
- Otherwise: shows a confirmation modal before restarting.

### Git Worktree Flow (new session with group repo URL)

1. Parse the group's repo URL (`git.ParseRepoURL`)
2. Bare-clone or fetch-update the repo under `ReposDir`
3. Sanitize the session title into a branch name (`git.SanitizeBranchName`)
4. Create a worktree at `WorktreesDir/<host>/<owner>/<repo>/<branch>` from the configured base branch
5. Run the group's pre-launch command if set (`session.RunPreLaunchCommand`)
6. Launch the tmux session with `Cwd` set to the worktree path
7. On session delete: kill tmux session, then `git worktree remove` (with force-delete fallback)

### In-session Key Bindings

A single leader key `Ctrl+\` is bound on attach and unbound on detach. Pressing it opens a `tmux display-popup` running `agent-workspace menu <tmux-session-name>`, handled by the `menu` subcommand in `main.go` via `internal/ui/menucmd`. The popup CWD is the active pane's directory (set by tmux), so `menucmd` reads it via `os.Getwd()`.

Menu keys: `s` git status, `d` git diff, `p` open PR, `n` notes, `t` terminal split, `x` detach.

The status bar reads running/waiting/total counts from a temp file updated every 5 seconds by a goroutine. Mouse mode is enabled on attach and disabled on detach.

### Subcommands

- `agent-workspace menu <tmux-session-name>` -- opens the command menu legend (`internal/ui/menucmd`). Dispatches tmux commands on keypress; for notes, opens a nested display-popup running the notes subcommand.
- `agent-workspace notes <tmux-session-name>` -- opens a standalone tview notes editor (`internal/ui/notescmd`). Looks up the session via `db.GetSessionByTmuxName`, shows an editable TextArea pre-filled with existing notes, saves to SQLite on Save.
