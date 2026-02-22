# Git Inline Display Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show git status and diff inline on the Git tab, auto-loading on open with a manual Refresh button, instead of buttons that open new tabs.

**Architecture:** Two new JSON API endpoints (`/git/status/text`, `/git/diff/text`) return raw git output as `{"output":"..."}`. The frontend fetches both concurrently when the Git tab renders, applies client-side diff coloring, and displays results in `<pre>` blocks. Existing HTML endpoints are untouched.

**Tech Stack:** Go (net/http, encoding/json), Vanilla JS, CSS.

---

### Task 1: Add backend JSON endpoints

**Files:**
- Modify: `internal/webserver/git.go`
- Modify: `internal/webserver/webserver.go`
- Modify: `internal/webserver/git_test.go`

**Step 1: Write the failing tests**

Add to the bottom of `internal/webserver/git_test.go`:

```go
func TestHandleGitStatusText_UnknownSession(t *testing.T) {
	srv, _ := newServer(t)
	req := httptest.NewRequest("GET", "/api/sessions/no-such-id/git/status/text", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleGitStatusText_NoPath(t *testing.T) {
	srv, store := newServer(t)
	seedSession(t, store, "")
	req := httptest.NewRequest("GET", "/api/sessions/git-test-id/git/status/text", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 422 {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestHandleGitStatusText_WithRepo(t *testing.T) {
	srv, store := newServer(t)
	repoDir := initGitRepo(t)
	seedSession(t, store, repoDir)
	req := httptest.NewRequest("GET", "/api/sessions/git-test-id/git/status/text", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	var body struct{ Output string }
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestHandleGitDiffText_UnknownSession(t *testing.T) {
	srv, _ := newServer(t)
	req := httptest.NewRequest("GET", "/api/sessions/no-such-id/git/diff/text", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleGitDiffText_WithRepo(t *testing.T) {
	srv, store := newServer(t)
	repoDir := initGitRepo(t)
	seedSession(t, store, repoDir)
	req := httptest.NewRequest("GET", "/api/sessions/git-test-id/git/diff/text", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	var body struct{ Output string }
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Clean repo has no diff — output field must exist but may be empty string.
	_ = body.Output
}
```

Note: `git_test.go` is in package `webserver_test` and already imports `"net/http/httptest"`, `"testing"`, etc. Add `"encoding/json"` to its import block.

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/webserver/... -run "TestHandleGitStatusText|TestHandleGitDiffText" -v
```

Expected: FAIL — handler not registered, 404 or 405 responses.

**Step 3: Add the two handlers to `internal/webserver/git.go`**

Add `"encoding/json"` to the import block, then append these two functions at the end of the file:

```go
func (s *Server) handleGitStatusText(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"output": string(out)})
}

func (s *Server) handleGitDiffText(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"output": string(out)})
}
```

**Step 4: Register the routes in `internal/webserver/webserver.go`**

Find the block at lines 100-102:

```go
	mux.HandleFunc("GET /api/sessions/{id}/git/status", s.handleGitStatus)
	mux.HandleFunc("GET /api/sessions/{id}/git/diff", s.handleGitDiff)
	mux.HandleFunc("GET /api/sessions/{id}/pr-url", s.handlePRURL)
```

Add two lines after `handleGitDiff`:

```go
	mux.HandleFunc("GET /api/sessions/{id}/git/status", s.handleGitStatus)
	mux.HandleFunc("GET /api/sessions/{id}/git/diff", s.handleGitDiff)
	mux.HandleFunc("GET /api/sessions/{id}/git/status/text", s.handleGitStatusText)
	mux.HandleFunc("GET /api/sessions/{id}/git/diff/text", s.handleGitDiffText)
	mux.HandleFunc("GET /api/sessions/{id}/pr-url", s.handlePRURL)
```

**Step 5: Run tests to verify they pass**

```bash
go test ./internal/webserver/... -run "TestHandleGitStatusText|TestHandleGitDiffText" -v
```

Expected: all PASS.

**Step 6: Run full test suite**

```bash
make test
```

Expected: all pass.

**Step 7: Commit**

```bash
git add internal/webserver/git.go internal/webserver/webserver.go internal/webserver/git_test.go
git commit -m "feat(api): add git status/diff text endpoints returning JSON"
```

---

### Task 2: Update CSS

**Files:**
- Modify: `internal/webserver/static/style.css`

**Step 1: Update `.git-panel` and add new classes**

Find the existing git-related CSS block:

```css
/* Git tab */
.git-panel { padding: 20px; display: flex; flex-direction: column; gap: 12px; }
.git-btn-row { display: flex; gap: 8px; flex-wrap: wrap; }
.git-btn {
  font-size: 12px; padding: 6px 14px;
  background: var(--surface); color: var(--text);
  border: 1px solid var(--border); border-radius: 3px;
  cursor: pointer; font-family: inherit;
  transition: border-color 0.1s, color 0.1s;
}
.git-btn:hover { border-color: var(--accent); color: var(--accent); }
.dirty-notice { font-size: 11px; color: var(--waiting); }
```

Replace it with:

```css
/* Git tab */
.git-panel { padding: 16px; display: flex; flex-direction: column; gap: 12px; overflow-y: auto; flex: 1; }
.git-action-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; flex-shrink: 0; }
.git-section-label {
  font-size: 10px; color: var(--muted);
  text-transform: uppercase; letter-spacing: 0.08em;
}
.git-output {
  background: var(--surface); color: var(--text);
  border: 1px solid var(--border); border-radius: 3px;
  padding: 10px 12px; font-family: inherit; font-size: 12px;
  white-space: pre-wrap; word-break: break-all; line-height: 1.5; margin: 0;
}
.git-btn-row { display: flex; gap: 8px; flex-wrap: wrap; }
.git-btn {
  font-size: 12px; padding: 6px 14px;
  background: var(--surface); color: var(--text);
  border: 1px solid var(--border); border-radius: 3px;
  cursor: pointer; font-family: inherit;
  transition: border-color 0.1s, color 0.1s;
}
.git-btn:hover { border-color: var(--accent); color: var(--accent); }
.dirty-notice { font-size: 11px; color: var(--waiting); }
.diff-add  { color: var(--running); }
.diff-del  { color: var(--error); }
.diff-hunk { color: var(--accent); }
.diff-hdr  { color: var(--muted); }
```

**Step 2: Build to verify**

```bash
make build
```

Expected: compiles with no errors.

**Step 3: Commit**

```bash
git add internal/webserver/static/style.css
git commit -m "feat(ui): add git inline display CSS"
```

---

### Task 3: Rewrite the git tab renderer in app.js

**Files:**
- Modify: `internal/webserver/static/app.js`

**Step 1: Add `colorDiffLines` helper function**

Find the `// --- State ---` comment near the top. Add this function immediately before it:

```js
// --- Diff coloring ---
function colorDiffLines(text) {
  return text.split('\n').map(line => {
    const esc = line.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    if (line.startsWith('+++') || line.startsWith('---')) return `<span class="diff-hdr">${esc}</span>`;
    if (line.startsWith('+')) return `<span class="diff-add">${esc}</span>`;
    if (line.startsWith('-')) return `<span class="diff-del">${esc}</span>`;
    if (line.startsWith('@@')) return `<span class="diff-hunk">${esc}</span>`;
    return esc;
  }).join('\n');
}
```

**Step 2: Replace the `git` branch of `renderTabContent`**

Find the existing git branch (starts with `if (tab === 'git') {` and ends with its closing `return;`). Replace the entire block with:

```js
  if (tab === 'git') {
    const panel = document.createElement('div');
    panel.className = 'git-panel';

    // Action row: dirty notice + Refresh + Open PR
    const actionRow = document.createElement('div');
    actionRow.className = 'git-action-row';

    if (s.HasUncommitted) {
      const notice = document.createElement('div');
      notice.className = 'dirty-notice';
      notice.textContent = '● uncommitted changes';
      actionRow.appendChild(notice);
    }

    const refreshBtn = document.createElement('button');
    refreshBtn.className = 'git-btn';
    refreshBtn.textContent = '↻ Refresh';

    const prBtn = document.createElement('button');
    prBtn.className = 'git-btn';
    prBtn.textContent = 'Open PR';
    prBtn.onclick = async () => {
      const res = await authFetch(`/api/sessions/${s.ID}/pr-url`);
      if (!res || !res.ok) { alert('No open PR found for this branch.'); return; }
      const { url } = await res.json();
      window.open(url, '_blank');
    };

    actionRow.appendChild(refreshBtn);
    actionRow.appendChild(prBtn);
    panel.appendChild(actionRow);

    if (s.ProjectPath || s.WorktreePath) {
      // Status section
      const statusLabel = document.createElement('div');
      statusLabel.className = 'git-section-label';
      statusLabel.textContent = 'Status';
      panel.appendChild(statusLabel);

      const statusPre = document.createElement('pre');
      statusPre.className = 'git-output';
      statusPre.textContent = 'loading...';
      panel.appendChild(statusPre);

      // Diff section
      const diffLabel = document.createElement('div');
      diffLabel.className = 'git-section-label';
      diffLabel.textContent = 'Diff';
      panel.appendChild(diffLabel);

      const diffPre = document.createElement('pre');
      diffPre.className = 'git-output';
      diffPre.textContent = 'loading...';
      panel.appendChild(diffPre);

      const loadGit = async () => {
        statusPre.textContent = 'loading...';
        diffPre.textContent = 'loading...';
        const [statusRes, diffRes] = await Promise.all([
          authFetch(`/api/sessions/${s.ID}/git/status/text`),
          authFetch(`/api/sessions/${s.ID}/git/diff/text`),
        ]);
        if (statusRes && statusRes.ok) {
          const { output } = await statusRes.json();
          statusPre.textContent = output || '(nothing to show)';
        } else {
          statusPre.textContent = '(error fetching status)';
        }
        if (diffRes && diffRes.ok) {
          const { output } = await diffRes.json();
          diffPre.innerHTML = output ? colorDiffLines(output) : '(no diff)';
        } else {
          diffPre.textContent = '(error fetching diff)';
        }
      };

      refreshBtn.onclick = loadGit;
      loadGit();
    } else {
      const msg = document.createElement('div');
      msg.style.cssText = 'font-size:12px;color:var(--muted)';
      msg.textContent = 'No git working directory for this session.';
      panel.appendChild(msg);
      refreshBtn.disabled = true;
    }

    container.appendChild(panel);
    return;
  }
```

**Step 3: Build to verify**

```bash
make build
```

Expected: compiles with no errors.

**Step 4: Commit**

```bash
git add internal/webserver/static/app.js
git commit -m "feat(ui): show git status and diff inline on Git tab"
```

---

### Task 4: Final verification and install

**Step 1: Run full test suite**

```bash
make test
```

Expected: all pass.

**Step 2: Install**

```bash
make install
```

Expected: installs to `~/.local/bin/agent-workspace`.

**Step 3: Manual smoke test**

Run `agent-workspace` and open the web UI. Click a session that has a git working directory and select the Git tab. Verify:
- Status and diff load automatically (no button click required)
- Diff shows colored output (green `+` lines, red `-` lines, cyan `@@` hunks)
- `↻ Refresh` button re-fetches both
- `Open PR` button opens the PR URL in a new tab
- Sessions without a working directory show the "No git working directory" message with a disabled Refresh button
