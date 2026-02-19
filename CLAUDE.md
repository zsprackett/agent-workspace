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

- **`internal/config/`** - Loads `~/.agent-workspace/config.json`; provides paths for DB, repos, and worktrees.
- **`internal/db/`** - All SQLite access. `types.go` defines `Session`, `Group`, `SessionStatus`, `Tool`. `db.go` has all queries. Single connection, WAL mode. Migrations use `ALTER TABLE ADD COLUMN` with duplicate-column error suppression for additive schema changes.
- **`internal/session/`** - `Manager` handles create/stop/restart/delete/attach. Creating a session always creates a real tmux session. `GenerateTitle()` produces adjective-noun names used as both the session title and git branch name.
- **`internal/tmux/`** - Raw tmux command wrappers. `status.go` contains the status detection logic: `ParseToolStatus` uses regex on the last 30 lines of pane output; `IsPaneWaitingForInput` uses `ps wchan=ttyin` on the pane's foreground process.
- **`internal/monitor/`** - Background goroutine (500ms tick) that polls all tmux sessions, updates statuses in DB, and triggers a UI redraw via `QueueUpdateDraw`.
- **`internal/syncer/`** - Background goroutine (2-minute tick) that runs `git fetch` on all bare repos associated with groups.
- **`internal/git/`** - Bare clone and worktree operations. Bare repos live at `~/.agent-workspace/repos/<host>/<owner>/<repo>.git`; worktrees at `~/.agent-workspace/worktrees/<host>/<owner>/<repo>/<branch>`.
- **`internal/ui/`** - TUI built on `tview`. `app.go` is the controller that wires DB, managers, and dialogs. `home.go` is the main screen. `dialogs/` contains modal forms.

### Status Detection

Session status is resolved by combining two signals in `monitor.refresh()`:
1. `tmux.ParseToolStatus` - regex patterns against last 30 lines of `tmux capture-pane` output; Claude-specific patterns differ from generic tool patterns.
2. `tmux.IsPaneWaitingForInput` - checks `ps -t <tty> -o stat=,wchan=` for a foreground process blocked on `ttyin`.

Claude's idle state (waiting for next message at the prompt) maps to `StatusWaiting`, not `StatusIdle`.

### Tmux Session Names

Names follow the pattern `agws_<sanitized-title>-<hex-timestamp>` where the prefix `agws_` is the constant `tmux.SessionPrefix`.

### Git Worktree Flow (new session with group repo URL)

1. Parse the group's repo URL (`git.ParseRepoURL`)
2. Bare-clone or fetch-update the repo under `ReposDir`
3. Sanitize the session title into a branch name (`git.SanitizeBranchName`)
4. Create a worktree at `WorktreesDir/<host>/<owner>/<repo>/<branch>` from the configured base branch
5. Launch the tmux session with `Cwd` set to the worktree path
6. On session delete: kill tmux session, then `git worktree remove` (with force-delete fallback)

### In-session Key Bindings

Key bindings are set on the tmux session in `tmux.AttachSession` and unbound on detach. The status bar reads running/waiting/total counts from a temp file updated every 5 seconds by a goroutine.
