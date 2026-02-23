# Git History Browser Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a read-only two-pane git history browser to the session menu, accessible via `Ctrl+\` then `h`.

**Architecture:** New `gitlogcmd` subcommand package following the `notescmd` pattern. `menucmd` schedules a `tmux display-popup` via `tmux run-shell -b` after the menu closes. The popup runs `agent-workspace gitlog <tmux-session-name>`, which opens a tview UI with a commit list on the left (`tview.Table`) and a diff viewer on the right (`tview.TextView` with ANSI color).

**Tech Stack:** Go, tview, tcell, os/exec

---

### Task 1: Create `gitlogcmd` package with commit type and log parser (TDD)

**Files:**
- Create: `internal/ui/gitlogcmd/gitlogcmd.go`
- Create: `internal/ui/gitlogcmd/gitlogcmd_test.go`

**Step 1: Write the failing test**

Create `internal/ui/gitlogcmd/gitlogcmd_test.go`:

```go
package gitlogcmd

import "testing"

func TestParseLogLine(t *testing.T) {
	c, ok := parseLogLine("abc1234\tFix auth bug\t2 hours ago")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if c.Hash != "abc1234" {
		t.Errorf("hash: got %q want %q", c.Hash, "abc1234")
	}
	if c.Subject != "Fix auth bug" {
		t.Errorf("subject: got %q want %q", c.Subject, "Fix auth bug")
	}
	if c.RelDate != "2 hours ago" {
		t.Errorf("reldate: got %q want %q", c.RelDate, "2 hours ago")
	}

	_, ok2 := parseLogLine("malformed no tabs")
	if ok2 {
		t.Error("expected ok=false for malformed line")
	}

	_, ok3 := parseLogLine("")
	if ok3 {
		t.Error("expected ok=false for empty line")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/ui/gitlogcmd/...
```

Expected: FAIL — `package not found` or `undefined: parseLogLine`

**Step 3: Create `internal/ui/gitlogcmd/gitlogcmd.go` with just the type and parser**

```go
package gitlogcmd

import "strings"

type commit struct {
	Hash    string
	Subject string
	RelDate string
}

// parseLogLine parses a single line from:
//   git log --pretty=format:"%h\t%s\t%ar"
func parseLogLine(line string) (commit, bool) {
	parts := strings.SplitN(line, "\t", 3)
	if len(parts) != 3 {
		return commit{}, false
	}
	return commit{Hash: parts[0], Subject: parts[1], RelDate: parts[2]}, true
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/ui/gitlogcmd/...
```

Expected: `ok  github.com/zsprackett/agent-workspace/internal/ui/gitlogcmd`

**Step 5: Commit**

```bash
git add internal/ui/gitlogcmd/
git commit -m "feat(gitlogcmd): add commit type and log line parser"
```

---

### Task 2: Implement `Run` in `gitlogcmd.go`

**Files:**
- Modify: `internal/ui/gitlogcmd/gitlogcmd.go`

**Step 1: Add the full implementation**

Replace the contents of `internal/ui/gitlogcmd/gitlogcmd.go` with:

```go
package gitlogcmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zsprackett/agent-workspace/internal/config"
	"github.com/zsprackett/agent-workspace/internal/db"
)

type commit struct {
	Hash    string
	Subject string
	RelDate string
}

// parseLogLine parses a single line from:
//
//	git log --pretty=format:"%h\t%s\t%ar"
func parseLogLine(line string) (commit, bool) {
	parts := strings.SplitN(line, "\t", 3)
	if len(parts) != 3 {
		return commit{}, false
	}
	return commit{Hash: parts[0], Subject: parts[1], RelDate: parts[2]}, true
}

func loadCommits(path string) ([]commit, error) {
	out, err := exec.Command("git", "-C", path, "log",
		"--pretty=format:%h\t%s\t%ar", "-n", "200").Output()
	if err != nil {
		return nil, err
	}
	var commits []commit
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if c, ok := parseLogLine(line); ok {
			commits = append(commits, c)
		}
	}
	return commits, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// Run opens a tview two-pane git history browser for the session identified
// by its tmux session name. Intended to be called inside a tmux display-popup.
func Run(tmuxSession string) error {
	store, err := db.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer store.Close()

	s, err := store.GetSessionByTmuxName(tmuxSession)
	if err != nil || s == nil {
		return fmt.Errorf("session not found: %s", tmuxSession)
	}

	path := s.WorktreePath
	if path == "" {
		path = s.ProjectPath
	}

	commits, _ := loadCommits(path)

	app := tview.NewApplication()

	// --- left pane: commit list ---
	table := tview.NewTable()
	table.SetBorder(true).
		SetTitle(fmt.Sprintf(" %s ", s.Title)).
		SetTitleAlign(tview.AlignLeft)
	table.SetBackgroundColor(tcell.ColorDefault)
	table.SetSelectable(true, false)

	if len(commits) == 0 {
		table.SetCell(0, 0, tview.NewTableCell("No commits found").
			SetTextColor(tcell.ColorGray))
	} else {
		for i, c := range commits {
			table.SetCell(i, 0, tview.NewTableCell(c.Hash+" ").
				SetTextColor(tcell.ColorYellow).
				SetSelectable(true))
			table.SetCell(i, 1, tview.NewTableCell(truncate(c.Subject, 38)+" ").
				SetSelectable(true))
			table.SetCell(i, 2, tview.NewTableCell(c.RelDate).
				SetTextColor(tcell.ColorGray).
				SetAlign(tview.AlignRight).
				SetSelectable(true))
		}
	}

	// --- right pane: diff viewer ---
	diff := tview.NewTextView()
	diff.SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	diff.SetBorder(true).
		SetTitle(" Diff ").
		SetTitleAlign(tview.AlignLeft)
	diff.SetBackgroundColor(tcell.ColorDefault)

	showCommit := func(hash string) {
		diff.Clear()
		out, err := exec.Command("git", "-C", path, "show", "--color=always", hash).Output()
		w := tview.ANSIWriter(diff)
		if err != nil {
			fmt.Fprintf(w, "error: %v\n", err)
			return
		}
		fmt.Fprint(w, string(out))
		diff.ScrollToBeginning()
	}

	if len(commits) > 0 {
		table.SetSelectionChangedFunc(func(row, _ int) {
			if row >= 0 && row < len(commits) {
				showCommit(commits[row].Hash)
			}
		})
		table.Select(0, 0)
		showCommit(commits[0].Hash)
	}

	// --- j/k scrolling in diff pane ---
	diff.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		row, col := diff.GetScrollOffset()
		switch event.Rune() {
		case 'j':
			diff.ScrollTo(row+1, col)
			return nil
		case 'k':
			if row > 0 {
				diff.ScrollTo(row-1, col)
			}
			return nil
		}
		return event
	})

	// --- layout ---
	panes := tview.NewFlex().
		AddItem(table, 0, 1, true).
		AddItem(diff, 0, 1, false)

	hint := tview.NewTextView().
		SetDynamicColors(true).
		SetText("  [::d][Tab][-] switch pane   [::d][↑↓][-] navigate   [::d][j/k][-] scroll diff   [::d][Esc][-] close")
	hint.SetBackgroundColor(tcell.ColorDefault)

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(panes, 0, 1, true).
		AddItem(hint, 1, 0, false)

	focusedLeft := true
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			app.Stop()
			return nil
		case tcell.KeyTab:
			focusedLeft = !focusedLeft
			if focusedLeft {
				app.SetFocus(table)
			} else {
				app.SetFocus(diff)
			}
			return nil
		}
		return event
	})

	return app.SetRoot(root, true).EnableMouse(false).Run()
}
```

**Step 2: Verify tests still pass (parser tests must not break)**

```bash
go test ./internal/ui/gitlogcmd/...
```

Expected: `ok  github.com/zsprackett/agent-workspace/internal/ui/gitlogcmd`

**Step 3: Build to verify it compiles**

```bash
make build
```

Expected: builds with no errors.

**Step 4: Commit**

```bash
git add internal/ui/gitlogcmd/gitlogcmd.go
git commit -m "feat(gitlogcmd): implement two-pane git history browser"
```

---

### Task 3: Register `gitlog` subcommand in `main.go`

**Files:**
- Modify: `main.go`

**Step 1: Add the import and subcommand handler**

In `main.go`, add the import (alongside the other `ui/` subpackages):

```go
"github.com/zsprackett/agent-workspace/internal/ui/gitlogcmd"
```

Add the subcommand handler after the `notes` block and before the `menu` block:

```go
// gitlog subcommand: invoked from within a tmux session via display-popup
if len(os.Args) == 3 && os.Args[1] == "gitlog" {
    if err := gitlogcmd.Run(os.Args[2]); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
    return
}
```

**Step 2: Build to verify**

```bash
make build
```

Expected: builds with no errors.

**Step 3: Commit**

```bash
git add main.go
git commit -m "feat: register gitlog subcommand"
```

---

### Task 4: Add `h` key to `menucmd`

**Files:**
- Modify: `internal/ui/menucmd/menucmd.go`

**Step 1: Update the menu text**

The current `menuText` const has git operations `s` and `d` then `p`. Insert `h` between `d` and `p`:

Replace:
```go
const menuText = `
  [green]s[-]  Git status
  [green]d[-]  Git diff
  [green]p[-]  Open PR in browser
  [green]n[-]  Session notes
  [green]t[-]  Open terminal split
  [green]x[-]  Detach to dashboard

  [::d]Esc  cancel[-]`
```

With:
```go
const menuText = `
  [green]s[-]  Git status
  [green]d[-]  Git diff
  [green]h[-]  Git history
  [green]p[-]  Open PR in browser
  [green]n[-]  Session notes
  [green]t[-]  Open terminal split
  [green]x[-]  Detach to dashboard

  [::d]Esc  cancel[-]`
```

**Step 2: Add the `h` case to the input handler**

In `Run`, inside the `switch event.Rune()` block, add `h` after `d`:

```go
case 'h':
    app.Stop()
    openGitLog(tmuxSession)
```

**Step 3: Add the `openGitLog` function**

At the bottom of the file, after `openPR`:

```go
func openGitLog(tmuxSession string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	gitlogCmd := fmt.Sprintf("%q gitlog %q", exe, tmuxSession)
	popupCmd := fmt.Sprintf("sleep 0.3 && tmux display-popup -E -t %q -w 160 -h 45 %q", tmuxSession, gitlogCmd)
	exec.Command("tmux", "run-shell", "-b", popupCmd).Run()
}
```

**Step 4: Build to verify**

```bash
make build
```

Expected: builds with no errors.

**Step 5: Run all tests**

```bash
make test
```

Expected: all packages pass.

**Step 6: Commit**

```bash
git add internal/ui/menucmd/menucmd.go
git commit -m "feat(menucmd): add h key to open git history browser"
```

---

### Task 5: Install and smoke test

**Step 1: Install**

```bash
make install
```

**Step 2: Manual smoke test**

Inside an active tmux session managed by agent-workspace:
1. Press `Ctrl+\` -- menu should appear with `h  Git history` listed
2. Press `h` -- menu closes, history popup opens after 0.3s
3. Verify: commit list populates on the left with hash / subject / relative date columns
4. Verify: diff for the first commit renders on the right with syntax highlighting
5. Press `↑`/`↓` -- diff pane updates to match selected commit
6. Press `Tab` -- focus moves to diff pane
7. Press `j`/`k` -- diff pane scrolls
8. Press `Tab` again -- focus returns to commit list
9. Press `Esc` -- popup closes

**Step 3: Push**

```bash
git push
```
