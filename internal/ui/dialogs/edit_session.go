package dialogs

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zsprackett/agent-workspace/internal/db"
)

type EditSessionResult struct {
	Title       string
	Tool        db.Tool
	Command     string
	ProjectPath string
	GroupPath   string
}

func EditSessionDialog(s *db.Session, groups []*db.Group,
	onSubmit func(EditSessionResult), onCancel func()) *tview.Form {

	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" Edit Session ").SetTitleAlign(tview.AlignLeft)
	form.SetBackgroundColor(tcell.ColorDefault)
	form.SetFieldBackgroundColor(tcell.ColorDefault)

	tools := []string{"claude", "opencode", "gemini", "codex", "shell", "custom"}
	currentToolIdx := 0
	for i, t := range tools {
		if t == string(s.Tool) {
			currentToolIdx = i
			break
		}
	}

	groupNames := make([]string, len(groups))
	groupPaths := make([]string, len(groups))
	currentGroupIdx := 0
	for i, g := range groups {
		groupNames[i] = g.Name
		groupPaths[i] = g.Path
		if g.Path == s.GroupPath {
			currentGroupIdx = i
		}
	}

	form.AddInputField("Title", s.Title, 30, nil, nil)
	form.AddDropDown("Tool", tools, currentToolIdx, nil)
	form.AddInputField("Project Path", s.ProjectPath, 40, nil, nil)
	if len(groups) > 0 {
		form.AddDropDown("Group", groupNames, currentGroupIdx, nil)
	}

	var commandShown bool
	currentCmd := ""
	if s.Tool == db.ToolCustom {
		currentCmd = s.Command
	}

	setCommandVisible := func(show bool) {
		if show == commandShown {
			return
		}
		if show {
			form.AddFormItem(tview.NewInputField().
				SetLabel("Command").
				SetFieldWidth(40).
				SetText(currentCmd))
			commandShown = true
		} else {
			if commandShown {
				currentCmd = form.GetFormItemByLabel("Command").(*tview.InputField).GetText()
			}
			form.RemoveFormItem(form.GetFormItemCount() - 1)
			commandShown = false
		}
	}

	toolDD := form.GetFormItemByLabel("Tool").(*tview.DropDown)
	toolDD.SetSelectedFunc(func(text string, _ int) {
		setCommandVisible(text == "custom")
	})

	if s.Tool == db.ToolCustom {
		setCommandVisible(true)
	}

	form.AddButton("Save", func() {
		title := form.GetFormItemByLabel("Title").(*tview.InputField).GetText()
		_, toolStr := form.GetFormItemByLabel("Tool").(*tview.DropDown).GetCurrentOption()
		projectPath := form.GetFormItemByLabel("Project Path").(*tview.InputField).GetText()

		groupPath := s.GroupPath
		if len(groups) > 0 {
			_, gName := form.GetFormItemByLabel("Group").(*tview.DropDown).GetCurrentOption()
			for i, n := range groupNames {
				if n == gName {
					groupPath = groupPaths[i]
					break
				}
			}
		}

		command := ""
		if commandShown {
			command = form.GetFormItemByLabel("Command").(*tview.InputField).GetText()
		}

		onSubmit(EditSessionResult{
			Title:       title,
			Tool:        db.Tool(toolStr),
			Command:     command,
			ProjectPath: projectPath,
			GroupPath:   groupPath,
		})
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
