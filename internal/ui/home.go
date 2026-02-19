package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/tmux"
)

const defaultGroupPath = "my-sessions"

// listItem represents a row in the session list (group header or session)
type listItem struct {
	isGroup    bool
	group      *db.Group
	session    *db.Session
	groupIndex int // 1-9 for group hotkey
}

// Home is the main screen containing the session list and preview pane.
type Home struct {
	*tview.Flex
	app     *tview.Application
	table   *tview.Table
	preview *tview.TextView
	header  *tview.TextView
	footer  *tview.TextView

	sessions []*db.Session
	groups   []*db.Group
	items    []listItem
	selected int

	onNew      func(groupPath string)
	onDelete   func(item listItem)
	onStop     func(item listItem)
	onRestart  func(item listItem)
	onEdit     func(item listItem)
	onNewGroup func()
	onMove     func(item listItem)
	onAttach   func(item listItem)
	onQuit     func()
}

func NewHome(app *tview.Application) *Home {
	h := &Home{app: app}

	h.header = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	h.header.SetBackgroundColor(ColorBackgroundPanel)

	h.table = tview.NewTable().
		SetSelectable(true, false).
		SetSelectedStyle(tcell.StyleDefault.
			Background(ColorSelected).
			Foreground(ColorSelectedText))
	h.table.SetBackgroundColor(ColorBackground)
	h.table.SetBorderPadding(0, 0, 0, 0)

	h.preview = tview.NewTextView().
		SetDynamicColors(false).
		SetScrollable(true).
		SetWrap(false)
	h.preview.SetBackgroundColor(ColorBackground)

	h.footer = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	h.footer.SetBackgroundColor(ColorBackgroundPanel)
	h.footer.SetText(
		"[green]↑↓[-] navigate  [green]←→[-] fold  [green]Enter/a[-] attach  " +
			"[green]n[-] new  [green]d[-] delete  [green]s[-] stop  [green]x[-] restart  " +
			"[green]e[-] edit  [green]g[-] group  [green]m[-] move  [green]?[-] help  [green]q[-] quit")

	previewFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(h.preview, 0, 1, false)

	separator := tview.NewBox().SetBackgroundColor(ColorBorder)

	content := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(h.table, 0, 35, true).
		AddItem(separator, 1, 0, false).
		AddItem(previewFlex, 0, 65, false)

	h.Flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(h.header, 1, 0, false).
		AddItem(content, 0, 1, true).
		AddItem(h.footer, 1, 0, false)

	h.setupInput()
	return h
}

func (h *Home) SetCallbacks(
	onNew func(groupPath string),
	onDelete func(listItem),
	onStop func(listItem),
	onRestart func(listItem),
	onEdit func(listItem),
	onNewGroup func(),
	onMove func(listItem),
	onAttach func(listItem),
	onQuit func(),
) {
	h.onNew = onNew
	h.onDelete = onDelete
	h.onStop = onStop
	h.onRestart = onRestart
	h.onEdit = onEdit
	h.onNewGroup = onNewGroup
	h.onMove = onMove
	h.onAttach = onAttach
	h.onQuit = onQuit
}

func (h *Home) Update(sessions []*db.Session, groups []*db.Group) {
	h.sessions = sessions
	h.groups = groups
	h.rebuildItems()
	h.renderTable()
	h.updateHeader()
}

func (h *Home) rebuildItems() {
	h.items = nil
	groupIdx := 1
	for _, g := range h.groups {
		idx := 0
		if groupIdx <= 9 {
			idx = groupIdx
			groupIdx++
		}
		h.items = append(h.items, listItem{isGroup: true, group: g, groupIndex: idx})
		if g.Expanded {
			for _, s := range h.sessions {
				if s.GroupPath == g.Path {
					h.items = append(h.items, listItem{session: s})
				}
			}
		}
	}
	// Sessions not in any group
	grouped := map[string]bool{}
	for _, g := range h.groups {
		grouped[g.Path] = true
	}
	for _, s := range h.sessions {
		if !grouped[s.GroupPath] {
			h.items = append(h.items, listItem{session: s})
		}
	}
}

func (h *Home) renderTable() {
	h.table.Clear()
	for i, item := range h.items {
		if item.isGroup {
			g := item.group
			arrow := "▶"
			if g.Expanded {
				arrow = "▼"
			}
			hotkey := ""
			if item.groupIndex > 0 {
				hotkey = fmt.Sprintf(" [%d]", item.groupIndex)
			}
			count := 0
			for _, s := range h.sessions {
				if s.GroupPath == g.Path {
					count++
				}
			}
			text := fmt.Sprintf(" %s %s (%d)%s", arrow, g.Name, count, hotkey)
			cell := tview.NewTableCell(text).
				SetTextColor(ColorPrimary).
				SetBackgroundColor(ColorBackgroundElem).
				SetExpansion(1).
				SetSelectable(true)
			h.table.SetCell(i, 0, cell)
		} else {
			s := item.session
			icon, color := StatusIcon(string(s.Status))
			title := s.Title
			if len(title) > 20 {
				title = title[:18] + ".."
			}
			age := formatAge(s.LastAccessed)
			text := fmt.Sprintf("   %s %-20s %s  %s", icon, title, s.Tool, age)
			cell := tview.NewTableCell(text).
				SetTextColor(color).
				SetBackgroundColor(ColorBackground).
				SetExpansion(1).
				SetSelectable(true)
			h.table.SetCell(i, 0, cell)
		}
	}

	// Clamp selection
	if h.selected >= len(h.items) && len(h.items) > 0 {
		h.selected = len(h.items) - 1
	}
	if len(h.items) > 0 {
		h.table.Select(h.selected, 0)
	}

	// Update preview when selection changes
	h.table.SetSelectionChangedFunc(func(row, col int) {
		h.selected = row
		h.updatePreview()
	})
}

func (h *Home) updateHeader() {
	running, waiting := 0, 0
	for _, s := range h.sessions {
		switch s.Status {
		case db.StatusRunning:
			running++
		case db.StatusWaiting:
			waiting++
		}
	}
	h.header.SetText(fmt.Sprintf(
		"[blue]AGENT WORKSPACE[-]   [green]● %d running[-]  [yellow]◐ %d waiting[-]  %d total",
		running, waiting, len(h.sessions)))
}

func (h *Home) updatePreview() {
	if h.selected < 0 || h.selected >= len(h.items) {
		h.preview.Clear()
		return
	}
	item := h.items[h.selected]
	if item.isGroup || item.session == nil || item.session.TmuxSession == "" {
		h.preview.Clear()
		return
	}
	s := item.session
	go func() {
		out, err := tmux.CapturePane(s.TmuxSession, tmux.CaptureOptions{StartLine: -100, Join: true})
		if err != nil {
			return
		}
		// Strip ANSI for the preview
		clean := tmux.StripAnsi(out)
		lines := strings.Split(clean, "\n")
		// Last 50 non-empty lines
		var kept []string
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				kept = append(kept, l)
			}
		}
		if len(kept) > 50 {
			kept = kept[len(kept)-50:]
		}
		h.app.QueueUpdateDraw(func() {
			h.preview.SetText(strings.Join(kept, "\n"))
			h.preview.ScrollToEnd()
		})
	}()
}

func (h *Home) selectedItem() (listItem, bool) {
	if h.selected < 0 || h.selected >= len(h.items) {
		return listItem{}, false
	}
	return h.items[h.selected], true
}

// selectedGroupPath returns the group path for the currently selected item:
// the group itself if a group header is selected, the session's group if a
// session is selected, or defaultGroupPath as a fallback.
func (h *Home) selectedGroupPath() string {
	item, ok := h.selectedItem()
	if !ok {
		return defaultGroupPath
	}
	if item.isGroup {
		return item.group.Path
	}
	if item.session != nil {
		return item.session.GroupPath
	}
	return defaultGroupPath
}

func (h *Home) setupInput() {
	h.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		row, _ := h.table.GetSelection()
		h.selected = row

		switch event.Key() {
		case tcell.KeyLeft:
			h.collapseOrUp()
			return nil
		case tcell.KeyRight:
			h.expandOrAttach()
			return nil
		case tcell.KeyEnter:
			if item, ok := h.selectedItem(); ok {
				if item.isGroup {
					h.toggleGroup(item.group)
				} else if h.onAttach != nil {
					h.onAttach(item)
				}
			}
			return nil
		}

		switch event.Rune() {
		case 'a':
			if item, ok := h.selectedItem(); ok {
				if !item.isGroup && h.onAttach != nil {
					h.onAttach(item)
				}
			}
			return nil
		case 'n':
			if h.onNew != nil {
				h.onNew(h.selectedGroupPath())
			}
			return nil
		case 'd':
			if item, ok := h.selectedItem(); ok {
				if h.onDelete != nil {
					h.onDelete(item)
				}
			}
			return nil
		case 's':
			if item, ok := h.selectedItem(); ok {
				if !item.isGroup && h.onStop != nil {
					h.onStop(item)
				}
			}
			return nil
		case 'x':
			if item, ok := h.selectedItem(); ok {
				if !item.isGroup && h.onRestart != nil {
					h.onRestart(item)
				}
			}
			return nil
		case 'e':
			if item, ok := h.selectedItem(); ok {
				if h.onEdit != nil {
					h.onEdit(item)
				}
			}
			return nil
		case 'g':
			if h.onNewGroup != nil {
				h.onNewGroup()
			}
			return nil
		case 'm':
			if item, ok := h.selectedItem(); ok {
				if !item.isGroup && h.onMove != nil {
					h.onMove(item)
				}
			}
			return nil
		case 'q':
			if h.onQuit != nil {
				h.onQuit()
			}
			return nil
		case '1', '2', '3', '4', '5', '6', '7', '8', '9':
			h.jumpToGroup(int(event.Rune() - '0'))
			return nil
		}
		return event
	})
}

func (h *Home) toggleGroup(g *db.Group) {
	g.Expanded = !g.Expanded
	h.rebuildItems()
	h.renderTable()
}

func (h *Home) collapseOrUp() {
	item, ok := h.selectedItem()
	if !ok {
		return
	}
	if item.isGroup && item.group.Expanded {
		h.toggleGroup(item.group)
		return
	}
	// Move selection up
	if h.selected > 0 {
		h.selected--
		h.table.Select(h.selected, 0)
	}
}

func (h *Home) expandOrAttach() {
	item, ok := h.selectedItem()
	if !ok {
		return
	}
	if item.isGroup && !item.group.Expanded {
		h.toggleGroup(item.group)
		return
	}
	if !item.isGroup && h.onAttach != nil {
		h.onAttach(item)
	}
}

func (h *Home) jumpToGroup(idx int) {
	for i, item := range h.items {
		if item.isGroup && item.groupIndex == idx {
			h.selected = i
			h.table.Select(i, 0)
			return
		}
	}
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
