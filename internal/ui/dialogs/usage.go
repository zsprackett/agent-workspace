package dialogs

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zsprackett/agent-workspace/internal/db"
)

const sparkChars = "▁▂▃▄▅▆▇█"

// UsageDialog is a tview.TextView-based dialog showing Claude Code usage stats.
type UsageDialog struct {
	*tview.TextView
	store   *db.DB
	onClose func()
	app     *tview.Application
}

// NewUsageDialog creates a usage dialog that loads data from the DB.
// onClose is called when the user presses Q or Escape.
// onRefresh is called when the user presses R; it should call the API, save the
// snapshot, then call dialog.Reload() to redisplay.
func NewUsageDialog(store *db.DB, app *tview.Application, onClose func(), onRefresh func()) *UsageDialog {
	d := &UsageDialog{
		TextView: tview.NewTextView(),
		store:    store,
		onClose:  onClose,
		app:      app,
	}
	d.SetBorder(true).SetTitle(" Claude Code Usage ").SetTitleAlign(tview.AlignLeft)
	d.SetDynamicColors(true)
	d.SetBackgroundColor(tcell.ColorDefault)

	d.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEscape, event.Rune() == 'q', event.Rune() == 'Q':
			onClose()
			return nil
		case event.Rune() == 'r', event.Rune() == 'R':
			d.SetText(d.buildText(nil, nil) + "\n\n[yellow]Refreshing...[-]")
			go func() {
				onRefresh()
			}()
			return nil
		}
		return event
	})

	d.Reload()
	return d
}

// Reload re-reads the DB and updates the displayed text.
func (d *UsageDialog) Reload() {
	latest, _ := d.store.GetLatestUsageSnapshot()
	history, _ := d.store.GetUsageSnapshots(48)
	d.SetText(d.buildText(latest, history))
}

func (d *UsageDialog) buildText(latest *db.UsageSnapshot, history []db.UsageSnapshot) string {
	var sb strings.Builder

	if latest == nil {
		sb.WriteString("\n  [yellow]No usage data yet.[-]\n\n")
		sb.WriteString("  Press [green]R[-] to fetch current usage from the API.\n")
		sb.WriteString("\n  [dim]Press Q or Esc to close.[-]")
		return sb.String()
	}

	sb.WriteString("\n")

	// API returns utilization in 0-100 scale; divide by 100 for internal 0-1 fraction.
	fiveHourFrac := latest.FiveHourUtil / 100
	sevenDayFrac := latest.SevenDayUtil / 100

	// 5-hour window
	sb.WriteString("  [yellow]5-Hour Window[-]\n")
	sb.WriteString(fmt.Sprintf("  %s  %s\n",
		progressBar(fiveHourFrac, 30), formatUtil(fiveHourFrac)))
	if latest.FiveHourResetsAt > 0 {
		sb.WriteString(fmt.Sprintf("  Resets %s\n", formatResetTime(latest.FiveHourResetsAt)))
	}
	sb.WriteString("\n")

	// 7-day window
	sb.WriteString("  [yellow]7-Day Window[-]\n")
	sb.WriteString(fmt.Sprintf("  %s  %s\n",
		progressBar(sevenDayFrac, 30), formatUtil(sevenDayFrac)))
	if latest.SevenDayResetsAt > 0 {
		sb.WriteString(fmt.Sprintf("  Resets %s\n", formatResetTime(latest.SevenDayResetsAt)))
	}
	sb.WriteString("\n")

	// Extra usage: API utilization is also 0-100 scale, capped at 100.
	if latest.ExtraEnabled {
		used := latest.ExtraUsedCredits / 100.0
		limit := latest.ExtraMonthlyLimit / 100.0
		extraFrac := latest.ExtraUtilization / 100
		sb.WriteString(fmt.Sprintf("  [yellow]Extra Usage[-]  $%.2f / $%.2f  %s\n\n",
			used, limit, formatUtil(extraFrac)))
	}

	// Sparklines (reverse history so oldest is first/left)
	if len(history) > 1 {
		sb.WriteString("  [yellow]History (newest right)[-]\n")
		fiveHourSpark := buildSparkline(history, func(s db.UsageSnapshot) float64 { return s.FiveHourUtil / 100 })
		sevenDaySpark := buildSparkline(history, func(s db.UsageSnapshot) float64 { return s.SevenDayUtil / 100 })
		sb.WriteString(fmt.Sprintf("  5-hr  %s\n", fiveHourSpark))
		sb.WriteString(fmt.Sprintf("  7-day %s\n", sevenDaySpark))

		oldest := time.UnixMilli(history[len(history)-1].TsMs)
		newest := time.UnixMilli(history[0].TsMs)
		sb.WriteString(fmt.Sprintf("  [dim]%s  →  %s[-]\n",
			oldest.Local().Format("Jan 2 15:04"),
			newest.Local().Format("Jan 2 15:04")))
		sb.WriteString("\n")
	}

	ts := time.UnixMilli(latest.TsMs).Local()
	sb.WriteString(fmt.Sprintf("  [dim]Last updated: %s[-]\n", ts.Format("Jan 2 15:04:05")))
	sb.WriteString("\n  [green]R[-] refresh  [green]Q/Esc[-] close")

	return sb.String()
}

// formatUtil formats a 0-1 utilization fraction as a colored percentage string.
// Values > 1.0 (over limit) show as "[red]100%+ (OVER)[-]".
func formatUtil(util float64) string {
	if util > 1.0 {
		return fmt.Sprintf("[red]%.0f%% (OVER)[-]", util*100)
	}
	color := "green"
	if util >= 0.8 {
		color = "red"
	} else if util >= 0.6 {
		color = "yellow"
	}
	return fmt.Sprintf("[%s]%.1f%%[-]", color, util*100)
}

// progressBar renders a simple text progress bar for a utilization in [0,1].
func progressBar(util float64, width int) string {
	if util < 0 {
		util = 0
	}
	if util > 1 {
		util = 1
	}
	filled := int(util * float64(width))
	empty := width - filled

	color := "green"
	if util >= 0.8 {
		color = "red"
	} else if util >= 0.6 {
		color = "yellow"
	}

	return fmt.Sprintf("[%s][%s%s][-]", color, strings.Repeat("█", filled), strings.Repeat("░", empty))
}

// buildSparkline builds a sparkline string from snapshots (history[0] is newest).
func buildSparkline(history []db.UsageSnapshot, val func(db.UsageSnapshot) float64) string {
	// Reverse so oldest is leftmost
	reversed := make([]db.UsageSnapshot, len(history))
	for i, s := range history {
		reversed[len(history)-1-i] = s
	}
	runes := []rune(sparkChars)
	var sb strings.Builder
	for _, s := range reversed {
		v := val(s)
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		idx := int(v * float64(len(runes)-1))
		sb.WriteRune(runes[idx])
	}
	return sb.String()
}

func formatResetTime(tsMs int64) string {
	t := time.UnixMilli(tsMs)
	d := time.Until(t)
	if d < 0 {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("in %dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("in %dh%02dm", h, m)
	}
	return fmt.Sprintf("in %dd", int(d.Hours()/24))
}

