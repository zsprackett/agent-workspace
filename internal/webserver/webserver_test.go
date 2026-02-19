package webserver_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/session"
	"github.com/zsprackett/agent-workspace/internal/webserver"
)

func TestSessionsEndpoint(t *testing.T) {
	store, _ := db.Open(":memory:")
	store.Migrate()
	defer store.Close()

	mgr := session.NewManager(store)
	srv := webserver.New(store, mgr, webserver.Config{Port: 0, Host: "127.0.0.1", Enabled: true})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := result["sessions"]; !ok {
		t.Error("expected 'sessions' key in response")
	}
}

func TestStopSessionEndpoint(t *testing.T) {
	store, _ := db.Open(":memory:")
	store.Migrate()
	defer store.Close()

	// Insert a session directly (no tmux) so stop just updates DB status.
	now := time.Now()
	sess := &db.Session{
		ID:          "test-stop-id",
		Title:       "test-session",
		GroupPath:   "my-sessions",
		Tool:        db.ToolClaude,
		Status:      db.StatusRunning,
		TmuxSession: "", // no real tmux session
		CreatedAt:   now,
		LastAccessed: now,
	}
	if err := store.SaveSession(sess); err != nil {
		t.Fatalf("save session: %v", err)
	}

	mgr := session.NewManager(store)
	srv := webserver.New(store, mgr, webserver.Config{Port: 0, Host: "127.0.0.1", Enabled: true})
	handler := srv.Handler()

	req := httptest.NewRequest("POST", fmt.Sprintf("/api/sessions/%s/stop", sess.ID), nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 204 {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	updated, err := store.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updated.Status != db.StatusStopped {
		t.Errorf("expected status stopped, got %s", updated.Status)
	}
}
