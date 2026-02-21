# Background Worktree Operations Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make git worktree create/delete operations non-blocking so the TUI stays interactive while slow git operations run in background goroutines.

**Architecture:** Add `StatusCreating`/`StatusDeleting` DB status constants. For create: insert the session row immediately with `StatusCreating`, run git work in a goroutine, update to `StatusRunning` or delete on failure. For delete: update to `StatusDeleting`, run worktree removal in a goroutine, delete the row on success or restore status on failure. Monitor skips these two states. Startup cleanup removes stale rows from crashes.

**Tech Stack:** Go, tview (TUI), SQLite via `modernc.org/sqlite`, tmux wrappers

---

### Task 1: Add StatusCreating and StatusDeleting constants

**Files:**
- Modify: `internal/db/types.go`
- Modify: `internal/ui/theme.go`
- Test: `internal/ui/theme_test.go` (create new)

**Step 1: Write the failing test**

Create `internal/ui/theme_test.go`:
```go
package ui

import (
	"testing"
)

func TestStatusIconCreating(t *testing.T) {
	icon, _ := StatusIcon("creating")
	if icon == "" {
		t.Error("expected non-empty icon for creating")
	}
}

func TestStatusIconDeleting(t *testing.T) {
	icon, _ := StatusIcon("deleting")
	if icon == "" {
		t.Error("expected non-empty icon for deleting")
	}
}

func TestStatusIconCreatingDistinctFromDefault(t *testing.T) {
	_, colorCreating := StatusIcon("creating")
	_, colorUnknown := StatusIcon("unknown-xyz")
	if colorCreating == colorUnknown {
		t.Error("creating should have distinct color from unknown status")
	}
}
```

**Step 2: Run to verify it fails**

```
go test ./internal/ui/... -run TestStatusIcon -v
```
Expected: FAIL (TestStatusIconCreatingDistinctFromDefault fails because creating falls through to default)

**Step 3: Add constants to `internal/db/types.go`**

After `StatusError`:
```go
StatusCreating SessionStatus = "creating"
StatusDeleting SessionStatus = "deleting"
```

**Step 4: Add cases to `StatusIcon` in `internal/ui/theme.go`**

Add before `default:`:
```go
case "creating":
    return "⟳", ColorAccent
case "deleting":
    return "⟳", ColorWarning
```

**Step 5: Run to verify tests pass**

```
go test ./internal/ui/... -run TestStatusIcon -v
```
Expected: PASS

**Step 6: Run full test suite**

```
make test
```
Expected: all pass

**Step 7: Commit**

```bash
git add internal/db/types.go internal/ui/theme.go internal/ui/theme_test.go
git commit -m "feat: add StatusCreating and StatusDeleting status constants"
```

---

### Task 2: Update home list rendering for creating/deleting sessions

**Files:**
- Modify: `internal/ui/home.go`

Sessions in `StatusCreating` or `StatusDeleting` must:
- Display "creating..." or "deleting..." text in place of the age field
- Block all key actions except `q` (quit)

**Step 1: Guard key handlers in `setupInput`**

In `setupInput` in `internal/ui/home.go`, add a helper at the top of the input capture function body (after `h.selected = row`):

```go
isPending := func(item listItem) bool {
    return item.session != nil &&
        (item.session.Status == db.StatusCreating || item.session.Status == db.StatusDeleting)
}
```

Then guard each action:
- `Enter` / `case tcell.KeyRight` attach path: add `if isPending(item) { return nil }` before calling `h.onAttach`
- `'a'`: add `if isPending(item) { return nil }` before calling `h.onAttach`
- `'n'` (notes path, not new): add `if isPending(item) { return nil }` before calling `h.onNotes`
- `'d'`: add `if isPending(item) { return nil }` before calling `h.onDelete`
- `'s'`: add `if isPending(item) { return nil }` before calling `h.onStop`
- `'x'`: add `if isPending(item) { return nil }` before calling `h.onRestart`
- `'e'`: add `if isPending(item) { return nil }` before calling `h.onEdit`
- `'m'`: add `if isPending(item) { return nil }` before calling `h.onMove`

**Step 2: Update `renderTable` to show status text for creating/deleting**

In `renderTable`, replace the session row rendering block with:

```go
s := item.session
icon, color := StatusIcon(string(s.Status))
title := s.Title
if len(title) > 20 {
    title = title[:18] + ".."
}
dirtyMark := "  "
if s.HasUncommitted {
    dirtyMark = "* "
}
var ageOrStatus string
switch s.Status {
case db.StatusCreating:
    ageOrStatus = "creating..."
case db.StatusDeleting:
    ageOrStatus = "deleting..."
default:
    ageOrStatus = formatAge(s.LastAccessed)
}
text := fmt.Sprintf("   %s %s%-20s %s  %s", icon, dirtyMark, title, s.Tool, ageOrStatus)
cell := tview.NewTableCell(text).
    SetTextColor(color).
    SetBackgroundColor(ColorBackground).
    SetExpansion(1).
    SetSelectable(true)
h.table.SetCell(i, 0, cell)
```

**Step 3: Run full test suite**

```
make test
```
Expected: all pass

**Step 4: Commit**

```bash
git add internal/ui/home.go
git commit -m "feat: render creating/deleting status in session list and guard key handlers"
```

---

### Task 3: Update monitor to skip creating/deleting sessions

**Files:**
- Modify: `internal/monitor/monitor.go`
- Test: `internal/monitor/monitor_test.go`

**Step 1: Write the failing test**

Add to `internal/monitor/monitor_test.go`:

```go
func TestMonitorSkipsCreatingSession(t *testing.T) {
	store, _ := db.Open(":memory:")
	store.Migrate()
	defer store.Close()

	now := time.Now()
	s := &db.Session{
		ID:          "creating-id",
		Title:       "pending-fox",
		GroupPath:   "my-sessions",
		Tool:        db.ToolClaude,
		Status:      db.StatusCreating,
		TmuxSession: "", // no tmux session yet
		CreatedAt:   now,
		LastAccessed: now,
	}
	store.SaveSession(s)

	updateCalled := false
	notifier := notify.New(notify.Config{}, discardLogger())
	mon := monitor.New(store, func() { updateCalled = true }, notifier, nil, discardLogger())

	// Expose refresh via a test helper or just verify indirectly:
	// The session should not be written to stopped even though TmuxSession is empty.
	// We verify by checking status is still StatusCreating after a refresh would normally
	// flip empty-TmuxSession sessions to stopped.
	//
	// Trigger one refresh cycle by starting and immediately stopping.
	mon.Start()
	time.Sleep(600 * time.Millisecond) // one tick
	mon.Stop()

	got, _ := store.GetSession("creating-id")
	if got.Status != db.StatusCreating {
		t.Errorf("expected StatusCreating, got %q", got.Status)
	}
	if updateCalled {
		t.Error("expected no update callback for creating session")
	}
}
```

**Step 2: Run to verify it fails**

```
go test ./internal/monitor/... -run TestMonitorSkipsCreatingSession -v
```
Expected: FAIL (status gets changed to stopped because TmuxSession is empty and the monitor writes stopped for empty tmux sessions)

**Step 3: Update monitor `refresh()` to skip creating/deleting sessions**

In `internal/monitor/monitor.go`, in `refresh()`, after `if s.TmuxSession == "" {`, change the block:

```go
// Skip sessions that are being created or deleted - they have no tmux session yet.
if s.Status == db.StatusCreating || s.Status == db.StatusDeleting {
    continue
}
if s.TmuxSession == "" {
    continue
}
```

**Step 4: Run to verify tests pass**

```
go test ./internal/monitor/... -run TestMonitorSkipsCreatingSession -v
```
Expected: PASS

**Step 5: Run full test suite**

```
make test
```
Expected: all pass

**Step 6: Commit**

```bash
git add internal/monitor/monitor.go internal/monitor/monitor_test.go
git commit -m "feat: monitor skips sessions in creating/deleting state"
```

---

### Task 4: Add LoadSessionsByStatus to DB

**Files:**
- Modify: `internal/db/db.go`
- Test: `internal/db/db_test.go`

**Step 1: Write the failing test**

Add to `internal/db/db_test.go`:

```go
func TestLoadSessionsByStatus(t *testing.T) {
	store, _ := db.Open(":memory:")
	defer store.Close()
	store.Migrate()

	now := time.Now().Truncate(time.Millisecond)
	sessions := []*db.Session{
		{ID: "a", Title: "alpha", GroupPath: "my-sessions", Status: db.StatusCreating, Tool: db.ToolClaude, CreatedAt: now, LastAccessed: now},
		{ID: "b", Title: "beta", GroupPath: "my-sessions", Status: db.StatusRunning, Tool: db.ToolClaude, CreatedAt: now, LastAccessed: now},
		{ID: "c", Title: "gamma", GroupPath: "my-sessions", Status: db.StatusDeleting, Tool: db.ToolClaude, CreatedAt: now, LastAccessed: now},
	}
	for _, s := range sessions {
		store.SaveSession(s)
	}

	got, err := store.LoadSessionsByStatus(db.StatusCreating, db.StatusDeleting)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(got))
	}
	ids := map[string]bool{got[0].ID: true, got[1].ID: true}
	if !ids["a"] || !ids["c"] {
		t.Errorf("expected sessions a and c, got %v", ids)
	}
}
```

**Step 2: Run to verify it fails**

```
go test ./internal/db/... -run TestLoadSessionsByStatus -v
```
Expected: FAIL (method does not exist)

**Step 3: Add `LoadSessionsByStatus` to `internal/db/db.go`**

Add after `LoadSessionsByGroupPath`:

```go
func (d *DB) LoadSessionsByStatus(statuses ...SessionStatus) ([]*Session, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(statuses))
	args := make([]any, len(statuses))
	for i, s := range statuses {
		placeholders[i] = "?"
		args[i] = string(s)
	}
	query := fmt.Sprintf(`
		SELECT id, title, project_path, group_path, sort_order,
			command, tool, status, tmux_session,
			created_at, last_accessed,
			parent_session_id, worktree_path, worktree_repo, worktree_branch,
			acknowledged, repo_url, has_uncommitted, notes
		FROM sessions WHERE status IN (%s) ORDER BY sort_order`,
		strings.Join(placeholders, ","))
	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []*Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}
```

**Step 4: Run to verify tests pass**

```
go test ./internal/db/... -run TestLoadSessionsByStatus -v
```
Expected: PASS

**Step 5: Run full test suite**

```
make test
```
Expected: all pass

**Step 6: Commit**

```bash
git add internal/db/db.go internal/db/db_test.go
git commit -m "feat: add LoadSessionsByStatus query to DB"
```

---

### Task 5: Refactor create flow to be async

**Files:**
- Modify: `internal/ui/app.go`

This task rewrites the worktree-session creation path in `onNew` to run all blocking git operations in a background goroutine.

**Step 1: Add uuid import if not already present**

Check imports in `app.go`. `github.com/google/uuid` is already used in `session.go` -- you need it in `app.go` too. Add to imports:

```go
"github.com/google/uuid"
```

Also add:
```go
"time"
```

**Step 2: Replace the worktree creation block in `onNew`**

The existing flow when `groupRepoURL != ""` is a synchronous sequence of git calls. Replace everything from `if groupRepoURL != "" {` through the final `createSession()` call with the async version below.

The new structure:

```go
if groupRepoURL != "" {
    host, owner, repo, err := git.ParseRepoURL(groupRepoURL)
    if err != nil {
        a.showError(fmt.Sprintf("Invalid group repo URL: %v", err))
        return
    }
    // Resolve title before inserting so branch name matches.
    title := result.Title
    if title == "" {
        title = session.GenerateTitle()
    }
    opts.Title = title
    command := opts.Command
    if command == "" {
        command = db.ToolCommand(opts.Tool, "")
    }

    // Insert a pending row immediately so the session appears in the list.
    sessions, _ := a.store.LoadSessions()
    now := time.Now()
    sessionID := uuid.NewString()
    pending := &db.Session{
        ID:           sessionID,
        Title:        title,
        GroupPath:    result.GroupPath,
        Tool:         result.Tool,
        Command:      command,
        Status:       db.StatusCreating,
        CreatedAt:    now,
        LastAccessed: now,
        SortOrder:    len(sessions),
        RepoURL:      groupRepoURL,
    }
    if err := a.store.SaveSession(pending); err != nil {
        a.showError(fmt.Sprintf("Create failed: %v", err))
        return
    }
    _ = a.store.InsertSessionEvent(sessionID, "created", "")
    a.store.Touch()
    a.refreshHome()

    bareRepoPath := git.BareRepoPath(a.cfg.ReposDir, host, owner, repo)
    branch := git.SanitizeBranchName(title)
    wtPath := git.WorktreePath(a.cfg.WorktreesDir, host, owner, repo, branch)

    cancelCreate := func(msg string) {
        a.store.DeleteSession(sessionID)
        _ = a.store.Touch()
        a.refreshHome()
        if msg != "" {
            a.showError(msg)
        }
    }

    go func() {
        // 1. Ensure bare repo directory exists.
        if err := os.MkdirAll(filepath.Dir(bareRepoPath), 0755); err != nil {
            a.tapp.QueueUpdateDraw(func() {
                cancelCreate(fmt.Sprintf("Create repos dir failed: %v", err))
            })
            return
        }

        // 2. Clone or fetch the bare repo.
        if !git.IsBareRepo(bareRepoPath) {
            if err := git.CloneBare(groupRepoURL, bareRepoPath); err != nil {
                a.tapp.QueueUpdateDraw(func() {
                    cancelCreate(fmt.Sprintf("Clone failed: %v", err))
                })
                return
            }
        } else {
            if err := git.FetchBare(bareRepoPath); err != nil {
                a.tapp.QueueUpdateDraw(func() {
                    cancelCreate(fmt.Sprintf("Fetch failed: %v", err))
                })
                return
            }
        }

        // 3. Ensure worktree parent directory exists.
        if err := os.MkdirAll(filepath.Dir(wtPath), 0755); err != nil {
            a.tapp.QueueUpdateDraw(func() {
                cancelCreate(fmt.Sprintf("Create worktrees dir failed: %v", err))
            })
            return
        }

        // 4. Create the worktree; handle the "already exists" case via a channel.
        if _, err := git.CreateWorktree(bareRepoPath, branch, wtPath, a.cfg.Worktree.DefaultBaseBranch); err != nil {
            if errors.Is(err, git.ErrWorktreeExists) {
                reuseCh := make(chan bool, 1)
                a.tapp.QueueUpdateDraw(func() {
                    modal := tview.NewModal().
                        SetText(fmt.Sprintf("Worktree for branch '%s' already exists.\n\nReuse it or cancel?", branch)).
                        AddButtons([]string{"Reuse", "Cancel"}).
                        SetDoneFunc(func(_ int, label string) {
                            a.closeDialog("worktree-exists")
                            reuseCh <- (label == "Reuse")
                        })
                    a.pages.AddPage("worktree-exists", modal, true, true)
                })
                if !<-reuseCh {
                    a.tapp.QueueUpdateDraw(func() { cancelCreate("") })
                    return
                }
                // else: fall through and use the existing worktree
            } else {
                a.tapp.QueueUpdateDraw(func() {
                    cancelCreate(fmt.Sprintf("Create worktree failed: %v", err))
                })
                return
            }
        }

        // 5. Run pre-launch command if set.
        if preLaunchCmd != "" {
            toolCmd := db.ToolCommand(opts.Tool, opts.Command)
            out, err := session.RunPreLaunchCommand(preLaunchCmd, toolCmd, bareRepoPath, wtPath)
            if err != nil {
                a.tapp.QueueUpdateDraw(func() {
                    cancelCreate(fmt.Sprintf("Pre-launch command failed: %v\n%s", err, out))
                })
                return
            }
        }

        // 6. Create the tmux session.
        tmuxName := tmux.GenerateSessionName(title)
        if err := tmux.CreateSession(tmux.CreateOptions{
            Name:    tmuxName,
            Command: command,
            Cwd:     wtPath,
        }); err != nil {
            a.tapp.QueueUpdateDraw(func() {
                cancelCreate(fmt.Sprintf("Create failed: %v", err))
            })
            return
        }

        // 7. Update the DB row to running.
        pending.TmuxSession = tmuxName
        pending.Status = db.StatusRunning
        pending.ProjectPath = wtPath
        pending.WorktreePath = wtPath
        pending.WorktreeRepo = bareRepoPath
        pending.WorktreeBranch = branch
        pending.LastAccessed = time.Now()
        if err := a.store.SaveSession(pending); err != nil {
            // tmux session was created; kill it to avoid orphan.
            tmux.KillSession(tmuxName)
            a.tapp.QueueUpdateDraw(func() {
                cancelCreate(fmt.Sprintf("Save failed: %v", err))
            })
            return
        }
        _ = a.store.Touch()

        a.tapp.QueueUpdateDraw(func() {
            a.refreshHome()
            a.onAttachSession(pending)
        })
    }()
} else {
    // Non-worktree path: unchanged.
    opts.ProjectPath = result.ProjectPath
    createSession()
}
```

Note: `createSession` closure still handles the non-worktree path. Remove the pre-launch logic from `createSession` for the worktree path since it's now in the goroutine -- but `createSession` is only called in the `else` branch, so no change is needed there.

**Step 3: Build to catch compile errors**

```
make build
```
Expected: success

**Step 4: Run full test suite**

```
make test
```
Expected: all pass

**Step 5: Commit**

```bash
git add internal/ui/app.go
git commit -m "feat: async worktree session creation with StatusCreating"
```

---

### Task 6: Refactor delete flow to be async

**Files:**
- Modify: `internal/ui/app.go`

**Step 1: Rewrite `doDelete` in `onDelete` to use background goroutine**

In `onDelete`, replace the `doDelete` closure with:

```go
doDelete := func() {
    s := item.session

    // Kill the tmux session synchronously (fast) before any async work.
    if s.TmuxSession != "" {
        tmux.KillSession(s.TmuxSession)
    }

    // Sessions without a worktree have no slow git work; delete synchronously.
    if s.WorktreePath == "" || s.WorktreeRepo == "" {
        a.mgr.Delete(s.ID)
        a.refreshHome()
        return
    }

    prevStatus := s.Status

    // Mark as deleting so the UI shows "deleting..." immediately.
    _ = a.store.WriteStatus(s.ID, db.StatusDeleting, s.Tool)
    _ = a.store.InsertSessionEvent(s.ID, "deleting", "")
    a.store.Touch()
    a.refreshHome()

    finishDelete := func() {
        _ = a.store.DeleteSession(s.ID)
        _ = a.store.InsertSessionEvent(s.ID, "deleted", "")
        _ = a.store.Touch()
        a.refreshHome()
    }

    restoreStatus := func(errMsg string) {
        _ = a.store.WriteStatus(s.ID, prevStatus, s.Tool)
        a.store.Touch()
        a.refreshHome()
        if errMsg != "" {
            a.showError(errMsg)
        }
    }

    go func() {
        if err := git.RemoveWorktree(s.WorktreeRepo, s.WorktreePath, false); err == nil {
            a.tapp.QueueUpdateDraw(finishDelete)
            return
        }

        // Normal removal failed; offer force delete.
        a.tapp.QueueUpdateDraw(func() {
            modal := tview.NewModal().
                SetText(fmt.Sprintf("Could not remove worktree:\n\n%v\n\nForce delete (discards uncommitted changes)?", err)).
                AddButtons([]string{"Force Delete", "Cancel"}).
                SetDoneFunc(func(_ int, label string) {
                    a.closeDialog("worktree-error")
                    if label != "Force Delete" {
                        restoreStatus("")
                        return
                    }
                    go func() {
                        if err2 := git.RemoveWorktree(s.WorktreeRepo, s.WorktreePath, true); err2 != nil {
                            a.tapp.QueueUpdateDraw(func() {
                                restoreStatus(fmt.Sprintf("Force delete failed: %v", err2))
                            })
                            return
                        }
                        a.tapp.QueueUpdateDraw(finishDelete)
                    }()
                })
            a.pages.AddPage("worktree-error", modal, true, true)
        })
    }()
}
```

Note: the `err` variable from the outer `git.RemoveWorktree` call is captured in the closure for the modal text. Ensure it's in scope -- it is, because the modal closure is defined inside the `go func()`.

Also remove the old group-delete branch reference to `doDelete` -- that path doesn't call `doDelete`, it only applies to sessions.

**Step 2: Build to catch compile errors**

```
make build
```
Expected: success

**Step 3: Run full test suite**

```
make test
```
Expected: all pass

**Step 4: Commit**

```bash
git add internal/ui/app.go
git commit -m "feat: async worktree session deletion with StatusDeleting"
```

---

### Task 7: Add startup cleanup for stale creating/deleting sessions

**Files:**
- Modify: `internal/ui/app.go`

**Step 1: Add cleanup block to `App.Run()`**

At the start of `Run()`, before `a.refreshHome()`, add:

```go
// Clean up sessions left in creating/deleting state from a previous crash.
if stale, err := a.store.LoadSessionsByStatus(db.StatusCreating, db.StatusDeleting); err == nil && len(stale) > 0 {
    for _, s := range stale {
        if s.Status == db.StatusDeleting && s.WorktreePath != "" && s.WorktreeRepo != "" {
            // Best-effort force removal; ignore error.
            _ = git.RemoveWorktree(s.WorktreeRepo, s.WorktreePath, true)
        }
        _ = a.store.DeleteSession(s.ID)
    }
    _ = a.store.Touch()
}
```

**Step 2: Build to catch compile errors**

```
make build
```
Expected: success

**Step 3: Run full test suite**

```
make test
```
Expected: all pass

**Step 4: Install and smoke test**

```
make install
```

Manually verify:
- Create a session in a group with a repo URL -- "creating..." appears in list, UI remains navigable while clone runs, session transitions to running and attaches automatically
- Delete a session with a worktree -- "deleting..." appears while removal runs, session disappears when done
- Quit and restart the app -- no stale "creating..."/"deleting..." rows appear

**Step 5: Commit**

```bash
git add internal/ui/app.go
git commit -m "feat: clean up stale creating/deleting sessions on startup"
```
