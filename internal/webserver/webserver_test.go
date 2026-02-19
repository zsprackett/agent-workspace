package webserver_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/webserver"
)

func TestSessionsEndpoint(t *testing.T) {
	store, _ := db.Open(":memory:")
	store.Migrate()
	defer store.Close()

	srv := webserver.New(store, webserver.Config{Port: 0, Host: "127.0.0.1", Enabled: true})
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
