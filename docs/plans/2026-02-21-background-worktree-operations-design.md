# Background Worktree Operations

**Date:** 2026-02-21

## Problem

Creating and deleting sessions with worktrees blocks the entire TUI. Clone, fetch, worktree creation, and pre-launch commands all run synchronously on the UI goroutine, freezing the interface until they complete.

## Goal

Keep the UI fully interactive during these operations. Sessions appear in the list immediately with a visible status, and the user can navigate or create other sessions while work proceeds in the background.

## Approach

New DB status constants (`StatusCreating`, `StatusDeleting`) track in-progress operations. Background goroutines perform the blocking git work and use `QueueUpdateDraw` to update the UI on completion or failure.

---

## Section 1: DB and Status Constants

Add to `internal/db/types.go`:

```go
StatusCreating SessionStatus = "creating"
StatusDeleting SessionStatus = "deleting"
```

No schema changes required -- `status` is already a `TEXT` column.

**Monitor:** Skip sessions with `StatusCreating` or `StatusDeleting` -- no tmux polling, no status writes, no notifications.

**Home list rendering:** Display `"creating..."` / `"deleting..."` in the status column using a muted color (same palette as `stopped`/`error`). Rows in these states are not selectable for attach, restart, stop, or edit -- keypress handlers no-op if status is `StatusCreating` or `StatusDeleting`.

---

## Section 2: Create Flow

After the new session dialog is submitted:

1. Resolve title (generate if empty). Build a `db.Session` row with `StatusCreating` and no `TmuxSession`. Insert to DB, call `refreshHome()` -- session appears in list immediately as "creating...".
2. Start a goroutine that performs all blocking work:
   - `git.CloneBare` or `git.FetchBare`
   - `git.CreateWorktree`
   - `session.RunPreLaunchCommand` (if set)
3. **On success:** create the tmux session, update the DB row (`TmuxSession`, `StatusRunning`, `ProjectPath`, worktree fields). `QueueUpdateDraw` to `refreshHome()` and `onAttachSession()`.
4. **On failure:** delete the DB row. `QueueUpdateDraw` to `refreshHome()` and `showError()`.

**Worktree-already-exists path:** The modal moves into the goroutine. It triggers `QueueUpdateDraw` to display the modal; the "Reuse" branch resumes the goroutine via a channel, the "Cancel" branch deletes the DB row and refreshes.

---

## Section 3: Delete Flow

After the user confirms deletion (or immediately if session is already stopped):

1. Kill the tmux session synchronously (fast). Update DB status to `StatusDeleting`. Call `refreshHome()` -- session shows "deleting..." in list.
2. Start a goroutine that runs `git.RemoveWorktree(false)`.
3. **On success:** `a.mgr.Delete()`. `QueueUpdateDraw` to `refreshHome()`.
4. **On failure:** `QueueUpdateDraw` to show force-delete modal.
   - User picks "Force Delete": start another goroutine with `git.RemoveWorktree(true)`.
     - Success: `a.mgr.Delete()`, `refreshHome()`.
     - Failure: restore previous status, `refreshHome()`, `showError()`.
   - User picks "Cancel": restore previous status, `refreshHome()`.

Sessions without a worktree (`WorktreePath == ""`) are unaffected -- they delete synchronously as today.

---

## Section 4: Startup Cleanup

On `App.Run()`, before starting the monitor, query for sessions with `StatusCreating` or `StatusDeleting`:

- **`StatusCreating`:** Delete the DB row. The session never got a tmux session; the worktree may be partially created on disk but is not recoverable.
- **`StatusDeleting`:** The tmux session was already killed. Attempt `git.RemoveWorktree(force=true)`, then delete the DB row regardless of outcome.

This prevents orphaned rows from persisting across crashes.
