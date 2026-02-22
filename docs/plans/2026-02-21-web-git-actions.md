# Web Git Actions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Git Status, Git Diff, Open PR, and Terminal toggle buttons to the web UI session detail panel.

**Architecture:** Two tasks: (1) backend adds three new API endpoints in a new `git.go` file under `internal/webserver/`; (2) frontend adds four buttons to the expanded session detail in `app.js` and converts the always-on terminal into a toggle. Git output is served as a styled HTML page in a new browser tab, authenticated via the existing `?token=` query param mechanism.

**Tech Stack:** Go (`os/exec`, `html` standard library), vanilla JS/HTML/CSS (no dependencies).

---

### Task 1: Backend - git and PR endpoints

**Files:**
- Create: `internal/webserver/git.go`
- Modify: `internal/webserver/webserver.go` (add 3 routes)
- Test: `internal/webserver/git_test.go`

**Step 1: Write the failing tests**

Create `internal/webserver/git_test.go`:

```go
package webserver_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/session"
	"github.com/zsprackett/agent-workspace/internal/webserver"
)

func newServer(t *testing.T) (*webserver.Server, *db.DB) {
	t.Helper()
	store, _ := db.Open(":memory:")
	store.Migrate()
	t.Cleanup(func() { store.Close() })
	mgr := session.NewManager(store)
	srv := webserver.New(store, mgr, webserver.Config{Port: 0, Host: "127.0.0.1", Enabled: true})
	return srv, store
}

func seedSession(t *testing.T, store *db.DB, projectPath string) *db.Session {
	t.Helper()
	now := time.Now()
	s := &db.Session{
		ID:           "git-test-id",
		Title:        "git-session",
		GroupPath:    "my-sessions",
		Tool:         db.ToolClaude,
		Status:       db.StatusRunning,
		ProjectPath:  projectPath,
		CreatedAt:    now,
		LastAccessed: now,
	}
	if err := store.SaveSession(s); err != nil {
		t.Fatalf("save session: %v", err)
	}
	return s
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t.com")
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	run("init")
	run("config", "user.email", "t@t.com")
	run("config", "user.name", "test")
	f := filepath.Join(dir, "README.md")
	os.WriteFile(f, []byte("hello\n"), 0644)
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func TestHandleGitStatus_UnknownSession(t *testing.T) {
	srv, _ := newServer(t)
	req := httptest.NewRequest("GET", "/api/sessions/no-such-id/git/status", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleGitStatus_NoPath(t *testing.T) {
	srv, store := newServer(t)
	seedSession(t, store, "") // no project path
	req := httptest.NewRequest("GET", "/api/sessions/git-test-id/git/status", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 422 {
		t.Fatalf("expected 422 for session with no path, got %d", w.Code)
	}
}

func TestHandleGitStatus_WithRepo(t *testing.T) {
	srv, store := newServer(t)
	repoDir := initGitRepo(t)
	seedSession(t, store, repoDir)
	req := httptest.NewRequest("GET", "/api/sessions/git-test-id/git/status", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html, got %q", ct)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty body")
	}
}

func TestHandleGitDiff_UnknownSession(t *testing.T) {
	srv, _ := newServer(t)
	req := httptest.NewRequest("GET", "/api/sessions/no-such-id/git/diff", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleGitDiff_WithRepo(t *testing.T) {
	srv, store := newServer(t)
	repoDir := initGitRepo(t)
	seedSession(t, store, repoDir)
	req := httptest.NewRequest("GET", "/api/sessions/git-test-id/git/diff", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("wrong content type: %q", w.Header().Get("Content-Type"))
	}
}

func TestHandlePRURL_UnknownSession(t *testing.T) {
	srv, _ := newServer(t)
	req := httptest.NewRequest("GET", "/api/sessions/no-such-id/pr-url", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandlePRURL_NoPath(t *testing.T) {
	srv, store := newServer(t)
	seedSession(t, store, "")
	req := httptest.NewRequest("GET", "/api/sessions/git-test-id/pr-url", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 422 {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestColorDiffLines(t *testing.T) {
	input := "+added line\n-removed line\n@@hunk\n+++file\ncontext\n"
	out := webserver.ColorDiffLines(input)
	tests := []struct{ want string }{
		{`class="add"`},
		{`class="del"`},
		{`class="hunk"`},
		{`class="hdr"`},
	}
	for _, tc := range tests {
		if !contains(out, tc.want) {
			t.Errorf("expected %q in output:\n%s", tc.want, out)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

**Step 2: Run to verify tests fail**

```bash
go test ./internal/webserver/... -run "TestHandleGit|TestHandlePR|TestColorDiff" -v
```
Expected: compile error — `handleGitStatus` etc. not defined, `ColorDiffLines` not exported.

**Step 3: Create `internal/webserver/git.go`**

```go
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

// sessionWorkDir returns the working directory for a session's git operations.
// Prefers WorktreePath, falls back to ProjectPath.
func sessionWorkDir(s interface{ GetWorkDir() string }) string {
	return s.GetWorkDir()
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
```

**Step 4: Register the 3 new routes in `webserver.go`**

In the `Handler()` method, after the existing session routes, add:

```go
mux.HandleFunc("GET /api/sessions/{id}/git/status", s.handleGitStatus)
mux.HandleFunc("GET /api/sessions/{id}/git/diff", s.handleGitDiff)
mux.HandleFunc("GET /api/sessions/{id}/pr-url", s.handlePRURL)
```

**Step 5: Run tests to verify they pass**

```bash
go test ./internal/webserver/... -run "TestHandleGit|TestHandlePR|TestColorDiff" -v
```
Expected: all pass.

**Step 6: Run full test suite**

```bash
make test
```
Expected: all pass.

**Step 7: Commit**

```bash
git add internal/webserver/git.go internal/webserver/git_test.go internal/webserver/webserver.go
git commit -m "feat: add git status, diff, and PR URL API endpoints"
```

---

### Task 2: Frontend - add action buttons and terminal toggle

**Files:**
- Modify: `internal/webserver/static/app.js`

No HTML or CSS changes needed — the existing `.action-btn` class already styles the buttons correctly.

**Step 1: Add `expandedTerminals` state and update row-collapse cleanup**

At the top of `app.js`, alongside `let expandedSessions = new Set();`, add:

```js
let expandedTerminals = new Set();
```

In the `row.onclick` handler (around line 355), update the collapse branch to also clean up the terminal state:

```js
row.onclick = () => {
  if (expandedSessions.has(s.ID)) {
    authFetch(`/api/sessions/${s.ID}/ttyd`, { method: 'DELETE' }).catch(() => {});
    expandedSessions.delete(s.ID);
    expandedTerminals.delete(s.ID);   // NEW: clean up terminal state
  } else {
    expandedSessions.add(s.ID);
  }
  render();
};
```

**Step 2: Move terminal rendering inside the toggle condition**

Currently the terminal container is always appended when `expandedSessions.has(s.ID) && s.TmuxSession`. Change it so the terminal container is only rendered when `expandedTerminals.has(s.ID)` as well.

In the expanded block (inside `if (expandedSessions.has(s.ID))`), find:

```js
// Terminal iframe -- populated after DOM append below.
if (s.TmuxSession) {
  const termContainer = document.createElement('div');
  termContainer.className = 'terminal-container';
  detail.appendChild(termContainer);
}
```

Replace with:

```js
if (s.TmuxSession && expandedTerminals.has(s.ID)) {
  const termContainer = document.createElement('div');
  termContainer.className = 'terminal-container';
  detail.appendChild(termContainer);
}
```

Similarly update the iframe spawn block (around line 369):

```js
if (expandedSessions.has(s.ID) && expandedTerminals.has(s.ID) && s.TmuxSession) {
```

**Step 3: Add the 4 new action buttons**

In the expanded block, after the existing `actions.appendChild(mkBtn('Delete', ...))` call, append 4 new buttons.

Replace the current actions block (which ends after the Delete button) with:

```js
if (s.Status !== 'stopped') {
  actions.appendChild(mkBtn('Stop', false, () => {
    if (confirm(`Stop session "${s.Title}"?`)) apiAction(`/api/sessions/${s.ID}/stop`, 'POST');
  }));
}
actions.appendChild(mkBtn('Restart', false, () => {
  if (confirm(`Restart session "${s.Title}"?`)) apiAction(`/api/sessions/${s.ID}/restart`, 'POST');
}));
actions.appendChild(mkBtn('Delete', true, () => {
  if (confirm(`Delete session "${s.Title}"?`)) {
    expandedSessions.delete(s.ID);
    expandedTerminals.delete(s.ID);
    apiAction(`/api/sessions/${s.ID}`, 'DELETE');
  }
}));

// Git / terminal actions (only for sessions with a git working directory)
if (s.ProjectPath || s.WorktreePath) {
  const token = getAccessToken();
  actions.appendChild(mkBtn('Git Status', false, () => {
    window.open(`/api/sessions/${s.ID}/git/status?token=${encodeURIComponent(token)}`, '_blank');
  }));
  actions.appendChild(mkBtn('Git Diff', false, () => {
    window.open(`/api/sessions/${s.ID}/git/diff?token=${encodeURIComponent(token)}`, '_blank');
  }));
  actions.appendChild(mkBtn('Open PR', false, async () => {
    const res = await authFetch(`/api/sessions/${s.ID}/pr-url`);
    if (!res || !res.ok) {
      alert('No open PR found for this branch.');
      return;
    }
    const { url } = await res.json();
    window.open(url, '_blank');
  }));
}

if (s.TmuxSession) {
  const isTermOpen = expandedTerminals.has(s.ID);
  actions.appendChild(mkBtn(isTermOpen ? 'Hide Terminal' : 'Terminal', false, () => {
    if (expandedTerminals.has(s.ID)) {
      authFetch(`/api/sessions/${s.ID}/ttyd`, { method: 'DELETE' }).catch(() => {});
      expandedTerminals.delete(s.ID);
    } else {
      expandedTerminals.add(s.ID);
    }
    render();
  }));
}
```

**Step 4: Verify auth-free mode still works**

When no JWT secret is configured, `getAccessToken()` returns null. The `?token=null` query param won't match a valid token, but the middleware won't be active either (no auth mode). In no-auth mode, the git endpoints don't require a token, so `window.open` will work. But `?token=null` is harmless - it's simply ignored since the middleware is not wrapping the mux.

No code change needed, this works by design.

**Step 5: Manual smoke test**

Build and run:
```bash
make build && ./agent-workspace
```
- Open the web UI, expand a session that has a `ProjectPath` or `WorktreePath`.
- Verify the 4 new buttons appear: "Git Status", "Git Diff", "Open PR", "Terminal".
- Click "Git Status" — new tab opens with dark-background HTML showing `git status` output.
- Click "Git Diff" — new tab opens with colored diff output.
- Click "Open PR" — if no PR, shows "No open PR found" alert.
- Click "Terminal" — terminal iframe appears below. Click "Hide Terminal" — iframe disappears.
- Sessions with no path (bare tmux session, no `ProjectPath`) show only "Terminal" button (no git buttons).

**Step 6: Commit**

```bash
git add internal/webserver/static/app.js
git commit -m "feat: add git status, diff, PR, and terminal toggle buttons in web UI"
```
