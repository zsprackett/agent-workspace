package dialogs

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func RenameDialog(current string, onSubmit func(string), onCancel func()) *tview.Form {
	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" Rename ").SetTitleAlign(tview.AlignLeft)
	form.SetBackgroundColor(tcell.ColorDefault)
	form.SetFieldBackgroundColor(tcell.ColorDefault)

	form.AddInputField("New name", current, 40, nil, nil)
	form.AddButton("Rename", func() {
		name := form.GetFormItemByLabel("New name").(*tview.InputField).GetText()
		if name != "" {
			onSubmit(name)
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
}
