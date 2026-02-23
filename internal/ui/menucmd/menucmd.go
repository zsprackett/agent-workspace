package menucmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const menuText = `
  [green]s[-]  Git status
  [green]d[-]  Git diff
  [green]h[-]  Git history
  [green]p[-]  Open PR in browser
  [green]n[-]  Session notes
  [green]t[-]  Open terminal split
  [green]x[-]  Detach to dashboard

  [::d]Esc  cancel[-]`

// Run shows the command legend and dispatches the chosen action.
// tmuxSession is the tmux session name. panePath is read from os.Getwd() --
// tmux sets the display-popup working directory to the active pane's CWD.
func Run(tmuxSession string) error {
	panePath, _ := os.Getwd()
	app := tview.NewApplication()

	tv := tview.NewTextView().
		SetDynamicColors(true).
		SetText(menuText)
	tv.SetBorder(true).
		SetTitle(fmt.Sprintf(" %s ", tmuxSession)).
		SetTitleAlign(tview.AlignLeft)
	tv.SetBackgroundColor(tcell.ColorDefault)

	tv.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			app.Stop()
			return nil
		}
		switch event.Rune() {
		case 's':
			app.Stop()
			gitStatus(panePath)
		case 'd':
			app.Stop()
			gitDiff(panePath)
		case 'h':
			app.Stop()
			openGitLog(tmuxSession)
		case 'p':
			app.Stop()
			openPR(panePath, tmuxSession)
		case 'n':
			app.Stop()
			openNotes(tmuxSession)
		case 't':
			app.Stop()
			openTerminal(panePath)
		case 'x':
			app.Stop()
			exec.Command("tmux", "detach-client").Run()
		}
		return nil
	})

	return app.SetRoot(tv, true).EnableMouse(false).Run()
}

func gitStatus(path string) {
	exec.Command("tmux", "split-window", "-v", "-l", "15", "-c", path,
		`git status; printf '\nPress enter to close...'; read`).Run()
}

func gitDiff(path string) {
	exec.Command("tmux", "split-window", "-v", "-l", "20", "-c", path,
		`out=$(git diff HEAD --color=always; git ls-files --others --exclude-standard -z | xargs -0 -I{} git diff --no-index --color=always -- /dev/null {} 2>/dev/null); if [ -n "$out" ]; then printf '%s\n' "$out" | less -RX; else printf 'No changes.\n\nPress enter to close...'; read; fi`).Run()
}

func openPR(path, tmuxSession string) {
	script := fmt.Sprintf(
		`cd %q && url=$(gh pr view --json url --jq .url 2>/dev/null) && [ -n "$url" ] && { open "$url" 2>/dev/null || xdg-open "$url" 2>/dev/null; } || tmux display-message -t %q "No open PR found for this branch"`,
		path, tmuxSession,
	)
	exec.Command("tmux", "run-shell", "-b", "sleep 0.3 && "+script).Run() //nolint:errcheck
}

func openNotes(tmuxSession string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	// display-popup cannot be opened from within an existing popup.
	// Use run-shell -b to schedule the notes popup from the tmux server
	// context after the menu popup closes.
	notesCmd := fmt.Sprintf("%q notes %q", exe, tmuxSession)
	popupCmd := fmt.Sprintf("sleep 0.3 && tmux display-popup -E -t %q -w 64 -h 22 %q", tmuxSession, notesCmd)
	exec.Command("tmux", "run-shell", "-b", popupCmd).Run()
}

func openTerminal(path string) {
	exec.Command("tmux", "split-window", "-v", "-c", path).Run()
}

func openGitLog(tmuxSession string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	gitlogCmd := fmt.Sprintf("%q gitlog %q", exe, tmuxSession)
	popupCmd := fmt.Sprintf("sleep 0.3 && tmux display-popup -E -t %q -w 90%% -h 90%% %q", tmuxSession, gitlogCmd)
	exec.Command("tmux", "run-shell", "-b", popupCmd).Run()
}

