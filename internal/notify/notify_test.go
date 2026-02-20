package notify_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/notify"
)

func TestNtfyNotification(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	n := notify.New(notify.Config{
		Enabled: true,
		NtfyURL: srv.URL + "/test-topic",
	})

	n.Notify(db.Session{
		ID:        "1",
		Title:     "swift-fox",
		Tool:      db.ToolClaude,
		GroupPath: "my-sessions",
		Status:    db.StatusWaiting,
	})

	if received == nil {
		t.Fatal("no POST received")
	}
	if received["title"] != "swift-fox is waiting" {
		t.Errorf("unexpected title: %v", received["title"])
	}
}
