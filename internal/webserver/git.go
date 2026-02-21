package webserver

import (
	"fmt"
	"html"
	"net/http"
	"os/exec"
	"strings"
)

const gitPageTmpl = `<!DOCTYPE html><html><head><title>%s</title><meta charset="UTF-8">` +
	`<style>body{background:#0d1117;color:#e6edf3;font-family:ui-monospace,'SF Mono',` +
	`Menlo,monospace;font-size:13px;padding:16px;margin:0}` +
	`pre{white-space:pre-wrap;word-break:break-all}` +
	`.add{color:#3fb950}.del{color:#f85149}.hunk{color:#58a6ff}.hdr{color:#8b949e}` +
	`</style></head><body><pre>%s</pre></body></html>`

// ColorDiffLines wraps diff output lines in HTML spans for syntax coloring.
// Exported so it can be tested directly.
func ColorDiffLines(output string) string {
	lines := strings.Split(output, "\n")
	var sb strings.Builder
	for _, line := range lines {
		esc := html.EscapeString(line)
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			fmt.Fprintf(&sb, `<span class="hdr">%s</span>`+"\n", esc)
		case strings.HasPrefix(line, "+"):
			fmt.Fprintf(&sb, `<span class="add">%s</span>`+"\n", esc)
		case strings.HasPrefix(line, "-"):
			fmt.Fprintf(&sb, `<span class="del">%s</span>`+"\n", esc)
		case strings.HasPrefix(line, "@@"):
			fmt.Fprintf(&sb, `<span class="hunk">%s</span>`+"\n", esc)
		default:
			sb.WriteString(esc + "\n")
		}
	}
	return sb.String()
}

func (s *Server) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := s.store.GetSession(id)
	if err != nil || sess == nil {
		http.Error(w, "session not found", 404)
		return
	}
	path := sess.WorktreePath
	if path == "" {
		path = sess.ProjectPath
	}
	if path == "" {
		http.Error(w, "session has no working directory", 422)
		return
	}

	cmd := exec.Command("git", "status")
	cmd.Dir = path
	out, _ := cmd.CombinedOutput()

	body := fmt.Sprintf(gitPageTmpl,
		html.EscapeString("git status — "+sess.Title),
		html.EscapeString(string(out)))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(body))
}

func (s *Server) handleGitDiff(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := s.store.GetSession(id)
	if err != nil || sess == nil {
		http.Error(w, "session not found", 404)
		return
	}
	path := sess.WorktreePath
	if path == "" {
		path = sess.ProjectPath
	}
	if path == "" {
		http.Error(w, "session has no working directory", 422)
		return
	}

	cmd := exec.Command("git", "diff", "HEAD")
	cmd.Dir = path
	out, _ := cmd.CombinedOutput()

	// Also append untracked files.
	untrackedCmd := exec.Command("git", "ls-files", "--others", "--exclude-standard")
	untrackedCmd.Dir = path
	untracked, _ := untrackedCmd.Output()
	if len(strings.TrimSpace(string(untracked))) > 0 {
		for _, f := range strings.Split(strings.TrimSpace(string(untracked)), "\n") {
			if f == "" {
				continue
			}
			diffCmd := exec.Command("git", "diff", "--no-index", "--", "/dev/null", f)
			diffCmd.Dir = path
			diffOut, _ := diffCmd.Output()
			out = append(out, diffOut...)
		}
	}

	body := fmt.Sprintf(gitPageTmpl,
		html.EscapeString("git diff — "+sess.Title),
		ColorDiffLines(string(out)))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(body))
}

func (s *Server) handlePRURL(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := s.store.GetSession(id)
	if err != nil || sess == nil {
		http.Error(w, "session not found", 404)
		return
	}
	path := sess.WorktreePath
	if path == "" {
		path = sess.ProjectPath
	}
	if path == "" {
		http.Error(w, "session has no working directory", 422)
		return
	}

	cmd := exec.Command("gh", "pr", "view", "--json", "url", "--jq", ".url")
	cmd.Dir = path
	out, err := cmd.Output()
	url := strings.TrimSpace(string(out))
	if err != nil || url == "" {
		http.Error(w, "no open PR found for this branch", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"url":%q}`, url)
}
