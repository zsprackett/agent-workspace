# Pre-Launch Command Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `PreLaunchCommand` field to groups that runs a shell command with positional args before the tool is launched in a new session.

**Architecture:** Four-layer change: DB schema + Go struct, a new pure function in `internal/session/`, the group dialog UI, and wiring in `app.go`. The pre-launch runner is a standalone function that takes the command string and variadic positional args, making it easy to test and call from both worktree and non-worktree paths.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), tview, exec.Command

---

## Argument contract

- No worktree: `<cmd> <tool> <project-path>`
- With worktree: `<cmd> <tool> <bare-repo-path> <worktree-path>`

`<tool>` is the raw `db.Tool` string value (e.g. `"claude"`, `"opencode"`).

---

### Task 1: Add PreLaunchCommand to the DB layer

**Files:**
- Modify: `internal/db/types.go`
- Modify: `internal/db/db.go`
- Modify: `internal/db/db_test.go`

**Step 1: Write the failing test**

Add to `internal/db/db_test.go`:

```go
func TestGroupPreLaunchCommand(t *testing.T) {
	store, _ := db.Open(":memory:")
	defer store.Close()
	store.Migrate()

	groups := []*db.Group{
		{
			Path:              "work",
			Name:              "Work",
			Expanded:          true,
			SortOrder:         0,
			PreLaunchCommand:  "/usr/local/bin/setup.sh",
		},
	}
	if err := store.SaveGroups(groups); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.LoadGroups()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].PreLaunchCommand != "/usr/local/bin/setup.sh" {
		t.Errorf("pre_launch_command: got %q want %q", got[0].PreLaunchCommand, "/usr/local/bin/setup.sh")
	}
}
```

**Step 2: Run test to verify it fails**

```
go test ./internal/db/... -run TestGroupPreLaunchCommand -v
```

Expected: FAIL — `got[0].PreLaunchCommand` will be empty string (field doesn't exist yet).

**Step 3: Add PreLaunchCommand to Group struct**

In `internal/db/types.go`, add the field to `Group`:

```go
type Group struct {
	Path              string
	Name              string
	Expanded          bool
	SortOrder         int
	DefaultPath       string
	RepoURL           string
	PreLaunchCommand  string
}
```

**Step 4: Add migration in db.go**

In `internal/db/db.go`, after the existing `ALTER TABLE groups ADD COLUMN repo_url` block (around line 95), add:

```go
// Add pre_launch_command column to existing groups tables; ignore "duplicate column" errors.
if _, alterErr := d.sql.Exec(`ALTER TABLE groups ADD COLUMN pre_launch_command TEXT NOT NULL DEFAULT ''`); alterErr != nil {
	if !isDuplicateColumnError(alterErr) {
		return fmt.Errorf("alter groups add pre_launch_command: %w", alterErr)
	}
}
```

Also update the `CREATE TABLE IF NOT EXISTS groups` statement to include the column (so fresh DBs get it without needing the ALTER):

```sql
CREATE TABLE IF NOT EXISTS groups (
    path              TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    expanded          INTEGER NOT NULL DEFAULT 1,
    sort_order        INTEGER NOT NULL DEFAULT 0,
    default_path      TEXT NOT NULL DEFAULT '',
    repo_url          TEXT NOT NULL DEFAULT '',
    pre_launch_command TEXT NOT NULL DEFAULT ''
)
```

**Step 5: Update SaveGroups in db.go**

Replace the INSERT in `SaveGroups`:

```go
if _, err := tx.Exec(
    "INSERT INTO groups (path, name, expanded, sort_order, default_path, repo_url, pre_launch_command) VALUES (?,?,?,?,?,?,?)",
    g.Path, g.Name, boolToInt(g.Expanded), g.SortOrder, g.DefaultPath, g.RepoURL, g.PreLaunchCommand,
); err != nil {
    return err
}
```

**Step 6: Update LoadGroups in db.go**

Replace the SELECT and Scan in `LoadGroups`:

```go
rows, err := d.sql.Query("SELECT path, name, expanded, sort_order, default_path, repo_url, pre_launch_command FROM groups ORDER BY sort_order")
```

```go
if err := rows.Scan(&g.Path, &g.Name, &expanded, &g.SortOrder, &g.DefaultPath, &g.RepoURL, &g.PreLaunchCommand); err != nil {
```

**Step 7: Run test to verify it passes**

```
go test ./internal/db/... -v
```

Expected: all PASS.

**Step 8: Commit**

```bash
git add internal/db/types.go internal/db/db.go internal/db/db_test.go
git commit -m "feat: add PreLaunchCommand to Group DB layer"
```

---

### Task 2: Implement RunPreLaunchCommand

**Files:**
- Create: `internal/session/prelaunch.go`
- Create: `internal/session/prelaunch_test.go`

**Step 1: Write the failing tests**

Create `internal/session/prelaunch_test.go`:

```go
package session_test

import (
	"strings"
	"testing"

	"github.com/zsprackett/agent-workspace/internal/session"
)

func TestRunPreLaunchCommand_Success(t *testing.T) {
	out, err := session.RunPreLaunchCommand("echo hello", "claude", "/tmp/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", out)
	}
}

func TestRunPreLaunchCommand_NonZeroExit(t *testing.T) {
	_, err := session.RunPreLaunchCommand("false")
	if err == nil {
		t.Fatal("expected error from non-zero exit, got nil")
	}
}

func TestRunPreLaunchCommand_ArgsAppended(t *testing.T) {
	// Use printf to echo all args; verify they are passed
	out, err := session.RunPreLaunchCommand("sh -c 'printf \"%s\\n\" \"$@\"' --", "claude", "/repos/foo.git", "/worktrees/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "claude") {
		t.Errorf("expected 'claude' in output, got %q", out)
	}
	if !strings.Contains(out, "/repos/foo.git") {
		t.Errorf("expected bare repo path in output, got %q", out)
	}
	if !strings.Contains(out, "/worktrees/foo") {
		t.Errorf("expected worktree path in output, got %q", out)
	}
}

func TestRunPreLaunchCommand_EmptyCommand(t *testing.T) {
	out, err := session.RunPreLaunchCommand("")
	if err != nil {
		t.Fatalf("empty command should be a no-op, got error: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty output for empty command, got %q", out)
	}
}
```

**Step 2: Run tests to verify they fail**

```
go test ./internal/session/... -run TestRunPreLaunchCommand -v
```

Expected: FAIL — `session.RunPreLaunchCommand` undefined.

**Step 3: Create prelaunch.go**

Create `internal/session/prelaunch.go`:

```go
package session

import (
	"os/exec"
	"strings"
)

// RunPreLaunchCommand executes cmd (a shell command string) with the given positional args
// appended. Returns combined stdout+stderr output and any error.
// If cmd is empty, returns immediately with no error.
func RunPreLaunchCommand(cmd string, args ...string) (string, error) {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return "", nil
	}
	allArgs := append(fields[1:], args...)
	c := exec.Command(fields[0], allArgs...)
	out, err := c.CombinedOutput()
	return string(out), err
}
```

**Step 4: Run tests to verify they pass**

```
go test ./internal/session/... -v
```

Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/session/prelaunch.go internal/session/prelaunch_test.go
git commit -m "feat: add RunPreLaunchCommand to session package"
```

---

### Task 3: Update the group dialog UI

**Files:**
- Modify: `internal/ui/dialogs/group.go`

No new tests needed for the dialog (it is a tview form; UI components are not unit-tested in this codebase).

**Step 1: Update GroupResult**

In `internal/ui/dialogs/group.go`, add `PreLaunchCommand` to the result struct:

```go
type GroupResult struct {
	Name             string
	RepoURL          string
	PreLaunchCommand string
}
```

**Step 2: Update GroupDialog signature**

Change the function signature to accept the current pre-launch command value:

```go
func GroupDialog(title, currentName, currentRepoURL, currentPreLaunchCommand string, onSubmit func(GroupResult), onCancel func()) *tview.Form {
```

**Step 3: Add the input field and update the submit handler**

Add the new field after the `GitHub URL` field, and update the `onSubmit` call:

```go
form.AddInputField("Group name", currentName, 40, nil, nil)
form.AddInputField("GitHub URL (optional)", currentRepoURL, 50, nil, nil)
form.AddInputField("Pre-launch command (optional)", currentPreLaunchCommand, 50, nil, nil)
form.AddButton("OK", func() {
    name := form.GetFormItemByLabel("Group name").(*tview.InputField).GetText()
    if name != "" {
        repoURL := form.GetFormItemByLabel("GitHub URL (optional)").(*tview.InputField).GetText()
        prelaunch := form.GetFormItemByLabel("Pre-launch command (optional)").(*tview.InputField).GetText()
        onSubmit(GroupResult{Name: name, RepoURL: repoURL, PreLaunchCommand: prelaunch})
    }
})
```

**Step 4: Build to confirm no compile errors**

```
make build
```

Expected: compile error in `app.go` because `GroupDialog` callers don't pass the new arg yet. That's expected — fix it in the next task.

---

### Task 4: Wire pre-launch into app.go

**Files:**
- Modify: `internal/ui/app.go`

**Step 1: Fix GroupDialog call in onNewGroup**

In `app.go`'s `onNewGroup()`, update the `GroupDialog` call to pass `""` as the new fourth arg (no existing pre-launch command for a new group):

```go
form := dialogs.GroupDialog("New Group", "", "", "", func(result dialogs.GroupResult) {
    ...
    groups = append(groups, &db.Group{
        Path:             path,
        Name:             result.Name,
        Expanded:         true,
        SortOrder:        len(groups),
        RepoURL:          result.RepoURL,
        PreLaunchCommand: result.PreLaunchCommand,
    })
    ...
}, func() { a.closeDialog("new-group") })
a.showDialog("new-group", form, 65, 16)
```

Note the dialog height increase from 12 to 16 (three input fields + two buttons need more space).

**Step 2: Fix GroupDialog call in onEdit**

In `app.go`'s `onEdit()`, for the group branch, update the `GroupDialog` call:

```go
form := dialogs.GroupDialog("Edit Group", item.group.Name, item.group.RepoURL, item.group.PreLaunchCommand,
    func(result dialogs.GroupResult) {
        a.closeDialog("edit")
        groups, _ := a.store.LoadGroups()
        for _, g := range groups {
            if g.Path == item.group.Path {
                g.Name = result.Name
                g.RepoURL = result.RepoURL
                g.PreLaunchCommand = result.PreLaunchCommand
            }
        }
        a.store.SaveGroups(groups)
        a.store.Touch()
        a.refreshHome()
    }, func() { a.closeDialog("edit") })
a.showDialog("edit", form, 65, 16)
```

**Step 3: Add pre-launch execution in onNew**

In `app.go`'s `onNew()`, the current code looks up the group's `RepoURL` but not the full group. Change the lookup to capture the full `PreLaunchCommand` too.

After the existing `groupRepoURL` lookup block (around line 145):

```go
var groupRepoURL string
var groupPreLaunchCommand string
for _, g := range groups {
    if g.Path == result.GroupPath {
        groupRepoURL = g.RepoURL
        groupPreLaunchCommand = g.PreLaunchCommand
        break
    }
}
```

**Step 4: Add the launchWithPreCheck helper inside onNew**

Replace the existing `createSession := func() { ... }` block with two closures. Keep `createSession` as-is, and add `launchWithPreCheck` immediately after it:

```go
createSession := func() {
    s, err := a.mgr.Create(opts)
    if err != nil {
        a.showError(fmt.Sprintf("Create failed: %v", err))
        return
    }
    a.refreshHome()
    a.onAttachSession(s)
}

launchWithPreCheck := func() {
    if groupPreLaunchCommand == "" {
        createSession()
        return
    }
    var cmdArgs []string
    cmdArgs = append(cmdArgs, string(opts.Tool))
    if opts.WorktreePath != "" {
        cmdArgs = append(cmdArgs, opts.WorktreeRepo, opts.WorktreePath)
    } else {
        cmdArgs = append(cmdArgs, opts.ProjectPath)
    }
    out, err := session.RunPreLaunchCommand(groupPreLaunchCommand, cmdArgs...)
    if err == nil {
        createSession()
        return
    }
    msg := fmt.Sprintf("Pre-launch command failed:\n\n%s\n\nContinue anyway?", out)
    if len(msg) > 300 {
        msg = msg[:300] + "..."
    }
    modal := tview.NewModal().
        SetText(msg).
        AddButtons([]string{"Continue", "Cancel"}).
        SetDoneFunc(func(_ int, label string) {
            a.closeDialog("prelaunch-warn")
            if label == "Continue" {
                createSession()
            }
        })
    a.pages.AddPage("prelaunch-warn", modal, true, true)
}
```

**Step 5: Replace createSession() calls with launchWithPreCheck()**

There are three places in `onNew()` where `createSession()` is called:

1. Inside the `"Reuse"` button handler of the `worktree-exists` modal (around line 209):

```go
if label == "Reuse" {
    opts.WorktreePath = wtPath
    opts.WorktreeRepo = bareRepoPath
    opts.WorktreeBranch = branch
    opts.ProjectPath = wtPath
    opts.RepoURL = groupRepoURL
    launchWithPreCheck()  // was createSession()
}
```

2. After the new worktree is created successfully (around line 218):

```go
opts.WorktreePath = wtPath
opts.WorktreeRepo = bareRepoPath
opts.WorktreeBranch = branch
opts.ProjectPath = wtPath
opts.RepoURL = groupRepoURL
// (fall through to end of if block)
```

3. The final call at the end of `onNew`'s submit handler:

```go
} else {
    opts.ProjectPath = result.ProjectPath
}
launchWithPreCheck()  // was createSession()
```

**Step 6: Ensure session package is imported**

Verify `internal/session` is already imported in `app.go` (it is, as `session.Manager` is used). No new import needed.

**Step 7: Build and verify**

```
make build
```

Expected: clean compile.

**Step 8: Run all tests**

```
make test
```

Expected: all PASS.

**Step 9: Commit**

```bash
git add internal/ui/app.go internal/ui/dialogs/group.go
git commit -m "feat: wire pre-launch command into group session creation"
```

---

### Task 5: Manual smoke test

These steps verify the feature end-to-end since there are no automated UI tests.

1. Run `make install` to install the binary.
2. Open agent-workspace: `agent-workspace`
3. Create a new group. Verify the "Pre-launch command (optional)" field appears.
4. Set a pre-launch command to a script that exits 0 (e.g. `echo`). Create a session in that group. Verify the session launches normally.
5. Set a pre-launch command to a script that exits non-zero (e.g. `false`). Create a session. Verify the warning modal appears with "Continue" and "Cancel" buttons.
6. Click "Continue" — verify the session launches.
7. Click "Cancel" — verify no session is created.
8. Edit the group. Verify the pre-launch command field is pre-populated with the saved value.
