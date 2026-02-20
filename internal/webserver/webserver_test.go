package webserver_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/session"
	"github.com/zsprackett/agent-workspace/internal/webserver"
)

func newAuthServer(t *testing.T) (*webserver.Server, *db.DB) {
	t.Helper()
	store, _ := db.Open(":memory:")
	store.Migrate()
	t.Cleanup(func() { store.Close() })
	mgr := session.NewManager(store)
	srv := webserver.New(store, mgr, webserver.Config{
		Port:    0,
		Host:    "127.0.0.1",
		Enabled: true,
		Auth: webserver.AuthConfig{
			JWTSecret:       "test-secret",
			RefreshTokenTTL: "168h",
		},
	})
	// seed an account
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	store.CreateAccount("alice", string(hash))
	return srv, store
}

func TestLoginEndpoint(t *testing.T) {
	srv, _ := newAuthServer(t)
	body := `{"username":"alice","password":"password"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["access_token"] == "" {
		t.Error("expected access_token in response")
	}
	if resp["refresh_token"] == "" {
		t.Error("expected refresh_token in response")
	}
}

func TestLoginEndpoint_WrongPassword(t *testing.T) {
	srv, _ := newAuthServer(t)
	body := `{"username":"alice","password":"wrong"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRefreshEndpoint(t *testing.T) {
	srv, store := newAuthServer(t)

	// Login first to get refresh token
	loginBody := `{"username":"alice","password":"password"}`
	loginReq := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(loginW, loginReq)
	var loginResp map[string]string
	json.NewDecoder(loginW.Body).Decode(&loginResp)

	_ = store // store available if needed
	body := fmt.Sprintf(`{"refresh_token":"%s"}`, loginResp["refresh_token"])
	req := httptest.NewRequest("POST", "/api/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["access_token"] == "" {
		t.Error("expected new access_token")
	}
	// Old refresh token should be gone (rotation)
	_, err := store.GetRefreshToken(loginResp["refresh_token"])
	if err == nil {
		t.Error("old refresh token should be deleted after rotation")
	}
}

func TestLogoutEndpoint(t *testing.T) {
	srv, store := newAuthServer(t)
	loginBody := `{"username":"alice","password":"password"}`
	loginReq := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(loginW, loginReq)
	var loginResp map[string]string
	json.NewDecoder(loginW.Body).Decode(&loginResp)

	body := fmt.Sprintf(`{"refresh_token":"%s"}`, loginResp["refresh_token"])
	req := httptest.NewRequest("POST", "/api/auth/logout", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	_, err := store.GetRefreshToken(loginResp["refresh_token"])
	if err == nil {
		t.Error("refresh token should be deleted after logout")
	}
}

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
