package dialogs

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type GroupResult struct {
	Name    string
	RepoURL string
}

func GroupDialog(title, currentName, currentRepoURL string, onSubmit func(GroupResult), onCancel func()) *tview.Form {
	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" " + title + " ").SetTitleAlign(tview.AlignLeft)
	form.SetBackgroundColor(tcell.ColorDefault)
	form.SetFieldBackgroundColor(tcell.ColorDefault)

	form.AddInputField("Group name", currentName, 40, nil, nil)
	form.AddInputField("GitHub URL (optional)", currentRepoURL, 50, nil, nil)
	form.AddButton("OK", func() {
		name := form.GetFormItemByLabel("Group name").(*tview.InputField).GetText()
		if name != "" {
			repoURL := form.GetFormItemByLabel("GitHub URL (optional)").(*tview.InputField).GetText()
			onSubmit(GroupResult{Name: name, RepoURL: repoURL})
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
