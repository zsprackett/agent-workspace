package tmux

import (
	"os/exec"
	"regexp"
	"strings"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07`)

func StripAnsi(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

type ToolStatus struct {
	IsWaiting bool
	IsBusy    bool
	HasError  bool
}

var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏", "✳", "✽", "✶", "✢"}

var claudeBusyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ctrl\+c to interrupt`),
	regexp.MustCompile(`….*tokens`),
}


var claudeExitedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)Resume this session with:`),
	regexp.MustCompile(`(?i)claude --resume`),
}

// claudePermissionPatterns matches Claude's permission dialog, which must
// take priority over busy/spinner detection. A spinner from a running bash
// tool can appear in the same screen area as the dialog.
var claudePermissionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)do you want to proceed`),
	regexp.MustCompile(`(?i)tab to amend`),
}

var genericWaitingPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\? \(y\/n\)`),
	regexp.MustCompile(`(?i)\[Y\/n\]`),
	regexp.MustCompile(`(?i)Press enter to continue`),
	regexp.MustCompile(`(?i)do you want to`),
}

var errorPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)error:`),
	regexp.MustCompile(`(?i)failed:`),
	regexp.MustCompile(`(?i)exception:`),
	regexp.MustCompile(`(?i)traceback`),
	regexp.MustCompile(`(?i)panic:`),
}

func hasSpinner(text string) bool {
	for _, ch := range spinnerChars {
		if strings.Contains(text, ch) {
			return true
		}
	}
	return false
}

func matchAny(patterns []*regexp.Regexp, text string) bool {
	for _, p := range patterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

func lastLines(text string, n int) string {
	lines := strings.Split(StripAnsi(text), "\n")
	// trim trailing empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// IsPaneWaitingForInput returns true if the foreground process in the pane
// is blocked on a terminal read (wchan == "ttyin"), indicating it is waiting
// for keyboard input regardless of what text is on screen.
func IsPaneWaitingForInput(sessionName string) bool {
	ttyOut, err := exec.Command("tmux", "display-message", "-p", "-t", sessionName, "#{pane_tty}").Output()
	if err != nil {
		return false
	}
	ttyDev := strings.TrimPrefix(strings.TrimSpace(string(ttyOut)), "/dev/")
	if ttyDev == "" {
		return false
	}
	// ps -t <tty> lists processes on that terminal; stat contains '+' for the
	// foreground process group; wchan "ttyin" means blocked waiting for tty read.
	psOut, err := exec.Command("ps", "-t", ttyDev, "-o", "stat=,wchan=").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(psOut)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		stat, wchan := fields[0], fields[1]
		if strings.Contains(stat, "+") && wchan == "ttyin" {
			return true
		}
	}
	return false
}

func ParseToolStatus(output, tool string) ToolStatus {
	last30 := lastLines(output, 30)
	last10 := lastLines(output, 10)

	var s ToolStatus
	if tool == "claude" {
		if matchAny(claudeExitedPatterns, last30) {
			return s
		}
		// Permission dialogs take priority: a spinner from a concurrent bash tool
		// can appear alongside the dialog and would otherwise make IsBusy=true.
		if matchAny(claudePermissionPatterns, last30) {
			s.IsWaiting = true
			return s
		}
		s.IsBusy = matchAny(claudeBusyPatterns, last30) || hasSpinner(last10)
		// IsWaiting is intentionally NOT set to !IsBusy here. During autonomous
		// inter-step gaps (tool just finished, next thinking phase not yet started),
		// IsBusy is false but the process is not blocked on stdin - Claude is about
		// to decide its next action. Setting IsWaiting=true in that case causes
		// rapid running→waiting oscillations and spurious user-input notifications.
		// The monitor's IsPaneWaitingForInput (wchan==ttyin) handles true
		// waiting-for-user detection independently.
	} else {
		s.IsWaiting = matchAny(genericWaitingPatterns, last30)
	}
	s.HasError = matchAny(errorPatterns, last30)
	return s
}
