package notescmd

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zsprackett/agent-workspace/internal/config"
	"github.com/zsprackett/agent-workspace/internal/db"
)

// Run opens a tview notes editor for the session identified by its tmux session
// name. Intended to be called as a subcommand inside a tmux display-popup.
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

	app := tview.NewApplication()

	form := tview.NewForm()
	form.SetBorder(true).
		SetTitle(fmt.Sprintf(" Notes: %s ", s.Title)).
		SetTitleAlign(tview.AlignLeft)
	form.SetBackgroundColor(tcell.ColorDefault)
	form.SetFieldBackgroundColor(tcell.ColorDefault)

	form.AddTextArea("", s.Notes, 56, 14, 0, nil)

	form.AddButton("Save", func() {
		notes := form.GetFormItem(0).(*tview.TextArea).GetText()
		store.UpdateSessionNotes(s.ID, notes)
		store.Touch()
		app.Stop()
	})

	form.AddButton("Cancel", func() {
		app.Stop()
	})

	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			app.Stop()
			return nil
		}
		return event
	})

	return app.SetRoot(form, true).EnableMouse(false).Run()
}
