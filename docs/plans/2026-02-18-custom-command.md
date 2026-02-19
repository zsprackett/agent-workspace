# Custom Command Input Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show a "Command" input field in the new-session and edit-session dialogs when and only when the `custom` tool is selected, and wire that value through to the tmux session.

**Architecture:** Add `Command string` to both dialog result structs and to `session.UpdateOptions`. Use tview's `AddFormItem`/`RemoveFormItem` to dynamically append/remove the Command field based on the Tool dropdown selection. `db.ToolCommand` already handles the custom string -- only `session.Update` needs fixing to forward it.

**Tech Stack:** Go, tview (TUI forms), SQLite (via internal/db)

---

### Task 1: Fix `session.Update` to forward custom command

**Files:**
- Modify: `internal/session/session.go` (lines 165-189)
- Test: `internal/session/session_test.go`

**Step 1: Write the failing test**

Add to `internal/session/session_test.go`:

```go
import "time"

func TestUpdatePreservesCustomCommand(t *testing.T) {
	store := newTestDB(t)

	now := time.Now().Truncate(time.Millisecond)
	s := &db.Session{
		ID:           "custom-test",
		Title:        "bold-wolf",
		ProjectPath:  "/tmp/proj",
		GroupPath:    "my-sessions",
		Command:      "old-tool --flag",
		Tool:         db.ToolCustom,
		Status:       db.StatusStopped,
		TmuxSession:  "",
		CreatedAt:    now,
		LastAccessed: now,
	}
	if err := store.SaveSession(s); err != nil {
		t.Fatalf("save: %v", err)
	}

	mgr := session.NewManager(store)
	if err := mgr.Update("custom-test", session.UpdateOptions{
		Title:       "bold-wolf",
		Tool:        db.ToolCustom,
		Command:     "new-tool --other",
		ProjectPath: "/tmp/proj",
		GroupPath:   "my-sessions",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := store.GetSession("custom-test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Command != "new-tool --other" {
		t.Errorf("command: got %q want %q", got.Command, "new-tool --other")
	}
}

func TestUpdateResetsCommandOnToolChange(t *testing.T) {
	store := newTestDB(t)

	now := time.Now().Truncate(time.Millisecond)
	s := &db.Session{
		ID:           "tool-change-test",
		Title:        "calm-deer",
		ProjectPath:  "/tmp/proj",
		GroupPath:    "my-sessions",
		Command:      "my-custom-tool",
		Tool:         db.ToolCustom,
		Status:       db.StatusStopped,
		TmuxSession:  "",
		CreatedAt:    now,
		LastAccessed: now,
	}
	if err := store.SaveSession(s); err != nil {
		t.Fatalf("save: %v", err)
	}

	mgr := session.NewManager(store)
	if err := mgr.Update("tool-change-test", session.UpdateOptions{
		Title:       "calm-deer",
		Tool:        db.ToolClaude,
		Command:     "",
		ProjectPath: "/tmp/proj",
		GroupPath:   "my-sessions",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := store.GetSession("tool-change-test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Command != "claude" {
		t.Errorf("command: got %q want %q", got.Command, "claude")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/session/... -run TestUpdatePreservesCustomCommand -v
go test ./internal/session/... -run TestUpdateResetsCommandOnToolChange -v
```

Expected: FAIL -- `UpdateOptions` has no `Command` field yet.

**Step 3: Add `Command` to `UpdateOptions` and fix `Update()`**

In `internal/session/session.go`, change:

```go
type UpdateOptions struct {
	Title       string
	Tool        db.Tool
	ProjectPath string
	GroupPath   string
}
```

to:

```go
type UpdateOptions struct {
	Title       string
	Tool        db.Tool
	Command     string
	ProjectPath string
	GroupPath   string
}
```

And change the line in `Update()`:

```go
s.Command = db.ToolCommand(opts.Tool, "")
```

to:

```go
s.Command = db.ToolCommand(opts.Tool, opts.Command)
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/session/... -v
```

Expected: PASS for all tests including the two new ones.

**Step 5: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "feat: forward custom command through session.Update"
```

---

### Task 2: Add dynamic Command field to new-session dialog

**Files:**
- Modify: `internal/ui/dialogs/new.go`

**Step 1: Add `Command` to `NewSessionResult`**

Change:

```go
type NewSessionResult struct {
	Title       string
	Tool        db.Tool
	ProjectPath string
	GroupPath   string
}
```

to:

```go
type NewSessionResult struct {
	Title       string
	Tool        db.Tool
	Command     string
	ProjectPath string
	GroupPath   string
}
```

**Step 2: Add dynamic show/hide logic**

In `NewSessionDialog`, after the form fields are added (after the `if len(groups) > 0` block that wires up the group->tool sync), add the following. Replace the existing `toolDD` variable assignment if it already exists in scope, or add these new lines:

```go
var commandShown bool
currentCmd := ""

setCommandVisible := func(show bool) {
	if show == commandShown {
		return
	}
	if show {
		form.AddFormItem(tview.NewInputField().
			SetLabel("Command").
			SetFieldWidth(40).
			SetText(currentCmd))
		commandShown = true
	} else {
		if commandShown {
			currentCmd = form.GetFormItemByLabel("Command").(*tview.InputField).GetText()
		}
		form.RemoveFormItem(form.GetFormItemCount() - 1)
		commandShown = false
	}
}
```

Wire the Tool dropdown to call `setCommandVisible`:

```go
toolDD := form.GetFormItemByLabel("Tool").(*tview.DropDown)
toolDD.SetSelectedFunc(func(text string, _ int) {
	setCommandVisible(text == "custom")
})
```

Also update the Group->Tool sync block to call `setCommandVisible` after updating the tool (so switching groups also updates command visibility). Replace the existing `groupDD.SetSelectedFunc` with:

```go
groupDD.SetSelectedFunc(func(text string, _ int) {
	for i, n := range groupNames {
		if n == text {
			newTool := resolveGroupTool(groups, groupPaths[i], defaultTool)
			for ti, t := range tools {
				if t == newTool {
					toolDD.SetCurrentOption(ti)
					break
				}
			}
			setCommandVisible(newTool == "custom")
			break
		}
	}
})
```

Show Command on initial load if the initial tool is custom:

```go
if initialTool == "custom" {
	setCommandVisible(true)
}
```

**Step 3: Read Command on submit**

In the `form.AddButton("Create", ...)` handler, after reading `toolStr`, add:

```go
command := ""
if commandShown {
	command = form.GetFormItemByLabel("Command").(*tview.InputField).GetText()
}
```

And add `Command: command` to the `onSubmit` call:

```go
onSubmit(NewSessionResult{
	Title:       title,
	Tool:        db.Tool(toolStr),
	Command:     command,
	ProjectPath: projectPath,
	GroupPath:   groupPath,
})
```

**Step 4: Build to verify no compile errors**

```bash
go build ./...
```

Expected: build succeeds (app.go doesn't use Command yet, but that's fine -- it compiles with the zero value).

**Step 5: Commit**

```bash
git add internal/ui/dialogs/new.go
git commit -m "feat: add dynamic Command field to new-session dialog"
```

---

### Task 3: Add dynamic Command field to edit-session dialog

**Files:**
- Modify: `internal/ui/dialogs/edit_session.go`

**Step 1: Add `Command` to `EditSessionResult`**

Change:

```go
type EditSessionResult struct {
	Title       string
	Tool        db.Tool
	ProjectPath string
	GroupPath   string
}
```

to:

```go
type EditSessionResult struct {
	Title       string
	Tool        db.Tool
	Command     string
	ProjectPath string
	GroupPath   string
}
```

**Step 2: Add dynamic show/hide logic**

In `EditSessionDialog`, after the form fields are added, add:

```go
var commandShown bool
currentCmd := ""
if s.Tool == db.ToolCustom {
	currentCmd = s.Command
}

setCommandVisible := func(show bool) {
	if show == commandShown {
		return
	}
	if show {
		form.AddFormItem(tview.NewInputField().
			SetLabel("Command").
			SetFieldWidth(40).
			SetText(currentCmd))
		commandShown = true
	} else {
		if commandShown {
			currentCmd = form.GetFormItemByLabel("Command").(*tview.InputField).GetText()
		}
		form.RemoveFormItem(form.GetFormItemCount() - 1)
		commandShown = false
	}
}

toolDD := form.GetFormItemByLabel("Tool").(*tview.DropDown)
toolDD.SetSelectedFunc(func(text string, _ int) {
	setCommandVisible(text == "custom")
})

// Show Command field immediately if session already uses custom tool
if s.Tool == db.ToolCustom {
	setCommandVisible(true)
}
```

**Step 3: Read Command on submit**

In the `form.AddButton("Save", ...)` handler, after reading `toolStr`, add:

```go
command := ""
if commandShown {
	command = form.GetFormItemByLabel("Command").(*tview.InputField).GetText()
}
```

And add `Command: command` to the `onSubmit` call:

```go
onSubmit(EditSessionResult{
	Title:       title,
	Tool:        db.Tool(toolStr),
	Command:     command,
	ProjectPath: projectPath,
	GroupPath:   groupPath,
})
```

**Step 4: Build to verify no compile errors**

```bash
go build ./...
```

Expected: build succeeds.

**Step 5: Commit**

```bash
git add internal/ui/dialogs/edit_session.go
git commit -m "feat: add dynamic Command field to edit-session dialog"
```

---

### Task 4: Wire Command through app.go

**Files:**
- Modify: `internal/ui/app.go`

**Step 1: Pass Command in new-session handler**

Find the `session.CreateOptions` literal in the new-session handler (around line 145). Add `Command: result.Command`:

```go
opts := session.CreateOptions{
	Title:     result.Title,
	Tool:      result.Tool,
	Command:   result.Command,
	GroupPath: result.GroupPath,
}
```

**Step 2: Pass Command in edit-session handler**

Find the `session.UpdateOptions` literal in the edit-session handler. Add `Command: result.Command`:

```go
if err := a.mgr.Update(item.session.ID, session.UpdateOptions{
	Title:       result.Title,
	Tool:        result.Tool,
	Command:     result.Command,
	ProjectPath: result.ProjectPath,
	GroupPath:   result.GroupPath,
}); err != nil {
```

**Step 3: Build and run all tests**

```bash
go build ./...
go test ./...
```

Expected: build succeeds, all tests pass.

**Step 4: Commit**

```bash
git add internal/ui/app.go
git commit -m "feat: wire custom command from dialogs to session manager"
```

---

### Task 5: Manual smoke test

Start the app and verify:

1. Open new-session dialog -- no Command field visible
2. Switch Tool to "custom" -- Command field appears at the bottom
3. Switch Tool to "claude" -- Command field disappears
4. Switch back to "custom", enter `my-tool --flag`, create session -- tmux session starts with `my-tool --flag`
5. Edit that session -- Command field shows pre-filled with `my-tool --flag`
6. Change command to `my-tool --verbose`, save, restart session -- new command used
7. Edit the session, switch tool to "claude", save -- Command field was hidden, session command is now `claude`
