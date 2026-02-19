# Group Default Tool Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `DefaultTool` field to groups so new sessions inherit the group's preferred tool, falling back to `cfg.DefaultTool` then `"claude"`.

**Architecture:** Three-layer change: DB migration adds `default_tool` column to groups, `GroupDialog` gains a Tool dropdown, and `NewSessionDialog` resolves the initial tool from the selected group's default. Sessions retain their own `Tool` field for per-session override.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), tview

---

### Task 1: Add DefaultTool to db.Group and migrate

**Files:**
- Modify: `internal/db/types.go`
- Modify: `internal/db/db.go`
- Modify: `internal/db/db_test.go`

**Step 1: Write the failing test**

Add to `internal/db/db_test.go`:

```go
func TestGroupDefaultToolRoundTrip(t *testing.T) {
	store, _ := db.Open(":memory:")
	defer store.Close()
	store.Migrate()

	groups := []*db.Group{
		{Path: "ai-work", Name: "AI Work", Expanded: true, SortOrder: 0, DefaultTool: db.ToolOpenCode},
		{Path: "shell-work", Name: "Shell Work", Expanded: true, SortOrder: 1, DefaultTool: db.ToolShell},
		{Path: "no-tool", Name: "No Tool", Expanded: true, SortOrder: 2},
	}
	if err := store.SaveGroups(groups); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.LoadGroups()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got[0].DefaultTool != db.ToolOpenCode {
		t.Errorf("DefaultTool[0]: got %q want %q", got[0].DefaultTool, db.ToolOpenCode)
	}
	if got[1].DefaultTool != db.ToolShell {
		t.Errorf("DefaultTool[1]: got %q want %q", got[1].DefaultTool, db.ToolShell)
	}
	if got[2].DefaultTool != "" {
		t.Errorf("DefaultTool[2]: got %q want empty", got[2].DefaultTool)
	}
}
```

**Step 2: Run test to confirm it fails**

```bash
go test ./internal/db/... -run TestGroupDefaultToolRoundTrip -v
```

Expected: compile error - `db.Group` has no field `DefaultTool`.

**Step 3: Add DefaultTool to db.Group**

In `internal/db/types.go`, change:

```go
type Group struct {
	Path        string
	Name        string
	Expanded    bool
	SortOrder   int
	DefaultPath string
	RepoURL     string
}
```

To:

```go
type Group struct {
	Path        string
	Name        string
	Expanded    bool
	SortOrder   int
	DefaultPath string
	RepoURL     string
	DefaultTool Tool
}
```

**Step 4: Add migration in db.go**

In `internal/db/db.go`, after the existing `ALTER TABLE groups ADD COLUMN repo_url` block, add:

```go
// Add default_tool column to existing groups tables; ignore "duplicate column" errors.
if _, alterErr := d.sql.Exec(`ALTER TABLE groups ADD COLUMN default_tool TEXT NOT NULL DEFAULT ''`); alterErr != nil {
	if !isDuplicateColumnError(alterErr) {
		return fmt.Errorf("alter groups add default_tool: %w", alterErr)
	}
}
```

**Step 5: Update SaveGroups and LoadGroups**

In `SaveGroups`, change:

```go
if _, err := tx.Exec(
    "INSERT INTO groups (path, name, expanded, sort_order, default_path, repo_url) VALUES (?,?,?,?,?,?)",
    g.Path, g.Name, boolToInt(g.Expanded), g.SortOrder, g.DefaultPath, g.RepoURL,
); err != nil {
```

To:

```go
if _, err := tx.Exec(
    "INSERT INTO groups (path, name, expanded, sort_order, default_path, repo_url, default_tool) VALUES (?,?,?,?,?,?,?)",
    g.Path, g.Name, boolToInt(g.Expanded), g.SortOrder, g.DefaultPath, g.RepoURL, string(g.DefaultTool),
); err != nil {
```

In `LoadGroups`, change the query and scan:

```go
rows, err := d.sql.Query("SELECT path, name, expanded, sort_order, default_path, repo_url, default_tool FROM groups ORDER BY sort_order")
```

Change the scan block from:

```go
var expanded int
if err := rows.Scan(&g.Path, &g.Name, &expanded, &g.SortOrder, &g.DefaultPath, &g.RepoURL); err != nil {
    return nil, err
}
g.Expanded = expanded == 1
```

To:

```go
var expanded int
var defaultTool string
if err := rows.Scan(&g.Path, &g.Name, &expanded, &g.SortOrder, &g.DefaultPath, &g.RepoURL, &defaultTool); err != nil {
    return nil, err
}
g.Expanded = expanded == 1
g.DefaultTool = Tool(defaultTool)
```

**Step 6: Run test to confirm it passes**

```bash
go test ./internal/db/... -v
```

Expected: all tests pass.

**Step 7: Commit**

```bash
git add internal/db/types.go internal/db/db.go internal/db/db_test.go
git commit -m "feat: add DefaultTool field to Group with DB migration"
```

---

### Task 2: Add Default Tool dropdown to GroupDialog

**Files:**
- Modify: `internal/ui/dialogs/group.go`
- Modify: `internal/ui/app.go`

**Step 1: Update GroupResult and GroupDialog signature**

In `internal/ui/dialogs/group.go`, change:

```go
type GroupResult struct {
	Name    string
	RepoURL string
}

func GroupDialog(title, currentName, currentRepoURL string, onSubmit func(GroupResult), onCancel func()) *tview.Form {
```

To:

```go
type GroupResult struct {
	Name        string
	RepoURL     string
	DefaultTool string
}

func GroupDialog(title, currentName, currentRepoURL, currentDefaultTool string, onSubmit func(GroupResult), onCancel func()) *tview.Form {
```

**Step 2: Add the Tool dropdown inside GroupDialog**

The full updated function body (replace everything after the signature line):

```go
	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" " + title + " ").SetTitleAlign(tview.AlignLeft)
	form.SetBackgroundColor(tcell.ColorDefault)
	form.SetFieldBackgroundColor(tcell.ColorDefault)

	toolLabels := []string{"(none)", "claude", "opencode", "gemini", "codex", "shell", "custom"}
	toolValues := []string{"", "claude", "opencode", "gemini", "codex", "shell", "custom"}
	currentToolIdx := 0
	for i, v := range toolValues {
		if v == currentDefaultTool {
			currentToolIdx = i
			break
		}
	}

	form.AddInputField("Group name", currentName, 40, nil, nil)
	form.AddInputField("GitHub URL (optional)", currentRepoURL, 50, nil, nil)
	form.AddDropDown("Default Tool", toolLabels, currentToolIdx, nil)
	form.AddButton("OK", func() {
		name := form.GetFormItemByLabel("Group name").(*tview.InputField).GetText()
		if name != "" {
			repoURL := form.GetFormItemByLabel("GitHub URL (optional)").(*tview.InputField).GetText()
			_, toolLabel := form.GetFormItemByLabel("Default Tool").(*tview.DropDown).GetCurrentOption()
			defaultTool := ""
			for i, l := range toolLabels {
				if l == toolLabel {
					defaultTool = toolValues[i]
					break
				}
			}
			onSubmit(GroupResult{Name: name, RepoURL: repoURL, DefaultTool: defaultTool})
		}
	})
	form.AddButton("Cancel", onCancel)
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			onCancel()
			return nil
		}
		return event
	})
	return form
```

**Step 3: Update call sites in app.go**

In `onNewGroup`, change:

```go
form := dialogs.GroupDialog("New Group", "", "", func(result dialogs.GroupResult) {
```

To:

```go
form := dialogs.GroupDialog("New Group", "", "", "", func(result dialogs.GroupResult) {
```

And inside the submit callback, change:

```go
groups = append(groups, &db.Group{
    Path:      path,
    Name:      result.Name,
    Expanded:  true,
    SortOrder: len(groups),
    RepoURL:   result.RepoURL,
})
```

To:

```go
groups = append(groups, &db.Group{
    Path:        path,
    Name:        result.Name,
    Expanded:    true,
    SortOrder:   len(groups),
    RepoURL:     result.RepoURL,
    DefaultTool: db.Tool(result.DefaultTool),
})
```

In `onEdit` (the group branch), change:

```go
form := dialogs.GroupDialog("Edit Group", item.group.Name, item.group.RepoURL,
    func(result dialogs.GroupResult) {
        a.closeDialog("edit")
        groups, _ := a.store.LoadGroups()
        for _, g := range groups {
            if g.Path == item.group.Path {
                g.Name = result.Name
                g.RepoURL = result.RepoURL
            }
        }
```

To:

```go
form := dialogs.GroupDialog("Edit Group", item.group.Name, item.group.RepoURL, string(item.group.DefaultTool),
    func(result dialogs.GroupResult) {
        a.closeDialog("edit")
        groups, _ := a.store.LoadGroups()
        for _, g := range groups {
            if g.Path == item.group.Path {
                g.Name = result.Name
                g.RepoURL = result.RepoURL
                g.DefaultTool = db.Tool(result.DefaultTool)
            }
        }
```

Also update the `showDialog` height for group dialogs from `12` to `14` in both `onNewGroup` and `onEdit` (group branch).

**Step 4: Build to verify**

```bash
make build
```

Expected: binary built with no errors.

**Step 5: Commit**

```bash
git add internal/ui/dialogs/group.go internal/ui/app.go
git commit -m "feat: add Default Tool dropdown to group create/edit dialogs"
```

---

### Task 3: Use group's DefaultTool in NewSessionDialog

**Files:**
- Modify: `internal/ui/dialogs/new.go`

**Step 1: Add resolveGroupTool helper and update initial tool selection**

In `internal/ui/dialogs/new.go`, add a package-level helper (above `NewSessionDialog`):

```go
// resolveGroupTool returns the tool to pre-select for a given group.
// Priority: group DefaultTool > cfgDefault > "claude".
func resolveGroupTool(groups []*db.Group, groupPath, cfgDefault string) string {
	for _, g := range groups {
		if g.Path == groupPath {
			if g.DefaultTool != "" {
				return string(g.DefaultTool)
			}
			break
		}
	}
	if cfgDefault != "" {
		return cfgDefault
	}
	return "claude"
}
```

**Step 2: Update initial tool index computation**

In `NewSessionDialog`, replace:

```go
tools := []string{"claude", "opencode", "gemini", "codex", "shell", "custom"}
defaultToolIdx := 0
for i, t := range tools {
    if t == defaultTool {
        defaultToolIdx = i
        break
    }
}
```

With:

```go
tools := []string{"claude", "opencode", "gemini", "codex", "shell", "custom"}
initialTool := resolveGroupTool(groups, defaultGroup, defaultTool)
defaultToolIdx := 0
for i, t := range tools {
    if t == initialTool {
        defaultToolIdx = i
        break
    }
}
```

**Step 3: Add Group dropdown change handler**

After the `if len(groups) > 0 { form.AddDropDown("Group", ...) }` block, add:

```go
if len(groups) > 0 {
    groupDD := form.GetFormItemByLabel("Group").(*tview.DropDown)
    toolDD := form.GetFormItemByLabel("Tool").(*tview.DropDown)
    groupDD.SetChangedFunc(func(text string, _ int) {
        for i, n := range groupNames {
            if n == text {
                newTool := resolveGroupTool(groups, groupPaths[i], defaultTool)
                for ti, t := range tools {
                    if t == newTool {
                        toolDD.SetCurrentOption(ti)
                        break
                    }
                }
                break
            }
        }
    })
}
```

**Step 4: Build to verify**

```bash
make build
```

Expected: binary built with no errors.

**Step 5: Run all tests**

```bash
make test
```

Expected: all pass.

**Step 6: Commit**

```bash
git add internal/ui/dialogs/new.go
git commit -m "feat: pre-select group's default tool in new session dialog"
```
