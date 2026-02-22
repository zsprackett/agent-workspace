package tmux_test

import (
	"testing"
	"github.com/zsprackett/agent-workspace/internal/tmux"
)

func TestStripAnsi(t *testing.T) {
	input := "\x1b[32mhello\x1b[0m world"
	got := tmux.StripAnsi(input)
	if got != "hello world" {
		t.Errorf("got %q", got)
	}
}

func TestParseToolStatus_ClaudeWaiting(t *testing.T) {
	output := "Some output\nDo you want to proceed?\n1. Yes\n2. No"
	status := tmux.ParseToolStatus(output, "claude")
	if !status.IsWaiting {
		t.Error("expected waiting")
	}
}

func TestParseToolStatus_ClaudeBusy(t *testing.T) {
	output := "⠋ Thinking... ctrl+c to interrupt"
	status := tmux.ParseToolStatus(output, "claude")
	if !status.IsBusy {
		t.Error("expected busy")
	}
}

func TestParseToolStatus_ClaudeAtPrompt(t *testing.T) {
	// Claude at its input prompt looks identical to an inter-step gap from the
	// perspective of pane text alone. ParseToolStatus returns neither busy nor
	// waiting; the monitor's IsPaneWaitingForInput (wchan==ttyin) distinguishes
	// "truly waiting for user" from "between autonomous steps".
	output := "> "
	status := tmux.ParseToolStatus(output, "claude")
	if status.IsWaiting {
		t.Error("expected NOT waiting: at-prompt vs inter-step is determined by IsPaneWaitingForInput, not pane text")
	}
	if status.IsBusy {
		t.Error("expected not busy")
	}
}

// TestParseToolStatus_ClaudePermissionDialogWithSpinner reproduces the bug where
// a Braille spinner from a running bash tool caused the permission dialog to be
// classified as RUNNING instead of WAITING.
func TestParseToolStatus_ClaudePermissionDialogWithSpinner(t *testing.T) {
	// Simulates terminal output with a spinner-prefixed bash "Running..." line
	// visible above the permission dialog in the same 30-line window.
	output := "⠋ Running...\n" +
		"3 tasks (1 done, 1 in progress, 1 open)\n" +
		"■ Implement the search action\n" +
		"┌ Bash command ────────────────┐\n" +
		"│  cat tool.go | sed -n '54p' │\n" +
		"│ Do you want to proceed?      │\n" +
		"│ › 1. Yes                     │\n" +
		"│   2. No                      │\n" +
		"│ Esc to cancel · Tab to amend │\n" +
		"└──────────────────────────────┘"
	status := tmux.ParseToolStatus(output, "claude")
	if !status.IsWaiting {
		t.Error("expected waiting: permission dialog must override spinner")
	}
	if status.IsBusy {
		t.Error("expected not busy when permission dialog is visible")
	}
}

// TestParseToolStatus_ClaudeBetweenSteps verifies that when Claude is between
// tool uses (tool just finished, no active spinner or thinking indicator), the
// status is NOT "waiting". The monitor's IsPaneWaitingForInput handles true
// user-input detection; ParseToolStatus must not fire false-positive "waiting"
// during autonomous inter-step gaps.
func TestParseToolStatus_ClaudeBetweenSteps(t *testing.T) {
	output := "⏺ Read(internal/tmux/status.go)\n" +
		"  ⎿ Read 143 lines\n" +
		"\n" +
		"──────────────────────────────────────\n" +
		"❯ \n" +
		"──────────────────────────────────────\n" +
		"  project | user@host [ctx: 50%]"
	status := tmux.ParseToolStatus(output, "claude")
	if status.IsWaiting {
		t.Error("IsWaiting must be false during inter-step gap: use IsPaneWaitingForInput for true waiting detection")
	}
	if status.IsBusy {
		t.Error("expected not busy: no active spinner or thinking indicator")
	}
}

func TestParseToolStatus_Error(t *testing.T) {
	output := "error: something went wrong"
	status := tmux.ParseToolStatus(output, "shell")
	if !status.HasError {
		t.Error("expected error")
	}
}
