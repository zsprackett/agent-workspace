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
