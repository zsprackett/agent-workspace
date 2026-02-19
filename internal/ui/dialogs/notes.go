package dialogs

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// NotesDialog returns a form for viewing and editing session notes.
// onSave is called with the updated notes text; onCancel dismisses without saving.
func NotesDialog(sessionTitle, currentNotes string, onSave func(notes string), onCancel func()) *tview.Form {
	form := tview.NewForm()
	form.SetBorder(true).
		SetTitle(fmt.Sprintf(" Notes: %s ", sessionTitle)).
		SetTitleAlign(tview.AlignLeft)
	form.SetBackgroundColor(tcell.ColorDefault)
	form.SetFieldBackgroundColor(tcell.ColorDefault)

	form.AddTextArea("Notes", currentNotes, 54, 10, 0, nil)

	form.AddButton("Save", func() {
		notes := form.GetFormItemByLabel("Notes").(*tview.TextArea).GetText()
		onSave(notes)
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
}
