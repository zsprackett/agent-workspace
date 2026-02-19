package dialogs

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zsprackett/agent-workspace/internal/db"
)

func MoveDialog(groups []*db.Group, onSubmit func(groupPath string), onCancel func()) *tview.List {
	list := tview.NewList()
	list.SetBorder(true).SetTitle(" Move to Group ").SetTitleAlign(tview.AlignLeft)
	list.SetBackgroundColor(tcell.ColorDefault)

	for _, g := range groups {
		path := g.Path
		list.AddItem(g.Name, "", 0, func() {
			onSubmit(path)
		})
	}

	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			onCancel()
			return nil
		}
		return event
	})
	return list
}
