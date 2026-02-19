package dialogs

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const helpText = `[yellow]Dashboard Keys[-]

  [green]↑/k[-]      Navigate up
  [green]↓/j[-]      Navigate down
  [green]←/h[-]      Collapse group
  [green]→/l[-]      Expand group
  [green]Enter/a[-]  Attach to session
  [green]n[-]        New session (on group) / Session notes (on session)
  [green]d[-]        Delete session or group
  [green]s[-]        Stop session
  [green]x[-]        Restart session
  [green]e[-]        Edit session or group
  [green]g[-]        New group
  [green]m[-]        Move session to group
  [green]1-9[-]      Jump to group
  [green]?[-]        This help
  [green]q[-]        Quit

[yellow]Inside Session[-]

  [green]Ctrl+\[-]   Open command menu

  [yellow]Menu keys[-]
  [green]s[-]         Git status
  [green]d[-]         Git diff
  [green]p[-]         Open pull request in browser
  [green]n[-]         Session notes
  [green]t[-]         Open terminal split
  [green]x[-]         Detach to dashboard

Press [green]Escape[-] or [green]?[-] to close.`

func HelpDialog(onClose func()) *tview.TextView {
	tv := tview.NewTextView()
	tv.SetBorder(true).SetTitle(" Help ").SetTitleAlign(tview.AlignLeft)
	tv.SetDynamicColors(true)
	tv.SetBackgroundColor(tcell.ColorDefault)
	tv.SetText(helpText)
	tv.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Rune() == '?' {
			onClose()
			return nil
		}
		return event
	})
	return tv
}
