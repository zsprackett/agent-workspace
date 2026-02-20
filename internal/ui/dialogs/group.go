package dialogs

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type GroupResult struct {
	Name             string
	RepoURL          string
	DefaultTool      string
	PreLaunchCommand string
}

func GroupDialog(title, currentName, currentRepoURL, currentDefaultTool, currentPreLaunchCommand string, onSubmit func(GroupResult), onCancel func()) *tview.Form {
	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" " + title + " ").SetTitleAlign(tview.AlignLeft)
	form.SetBackgroundColor(tcell.ColorDefault)
	form.SetFieldBackgroundColor(tcell.ColorDefault)

	toolLabels := []string{"(none)", "claude", "opencode", "gemini", "codex", "custom"}
	toolValues := []string{"", "claude", "opencode", "gemini", "codex", "custom"}
	currentToolIdx := 0
	for i, v := range toolValues {
		if v == currentDefaultTool {
			currentToolIdx = i
			break
		}
	}

	form.AddInputField("Group name", currentName, 40, nil, nil)
	form.AddInputField("GitHub URL (optional)", currentRepoURL, 50, nil, nil)
	form.AddDropDown("Default Tool", toolLabels, currentToolIdx, nil)
	form.AddInputField("Pre-launch command (optional)", currentPreLaunchCommand, 50, nil, nil)
	form.AddButton("OK", func() {
		name := form.GetFormItemByLabel("Group name").(*tview.InputField).GetText()
		if name != "" {
			repoURL := form.GetFormItemByLabel("GitHub URL (optional)").(*tview.InputField).GetText()
			_, toolLabel := form.GetFormItemByLabel("Default Tool").(*tview.DropDown).GetCurrentOption()
			defaultTool := ""
			for i, l := range toolLabels {
				if l == toolLabel {
					defaultTool = toolValues[i]
					break
				}
			}
			prelaunch := form.GetFormItemByLabel("Pre-launch command (optional)").(*tview.InputField).GetText()
			onSubmit(GroupResult{Name: name, RepoURL: repoURL, DefaultTool: defaultTool, PreLaunchCommand: prelaunch})
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
