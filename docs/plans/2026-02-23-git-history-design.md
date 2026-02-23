# Git History Browser Design

**Goal:** Add a read-only git history browser to the session menu, accessible via `Ctrl+\` then `h`.

---

## Architecture

Follows the same pattern as `notescmd`:

- New `internal/ui/gitlogcmd/gitlogcmd.go` package with a `Run(tmuxSession string) error` function
- New `agent-workspace gitlog <tmux-session-name>` subcommand in `main.go`
- `menucmd` adds an `h` key that schedules a `display-popup` (via `tmux run-shell -b`, same as notes) running `agent-workspace gitlog <tmux-session-name>`
- `gitlogcmd.Run` opens the DB, looks up the session by tmux name to get the worktree path, then launches the tview UI

---

## Layout

```
┌─ Git Log: <session-title> ────────────────────────────────────────────────────┐
│ ┌──────────────────────────────────────┐ ┌───────────────────────────────────┐│
│ │ abc1234  Fix auth bug        2h ago  │ │ commit abc1234...                 ││
│ │ def5678  Add worktree flow   4h ago  │ │ Author: Zac <zac@example.com>     ││
│ │ 9ab1234  Refactor session…   1d ago  │ │ Date:   2 hours ago               ││
│ │                                      │ │                                   ││
│ │                                      │ │ Fix authentication for terminal   ││
│ │                                      │ │                                   ││
│ │                                      │ │ diff --git a/internal/...         ││
│ │                                      │ │ -old line                         ││
│ │                                      │ │ +new line                         ││
│ └──────────────────────────────────────┘ └───────────────────────────────────┘│
│  [Tab] switch pane   [↑↓] navigate   [Esc] close                              │
└───────────────────────────────────────────────────────────────────────────────┘
```

Left pane (~45% width): `tview.Table`, three columns -- short hash, subject (truncated), relative date. Right pane (~55% width): `tview.TextView` with ANSI color enabled, renders `git show --color=always <hash>`.

---

## Data Flow

1. On open: run `git -C <path> log --pretty=format:'%h\t%s\t%ar' -n 200` to populate the table
2. On row focus change: run `git -C <path> show --color=always <hash>` and write output to the TextView via `tview.ANSIWriter`
3. The diff pane scrolls independently when focused; Tab toggles focus between panes
4. Esc closes regardless of which pane has focus

---

## Key Bindings

- `↑` / `↓`: navigate commits in list pane (when list focused)
- `j` / `k`: scroll diff pane (when diff focused)
- `Tab`: toggle focus between list and diff pane
- `Esc`: close from either pane

---

## Error Handling

- If the session is not found in the DB or has no worktree path: print an error and exit (same behavior as `notescmd`)
- If `git log` returns no output (empty repo or detached HEAD with no commits visible): show "No commits found" in the list pane, leave diff pane blank
- If `git show` fails: display the error text in the diff pane rather than crashing

---

## Menu Change

Add `h` to `menucmd`, placed between `d` (diff) and `p` (open PR) so git operations are grouped:

```
[green]h[-]  Git history
```
