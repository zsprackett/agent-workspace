package dialogs

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ConfirmDialog shows a modal with a message and Yes/No buttons.
// onConfirm is called when the user selects Yes; onCancel on No or Escape.
func ConfirmDialog(message string, onConfirm func(), onCancel func()) *tview.Modal {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(_ int, label string) {
			if label == "Yes" {
				onConfirm()
			} else {
				onCancel()
			}
		})
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			onCancel()
			return nil
		}
		return event
	})
	return modal
}
