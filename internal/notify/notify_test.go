package notify_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/notify"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

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
	}, discardLogger())

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

func TestNotify_WebhookErrorLogged(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Invalid URL forces a POST error.
	n := notify.New(notify.Config{Enabled: true, Webhook: "http://127.0.0.1:1"}, logger)
	n.Notify(db.Session{Title: "test", Tool: "claude"})

	if !strings.Contains(buf.String(), "webhook") {
		t.Errorf("expected warn log mentioning webhook, got: %q", buf.String())
	}
}

func TestNotify_DisabledNoOp(t *testing.T) {
	n := notify.New(notify.Config{Enabled: false}, discardLogger())
	// Must not panic.
	n.Notify(db.Session{Title: "test", Tool: "claude"})
}
