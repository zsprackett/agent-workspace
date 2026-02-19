# Pre-Launch Command for Groups

**Date:** 2026-02-18

## Summary

Add a `PreLaunchCommand` field to the `Group` type. When a new session is created in a group that has this field set, the command is executed synchronously before the tool is launched. If the command exits non-zero, the user is shown a warning dialog and asked whether to proceed or abort.

## Argument Contract

The command is a plain shell command string. Positional arguments are appended based on context:

- No worktree: `<cmd> <tool> <project-path>`
- With worktree: `<cmd> <tool> <bare-repo-path> <worktree-path>`

Where:
- `<tool>` is the tool identifier string (e.g. `claude`, `opencode`, `gemini`)
- `<project-path>` is the session's working directory
- `<bare-repo-path>` is the bare git repo path (e.g. `~/.agent-workspace/repos/github.com/owner/repo.git`)
- `<worktree-path>` is the checked-out worktree path (e.g. `~/.agent-workspace/worktrees/github.com/owner/repo/branch`)

## Data Model

Add `PreLaunchCommand string` to `db.Group`.

Add column to the `groups` table:

```sql
ALTER TABLE groups ADD COLUMN pre_launch_command TEXT NOT NULL DEFAULT ''
```

Migration uses the existing `ALTER TABLE ADD COLUMN` + duplicate-column-error suppression pattern.

## UI Changes

`GroupDialog` gains a third input field: `Pre-launch command (optional)`. Placeholder hint shows the argument format. `GroupResult` gains `PreLaunchCommand string`. Dialog height increases from 12 to 14.

## Execution

Add `RunPreLaunchCommand(cmd, tool, arg2 string, arg3 ...string) (string, error)` in `internal/session/` (or a new `internal/session/prelaunch.go`). It splits `cmd` via `strings.Fields`, appends positional args, and runs via `exec.Command`. Captures combined stdout+stderr for display in the warning dialog.

In `app.go`'s `onNew()`, after the project path or worktree path is resolved but before `createSession()` is called:
1. If `group.PreLaunchCommand != ""`, run the command.
2. On success (exit 0), call `createSession()` immediately.
3. On failure, show a modal with the command output and "Continue" / "Cancel" buttons. "Continue" calls `createSession()`; "Cancel" aborts.

## Files Affected

- `internal/db/types.go` - add `PreLaunchCommand` to `Group`
- `internal/db/db.go` - migration, `SaveGroups`, `LoadGroups`
- `internal/session/prelaunch.go` - new file, `RunPreLaunchCommand`
- `internal/ui/dialogs/group.go` - add input field, update `GroupResult`
- `internal/ui/app.go` - wire pre-launch execution in `onNew()`
