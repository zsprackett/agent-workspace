package session_test

import (
	"strings"
	"testing"

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
