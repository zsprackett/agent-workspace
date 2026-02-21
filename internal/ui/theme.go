package ui

import "github.com/gdamore/tcell/v2"

// Theme colors for the TUI.
var (
	ColorBackground      = tcell.NewHexColor(0x1e1e2e)
	ColorBackgroundPanel = tcell.NewHexColor(0x181825)
	ColorBackgroundElem  = tcell.NewHexColor(0x313244)
	ColorPrimary         = tcell.NewHexColor(0x89b4fa) // blue
	ColorAccent          = tcell.NewHexColor(0xcba6f7) // mauve
	ColorText            = tcell.NewHexColor(0xcdd6f4)
	ColorTextMuted       = tcell.NewHexColor(0x6c7086)
	ColorSuccess         = tcell.NewHexColor(0xa6e3a1) // green
	ColorWarning         = tcell.NewHexColor(0xf9e2af) // yellow
	ColorError           = tcell.NewHexColor(0xf38ba8) // red
	ColorBorder          = tcell.NewHexColor(0x45475a)
	ColorSelected        = tcell.NewHexColor(0x89b4fa)
	ColorSelectedText    = tcell.NewHexColor(0x1e1e2e)
)

// Status icons
const (
	IconRunning = "●"
	IconWaiting = "◐"
	IconIdle    = "○"
	IconStopped = "◻"
	IconError   = "✗"
)

func StatusIcon(status string) (string, tcell.Color) {
	switch status {
	case "running":
		return IconRunning, ColorSuccess
	case "waiting":
		return IconWaiting, ColorWarning
	case "error":
		return IconError, ColorError
	case "stopped":
		return IconStopped, ColorTextMuted
	case "creating":
		return "⟳", ColorAccent
	case "deleting":
		return "⟳", ColorWarning
	default:
		return IconIdle, ColorTextMuted
	}
}
