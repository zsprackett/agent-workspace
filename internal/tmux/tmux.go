package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const SessionPrefix = "agws_"

func IsAvailable() bool {
	return exec.Command("tmux", "-V").Run() == nil
}

func GenerateSessionName(title string) string {
	safe := strings.ToLower(title)
	safe = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, safe)
	safe = strings.Trim(safe, "-")
	if len(safe) > 20 {
		safe = safe[:20]
	}
	ts := fmt.Sprintf("%x", time.Now().UnixMilli())
	return fmt.Sprintf("%s%s-%s", SessionPrefix, safe, ts)
}

type CreateOptions struct {
	Name    string
	Command string
	Cwd     string
	Env     map[string]string
}

func CreateSession(opts CreateOptions) error {
	cwd := opts.Cwd
	if cwd == "" {
		cwd = os.Getenv("HOME")
	}

	args := []string{"new-session", "-d", "-s", opts.Name, "-c", cwd}

	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	if opts.Command != "" {
		cmd := opts.Command
		if strings.Contains(cmd, "$(") {
			escaped := strings.ReplaceAll(cmd, "'", `'"'"'`)
			cmd = fmt.Sprintf("bash -c '%s'", escaped)
		}
		args = append(args, cmd)
	}

	if err := exec.Command("tmux", args...).Run(); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func KillSession(name string) error {
	return exec.Command("tmux", "kill-session", "-t", name).Run()
}

func SendKeys(name, keys string) error {
	return exec.Command("tmux", "send-keys", "-t", name, keys, "Enter").Run()
}

// SendText sends literal text to a tmux pane without appending Enter.
// Uses the -l flag to pass the text through literally without key binding lookup.
func SendText(name, text string) error {
	return exec.Command("tmux", "send-keys", "-t", name, "-l", text).Run()
}

// PipePane redirects tmux pane output to a shell command.
// The -o flag opens the pipe only if not already open.
func PipePane(name, command string) error {
	return exec.Command("tmux", "pipe-pane", "-o", "-t", name, command).Run()
}

// StopPipePane closes the pipe-pane for the given session.
func StopPipePane(name string) error {
	return exec.Command("tmux", "pipe-pane", "-t", name).Run()
}

// ResizePane resizes the active pane in a tmux session to the given dimensions.
func ResizePane(name string, cols, rows int) error {
	return exec.Command("tmux", "resize-pane", "-t", name,
		"-x", fmt.Sprintf("%d", cols),
		"-y", fmt.Sprintf("%d", rows)).Run()
}

type CaptureOptions struct {
	StartLine int
	EndLine   int
	Join      bool
	EscapeSeq bool // adds -e flag for ANSI escape sequences
}

func CapturePane(name string, opts CaptureOptions) (string, error) {
	args := []string{"capture-pane", "-t", name, "-p",
		"-S", fmt.Sprintf("%d", opts.StartLine)}
	if opts.EndLine != 0 {
		args = append(args, "-E", fmt.Sprintf("%d", opts.EndLine))
	}
	if opts.Join {
		args = append(args, "-J")
	}
	if opts.EscapeSeq {
		args = append(args, "-e")
	}
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

type SessionInfo struct {
	Name     string
	Activity int64
}

func ListSessions() ([]SessionInfo, error) {
	out, err := exec.Command("tmux", "list-windows", "-a",
		"-F", "#{session_name}\t#{window_activity}").Output()
	if err != nil {
		return nil, nil // tmux not running
	}
	var sessions []SessionInfo
	seen := map[string]int64{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		var ts int64
		fmt.Sscanf(parts[1], "%d", &ts)
		if existing, ok := seen[parts[0]]; !ok || ts > existing {
			seen[parts[0]] = ts
		}
	}
	for name, ts := range seen {
		sessions = append(sessions, SessionInfo{Name: name, Activity: ts})
	}
	return sessions, nil
}

func SessionExists(name string, sessions []SessionInfo) bool {
	for _, s := range sessions {
		if s.Name == name {
			return true
		}
	}
	return false
}

func IsSessionActive(name string, sessions []SessionInfo, thresholdSecs int64) bool {
	for _, s := range sessions {
		if s.Name == name {
			now := time.Now().Unix()
			return now-s.Activity < thresholdSecs
		}
	}
	return false
}

func writeStatsFile(path, title string, running, waiting, total int) {
	content := fmt.Sprintf(
		"#[fg=#89b4fa,bold] AGENT WORKSPACE #[fg=#cdd6f4,nobold] %s #[fg=#6c7086]| "+
			"#[fg=#a6e3a1]● %d running  #[fg=#f9e2af]◐ %d waiting  #[fg=#cdd6f4]%d total",
		title, running, waiting, total)
	os.WriteFile(path, []byte(content), 0600)
}

// AttachSession attaches to a tmux session synchronously.
// Ctrl+D detaches, Ctrl+T opens a terminal split.
// getStats is called periodically to update the header counts.
// Blocks until the user detaches.
func AttachSession(name, title string, getStats func() (running, waiting, total int)) error {
	// Bind single leader key (Ctrl+\) that opens a command menu popup.
	// All session actions live behind the leader -- no individual Ctrl combos stolen.
	// Note: #{pane_current_path} is intentionally omitted; tmux opens display-popup
	// with the active pane's CWD, so menucmd reads it via os.Getwd().
	if exe, err := os.Executable(); err == nil {
		exec.Command("tmux", "bind-key", "-n", "C-\\",
			"display-popup", "-E", "-w", "44", "-h", "18",
			fmt.Sprintf("%q menu %q", exe, name)).Run()
	}

	// Write initial stats to temp file. The status bar's #(cat file) is re-run on
	// every status-interval tick, giving live updates in the top header bar.
	statsFile := fmt.Sprintf("%s/agws-%s.txt", os.TempDir(), name)
	r, w, t := getStats()
	writeStatsFile(statsFile, title, r, w, t)

	// Header bar at top - status bar with #(cat file) live-updates on status-interval
	exec.Command("tmux", "set-option", "-t", name, "status", "on").Run()
	exec.Command("tmux", "set-option", "-t", name, "status-position", "top").Run()
	exec.Command("tmux", "set-option", "-t", name, "status-style", "bg=#1e1e2e,fg=#cdd6f4").Run()
	exec.Command("tmux", "set-option", "-t", name, "status-interval", "5").Run()
	exec.Command("tmux", "set-option", "-t", name, "status-left-length", "200").Run()
	exec.Command("tmux", "set-option", "-t", name, "status-left",
		fmt.Sprintf("#(cat '%s')", statsFile)).Run()
	exec.Command("tmux", "set-option", "-t", name, "status-right", "").Run()
	exec.Command("tmux", "set-window-option", "-t", name, "window-status-format", "").Run()
	exec.Command("tmux", "set-window-option", "-t", name, "window-status-current-format", "").Run()

	// Shortcuts in pane border at bottom
	shortcuts := "#[bg=#1e1e2e]  #[fg=#89b4fa]Ctrl+\\#[fg=#6c7086] open menu"
	exec.Command("tmux", "set-window-option", "-t", name, "pane-border-status", "bottom").Run()
	exec.Command("tmux", "set-window-option", "-t", name, "pane-border-style", "bg=#1e1e2e,fg=#6c7086").Run()
	exec.Command("tmux", "set-window-option", "-t", name, "pane-active-border-style", "bg=#1e1e2e,fg=#6c7086").Run()
	exec.Command("tmux", "set-window-option", "-t", name, "pane-border-format", shortcuts).Run()

	// Enable mouse for scrolling and text selection
	exec.Command("tmux", "set-option", "-t", name, "mouse", "on").Run()

	// Keep the file fresh; tmux re-reads it via #() on each status-interval tick
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r, w, t := getStats()
				writeStatsFile(statsFile, title, r, w, t)
			case <-done:
				return
			}
		}
	}()

	// Exit alternate screen (tview uses it), clear, show cursor
	os.Stdout.WriteString("\x1b[?1049l\x1b[2J\x1b[H\x1b[?25h")

	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	close(done)

	// Unbind leader key
	exec.Command("tmux", "unbind-key", "-n", "C-\\").Run()
	exec.Command("tmux", "set-option", "-t", name, "mouse", "off").Run()
	exec.Command("tmux", "set-window-option", "-t", name, "pane-border-status", "off").Run()
	exec.Command("tmux", "set-window-option", "-u", "-t", name, "window-status-format").Run()
	exec.Command("tmux", "set-window-option", "-u", "-t", name, "window-status-current-format").Run()
	os.Remove(statsFile)

	// Re-enter alternate screen for tview
	os.Stdout.WriteString("\x1b[2J\x1b[H\x1b[?1049h")
	return err
}

func InsideTmux() bool {
	return os.Getenv("TMUX") != ""
}
