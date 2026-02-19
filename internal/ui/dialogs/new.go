package dialogs

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zsprackett/agent-workspace/internal/db"
)

type NewSessionResult struct {
	Title       string
	Tool        db.Tool
	Command     string
	ProjectPath string
	GroupPath   string
}

// resolveGroupTool returns the tool to pre-select for a given group.
// Priority: group DefaultTool > cfgDefault > "claude".
func resolveGroupTool(groups []*db.Group, groupPath, cfgDefault string) string {
	for _, g := range groups {
		if g.Path == groupPath {
			if g.DefaultTool != "" {
				return string(g.DefaultTool)
			}
			break
		}
	}
	if cfgDefault != "" {
		return cfgDefault
	}
	return "claude"
}

// NewSessionDialog shows a form to create a new session.
// onSubmit is called with the result; onCancel on Escape.
func NewSessionDialog(groups []*db.Group, defaultTool string, defaultGroup string,
	onSubmit func(NewSessionResult), onCancel func()) *tview.Form {

	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" New Session ").SetTitleAlign(tview.AlignLeft)
	form.SetBackgroundColor(tcell.ColorDefault)
	form.SetFieldBackgroundColor(tcell.ColorDefault)

	tools := []string{"claude", "opencode", "gemini", "codex", "shell", "custom"}
	initialTool := resolveGroupTool(groups, defaultGroup, defaultTool)
	defaultToolIdx := 0
	for i, t := range tools {
		if t == initialTool {
			defaultToolIdx = i
			break
		}
	}

	groupNames := make([]string, len(groups))
	groupPaths := make([]string, len(groups))
	defaultGroupIdx := 0
	for i, g := range groups {
		groupNames[i] = g.Name
		groupPaths[i] = g.Path
		if g.Path == defaultGroup {
			defaultGroupIdx = i
		}
	}

	form.AddInputField("Title (optional)", "", 30, nil, nil)
	form.AddDropDown("Tool", tools, defaultToolIdx, nil)
	form.AddInputField("Project Path", "", 40, nil, nil)
	if len(groups) > 0 {
		form.AddDropDown("Group", groupNames, defaultGroupIdx, nil)
	}

	var commandShown bool
	currentCmd := ""

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

	if len(groups) > 0 {
		groupDD := form.GetFormItemByLabel("Group").(*tview.DropDown)
		toolDD := form.GetFormItemByLabel("Tool").(*tview.DropDown)
		groupDD.SetSelectedFunc(func(text string, _ int) {
			for i, n := range groupNames {
				if n == text {
					newTool := resolveGroupTool(groups, groupPaths[i], defaultTool)
					for ti, t := range tools {
						if t == newTool {
							toolDD.SetCurrentOption(ti)
							break
						}
					}
					setCommandVisible(newTool == "custom")
					break
				}
			}
		})
	}

	toolDD := form.GetFormItemByLabel("Tool").(*tview.DropDown)
	toolDD.SetSelectedFunc(func(text string, _ int) {
		setCommandVisible(text == "custom")
	})

	if initialTool == "custom" {
		setCommandVisible(true)
	}

	form.AddButton("Create", func() {
		title := form.GetFormItemByLabel("Title (optional)").(*tview.InputField).GetText()
		_, toolStr := form.GetFormItemByLabel("Tool").(*tview.DropDown).GetCurrentOption()
		projectPath := form.GetFormItemByLabel("Project Path").(*tview.InputField).GetText()

		groupPath := defaultGroup
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

		onSubmit(NewSessionResult{
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
