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
		case 'p':
			app.Stop()
			openPR(panePath)
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

func openPR(path string) {
	exec.Command("sh", "-c",
		fmt.Sprintf(`cd %q && url=$(gh pr view --json url --jq .url 2>/dev/null) && [ -n "$url" ] && { open "$url" 2>/dev/null || xdg-open "$url" 2>/dev/null; } || tmux display-message "No open PR found for this branch"`, path),
	).Run()
}

func openNotes(tmuxSession string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	exec.Command("tmux", "display-popup", "-E", "-w", "64", "-h", "22",
		fmt.Sprintf("%q notes %q", exe, tmuxSession)).Run()
}

func openTerminal(path string) {
	exec.Command("tmux", "split-window", "-v", "-c", path).Run()
}

