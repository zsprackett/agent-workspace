package session_test

import (
	"strings"
	"testing"
	"time"

	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/session"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestUpdatePreservesCustomCommand(t *testing.T) {
	store := newTestDB(t)

	now := time.Now().Truncate(time.Millisecond)
	s := &db.Session{
		ID:           "custom-test",
		Title:        "bold-wolf",
		ProjectPath:  "/tmp/proj",
		GroupPath:    "my-sessions",
		Command:      "old-tool --flag",
		Tool:         db.ToolCustom,
		Status:       db.StatusStopped,
		TmuxSession:  "",
		CreatedAt:    now,
		LastAccessed: now,
	}
	if err := store.SaveSession(s); err != nil {
		t.Fatalf("save: %v", err)
	}

	mgr := session.NewManager(store)
	if err := mgr.Update("custom-test", session.UpdateOptions{
		Title:       "bold-wolf",
		Tool:        db.ToolCustom,
		Command:     "new-tool --other",
		ProjectPath: "/tmp/proj",
		GroupPath:   "my-sessions",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := store.GetSession("custom-test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Command != "new-tool --other" {
		t.Errorf("command: got %q want %q", got.Command, "new-tool --other")
	}
}

func TestUpdateResetsCommandOnToolChange(t *testing.T) {
	store := newTestDB(t)

	now := time.Now().Truncate(time.Millisecond)
	s := &db.Session{
		ID:           "tool-change-test",
		Title:        "calm-deer",
		ProjectPath:  "/tmp/proj",
		GroupPath:    "my-sessions",
		Command:      "my-custom-tool",
		Tool:         db.ToolCustom,
		Status:       db.StatusStopped,
		TmuxSession:  "",
		CreatedAt:    now,
		LastAccessed: now,
	}
	if err := store.SaveSession(s); err != nil {
		t.Fatalf("save: %v", err)
	}

	mgr := session.NewManager(store)
	if err := mgr.Update("tool-change-test", session.UpdateOptions{
		Title:       "calm-deer",
		Tool:        db.ToolClaude,
		Command:     "",
		ProjectPath: "/tmp/proj",
		GroupPath:   "my-sessions",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := store.GetSession("tool-change-test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Command != "claude" {
		t.Errorf("command: got %q want %q", got.Command, "claude")
	}
}

func TestGenerateTitle(t *testing.T) {
	title := session.GenerateTitle()
	if title == "" {
		t.Error("expected non-empty title")
	}
	// Should be adjective-noun format
	parts := strings.Split(title, "-")
	if len(parts) < 2 {
		t.Errorf("expected adjective-noun, got %q", title)
	}
}
