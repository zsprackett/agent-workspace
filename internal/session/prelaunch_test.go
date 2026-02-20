package session_test

import (
	"strings"
	"testing"

	"github.com/zsprackett/agent-workspace/internal/session"
)

func TestRunPreLaunchCommand_Success(t *testing.T) {
	out, err := session.RunPreLaunchCommand("echo hello", "claude", "/tmp/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", out)
	}
}

func TestRunPreLaunchCommand_NonZeroExit(t *testing.T) {
	_, err := session.RunPreLaunchCommand("false")
	if err == nil {
		t.Fatal("expected error from non-zero exit, got nil")
	}
}

func TestRunPreLaunchCommand_ArgsAppended(t *testing.T) {
	// Use echo to verify positional args are appended to the command.
	out, err := session.RunPreLaunchCommand("echo", "claude", "/repos/foo.git", "/worktrees/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "claude") {
		t.Errorf("expected 'claude' in output, got %q", out)
	}
	if !strings.Contains(out, "/repos/foo.git") {
		t.Errorf("expected bare repo path in output, got %q", out)
	}
	if !strings.Contains(out, "/worktrees/foo") {
		t.Errorf("expected worktree path in output, got %q", out)
	}
}

func TestRunPreLaunchCommand_EmptyCommand(t *testing.T) {
	out, err := session.RunPreLaunchCommand("")
	if err != nil {
		t.Fatalf("empty command should be a no-op, got error: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty output for empty command, got %q", out)
	}
}
