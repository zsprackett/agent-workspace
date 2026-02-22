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
	// Claude at its input prompt (not busy) is always waiting for user input.
	output := "> "
	status := tmux.ParseToolStatus(output, "claude")
	if !status.IsWaiting {
		t.Error("expected waiting: Claude at prompt should be waiting")
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

func TestParseToolStatus_Error(t *testing.T) {
	output := "error: something went wrong"
	status := tmux.ParseToolStatus(output, "shell")
	if !status.HasError {
		t.Error("expected error")
	}
}
