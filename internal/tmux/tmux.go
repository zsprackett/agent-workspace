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

type CaptureOptions struct {
	StartLine int
	EndLine   int
	Join      bool
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

func writeStatsFile(path string, running, waiting, total int) {
	content := fmt.Sprintf(
		"#[fg=#89b4fa,bold] AGENT WORKSPACE #[fg=#6c7086,nobold]  "+
			"#[fg=#a6e3a1]● %d running  #[fg=#f9e2af]◐ %d waiting  #[fg=#cdd6f4]%d total",
		running, waiting, total)
	os.WriteFile(path, []byte(content), 0600)
}

// AttachSession attaches to a tmux session synchronously.
// Ctrl+D detaches, Ctrl+T opens a terminal split.
// getStats is called periodically to update the header counts.
// Blocks until the user detaches.
func AttachSession(name, title string, getStats func() (running, waiting, total int)) error {
	// Bind Ctrl+D to detach
	exec.Command("tmux", "bind-key", "-n", "C-d", "detach-client").Run()
	// Bind Ctrl+T to open a horizontal split
	exec.Command("tmux", "bind-key", "-n", "C-t", "split-window", "-v", "-c", "#{pane_current_path}").Run()
	// Bind Ctrl+G to show git status in a temporary split pane
	exec.Command("tmux", "bind-key", "-n", "C-g",
		"split-window", "-v", "-l", "15", "-c", "#{pane_current_path}",
		`git status; printf '\nPress enter to close...'; read`).Run()
	// Bind Ctrl+F to show git diff in a scrollable split pane
	exec.Command("tmux", "bind-key", "-n", "C-f",
		"split-window", "-v", "-l", "20", "-c", "#{pane_current_path}",
		`out=$(git diff HEAD --color=always; git ls-files --others --exclude-standard -z | xargs -0 -I{} git diff --no-index --color=always -- /dev/null {} 2>/dev/null); if [ -n "$out" ]; then printf '%s\n' "$out" | less -RX; else printf 'No changes.\n\nPress enter to close...'; read; fi`).Run()
	// Bind Ctrl+P to open the current branch's GitHub PR in the browser
	exec.Command("tmux", "bind-key", "-n", "C-p", "run-shell",
		`cd '#{pane_current_path}' && url=$(gh pr view --json url --jq .url 2>/dev/null) && [ -n "$url" ] && { open "$url" 2>/dev/null || xdg-open "$url" 2>/dev/null; tmux display-message "Opening PR: $url"; } || tmux display-message "No open PR found for this branch"`).Run()
	// Bind Ctrl+N to open a notes popup using the agent-workspace notes subcommand
	if exe, err := os.Executable(); err == nil {
		exec.Command("tmux", "bind-key", "-n", "C-n",
			"display-popup", "-E", "-w", "64", "-h", "22",
			fmt.Sprintf("%q notes %q", exe, name)).Run()
	}

	// Write initial stats to temp file. The status bar's #(cat file) is re-run on
	// every status-interval tick, giving live updates in the top header bar.
	statsFile := fmt.Sprintf("%s/agws-%s.txt", os.TempDir(), name)
	r, w, t := getStats()
	writeStatsFile(statsFile, r, w, t)

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

	// Shortcuts in pane border at bottom (static text, no refresh needed)
	shortcuts := "#[bg=#1e1e2e] " +
		"#[fg=#89b4fa]Ctrl+G#[fg=#6c7086] status  " +
		"#[fg=#89b4fa]Ctrl+F#[fg=#6c7086] diff  " +
		"#[fg=#89b4fa]Ctrl+P#[fg=#6c7086] PR  " +
		"#[fg=#89b4fa]Ctrl+N#[fg=#6c7086] notes  " +
		"#[fg=#89b4fa]Ctrl+T#[fg=#6c7086] terminal  " +
		"#[fg=#89b4fa]Ctrl+D#[fg=#6c7086] detach"
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
				writeStatsFile(statsFile, r, w, t)
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

	// Unbind session keys
	exec.Command("tmux", "unbind-key", "-n", "C-d").Run()
	exec.Command("tmux", "unbind-key", "-n", "C-t").Run()
	exec.Command("tmux", "unbind-key", "-n", "C-g").Run()
	exec.Command("tmux", "unbind-key", "-n", "C-f").Run()
	exec.Command("tmux", "unbind-key", "-n", "C-p").Run()
	exec.Command("tmux", "unbind-key", "-n", "C-n").Run()
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
