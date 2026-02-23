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

// leftPaneWidth computes the minimum width needed to display the commit list
// without truncating any column: 2 (border) + 8 (hash+space) + 39 (subject+space) + maxRelDate + 2 (buffer).
func leftPaneWidth(commits []commit) int {
	maxRelDate := 10
	for _, c := range commits {
		if l := len(c.RelDate); l > maxRelDate {
			maxRelDate = l
		}
	}
	return 51 + maxRelDate
}

// Run opens a tview three-pane git history browser for the session identified
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

	// --- top-left pane: full commit message ---
	msgView := tview.NewTextView()
	msgView.SetScrollable(true).SetWrap(true)
	msgView.SetBorder(true).
		SetTitle(" Message ").
		SetTitleAlign(tview.AlignLeft)
	msgView.SetBackgroundColor(tcell.ColorDefault)

	// --- bottom-left pane: commit list ---
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
		// Update commit message pane.
		msgView.Clear()
		if msgOut, err := exec.Command("git", "-C", path, "log", "-1", "--pretty=format:%B", hash).Output(); err == nil {
			fmt.Fprint(msgView, strings.TrimSpace(string(msgOut)))
		}
		msgView.ScrollToBeginning()

		// Update diff pane.
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
	// Left column: message pane (1/4) above commit list (3/4), fixed width.
	leftCol := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(msgView, 0, 1, false).
		AddItem(table, 0, 3, true)

	panes := tview.NewFlex().
		AddItem(leftCol, leftPaneWidth(commits), 0, true).
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
